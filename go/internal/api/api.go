// Package api is contacts-api-impl.xml on net/http: the two HTTP flows, their validations and
// their DataWeave transforms.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"muletocode/internal/db"
	"muletocode/internal/zippo"
)

// Repository is the database side of get-contacts-by-city-flow; *db.Repository implements it.
type Repository interface {
	// QueryByCity is <db:select doc:name="Select contacts by city">.
	QueryByCity(ctx context.Context, city string, limit int) ([]db.ContactRow, error)
}

// PostalLookup is the outbound side of normalize-contacts-flow; *zippo.Client implements it.
type PostalLookup interface {
	// Lookup is <http:request method="GET" path="/{country}/{postalCode}">.
	Lookup(ctx context.Context, country, postalCode string) (zippo.Wire, error)
}

type server struct {
	repo        Repository
	lookup      PostalLookup
	maxContacts int
	log         *slog.Logger
}

// NewHandler registers the two flows on a ServeMux: GET /contacts and POST /contacts/normalize.
// maxContacts is ${normalize.max_contacts}. A nil logger discards the flows' error loggers.
func NewHandler(repo Repository, lookup PostalLookup, maxContacts int, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &server{repo: repo, lookup: lookup, maxContacts: maxContacts, log: logger}
	mux := http.NewServeMux()
	// <http:listener path="/contacts" allowedMethods="GET">
	mux.HandleFunc("GET /contacts", s.getContactsByCity)
	// <http:listener path="/contacts/normalize" allowedMethods="POST">
	mux.HandleFunc("POST /contacts/normalize", s.normalizeContacts)
	return recoverer(mux)
}

// errorBody is the <http:error-response> body: {error: error.description}.
type errorBody struct {
	Error string `json:"error"`
}

// writeError is <http:error-response statusCode="#[vars.httpStatus default 500]">.
func writeError(w http.ResponseWriter, status int, description string) {
	writeJSON(w, status, errorBody{Error: description})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body) // the client has gone if this fails; nothing left to report to
}

// recoverer turns a panic in a handler into the 500 the error-response default would produce.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprint(p))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
