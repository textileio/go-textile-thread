package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/phayes/freeport"
	"github.com/textileio/go-threads/api"
	. "github.com/textileio/go-threads/api/client"
	pb "github.com/textileio/go-threads/api/pb"
	"github.com/textileio/go-threads/core/thread"
	sym "github.com/textileio/go-threads/crypto/symmetric"
	"github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/util"
	"google.golang.org/grpc"
)

func TestNewDB(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test new db", func(t *testing.T) {
		if err := client.NewDB(context.Background(), thread.NewIDV1(thread.Raw, 32)); err != nil {
			t.Fatalf("failed to create new db: %v", err)
		}
	})
}

func TestNewDBFromAddr(t *testing.T) {
	t.Parallel()
	client1, done1 := setup(t)
	defer done1()
	client2, done2 := setup(t)
	defer done2()

	dbID := thread.NewIDV1(thread.Raw, 32)
	err := client1.NewDB(context.Background(), dbID)
	checkErr(t, err)
	info, err := client1.GetDBInfo(context.Background(), dbID)
	checkErr(t, err)

	t.Run("test new db from address", func(t *testing.T) {
		addr, err := ma.NewMultiaddr(info.Addresses[0])
		checkErr(t, err)
		fk, err := sym.FromBytes(info.FollowKey)
		checkErr(t, err)
		rk, err := sym.FromBytes(info.FollowKey)
		checkErr(t, err)
		if err := client2.NewDBFromAddr(context.Background(), addr, fk, rk); err != nil {
			t.Fatalf("failed to create new db from address: %v", err)
		}
	})
}

func TestNewCollection(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test new collection", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		if err != nil {
			t.Fatalf("failed add new collection: %v", err)
		}
	})
}

func TestCreate(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test collection create", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		err = client.Create(context.Background(), dbID, collectionName, createPerson())
		if err != nil {
			t.Fatalf("failed to create collection: %v", err)
		}
	})
}

func TestGetDBInfo(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test get db info", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)

		info, err := client.GetDBInfo(context.Background(), dbID)
		if err != nil {
			t.Fatalf("failed to create collection: %v", err)
		}
		if info.FollowKey == nil {
			t.Fatal("got nil follow key")
		}
		if info.ReadKey == nil {
			t.Fatal("got nil read key")
		}
		if len(info.Addresses) == 0 {
			t.Fatal("got empty addresses")
		}
	})
}

func TestSave(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test collection save", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		person := createPerson()

		err = client.Create(context.Background(), dbID, collectionName, person)
		checkErr(t, err)

		person.Age = 30
		err = client.Save(context.Background(), dbID, collectionName, person)
		if err != nil {
			t.Fatalf("failed to save collection: %v", err)
		}
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test collection delete", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		person := createPerson()

		err = client.Create(context.Background(), dbID, collectionName, person)
		checkErr(t, err)

		err = client.Delete(context.Background(), dbID, collectionName, person.ID)
		if err != nil {
			t.Fatalf("failed to delete collection: %v", err)
		}
	})
}

func TestHas(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test collection has", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		person := createPerson()

		err = client.Create(context.Background(), dbID, collectionName, person)
		checkErr(t, err)

		exists, err := client.Has(context.Background(), dbID, collectionName, person.ID)
		if err != nil {
			t.Fatalf("failed to check collection has: %v", err)
		}
		if !exists {
			t.Fatal("collection should exist but it doesn't")
		}
	})
}

func TestFind(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test collection find", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		person := createPerson()

		err = client.Create(context.Background(), dbID, collectionName, person)
		checkErr(t, err)

		q := db.JSONWhere("lastName").Eq(person.LastName)

		rawResults, err := client.Find(context.Background(), dbID, collectionName, q, []*Person{})
		if err != nil {
			t.Fatalf("failed to find: %v", err)
		}
		results := rawResults.([]*Person)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, but got %v", len(results))
		}
		if !reflect.DeepEqual(results[0], person) {
			t.Fatal("collection found by query does't equal the original")
		}
	})
}

func TestFindWithIndex(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()
	t.Run("test collection find", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{
			Name:   collectionName,
			Schema: schema,
			Indexes: []db.IndexConfig{{
				Path:   "lastName",
				Unique: true,
			}},
		})
		checkErr(t, err)

		person := createPerson()

		err = client.Create(context.Background(), dbID, collectionName, person)
		checkErr(t, err)

		q := db.JSONWhere("lastName").Eq(person.LastName).UseIndex("lastName")

		rawResults, err := client.Find(context.Background(), dbID, collectionName, q, []*Person{})
		if err != nil {
			t.Fatalf("failed to find: %v", err)
		}
		results := rawResults.([]*Person)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, but got %v", len(results))
		}
		if !reflect.DeepEqual(results[0], person) {
			t.Fatal("collection found by query does't equal the original")
		}
	})
}

