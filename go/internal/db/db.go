// Package db is the Database connector of both applications on database/sql: it replaces
// <db:config><db:generic-connection url= driverClassName=> with modernc.org/sqlite or the pgx
// stdlib driver, and implements <db:bulk-insert> and <db:select>.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"
	_ "modernc.org/sqlite"             // database/sql driver "sqlite"

	"muletocode/internal/config"
	"muletocode/internal/contacts"
	"muletocode/internal/resources"
)

// Error is a failed database operation (the Mule DB:CONNECTIVITY / DB:QUERY_EXECUTION errors);
// the driver error is wrapped.
type Error struct {
	Op  string
	Err error
}

func (e *Error) Error() string { return e.Op + ": " + e.Err.Error() }

// Unwrap returns the driver error.
func (e *Error) Unwrap() error { return e.Err }

// ContactRow holds the columns selected by <db:select doc:name="Select contacts by city">.
type ContactRow struct {
	ContactID         *string
	FirstName         *string
	LastName          *string
	Email             *string
	Phone             *string
	Title             *string
	Department        *string
	MailingStreet     *string
	MailingCity       *string
	MailingState      *string
	MailingPostalCode *string
	MailingCountry    *string
}

// Repository is the contacts table behind the two database operations of the Mule applications.
type Repository struct {
	db        *sql.DB
	profile   config.Profile
	insertSQL string
	selectSQL string
}

// Open connects to the database of profile, verifies the connection and applies the schema:
// docker/sqlite/contacts.sqlite.sql on every start (it is idempotent), docker/init/01-contacts.sql
// only when the contacts table does not exist yet. For SQLite dsn is the file path, whose
// directory is created; for Postgres it is a URL or key=value string.
func Open(ctx context.Context, profile config.Profile, dsn string) (*Repository, error) {
	if dsn == "" {
		return nil, errors.New("db: empty DSN")
	}
	var driver string
	switch profile {
	case config.SQLite:
		driver = "sqlite"
		if dir := filepath.Dir(dsn); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("db: creating directory of %s: %w", dsn, err)
			}
		}
	case config.Postgres:
		driver = "pgx"
	default:
		return nil, fmt.Errorf("db: unknown profile %q", profile)
	}

	sqlDB, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, &Error{Op: fmt.Sprintf("Cannot open the %s database", profile), Err: err}
	}
	if profile == config.SQLite {
		// One writer at a time keeps SQLite from reporting "database is locked" under concurrent requests.
		sqlDB.SetMaxOpenConns(1)
	}
	r := &Repository{
		db:        sqlDB,
		profile:   profile,
		insertSQL: insertSQL(profile),
		selectSQL: selectSQL(profile),
	}
	if err := r.applySchema(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return r, nil
}

// Profile is the profile the repository binds values for.
func (r *Repository) Profile() config.Profile { return r.profile }

// Close releases the connection pool.
func (r *Repository) Close() error { return r.db.Close() }

// BulkInsert is <db:bulk-insert doc:name="Contact Data">: all rows in one transaction, one INSERT
// per row. Any failure rolls the transaction back and returns an *Error.
func (r *Repository) BulkInsert(ctx context.Context, rows []contacts.Contact) error {
	if len(rows) == 0 {
		return nil
	}
	op := fmt.Sprintf("Bulk insert of %d contacts failed", len(rows))
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return &Error{Op: op, Err: err}
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, r.insertSQL)
	if err != nil {
		return &Error{Op: op, Err: err}
	}
	defer stmt.Close()
	for _, c := range rows {
		if _, err := stmt.ExecContext(ctx, r.Bind(c)...); err != nil {
			return &Error{Op: op, Err: err}
		}
	}
	if err := tx.Commit(); err != nil {
		return &Error{Op: op, Err: err}
	}
	return nil
}

