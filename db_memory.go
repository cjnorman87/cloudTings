package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
)

var _ TreatDatabase = &memoryDB{}

// memoryDB is a simple in-memory persistence layer for treats.
type memoryDB struct {
	mu     sync.Mutex
	nextID int64            // next ID to assign to a treat.
	treats  map[string]*Treat // maps from Treat ID to Treat.
}

func newMemoryDB() *memoryDB {
	return &memoryDB{
		treats:  make(map[string]*Treat),
		nextID: 1,
	}
}

// Close closes the database.
func (db *memoryDB) Close(context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.treats = nil

	return nil
}

// GetTreat retrieves a treat by its ID.
func (db *memoryDB) GetTreat(_ context.Context, id string) (*Treat, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	treat, ok := db.treats[id]
	if !ok {
		return nil, fmt.Errorf("memorydb: treat not found with ID %q", id)
	}
	return treat, nil
}

// AddTreat saves a given treat, assigning it a new ID.
func (db *memoryDB) AddTreat(_ context.Context, t *Treat) (id string, err error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	t.ID = strconv.FormatInt(db.nextID, 10)
	db.treats[t.ID] = t

	db.nextID++

	return t.ID, nil
}

// DeleteTreat removes a given treat by its ID.
func (db *memoryDB) DeleteTreat(_ context.Context, id string) error {
	if id == "" {
		return errors.New("memorydb: treat with unassigned ID passed into DeleteTreat")
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if _, ok := db.treats[id]; !ok {
		return fmt.Errorf("memorydb: could not delete treat with ID %q, does not exist", id)
	}
	delete(db.treats, id)
	return nil
}

// UpdateBook updates the entry for a given book.
func (db *memoryDB) UpdateTreat(_ context.Context, t *Treat) error {
	if t.ID == "" {
		return errors.New("memorydb: treat with unassigned ID passed into UpdateTreat")
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	db.treats[t.ID] = t
	return nil
}

// ListBooks returns a list of books, ordered by title.
func (db *memoryDB) ListTreats(_ context.Context) ([]*Treat, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var treats []*Treat
	for _, t := range db.treats {
		treats = append(treats, t)
	}

	sort.Slice(treats, func(i, j int) bool {
		return treats[i].Title < treats[j].Title
	})
	return treats, nil
}