// Package contacts holds the record model of the contacts table and the DataWeave transforms of
// batch-contacts-csv-to-db that work on one CSV row: the CSV reader ("CSV to Java"), the typed
// mapping ("CSV to SQL") with its coercions, the CSV line writer used by "Create Error Record"
// and the e-mail validation.
package contacts

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Columns are the CSV / table column names in the order of the upstream INSERT statement.
var Columns = []string{
	"account_id",
	"active_tracker_count",
	"assistant_name",
	"assistant_phone",
	"birthdate",
	"can_allow_portal_self_reg",
	"contact_id",
	"contact_name",
	"created_by_id",
	"created_date",
	"currency_iso_code",
	"department",
	"do_not_call",
	"email",
	"external_id",
	"fax",
	"first_name",
	"has_opted_out_of_email",
	"has_opted_out_of_fax",
	"has_privacy_hold",
	"home_phone",
	"is_deleted",
	"is_email_bounced",
	"is_locked",
	"is_person_account",
	"is_priority_record",
	"last_modified_by_id",
	"last_modified_date",
	"last_name",
	"lead_source",
	"mailing_city",
	"mailing_country",
	"mailing_postal_code",
	"mailing_state",
	"mailing_street",
	"may_edit",
	"middle_name",
	"mobile_phone",
	"other_city",
	"other_country",
	"other_phone",
	"other_postal_code",
	"other_state",
	"other_street",
	"owner_id",
	"phone",
	"photo_url",
	"record_type_id",
	"salutation",
	"suffix",
	"system_mod_stamp",
	"title",
}

// Date is a calendar date without a time of day, the result of DataWeave `as Date`.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// String formats the date as yyyy-MM-dd.
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// MarshalJSON writes the date as a yyyy-MM-dd string.
func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// Time returns midnight UTC of the date.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// Precision selects how the three timestamp columns (created_date, last_modified_date,
// system_mod_stamp) are coerced; TIMESTAMP_PRECISION in the configuration (MAPPING.md deviation 4).
type Precision string

const (
	// PrecisionDate is Mule's `as Date`: the calendar date as written, time and zone dropped. The default.
	PrecisionDate Precision = "date"
	// PrecisionInstant keeps the full timestamp, normalised to UTC with milliseconds; a plain date stays a date.
	PrecisionInstant Precision = "instant"
)

// ParsePrecision reads a TIMESTAMP_PRECISION value, case-insensitively; empty means PrecisionDate.
func ParsePrecision(value string) (Precision, error) {
	switch p := Precision(strings.ToLower(strings.TrimSpace(value))); p {
	case "", PrecisionDate:
		return PrecisionDate, nil
	case PrecisionInstant:
		return PrecisionInstant, nil
	default:
		return "", fmt.Errorf("TIMESTAMP_PRECISION must be 'date' or 'instant', got %q", value)
	}
}

// Timestamp is a timestamp column value as coerced under a Precision: always the calendar date as
// written and, in instant mode for an input that carried a time, the UTC instant as well.
type Timestamp struct {
	// Date is the calendar date as written in the input.
	Date Date
	// Instant is the UTC instant; zero when DateOnly.
	Instant time.Time
	// DateOnly reports a plain date: always under PrecisionDate, or when the input had no time part.
	DateOnly bool
}

// String is yyyy-MM-dd for a plain date, otherwise yyyy-MM-ddTHH:mm:ss.fffZ (fixed three-digit milliseconds).
func (t Timestamp) String() string {
	if t.DateOnly {
		return t.Date.String()
	}
	return t.Instant.UTC().Format("2006-01-02T15:04:05.000Z")
}

// Time is the value for a native TIMESTAMPTZ column: midnight UTC of the date, or the instant.
func (t Timestamp) Time() time.Time {
	if t.DateOnly {
		return t.Date.Time()
	}
	return t.Instant.UTC()
}

// MarshalJSON writes the timestamp as its String text.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// Contact is one row of the contacts table, typed the way the "CSV to SQL" DataWeave produced it:
// `as Number` → *int64, `as Boolean` → *bool, `as Date` → *Date for birthdate, and *Timestamp for
// created_date, last_modified_date and system_mod_stamp, which follow the configured Precision
// (Mule's `as Date` by default). A nil field is a column absent from the CSV and is inserted as
// NULL; an empty text cell stays "" (MAPPING.md deviation 6).
type Contact struct {
	AccountID             *string    `json:"account_id"`
	ActiveTrackerCount    *int64     `json:"active_tracker_count"`
	AssistantName         *string    `json:"assistant_name"`
	AssistantPhone        *string    `json:"assistant_phone"`
	Birthdate             *Date      `json:"birthdate"`
	CanAllowPortalSelfReg *bool      `json:"can_allow_portal_self_reg"`
	ContactID             *string    `json:"contact_id"`
	ContactName           *string    `json:"contact_name"`
	CreatedByID           *string    `json:"created_by_id"`
	CreatedDate           *Timestamp `json:"created_date"`
	CurrencyISOCode       *string    `json:"currency_iso_code"`
	Department            *string    `json:"department"`
	DoNotCall             *bool      `json:"do_not_call"`
	Email                 *string    `json:"email"`
	ExternalID            *string    `json:"external_id"`
	Fax                   *string    `json:"fax"`
	FirstName             *string    `json:"first_name"`
	HasOptedOutOfEmail    *bool      `json:"has_opted_out_of_email"`
	HasOptedOutOfFax      *bool      `json:"has_opted_out_of_fax"`
	HasPrivacyHold        *bool      `json:"has_privacy_hold"`
	HomePhone             *string    `json:"home_phone"`
	IsDeleted             *bool      `json:"is_deleted"`
	IsEmailBounced        *bool      `json:"is_email_bounced"`
	IsLocked              *bool      `json:"is_locked"`
	IsPersonAccount       *bool      `json:"is_person_account"`
	IsPriorityRecord      *bool      `json:"is_priority_record"`
	LastModifiedByID      *string    `json:"last_modified_by_id"`
	LastModifiedDate      *Timestamp `json:"last_modified_date"`
	LastName              *string    `json:"last_name"`
	LeadSource            *string    `json:"lead_source"`
	MailingCity           *string    `json:"mailing_city"`
	MailingCountry        *string    `json:"mailing_country"`
	MailingPostalCode     *string    `json:"mailing_postal_code"`
	MailingState          *string    `json:"mailing_state"`
	MailingStreet         *string    `json:"mailing_street"`
	MayEdit               *bool      `json:"may_edit"`
	MiddleName            *string    `json:"middle_name"`
	MobilePhone           *string    `json:"mobile_phone"`
	OtherCity             *string    `json:"other_city"`
	OtherCountry          *string    `json:"other_country"`
	OtherPhone            *string    `json:"other_phone"`
	OtherPostalCode       *string    `json:"other_postal_code"`
	OtherState            *string    `json:"other_state"`
	OtherStreet           *string    `json:"other_street"`
	OwnerID               *string    `json:"owner_id"`
	Phone                 *string    `json:"phone"`
	PhotoURL              *string    `json:"photo_url"`
	RecordTypeID          *string    `json:"record_type_id"`
	Salutation            *string    `json:"salutation"`
	Suffix                *string    `json:"suffix"`
	SystemModStamp        *Timestamp `json:"system_mod_stamp"`
	Title                 *string    `json:"title"`
}
