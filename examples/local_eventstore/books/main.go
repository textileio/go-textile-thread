package main

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	core "github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/db"
)

type book struct {
	ID     core.InstanceID
	Title  string
	Author string
	Meta   bookStats
}

type bookStats struct {
	TotalReads int
	Rating     float64
}

func main() {
	d, clean := createMemDB()
	defer clean()

	collection, err := d.NewCollectionFromInstance("Book", &book{})
	checkErr(err)

	// Bootstrap the collection with some books: two from Author1 and one from Author2
	{
		// Create a two books for Author1
		book1 := &book{ // Notice ID will be autogenerated
			Title:  "Title1",
			Author: "Author1",
			Meta:   bookStats{TotalReads: 100, Rating: 3.2},
		}
		book2 := &book{
			Title:  "Title2",
			Author: "Author1",
			Meta:   bookStats{TotalReads: 150, Rating: 4.1},
		}
		checkErr(collection.Create(book1, book2)) // Note you can create multiple books at the same time (variadic)

		// Create book for Author2
		book3 := &book{
			Title:  "Title3",
			Author: "Author2",
			Meta:   bookStats{TotalReads: 500, Rating: 4.9},
		}
		checkErr(collection.Create(book3))
	}

	// Query all the books
	{
		var books []*book
		err := collection.Find(&books, &db.Query{})
		checkErr(err)
		if len(books) != 3 {
			panic("there should be three books")
		}
	}

	// Query the books from Author2
	{
		var books []*book
		err := collection.Find(&books, db.Where("Author").Eq("Author1"))
		checkErr(err)
		if len(books) != 2 {
			panic("Author1 should have two books")
		}
	}

	// Query with nested condition
	{
		var books []*book
		err := collection.Find(&books, db.Where("Meta.TotalReads").Eq(100))
		checkErr(err)
		if len(books) != 1 {
			panic("There should be one book with 100 total reads")
		}
	}

	// Query book by two conditions
	{
		var books []*book
		err := collection.Find(&books, db.Where("Author").Eq("Author1").And("Title").Eq("Title2"))
		checkErr(err)
		if len(books) != 1 {
			panic("Author1 should have only one book with Title2")
		}
	}

	// Query book by OR condition
	{
		var books []*book
		err := collection.Find(&books, db.Where("Author").Eq("Author1").Or(db.Where("Author").Eq("Author2")))
		checkErr(err)
		if len(books) != 3 {
			panic("Author1 & Author2 have should have 3 books in total")
		}
	}

	// Sorted query
	{
		var books []*book
		// Ascending
		err := collection.Find(&books, db.Where("Author").Eq("Author1").OrderBy("Meta.TotalReads"))
		checkErr(err)
		if books[0].Meta.TotalReads != 100 || books[1].Meta.TotalReads != 150 {
			panic("books aren't ordered asc correctly")
		}
		// Descending
		err = collection.Find(&books, db.Where("Author").Eq("Author1").OrderByDesc("Meta.TotalReads"))
		checkErr(err)
		if books[0].Meta.TotalReads != 150 || books[1].Meta.TotalReads != 100 {
			panic("books aren't ordered desc correctly")
		}
	}

	// Query, Update, and Save
	{
		var books []*book
		err := collection.Find(&books, db.Where("Title").Eq("Title3"))
		checkErr(err)

		// Modify title
		book := books[0]
		book.Title = "ModifiedTitle"
		_ = collection.Save(book)
		err = collection.Find(&books, db.Where("Title").Eq("Title3"))
		checkErr(err)
		if len(books) != 0 {
			panic("Book with Title3 shouldn't exist")
		}

		// Delete it
		err = collection.Find(&books, db.Where("Title").Eq("ModifiedTitle"))
		checkErr(err)
		if len(books) != 1 {
			panic("Book with ModifiedTitle should exist")
		}
		_ = collection.Delete(books[0].ID)
		err = collection.Find(&books, db.Where("Title").Eq("ModifiedTitle"))
		checkErr(err)
		if len(books) != 0 {
			panic("Book with ModifiedTitle shouldn't exist")
		}
	}
}

func createMemDB() (*db.DB, func()) {
	dir, err := ioutil.TempDir("", "")
	checkErr(err)
	ts, err := db.DefaultService(dir)
	checkErr(err)
	id := thread.NewIDV1(thread.Raw, 32)
	d, err := db.NewDB(context.Background(), ts, id, db.WithRepoPath(dir))
	checkErr(err)
	return d, func() {
		time.Sleep(time.Second) // Give threads a chance to finish work
		if err := ts.Close(); err != nil {
			panic(err)
		}
		_ = os.RemoveAll(dir)
	}
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}