// QueryByCity is <db:select doc:name="Select contacts by city">: contacts whose mailing_city equals
// city case-insensitively, ordered by last name, first name, contact id, at most limit rows.
func (r *Repository) QueryByCity(ctx context.Context, city string, limit int) ([]ContactRow, error) {
	const op = "Select contacts by city failed"
	rows, err := r.db.QueryContext(ctx, r.selectSQL, city, limit)
	if err != nil {
		return nil, &Error{Op: op, Err: err}
	}
	defer rows.Close()

	var out []ContactRow
	for rows.Next() {
		var c [12]sql.NullString
		if err := rows.Scan(&c[0], &c[1], &c[2], &c[3], &c[4], &c[5], &c[6], &c[7], &c[8], &c[9], &c[10], &c[11]); err != nil {
			return nil, &Error{Op: op, Err: err}
		}
		out = append(out, ContactRow{
			ContactID:         nullable(c[0]),
			FirstName:         nullable(c[1]),
			LastName:          nullable(c[2]),
			Email:             nullable(c[3]),
			Phone:             nullable(c[4]),
			Title:             nullable(c[5]),
			Department:        nullable(c[6]),
			MailingStreet:     nullable(c[7]),
			MailingCity:       nullable(c[8]),
			MailingState:      nullable(c[9]),
			MailingPostalCode: nullable(c[10]),
			MailingCountry:    nullable(c[11]),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, &Error{Op: op, Err: err}
	}
	return out, nil
}

// Bind returns the INSERT parameters of c in contacts.Columns order, represented as MAPPING.md
// deviation 7 prescribes for the profile. Exported so a test can assert the exact SQLite values.
func (r *Repository) Bind(c contacts.Contact) []any {
	return []any{
		text(c.AccountID),
		integer(c.ActiveTrackerCount),
		text(c.AssistantName),
		text(c.AssistantPhone),
		r.date(c.Birthdate),
		r.boolean(c.CanAllowPortalSelfReg),
		text(c.ContactID),
		text(c.ContactName),
		text(c.CreatedByID),
		r.timestamp(c.CreatedDate),
		text(c.CurrencyISOCode),
		text(c.Department),
		r.boolean(c.DoNotCall),
		text(c.Email),
		text(c.ExternalID),
		text(c.Fax),
		text(c.FirstName),
		r.boolean(c.HasOptedOutOfEmail),
		r.boolean(c.HasOptedOutOfFax),
		r.boolean(c.HasPrivacyHold),
		text(c.HomePhone),
		r.boolean(c.IsDeleted),
		r.boolean(c.IsEmailBounced),
		r.boolean(c.IsLocked),
		r.boolean(c.IsPersonAccount),
		r.boolean(c.IsPriorityRecord),
		text(c.LastModifiedByID),
		r.timestamp(c.LastModifiedDate),
		text(c.LastName),
		text(c.LeadSource),
		text(c.MailingCity),
		text(c.MailingCountry),
		text(c.MailingPostalCode),
		text(c.MailingState),
		text(c.MailingStreet),
		r.boolean(c.MayEdit),
		text(c.MiddleName),
		text(c.MobilePhone),
		text(c.OtherCity),
		text(c.OtherCountry),
		text(c.OtherPhone),
		text(c.OtherPostalCode),
		text(c.OtherState),
		text(c.OtherStreet),
		text(c.OwnerID),
		text(c.Phone),
		text(c.PhotoURL),
		text(c.RecordTypeID),
		text(c.Salutation),
		text(c.Suffix),
		r.timestamp(c.SystemModStamp),
		text(c.Title),
	}
}

func text(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func integer(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// boolean: SQLite has no BOOLEAN, so 0/1. Postgres keeps the native type.
func (r *Repository) boolean(v *bool) any {
	if v == nil {
		return nil
	}
	if r.profile == config.SQLite {
		if *v {
			return int64(1)
		}
		return int64(0)
	}
	return *v
}

// date: SQLite has no DATE, so yyyy-MM-dd text. Postgres keeps the native type.
func (r *Repository) date(v *contacts.Date) any {
	if v == nil {
		return nil
	}
	if r.profile == config.SQLite {
		return v.String()
	}
	return v.Time()
}

// timestamp: SQLite has no TIMESTAMPTZ, so text — yyyy-MM-dd for a date (the default precision),
// yyyy-MM-ddTHH:mm:ss.fffZ for an instant. Postgres gets midnight UTC, or the instant.
func (r *Repository) timestamp(v *contacts.Timestamp) any {
	if v == nil {
		return nil
	}
	if r.profile == config.SQLite {
		return v.String()
	}
	return v.Time()
}

func nullable(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

func (r *Repository) applySchema(ctx context.Context) error {
	op := fmt.Sprintf("Cannot open the %s database or apply its schema", r.profile)
	if err := r.db.PingContext(ctx); err != nil {
		return &Error{Op: op, Err: err}
	}

	var (
		schema []byte
		err    error
	)
	if r.profile == config.Postgres {
		var exists bool
		if err := r.db.QueryRowContext(ctx, "SELECT to_regclass('public.contacts') IS NOT NULL").Scan(&exists); err != nil {
			return &Error{Op: op, Err: err}
		}
		if exists {
			return nil
		}
		schema, err = resources.PostgresSchema()
	} else {
		schema, err = resources.SQLiteSchema()
	}
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	// Both drivers run a multi-statement script in one parameterless Exec (pgx uses the simple protocol then).
	if _, err := r.db.ExecContext(ctx, string(schema)); err != nil {
		return &Error{Op: op, Err: err}
	}
	return nil
}

// placeholder is the parameter marker of each driver: "?" for SQLite, "$n" for pgx.
func placeholder(profile config.Profile, n int) string {
	if profile == config.Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// insertSQL is the upstream INSERT of <db:bulk-insert> with its ':name' parameters spelled for the driver.
func insertSQL(profile config.Profile) string {
	marks := make([]string, len(contacts.Columns))
	for i := range contacts.Columns {
		marks[i] = placeholder(profile, i+1)
	}
	return "INSERT INTO contacts(" + strings.Join(contacts.Columns, ", ") + ")\nVALUES (" + strings.Join(marks, ", ") + ")"
}

// selectSQL is the upstream SELECT of <db:select> with :city and :limit spelled for the driver.
func selectSQL(profile config.Profile) string {
	return "SELECT contact_id, first_name, last_name, email, phone, title, department,\n" +
		"       mailing_street, mailing_city, mailing_state, mailing_postal_code, mailing_country\n" +
		"FROM contacts\n" +
		"WHERE LOWER(mailing_city) = LOWER(" + placeholder(profile, 1) + ")\n" +
		"ORDER BY last_name, first_name, contact_id\n" +
		"LIMIT " + placeholder(profile, 2)
}
