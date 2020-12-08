package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	logging "github.com/ipfs/go-log/v2"
	"github.com/namsral/flag"
	badger "github.com/textileio/go-ds-badger"
	mongods "github.com/textileio/go-ds-mongo"
)

var log = logging.Logger("dscopy")

func main() {
	fs := flag.NewFlagSet(os.Args[0], 0)

	fromBadgerRepo := fs.String("fromBadgerRepo", "", "Source badger repo path")
	toBadgerRepo := fs.String("toBadgerRepo", "", "Destination badger repo path")

	fromMongoUri := fs.String("fromMongoUri", "", "Source MongoDB URI")
	fromMongoDatabase := fs.String("fromMongoDatabase", "", "Source MongoDB database")
	fromMongoCollection := fs.String("fromMongoCollection", "", "Source MongoDB collection")
	toMongoUri := fs.String("toMongoUri", "", "Destination MongoDB URI")
	toMongoDatabase := fs.String("toMongoDatabase", "", "Destination MongoDB database")
	toMongoCollection := fs.String("toMongoCollection", "", "Destination MongoDB collection")

	parallel := fs.Int("parallel", 1000, "Number of parallel copy operations")

	verbose := fs.Bool("verbose", false, "More verbose output")
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	logging.SetupLogging(logging.Config{
		Format: logging.ColorizedOutput,
		Stderr: true,
		Level:  logging.LevelError,
	})
	if err := logging.SetLogLevel("dscopy", "info"); err != nil {
		log.Fatal(err)
	}

	if len(*fromBadgerRepo) != 0 && len(*fromMongoUri) != 0 {
		log.Fatal("multiple sources specified")
	}
	if len(*fromBadgerRepo) == 0 && len(*fromMongoUri) == 0 {
		log.Fatal("source not specified")
	}
	if len(*toBadgerRepo) != 0 && len(*toMongoUri) != 0 {
		log.Fatal("multiple destinations specified")
	}
	if len(*toBadgerRepo) == 0 && len(*toMongoUri) == 0 {
		log.Fatal("destination not specified")
	}

	var from, to ds.Datastore
	var err error
	if len(*fromBadgerRepo) != 0 {
		from, err = badger.NewDatastore(*fromBadgerRepo, &badger.DefaultOptions)
		if err != nil {
			log.Fatalf("connecting to badger source: %v", err)
		}
		log.Infof("connected to badger source: %s", *fromBadgerRepo)
	}
	if len(*toBadgerRepo) != 0 {
		to, err = badger.NewDatastore(*toBadgerRepo, &badger.DefaultOptions)
		if err != nil {
			log.Fatalf("connecting to badger destination: %v", err)
		}
		log.Infof("connected to badger destination: %s", *toBadgerRepo)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if len(*fromMongoUri) != 0 {
		uri, err := url.Parse(*fromMongoUri)
		if err != nil {
			log.Fatalf("parsing source mongo URI: %v", err)
		}
		if len(*fromMongoDatabase) == 0 {
			log.Fatal("source mongo database not specified")
		}
		if len(*fromMongoCollection) == 0 {
			log.Fatal("source mongo collection not specified")
		}
		from, err = mongods.New(ctx, *fromMongoUri, *fromMongoDatabase, mongods.WithCollName(*fromMongoCollection))
		if err != nil {
			log.Fatalf("connecting to mongo source: %v", err)
		}
		log.Infof("connected to mongo source: %s", uri.Redacted())
	}
	if len(*toMongoUri) != 0 {
		uri, err := url.Parse(*toMongoUri)
		if err != nil {
			log.Fatalf("parsing destination mongo URI: %v", err)
		}
		if len(*toMongoDatabase) == 0 {
			log.Fatal("destination mongo database not specified")
		}
		if len(*toMongoCollection) == 0 {
			log.Fatal("destination mongo collection not specified")
		}
		to, err = mongods.New(ctx, *toMongoUri, *toMongoDatabase, mongods.WithCollName(*toMongoCollection))
		if err != nil {
			log.Fatalf("connecting to mongo destination: %v", err)
		}
		log.Infof("connected to mongo destination: %s", uri.Redacted())
	}

	res, err := from.Query(query.Query{})
	if err != nil {
		log.Fatalf("querying source: %v", err)
	}
	defer res.Close()

	var lock sync.Mutex
	var errors []string
	var count int
	start := time.Now()
	lim := make(chan struct{}, *parallel)
	for r := range res.Next() {
		if r.Error != nil {
			log.Fatalf("getting next source result: %v", r.Error)
		}
		lim <- struct{}{}

		r := r
		go func() {
			defer func() { <-lim }()

			if err := to.Put(ds.NewKey(r.Key), r.Value); err != nil {
				lock.Lock()
				errors = append(errors, fmt.Sprintf("copying %s: %v", r.Key, err))
				lock.Unlock()
				return
			}
			if *verbose {
				log.Infof("copied %s", r.Key)
			}
			lock.Lock()
			count++
			lock.Unlock()
			if count%*parallel == 0 {
				log.Infof("copied %d keys", count)
			}
		}()
	}
	for i := 0; i < cap(lim); i++ {
		lim <- struct{}{}
	}

	if len(errors) > 0 {
		for _, m := range errors {
			log.Error(m)
		}
		log.Fatalf("had %d errors", len(errors))
	}

	log.Infof("copied %d keys in %s", count, time.Since(start))
}
