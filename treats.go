package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"cloud.google.com/go/errorreporting"
	"cloud.google.com/go/storage"
)

// Treat holds metadata about a treat.
type Treat struct {
	ID            string
	Title         string
	Author        string
	PublishedDate string
	ImageURL      string
	Description   string
}

// TreatDatabase provides thread-safe access to a database of treats.
type TreatDatabase interface {
	// ListTreats returns a list of Treats, ordered by title.
	ListTreats(context.Context) ([]*Treat, error)

	// GetTreat retrieves a Treat by its ID.
	GetTreat(ctx context.Context, id string) (*Treat, error)

	// AddTreat saves a given Treat, assigning it a new ID.
	AddTreat(ctx context.Context, t *Treat) (id string, err error)

	// DeleteTreat removes a given Treat by its ID.
	DeleteTreat(ctx context.Context, id string) error

	// UpdateTreat updates the entry for a given Treat.
	UpdateTreat(ctx context.Context, t *Treat) error
}

// Treatshelf holds a TreatDatabase and storage info.
type Treatshelf struct {
	DB TreatDatabase

	StorageBucket     *storage.BucketHandle
	StorageBucketName string

	// logWriter is used for request logging and can be overridden for tests.
	//
	// See https://cloud.google.com/logging/docs/setup/go for how to use the
	// Cloud Logging client. Output to stdout and stderr is automaticaly
	// sent to Cloud Logging when running on App Engine.
	logWriter io.Writer

	errorClient *errorreporting.Client
}

// NewTreatshelf creates a new Treatshelf.
func NewTreatshelf(projectID string, db TreatDatabase) (*Treatshelf, error) {
	ctx := context.Background()

	// This Cloud Storage bucket must exist to be able to upload treat pictures.
	// You can create it and make it public by running:
	//     gsutil mb my-project_bucket
	//     gsutil defacl set public-read gs://my-project_bucket
	// replacing my-project with your project ID.
	bucketName := projectID + "_bucket"
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.NewClient: %v", err)
	}

	errorClient, err := errorreporting.NewClient(ctx, projectID, errorreporting.Config{
		ServiceName: "Treatshelf",
		OnError: func(err error) {
			fmt.Fprintf(os.Stderr, "Could not log error: %v", err)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("errorreporting.NewClient: %v", err)
	}

	t := &Treatshelf{
		logWriter:         os.Stderr,
		errorClient:       errorClient,
		DB:                db,
		StorageBucketName: bucketName,
		StorageBucket:     storageClient.Bucket(bucketName),
	}
	return t, nil
}