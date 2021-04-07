package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"runtime/debug"

	"cloud.google.com/go/errorreporting"
	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/gofrs/uuid"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

var (
	// See template.go.
	listTmpl   = parseTemplate("list.html")
	editTmpl   = parseTemplate("edit.html")
	aboutTmpl  = parseTemplate("about.html")
	detailTmpl = parseTemplate("detail.html")
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT must be set")
	}

	ctx := context.Background()

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("firestore.NewClient: %v", err)
	}
	db, err := newFirestoreDB(client)
	if err != nil {
		log.Fatalf("newFirestoreDB: %v", err)
	}
	t, err := NewTreatshelf(projectID, db)
	if err != nil {
		log.Fatalf("NewTreatshelf: %v", err)
	}

	t.registerHandlers()

	log.Printf("Listening on localhost:%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func (t *Treatshelf) registerHandlers() {
	// Use gorilla/mux for rich routing.
	// See https://www.gorillatoolkit.org/pkg/mux.
	r := mux.NewRouter()

	r.Handle("/", http.RedirectHandler("/treats", http.StatusFound))

	r.Methods("GET").Path("/treats").
		Handler(appHandler(t.listHandler))
	r.Methods("GET").Path("/treats/add").
		Handler(appHandler(t.addFormHandler))
	r.Methods("GET").Path("/about").
		Handler(appHandler(t.addAboutHandler))
	r.Methods("GET").Path("/treats/{id:[0-9a-zA-Z_\\-]+}").
		Handler(appHandler(t.detailHandler))
	r.Methods("GET").Path("/treats/{id:[0-9a-zA-Z_\\-]+}/edit").
		Handler(appHandler(t.editFormHandler))

	r.Methods("POST").Path("/treats").
		Handler(appHandler(t.createHandler))
	r.Methods("POST", "PUT").Path("/treats/{id:[0-9a-zA-Z_\\-]+}").
		Handler(appHandler(t.updateHandler))
	r.Methods("POST").Path("/treats/{id:[0-9a-zA-Z_\\-]+}:delete").
		Handler(appHandler(t.deleteHandler)).Name("delete")

	r.Methods("GET").Path("/logs").Handler(appHandler(t.sendLog))
	r.Methods("GET").Path("/errors").Handler(appHandler(t.sendError))

	// Delegate all of the HTTP routing and serving to the gorilla/mux router.
	// Log all requests using the standard Apache format.
	http.Handle("/", handlers.CombinedLoggingHandler(t.logWriter, r))
}

// listHandler displays a list with summaries of treats in the database.
func (t *Treatshelf) listHandler(w http.ResponseWriter, r *http.Request) *appError {
	ctx := r.Context()
	treats, err := t.DB.ListTreats(ctx)
	if err != nil {
		return t.appErrorf(r, err, "could not list treats: %v", err)
	}

	return listTmpl.Execute(t, w, r, treats)
}

// treatFromRequest retrieves a treat from the database given a treat ID in the
// URL's path.
func (t *Treatshelf) treatFromRequest(r *http.Request) (*Treat, error) {
	ctx := r.Context()
	id := mux.Vars(r)["id"]
	if id == "" {
		return nil, errors.New("no treat with empty ID")
	}
	treat, err := t.DB.GetTreat(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("could not find treat: %v", err)
	}
	return treat, nil
}

// detailHandler displays the details of a given treat.
func (t *Treatshelf) detailHandler(w http.ResponseWriter, r *http.Request) *appError {
	treat, err := t.treatFromRequest(r)
	if err != nil {
		return t.appErrorf(r, err, "%v", err)
	}

	return detailTmpl.Execute(t, w, r, treat)
}

// addFormHandler displays a form that captures details of a new treat to add to
// the database.
func (t *Treatshelf) addFormHandler(w http.ResponseWriter, r *http.Request) *appError {
	return editTmpl.Execute(t, w, r, nil)
}

// addFormHandler displays a form that captures details of a new treat to add to
// the database.
func (t *Treatshelf) addAboutHandler(w http.ResponseWriter, r *http.Request) *appError {
	return about.Execute(t, w, r, nil)
}

// editFormHandler displays a form that allows the user to edit the details of
// a given treat.
func (t *Treatshelf) editFormHandler(w http.ResponseWriter, r *http.Request) *appError {
	treat, err := t.treatFromRequest(r)
	if err != nil {
		return t.appErrorf(r, err, "%v", err)
	}

	return editTmpl.Execute(t, w, r, treat)
}

// treatFromForm populates the fields of a Treat from form values
// (see templates/edit.html).
func (t *Treatshelf) treatFromForm(r *http.Request) (*Treat, error) {
	ctx := r.Context()
	imageURL, err := t.uploadFileFromForm(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("could not upload file: %v", err)
	}
	if imageURL == "" {
		imageURL = r.FormValue("imageURL")
	}

	treat := &Treat{
		Title:         r.FormValue("title"),
		Author:        r.FormValue("author"),
		PublishedDate: r.FormValue("publishedDate"),
		ImageURL:      imageURL,
		Description:   r.FormValue("description"),
	}

	return treat, nil
}

