package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"muletocode/internal/db"
)

// DefaultLimit is the default of `attributes.queryParams.limit default '50'`.
const DefaultLimit = 50

var (
	// ErrCityRequired is the message of <validation:is-not-blank-string doc:name="city is required">.
	ErrCityRequired = errors.New("Query parameter 'city' is required")
	// ErrLimitRange is the message of <validation:is-number doc:name="limit in range" minValue="1" maxValue="500">.
	ErrLimitRange = errors.New("Query parameter 'limit' must be between 1 and 500")
)

// ContactsByCity is the response body of GET /contacts (ContactsByCity in openapi.yaml).
type ContactsByCity struct {
	City     string        `json:"city"`
	Count    int           `json:"count"`
	Contacts []ContactView `json:"contacts"`
}

// ContactView is one contact of ContactsByCity (ContactView in openapi.yaml).
type ContactView struct {
	ContactID  *string     `json:"contact_id"`
	FirstName  *string     `json:"first_name"`
	LastName   *string     `json:"last_name"`
	Email      *string     `json:"email"`
	Phone      *string     `json:"phone"`
	Title      *string     `json:"title"`
	Department *string     `json:"department"`
	Mailing    MailingView `json:"mailing"`
}

// MailingView is the nested mailing object of ContactView.
type MailingView struct {
	Street     *string `json:"street"`
	City       *string `json:"city"`
	State      *string `json:"state"`
	PostalCode *string `json:"postal_code"`
	Country    *string `json:"country"`
}

// ValidateQuery is the two validations of get-contacts-by-city-flow. city is
// `attributes.queryParams.city default ”`, limit is `attributes.queryParams.limit default '50'`
// unparsed. It returns the parsed limit, or the error whose text is the message of the first
// failing check.
func ValidateQuery(city, limit string) (int, error) {
	// <validation:is-not-blank-string value="#[vars.city]">
	if strings.TrimSpace(city) == "" {
		return 0, ErrCityRequired
	}
	// `(... default '50') as Number` followed by <validation:is-number numberType="INTEGER" minValue="1" maxValue="500">.
	// A non-numeric value fails the coercion (an EXPRESSION error in Mule); it is reported with the same message.
	if limit == "" {
		return DefaultLimit, nil
	}
	n, err := strconv.Atoi(limit)
	if err != nil || n < 1 || n > 500 {
		return 0, ErrLimitRange
	}
	return n, nil
}

// RowsToJSON is <ee:transform doc:name="Rows to JSON">.
func RowsToJSON(city string, rows []db.ContactRow) ContactsByCity {
	views := make([]ContactView, 0, len(rows))
	for _, row := range rows {
		views = append(views, ContactView{
			ContactID:  row.ContactID,
			FirstName:  row.FirstName,
			LastName:   row.LastName,
			Email:      row.Email,
			Phone:      row.Phone,
			Title:      row.Title,
			Department: row.Department,
			Mailing: MailingView{
				Street:     row.MailingStreet,
				City:       row.MailingCity,
				State:      row.MailingState,
				PostalCode: row.MailingPostalCode,
				Country:    row.MailingCountry,
			},
		})
	}
	return ContactsByCity{City: city, Count: len(views), Contacts: views}
}

// getContactsByCity is <flow name="get-contacts-by-city-flow">.
func (s *server) getContactsByCity(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	// <set-variable variableName="city" value="#[attributes.queryParams.city default '']">
	city := query.Get("city")
	// <set-variable variableName="limit" value="#[(attributes.queryParams.limit default '50') as Number]">
	limit, err := ValidateQuery(city, query.Get("limit"))
	if err != nil {
		// <on-error-propagate type="VALIDATION:INVALID_STRING, VALIDATION:INVALID_NUMBER, VALIDATION:NOT_A_NUMBER"> → 400
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// <db:select doc:name="Select contacts by city">
	rows, err := s.repo.QueryByCity(r.Context(), city, limit)
	if err != nil {
		var dbErr *db.Error
		if errors.As(err, &dbErr) {
			// <on-error-propagate type="DB:CONNECTIVITY, DB:QUERY_EXECUTION"> → 503
			s.log.Error("Database error", "description", err.Error())
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// <ee:transform doc:name="Rows to JSON">
	writeJSON(w, http.StatusOK, RowsToJSON(city, rows))
}