func TestFindByID(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test collection find by ID", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		person := createPerson()

		err = client.Create(context.Background(), dbID, collectionName, person)
		checkErr(t, err)

		newPerson := &Person{}
		err = client.FindByID(context.Background(), dbID, collectionName, person.ID, newPerson)
		if err != nil {
			t.Fatalf("failed to find collection by id: %v", err)
		}
		if !reflect.DeepEqual(newPerson, person) {
			t.Fatal("collection found by id does't equal the original")
		}
	})
}

func TestReadTransaction(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test read transaction", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		person := createPerson()
		err = client.Create(context.Background(), dbID, collectionName, person)
		checkErr(t, err)

		txn, err := client.ReadTransaction(context.Background(), dbID, collectionName)
		if err != nil {
			t.Fatalf("failed to create read txn: %v", err)
		}

		end, err := txn.Start()
		defer func() {
			err = end()
			if err != nil {
				t.Fatalf("failed to end txn: %v", err)
			}
		}()
		if err != nil {
			t.Fatalf("failed to start read txn: %v", err)
		}

		has, err := txn.Has(person.ID)
		if err != nil {
			t.Fatalf("failed to read txn has: %v", err)
		}
		if !has {
			t.Fatal("expected has to be true but it wasn't")
		}

		foundPerson := &Person{}
		err = txn.FindByID(person.ID, foundPerson)
		if err != nil {
			t.Fatalf("failed to txn find by id: %v", err)
		}
		if !reflect.DeepEqual(foundPerson, person) {
			t.Fatal("txn collection found by id does't equal the original")
		}

		q := db.JSONWhere("lastName").Eq(person.LastName)

		rawResults, err := txn.Find(q, []*Person{})
		if err != nil {
			t.Fatalf("failed to find: %v", err)
		}
		results := rawResults.([]*Person)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, but got %v", len(results))
		}
		if !reflect.DeepEqual(results[0], person) {
			t.Fatal("collection found by query does't equal the original")
		}
	})
}

func TestWriteTransaction(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test write transaction", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		existingPerson := createPerson()

		err = client.Create(context.Background(), dbID, collectionName, existingPerson)
		checkErr(t, err)

		txn, err := client.WriteTransaction(context.Background(), dbID, collectionName)
		if err != nil {
			t.Fatalf("failed to create write txn: %v", err)
		}

		end, err := txn.Start()
		defer func() {
			err = end()
			if err != nil {
				t.Fatalf("failed to end txn: %v", err)
			}
		}()
		if err != nil {
			t.Fatalf("failed to start write txn: %v", err)
		}

		person := createPerson()

		err = txn.Create(person)
		if err != nil {
			t.Fatalf("failed to create in write txn: %v", err)
		}
		if person.ID == "" {
			t.Fatalf("expected an instance id to be set but it wasn't")
		}

		has, err := txn.Has(existingPerson.ID)
		if err != nil {
			t.Fatalf("failed to write txn has: %v", err)
		}
		if !has {
			t.Fatalf("expected has to be true but it wasn't")
		}

		foundExistingPerson := &Person{}
		err = txn.FindByID(existingPerson.ID, foundExistingPerson)
		if err != nil {
			t.Fatalf("failed to txn find by id: %v", err)
		}
		if !reflect.DeepEqual(foundExistingPerson, existingPerson) {
			t.Fatalf("txn collection found by id does't equal the original")
		}

		q := db.JSONWhere("lastName").Eq(person.LastName)

		rawResults, err := txn.Find(q, []*Person{})
		if err != nil {
			t.Fatalf("failed to find: %v", err)
		}
		results := rawResults.([]*Person)
		if len(results) != 1 {
			t.Fatalf("expected 1 result, but got %v", len(results))
		}
		if !reflect.DeepEqual(results[0], existingPerson) {
			t.Fatal("collection found by query does't equal the original")
		}

		existingPerson.Age = 99
		err = txn.Save(existingPerson)
		if err != nil {
			t.Fatalf("failed to save in write txn: %v", err)
		}

		err = txn.Delete(existingPerson.ID)
		if err != nil {
			t.Fatalf("failed to delete in write txn: %v", err)
		}
	})
}