// uploadFileFromForm uploads a file if it's present in the "image" form field.
func (t *Treatshelf) uploadFileFromForm(ctx context.Context, r *http.Request) (url string, err error) {
	f, fh, err := r.FormFile("image")
	if err == http.ErrMissingFile {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	if t.StorageBucket == nil {
		return "", errors.New("storage bucket is missing: check treat.go")
	}
	if _, err := t.StorageBucket.Attrs(ctx); err != nil {
		if err == storage.ErrBucketNotExist {
			return "", fmt.Errorf("bucket %q does not exist: check treat.go", t.StorageBucketName)
		}
		return "", fmt.Errorf("could not get bucket: %v", err)
	}

	// random filename, retaining existing extension.
	name := uuid.Must(uuid.NewV4()).String() + path.Ext(fh.Filename)

	w := t.StorageBucket.Object(name).NewWriter(ctx)

	// Warning: storage.AllUsers gives public read access to anyone.
	w.ACL = []storage.ACLRule{{Entity: storage.AllUsers, Role: storage.RoleReader}}
	w.ContentType = fh.Header.Get("Content-Type")

	// Entries are immutable, be aggressive about caching (1 day).
	w.CacheControl = "public, max-age=86400"

	if _, err := io.Copy(w, f); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	const publicURL = "https://storage.googleapis.com/%s/%s"
	return fmt.Sprintf(publicURL, t.StorageBucketName, name), nil
}

// createHandler adds a treat to the database.
func (t *Treatshelf) createHandler(w http.ResponseWriter, r *http.Request) *appError {
	ctx := r.Context()
	treat, err := t.treatFromForm(r)
	if err != nil {
		return t.appErrorf(r, err, "could not parse treat from form: %v", err)
	}
	id, err := t.DB.AddTreat(ctx, treat)
	if err != nil {
		return t.appErrorf(r, err, "could not save treat: %v", err)
	}
	http.Redirect(w, r, fmt.Sprintf("/treats/%s", id), http.StatusFound)
	return nil
}

// updateHandler updates the details of a given treat.
func (t *Treatshelf) updateHandler(w http.ResponseWriter, r *http.Request) *appError {
	ctx := r.Context()
	id := mux.Vars(r)["id"]
	if id == "" {
		return t.appErrorf(r, errors.New("no treat with empty ID"), "no treat with empty ID")
	}
	treat, err := t.treatFromForm(r)
	if err != nil {
		return t.appErrorf(r, err, "could not parse treat from form: %v", err)
	}
	treat.ID = id

	if err := t.DB.UpdateTreat(ctx, treat); err != nil {
		return t.appErrorf(r, err, "UpdateTreat: %v", err)
	}
	http.Redirect(w, r, fmt.Sprintf("/treats/%s", treat.ID), http.StatusFound)
	return nil
}

// deleteHandler deletes a given treat.
func (t *Treatshelf) deleteHandler(w http.ResponseWriter, r *http.Request) *appError {
	ctx := r.Context()
	id := mux.Vars(r)["id"]
	if err := t.DB.DeleteTreat(ctx, id); err != nil {
		return t.appErrorf(r, err, "DeleteTreat: %v", err)
	}
	http.Redirect(w, r, "/treats", http.StatusFound)
	return nil
}

// sendLog logs a message.
//
// See https://cloud.google.com/logging/docs/setup/go for how to use the
// Stackdriver logging client. Output to stdout and stderr is automaticaly
// sent to Stackdriver when running on App Engine.
func (t *Treatshelf) sendLog(w http.ResponseWriter, r *http.Request) *appError {
	fmt.Fprintln(t.logWriter, "Hey, you triggered a custom log entry. Good job!")

	fmt.Fprintln(w, `<html>Log sent! Check the <a href="http://console.cloud.google.com/logs">logging section of the Cloud Console</a>.</html>`)

	return nil
}

// sendError triggers an error that is sent to Error Reporting.
func (t *Treatshelf) sendError(w http.ResponseWriter, r *http.Request) *appError {
	msg := `<html>Logging an error. Check <a href="http://console.cloud.google.com/errors">Error Reporting</a> (it may take a minute or two for the error to appear).</html>`
	err := errors.New("uh oh! an error occurred")
	return t.appErrorf(r, err, msg)
}

// https://blog.golang.org/error-handling-and-go
type appHandler func(http.ResponseWriter, *http.Request) *appError

type appError struct {
	err     error
	message string
	code    int
	req     *http.Request
	t       *Treatshelf
	stack   []byte
}

func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if e := fn(w, r); e != nil { // e is *appError, not os.Error.
		fmt.Fprintf(e.t.logWriter, "Handler error (reported to Error Reporting): status code: %d, message: %s, underlying err: %+v\n", e.code, e.message, e.err)
		w.WriteHeader(e.code)
		fmt.Fprint(w, e.message)

		e.t.errorClient.Report(errorreporting.Entry{
			Error: e.err,
			Req:   r,
			Stack: e.stack,
		})
		e.t.errorClient.Flush()
	}
}

func (t *Treatshelf) appErrorf(r *http.Request, err error, format string, v ...interface{}) *appError {
	return &appError{
		err:     err,
		message: fmt.Sprintf(format, v...),
		code:    500,
		req:     r,
		t:       t,
		stack:   debug.Stack(),
	}
}