package db

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"testing"

	"github.com/textileio/go-threads/common"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/util"
	"github.com/tidwall/sjson"
)

// Basic benchmarking template. Currently, shows marked speedups for indexes queries.
// The following tests don't push the limits of this in order to keep CI runs shorter.
// @todo: Run more nuanced benchmarks.

const (
	testBenchSchema = `{
		"$schema": "http://json-schema.org/draft-04/schema#",
		"$ref": "#/definitions/bench",
		"definitions": {
		   "bench": {
			  "required": [
				 "ID",
				 "Name",
				 "Age"
			  ],
			  "properties": {
				 "Name": {
					"type": "string"
				 },
				 "Age": {
					"type": "integer"
				 },
				 "ID": {
					"type": "string"
				 }
			  },
			  "additionalProperties": false,
			  "type": "object"
		   }
		}
	 }`
)

var (
	nameSize = 1000
)

func checkBenchErr(b *testing.B, err error) {
	b.Helper()
	if err != nil {
		b.Fatal(err)
	}
}

func createBenchDB(b *testing.B, opts ...Option) (*DB, func()) {
	dir, err := ioutil.TempDir("", "")
	checkBenchErr(b, err)
	n, err := common.DefaultNetwork(dir)
	checkBenchErr(b, err)
	opts = append(opts, WithRepoPath(dir))
	d, err := NewDB(context.Background(), n, thread.NewIDV1(thread.Raw, 32), opts...)
	checkBenchErr(b, err)
	return d, func() {
		if err := n.Close(); err != nil {
			panic(err)
		}
		_ = os.RemoveAll(dir)
	}
}

func BenchmarkNoIndexCreate(b *testing.B) {
	db, clean := createBenchDB(b)
	defer clean()
	collection, err := db.NewCollection(CollectionConfig{Name: "Dog", Schema: util.SchemaFromSchemaString(testBenchSchema)})
	checkBenchErr(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var benchItem = []byte(`{"ID": "", "Name": "Lucas", "Age": 7}`)
		var _, err = collection.Create(benchItem)
		if err != nil {
			b.Fatalf("Error creating instance: %s", err)
		}
	}
}

func BenchmarkIndexCreate(b *testing.B) {
	db, clean := createBenchDB(b)
	defer clean()
	collection, err := db.NewCollection(CollectionConfig{
		Name:   "Dog",
		Schema: util.SchemaFromSchemaString(testBenchSchema),
		Indexes: []IndexConfig{{
			Path:   "Name",
			Unique: false,
		}},
	})
	checkBenchErr(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var benchItem = []byte(`{"ID": "", "Name": "Lucas", "Age": 7}`)
		var _, err = collection.Create(benchItem)
		if err != nil {
			b.Fatalf("Error creating instance: %s", err)
		}
	}
}

func BenchmarkNoIndexSave(b *testing.B) {
	db, clean := createBenchDB(b)
	defer clean()
	collection, err := db.NewCollection(CollectionConfig{Name: "Dog", Schema: util.SchemaFromSchemaString(testBenchSchema)})
	checkBenchErr(b, err)

	var benchItem = []byte(`{"ID": "", "Name": "Lucas", "Age": 7}`)
	res, err := collection.Create(benchItem)
	checkBenchErr(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		updated, err := sjson.SetBytes(benchItem, "ID", res[i].String())
		if err != nil {
			b.Fatalf("Error setting instance id: %s", err)
		}
		updated, err = sjson.SetBytes(updated, "Age", rand.Int())
		if err != nil {
			b.Fatalf("Error modifying instance: %s", err)
		}
		err = collection.Save(updated)
		if err != nil {
			b.Fatalf("Error creating instance: %s", err)
		}
	}
}

func BenchmarkIndexSave(b *testing.B) {
	db, clean := createBenchDB(b)
	defer clean()
	collection, err := db.NewCollection(CollectionConfig{
		Name:   "Dog",
		Schema: util.SchemaFromSchemaString(testBenchSchema),
		Indexes: []IndexConfig{{
			Path:   "Age",
			Unique: false,
		}},
	})
	checkBenchErr(b, err)

	var benchItem = []byte(`{"ID": "", "Name": "Lucas", "Age": 7}`)
	res, err := collection.Create(benchItem)
	checkBenchErr(b, err)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		updated, err := sjson.SetBytes(benchItem, "ID", res[i].String())
		if err != nil {
			b.Fatalf("Error setting instance id: %s", err)
		}
		updated, err = sjson.SetBytes(updated, "Age", rand.Int())
		if err != nil {
			b.Fatalf("Error modifying instance: %s", err)
		}
		err = collection.Save(updated)
		if err != nil {
			b.Fatalf("Error creating instance: %s", err)
		}
	}
}

func BenchmarkNoIndexFind(b *testing.B) {
	db, clean := createBenchDB(b)
	defer clean()
	collection, err := db.NewCollection(CollectionConfig{Name: "Dog", Schema: util.SchemaFromSchemaString(testBenchSchema)})
	checkBenchErr(b, err)

	for j := 0; j < 10; j++ {
		for i := 0; i < nameSize; i++ {
			var benchItem = []byte(`{"ID": "", "Name": "Name", "Age": 7}`)
			newItem, err := sjson.SetBytes(benchItem, "Name", fmt.Sprintf("Name%d", j))
			if err != nil {
				b.Fatalf("Error modifying instance: %s", err)
			}
			_, err = collection.Create(newItem)
			if err != nil {
				b.Fatalf("Error creating instance: %s", err)
			}
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := collection.Find(Where("Name").Eq("Name0").Or(Where("Name").Eq("Name6")))
		if err != nil {
			b.Fatalf("Error finding data: %s", err)
		}
		if len(result) != 2*nameSize {
			b.Fatalf("Unexpected length %d, should be %d", len(result), nameSize)
		}
	}
}

func BenchmarkIndexFind(b *testing.B) {
	db, clean := createBenchDB(b)
	defer clean()
	collection, err := db.NewCollection(CollectionConfig{
		Name:   "Dog",
		Schema: util.SchemaFromSchemaString(testBenchSchema),
		Indexes: []IndexConfig{{
			Path:   "Name",
			Unique: false,
		}},
	})
	checkBenchErr(b, err)

	for j := 0; j < 10; j++ {
		for i := 0; i < nameSize; i++ {
			var benchItem = []byte(`{"ID": "", "Name": "Name", "Age": 7}`)
			newItem, err := sjson.SetBytes(benchItem, "Name", fmt.Sprintf("Name%d", j))
			if err != nil {
				b.Fatalf("Error modifying instance: %s", err)
			}
			_, err = collection.Create(newItem)
			if err != nil {
				b.Fatalf("Error creating instance: %s", err)
			}
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := collection.Find(Where("Name").Eq("Name0").Or(Where("Name").Eq("Name6")).UseIndex("Name"))
		if err != nil {
			b.Fatalf("Error finding data: %s", err)
		}
		if len(result) != 2*nameSize {
			b.Fatalf("Unexpected length %d, should be %d", len(result), nameSize)
		}
	}
}