func TestListen(t *testing.T) {
	t.Parallel()
	client, done := setup(t)
	defer done()

	t.Run("test listen", func(t *testing.T) {
		dbID := thread.NewIDV1(thread.Raw, 32)
		err := client.NewDB(context.Background(), dbID)
		checkErr(t, err)
		err = client.NewCollection(context.Background(), dbID, db.CollectionConfig{Name: collectionName, Schema: schema})
		checkErr(t, err)

		person := createPerson()

		err = client.Create(context.Background(), dbID, collectionName, person)
		checkErr(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		opt := ListenOption{
			Collection: collectionName,
			InstanceID: person.ID,
		}
		channel, err := client.Listen(ctx, dbID, opt)
		if err != nil {
			t.Fatalf("failed to call listen: %v", err)
		}

		go func() {
			time.Sleep(1 * time.Second)
			person.Age = 30
			_ = client.Save(context.Background(), dbID, collectionName, person)
			person.Age = 40
			_ = client.Save(context.Background(), dbID, collectionName, person)
		}()

		val, ok := <-channel
		if !ok {
			t.Fatal("channel no longer active at first event")
		} else {
			if val.Err != nil {
				t.Fatalf("failed to receive first listen result: %v", val.Err)
			}
			p := &Person{}
			if err := json.Unmarshal(val.Action.Instance, p); err != nil {
				t.Fatalf("failed to unmarshal listen result: %v", err)
			}
			if p.Age != 30 {
				t.Fatalf("expected listen result age = 30 but got: %v", p.Age)
			}
			if val.Action.InstanceID != person.ID {
				t.Fatalf("expected listen result id = %v but got: %v", person.ID, val.Action.InstanceID)
			}
		}

		val, ok = <-channel
		if !ok {
			t.Fatal("channel no longer active at second event")
		} else {
			if val.Err != nil {
				t.Fatalf("failed to receive second listen result: %v", val.Err)
			}
			p := &Person{}
			if err := json.Unmarshal(val.Action.Instance, p); err != nil {
				t.Fatalf("failed to unmarshal listen result: %v", err)
			}
			if p.Age != 40 {
				t.Fatalf("expected listen result age = 40 but got: %v", p.Age)
			}
			if val.Action.InstanceID != person.ID {
				t.Fatalf("expected listen result id = %v but got: %v", person.ID, val.Action.InstanceID)
			}
		}
	})
}

func TestClose(t *testing.T) {
	t.Parallel()
	addr, shutdown := makeServer(t)
	defer shutdown()
	target, err := util.TCPAddrFromMultiAddr(addr)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(target, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("test close", func(t *testing.T) {
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func setup(t *testing.T) (*Client, func()) {
	addr, shutdown := makeServer(t)
	target, err := util.TCPAddrFromMultiAddr(addr)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(target, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	return client, func() {
		shutdown()
		_ = client.Close()
	}
}

func makeServer(t *testing.T) (ma.Multiaddr, func()) {
	time.Sleep(time.Second * time.Duration(rand.Intn(5)))
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := db.DefaultService(
		dir,
		db.WithServiceDebug(true))
	if err != nil {
		t.Fatal(err)
	}
	ts.Bootstrap(util.DefaultBoostrapPeers())
	service, err := api.NewService(ts, api.Config{
		RepoPath: dir,
		Debug:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	port, err := freeport.GetFreePort()
	if err != nil {
		t.Fatal(err)
	}
	addr := util.MustParseAddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
	target, err := util.TCPAddrFromMultiAddr(addr)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	listener, err := net.Listen("tcp", target)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		pb.RegisterAPIServer(server, service)
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Fatalf("serve error: %v", err)
		}
	}()

	return addr, func() {
		time.Sleep(time.Second) // Give threads a chance to finish work
		server.GracefulStop()
		if err := ts.Close(); err != nil {
			t.Fatal(err)
		}
		_ = os.RemoveAll(dir)
	}
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func createPerson() *Person {
	return &Person{
		ID:        "",
		FirstName: "Adam",
		LastName:  "Doe",
		Age:       21,
	}
}

const (
	collectionName = "Person"

	schema = `{
	"$id": "https://example.com/person.schema.json",
	"$schema": "http://json-schema.org/draft-07/schema#",
	"title": "` + collectionName + `",
	"type": "object",
	"required": ["ID"],
	"properties": {
		"ID": {
			"type": "string",
			"description": "The instance's id."
		},
		"firstName": {
			"type": "string",
			"description": "The person's first name."
		},
		"lastName": {
			"type": "string",
			"description": "The person's last name."
		},
		"age": {
			"description": "Age in years which must be equal to or greater than zero.",
			"type": "integer",
			"minimum": 0
		}
	}
}`
)

type Person struct {
	ID        string `json:"ID"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	Age       int    `json:"age,omitempty"`
}
