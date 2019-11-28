package client

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/textileio/go-textile-threads/api"
	es "github.com/textileio/go-textile-threads/eventstore"
	"github.com/textileio/go-textile-threads/util"
)

const modelName = "Person"

const schema = `{
	"$id": "https://example.com/person.schema.json",
	"$schema": "http://json-schema.org/draft-07/schema#",
	"title": "` + modelName + `",
	"type": "object",
	"required": ["ID"],
	"properties": {
		"ID": {
			"type": "string",
			"description": "The entity's id."
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

type Person struct {
	ID        string `json:"ID"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	Age       int    `json:"age,omitempty"`
}

func createPerson() *Person {
	return &Person{
		ID:        "",
		FirstName: "Adam",
		LastName:  "Doe",
		Age:       21,
	}
}

func TestNewStore(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	_, err := client.NewStore()
	if err != nil {
		t.Fatalf("failed to create new store: %v", err)
	}
}

func TestRegisterSchema(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	if err != nil {
		t.Fatalf("failed to register schema: %v", err)
	}
}

func TestStart(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
}

func TestStartFromAddress(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)

	// TODO: figure out how to test this
	// client.StartFromAddress()
}

func TestModelCreate(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)

	err = client.ModelCreate(storeID, modelName, createPerson())
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}
}

func TestModelSave(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)

	person := createPerson()

	err = client.ModelCreate(storeID, modelName, person)
	checkErr(t, err)

	person.Age = 30
	err = client.ModelSave(storeID, modelName, person)
	if err != nil {
		t.Fatalf("failed to save model: %v", err)
	}
}

func TestModelDelete(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)

	person := createPerson()

	err = client.ModelCreate(storeID, modelName, person)
	checkErr(t, err)

	err = client.ModelDelete(storeID, modelName, person.ID)
	if err != nil {
		t.Fatalf("failed to delete model: %v", err)
	}
}

func TestModelHas(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)

	person := createPerson()

	err = client.ModelCreate(storeID, modelName, person)
	checkErr(t, err)

	exists, err := client.ModelHas(storeID, modelName, person.ID)
	if err != nil {
		t.Fatalf("failed to check model has: %v", err)
	}
	if !exists {
		t.Fatal("model should exist but it doesn't")
	}
}

func TestModelFind(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)

	person := createPerson()

	// TODO: this add is prob wrong, see todo in ModelCreate
	err = client.ModelCreate(storeID, modelName, person)
	checkErr(t, err)

	q := es.JSONQuery{
		Ands: []es.JSONCriterion{
			es.JSONCriterion{
				FieldPath: "lastName",
				Operation: es.Eq,
				Value: es.JSONValue{
					String: &person.LastName,
				},
			},
		},
	}

	// TODO: this is failing with "error when matching entry with query: instance field lastName do..."
	foo, err := client.ModelFind(storeID, modelName, q)
	if err != nil {
		t.Fatalf("failed to find: %v", err)
	}
	t.Logf("got resulst: %v", foo)
}

func TestModelFindByID(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)

	person := createPerson()

	err = client.ModelCreate(storeID, modelName, person)
	checkErr(t, err)

	newPerson := &Person{}
	err = client.ModelFindByID(storeID, modelName, person.ID, newPerson)
	if err != nil {
		t.Fatalf("failed to find model by id: %v", err)
	}
	// TODO: seems that the newPerson has the correct ID but default values for everything else
	if !reflect.DeepEqual(newPerson, person) {
		t.Fatal("model found by id does't equal the original")
	}
}

func TestReadTransaction(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, err := client.NewStore()
	checkErr(t, err)
	err = client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)
	person := createPerson()
	err = client.ModelCreate(storeID, modelName, person)
	checkErr(t, err)

	txn, err := client.ReadTransaction(storeID, modelName)
	if err != nil {
		t.Fatalf("failed to create read txn: %v", err)
	}

	err = txn.Start()
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

	newPerson := &Person{}
	err = txn.FindByID(person.ID, newPerson)
	if err != nil {
		t.Fatalf("failed to txn find by id: %v", err)
	}
	// TODO: seems that the newPerson has the correct ID but default values for everything else
	if !reflect.DeepEqual(newPerson, person) {
		t.Fatal("txn model found by id does't equal the original")
	}

	err = txn.End()
	if err != nil {
		t.Fatalf("failed to end txn: %v", err)
	}
}

func TestWriteTransaction(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, err := client.NewStore()
	checkErr(t, err)
	err = client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)

	txn, err := client.WriteTransaction(storeID, modelName)
	if err != nil {
		t.Fatalf("failed to create write txn: %v", err)
	}

	err = txn.Start()
	if err != nil {
		t.Fatalf("failed to start write txn: %v", err)
	}

	person := createPerson()

	err = txn.Create(person)
	if err != nil {
		t.Fatalf("failed to create in write txn: %v", err)
	}
	if person.ID == "" {
		t.Fatal("expected an entity id to be set but it wasn't")
	}

	has, err := txn.Has(person.ID)
	if err != nil {
		t.Fatalf("failed to write txn has: %v", err)
	}
	if !has {
		t.Fatal("expected has to be true but it wasn't")
	}

	newPerson := &Person{}
	err = txn.FindByID(person.ID, newPerson)
	if err != nil {
		t.Fatalf("failed to txn find by id: %v", err)
	}
	// TODO: seems that the newPerson has the correct ID but default values for everything else
	if !reflect.DeepEqual(newPerson, person) {
		t.Fatal("txn model found by id does't equal the original")
	}

	person.Age = 99
	err = txn.Save(person)
	if err != nil {
		t.Fatalf("failed to save in write txn: %v", err)
	}

	err = txn.Delete(person.ID)
	if err != nil {
		t.Fatalf("failed to delete in write txn: %v", err)
	}

	err = txn.End()
	if err != nil {
		t.Fatalf("failed to end txn: %v", err)
	}
}

func TestListen(t *testing.T) {
	_, clean := server(t)
	defer clean()
	client := client(t)

	storeID, _ := client.NewStore()
	err := client.RegisterSchema(storeID, modelName, schema)
	checkErr(t, err)
	err = client.Start(storeID)
	checkErr(t, err)

	person := createPerson()

	err = client.ModelCreate(storeID, modelName, person)
	checkErr(t, err)

	channel, err := client.Listen(storeID, modelName, person.ID, &Person{})
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	go func() {
		person.Age = 30
		_ = client.ModelSave(storeID, modelName, person)
		person.Age = 40
		_ = client.ModelSave(storeID, modelName, person)
	}()

	val, ok := <-channel
	if !ok {
		t.Fatal("channel no longer active at first event")
	} else {
		fmt.Println(val, ok)
	}

	val, ok = <-channel
	if !ok {
		t.Fatal("channel no longer active at second event")
	} else {
		fmt.Println(val, ok)
	}
}

func server(t *testing.T) (*api.Server, func()) {
	dir := "/tmp/threads"
	ts, err := es.DefaultThreadservice(
		dir,
		es.ListenPort(4006),
		es.ProxyPort(5050),
		es.Debug(true))
	checkErr(t, err)
	ts.Bootstrap(util.DefaultBoostrapPeers())

	server, err := api.NewServer(context.Background(), ts, api.Config{
		RepoPath: dir,
		Debug:    true,
	})
	checkErr(t, err)
	return server, func() {
		if err := ts.Close(); err != nil {
			panic(err)
		}
		server.Close()
		os.RemoveAll(dir)
	}
}

func client(t *testing.T) *Client {
	client, err := NewClient("localhost", 9090)
	checkErr(t, err)
	return client
}

func checkErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
