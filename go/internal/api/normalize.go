package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"muletocode/internal/zippo"
)

var (
	// ErrContactsPresent is the message of <validation:is-true doc:name="contacts present">.
	ErrContactsPresent = errors.New("Body must contain a non-empty 'contacts' array")
	// ErrPostalCodeRequired is the first message inside <validation:all>.
	ErrPostalCodeRequired = errors.New("Every contact needs 'mailing_postal_code'")
	// ErrCountryRequired is the second message inside <validation:all>.
	ErrCountryRequired = errors.New("Every contact needs 'mailing_country'")
)

// TooManyContactsError is the message of <validation:is-true doc:name="contacts not too many">:
// `'At most ' ++ p('normalize.max_contacts') ++ ' contacts per request'`.
func TooManyContactsError(maxContacts int) error {
	return fmt.Errorf("At most %d contacts per request", maxContacts)
}

// NormalizeRequest is the request body of POST /contacts/normalize (NormalizeRequest in openapi.yaml).
type NormalizeRequest struct {
	// Contacts is `payload.contacts default []`.
	Contacts []NormalizeContact `json:"contacts"`
}

// NormalizeContact is one input contact of NormalizeRequest.
type NormalizeContact struct {
	ContactID         *string `json:"contact_id"`
	MailingPostalCode *string `json:"mailing_postal_code"`
	MailingCountry    *string `json:"mailing_country"`
}

// NormalizeResult is one element of NormalizeResponse.Results.
type NormalizeResult struct {
	ContactID  *string      `json:"contact_id"`
	PostalCode *string      `json:"postal_code"`
	Country    *string      `json:"country"`
	Status     string       `json:"status"`
	Place      *zippo.Place `json:"place"`
}

// NormalizeResponse is the response body of POST /contacts/normalize (NormalizeResponse in openapi.yaml).
type NormalizeResponse struct {
	Count   int               `json:"count"`
	Results []NormalizeResult `json:"results"`
}

// ValidateNormalize is the three validations of normalize-contacts-flow, in order, all before any
// upstream call. It returns the error whose text is the message of the first failing one.
// <validation:all> reports every failing check of its group joined by newlines (a
// VALIDATION:MULTIPLE error), so both ErrPostalCodeRequired and ErrCountryRequired may be present.
func ValidateNormalize(contacts []NormalizeContact, maxContacts int) error {
	// <validation:is-true expression="#[sizeOf(vars.contacts) > 0]">
	if len(contacts) == 0 {
		return ErrContactsPresent
	}
	// <validation:is-true expression="#[sizeOf(vars.contacts) <= (p('normalize.max_contacts') as Number)]">
	if len(contacts) > maxContacts {
		return TooManyContactsError(maxContacts)
	}
	// <validation:all doc:name="each contact has postal code and country">
	var failures []error
	if !every(contacts, func(c NormalizeContact) bool { return !isBlank(c.MailingPostalCode) }) {
		failures = append(failures, ErrPostalCodeRequired)
	}
	if !every(contacts, func(c NormalizeContact) bool { return !isBlank(c.MailingCountry) }) {
		failures = append(failures, ErrCountryRequired)
	}
	return errors.Join(failures...)
}

// ZippoToResult is <ee:transform doc:name="Zippopotam to result">: status "ok" with places[0].
func ZippoToResult(contact NormalizeContact, wire zippo.Wire) (NormalizeResult, error) {
	place, err := zippo.FromWire(wire)
	if err != nil {
		return NormalizeResult{}, err
	}
	return NormalizeResult{
		ContactID:  contact.ContactID,
		PostalCode: contact.MailingPostalCode,
		Country:    upper(contact.MailingCountry),
		Status:     "ok",
		Place:      &place,
	}, nil
}

// NotFoundResult is <ee:transform doc:name="Not found result"> inside <on-error-continue type="HTTP:NOT_FOUND">.
func NotFoundResult(contact NormalizeContact) NormalizeResult {
	return NormalizeResult{
		ContactID:  contact.ContactID,
		PostalCode: contact.MailingPostalCode,
		Country:    upper(contact.MailingCountry),
		Status:     "not_found",
		Place:      nil,
	}
}

// ResultsToJSON is <ee:transform doc:name="Results to JSON">.
func ResultsToJSON(results []NormalizeResult) NormalizeResponse {
	if results == nil {
		results = []NormalizeResult{}
	}
	return NormalizeResponse{Count: len(results), Results: results}
}

// normalizeContacts is <flow name="normalize-contacts-flow">.
func (s *server) normalizeContacts(w http.ResponseWriter, r *http.Request) {
	// The listener only parses a JSON body; anything else fails `payload.contacts` with an EXPRESSION error → 400.
	if r.ContentLength > 0 && !hasJSONContentType(r) {
		writeError(w, http.StatusBadRequest, "Request body must be application/json")
		return
	}
	var body NormalizeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		// An unparsable body is an EXPRESSION error in Mule → 400. An empty body is not: `payload.contacts
		// default []` yields [], and the first validation answers.
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	// <set-variable variableName="contacts" value="#[payload.contacts default []]">
	contacts := body.Contacts

	// <on-error-propagate type="VALIDATION:INVALID_BOOLEAN, VALIDATION:MULTIPLE, EXPRESSION"> → 400
	if err := ValidateNormalize(contacts, s.maxContacts); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// <set-variable variableName="results" value="#[[]]">
	results := make([]NormalizeResult, 0, len(contacts))
	// <foreach doc:name="For each contact" collection="#[vars.contacts]">
	for _, contact := range contacts {
		// <try doc:name="Try lookup"> ... <http:request doc:name="GET /{country}/{postal-code}" path="/{country}/{postalCode}">
		wire, err := s.lookup.Lookup(r.Context(), deref(contact.MailingCountry), deref(contact.MailingPostalCode))
		var result NormalizeResult
		switch {
		case err == nil:
			result, err = ZippoToResult(contact, wire)
			if err != nil {
				// A failed `as Number` inside the transform is an EXPRESSION error → 400 by the flow's handler.
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case errors.Is(err, zippo.ErrNotFound):
			// <on-error-continue doc:name="404 -> not_found" type="HTTP:NOT_FOUND">
			result = NotFoundResult(contact)
		default:
			var upstream *zippo.UpstreamError
			if errors.As(err, &upstream) {
				// <on-error-propagate type="HTTP:CONNECTIVITY, HTTP:TIMEOUT, HTTP:INTERNAL_SERVER_ERROR, HTTP:SERVICE_UNAVAILABLE, HTTP:BAD_GATEWAY"> → 502
				// The "Upstream error: " prefix belongs to the <logger> only; the body is {error: error.description}.
				s.log.Error("Upstream error: " + err.Error())
				writeError(w, http.StatusBadGateway, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// <set-variable variableName="results" value="#[vars.results + payload]">
		results = append(results, result)
	}

	// <ee:transform doc:name="Results to JSON">
	writeJSON(w, http.StatusOK, ResultsToJSON(results))
}

// hasJSONContentType accepts application/json and any +json media type.
func hasJSONContentType(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func every(contacts []NormalizeContact, ok func(NormalizeContact) bool) bool {
	for _, c := range contacts {
		if !ok(c) {
			return false
		}
	}
	return true
}

// isBlank is DataWeave isBlank(value default ”): nil, empty or whitespace only.
func isBlank(v *string) bool {
	return v == nil || strings.TrimSpace(*v) == ""
}

func upper(v *string) *string {
	if v == nil {
		return nil
	}
	u := strings.ToUpper(*v)
	return &u
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
