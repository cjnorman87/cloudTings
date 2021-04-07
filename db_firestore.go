package main

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// firestoreDB persists books to Cloud Firestore.
// See https://cloud.google.com/firestore/docs.
type firestoreDB struct {
	client     *firestore.Client
	collection string
}

// Ensure firestoreDB conforms to the TreatDatabase interface.
var _ TreatDatabase = &firestoreDB{}

// [START getting_started_bookshelf_firestore]

// newFirestoreDB creates a new BookDatabase backed by Cloud Firestore.
// See the firestore package for details on creating a suitable
// firestore.Client: https://godoc.org/cloud.google.com/go/firestore.
func newFirestoreDB(client *firestore.Client) (*firestoreDB, error) {
	ctx := context.Background()
	// Verify that we can communicate and authenticate with the Firestore
	// service.
	err := client.RunTransaction(ctx, func(ctx context.Context, t *firestore.Transaction) error {
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("firestoredb: could not connect: %v", err)
	}
	return &firestoreDB{
		client:     client,
		collection: "books",
	}, nil
}

// Close closes the database.
func (db *firestoreDB) Close(context.Context) error {
	return db.client.Close()
}

// Book retrieves a book by its ID.
func (db *firestoreDB) GetTreat(ctx context.Context, id string) (*Treat, error) {
	ds, err := db.client.Collection(db.collection).Doc(id).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("firestoredb: Get: %v", err)
	}
	t := &Treat{}
	ds.DataTo(t)
	return t, nil
}

// [END getting_started_bookshelf_firestore]

// AddBook saves a given book, assigning it a new ID.
func (db *firestoreDB) AddTreat(ctx context.Context, t *Treat) (id string, err error) {
	ref := db.client.Collection(db.collection).NewDoc()
	t.ID = ref.ID
	if _, err := ref.Create(ctx, t); err != nil {
		return "", fmt.Errorf("Create: %v", err)
	}
	return ref.ID, nil
}

// DeleteBook removes a given book by its ID.
func (db *firestoreDB) DeleteTreat(ctx context.Context, id string) error {
	if _, err := db.client.Collection(db.collection).Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("firestore: Delete: %v", err)
	}
	return nil
}

// UpdateBook updates the entry for a given book.
func (db *firestoreDB) UpdateTreat(ctx context.Context, t *Treat) error {
	if _, err := db.client.Collection(db.collection).Doc(t.ID).Set(ctx, t); err != nil {
		return fmt.Errorf("firestsore: Set: %v", err)
	}
	return nil
}

// ListTreats returns a list of treats, ordered by title.
func (db *firestoreDB) ListTreats(ctx context.Context) ([]*Treat, error) {
	treats := make([]*Treat, 0)
	iter := db.client.Collection(db.collection).Query.OrderBy("Title", firestore.Asc).Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("firestoredb: could not list books: %v", err)
		}
		t := &Treat{}
		doc.DataTo(t)
		log.Printf("Treat %q ID: %q", t.Title, t.ID)
		treats = append(treats, t)
	}

	return treats, nil
}