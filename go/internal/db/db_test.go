package db_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"muletocode/internal/config"
	"muletocode/internal/contacts"
	"muletocode/internal/db"
	"muletocode/internal/testutil"
)

// loadContacts maps every row of a CSV fixture through FromRow.
func loadContacts(t testing.TB, rel string) []contacts.Contact {
	t.Helper()
	f, err := os.Open(testutil.Fixture(t, rel))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []contacts.Contact
	for row, err := range contacts.ReadRows(f) {
		if err != nil {
			t.Fatal(err)
		}
		c, err := contacts.FromRow(row, contacts.PrecisionDate)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

// contactsByCity is the shape of testdata/api/get-contacts-saint-louis.json.
type contactsByCity struct {
	City     string `json:"city"`
	Count    int    `json:"count"`
	Contacts []struct {
		ContactID  string `json:"contact_id"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		Email      string `json:"email"`
		Phone      string `json:"phone"`
		Title      string `json:"title"`
		Department string `json:"department"`
		Mailing    struct {
			Street     string `json:"street"`
			City       string `json:"city"`
			State      string `json:"state"`
			PostalCode string `json:"postal_code"`
			Country    string `json:"country"`
		} `json:"mailing"`
	} `json:"contacts"`
}

func str(v *string) string {
	if v == nil {
		return "<nil>"
	}
	return *v
}

// RepositoryContract is the behaviour both profiles must share: it is run against SQLite here and
// against PostgreSQL in postgres_test.go. The repository must be empty on entry.
func RepositoryContract(t *testing.T, repo *db.Repository) {
	t.Helper()
	ctx := context.Background()

	if err := repo.BulkInsert(ctx, nil); err != nil {
		t.Fatalf("an empty bulk insert must be a no-op: %v", err)
	}
	all := loadContacts(t, "contact-data-100.csv")
	if err := repo.BulkInsert(ctx, all); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}

	var want contactsByCity
	testutil.ReadJSON(t, "api/get-contacts-saint-louis.json", &want)
	check := func(t *testing.T, rows []db.ContactRow) {
		t.Helper()
		if len(rows) != want.Count {
			t.Fatalf("got %d rows, want %d", len(rows), want.Count)
		}
		for i, w := range want.Contacts {
			g := rows[i]
			got := []string{str(g.ContactID), str(g.FirstName), str(g.LastName), str(g.Email), str(g.Phone), str(g.Title), str(g.Department),
				str(g.MailingStreet), str(g.MailingCity), str(g.MailingState), str(g.MailingPostalCode), str(g.MailingCountry)}
			exp := []string{w.ContactID, w.FirstName, w.LastName, w.Email, w.Phone, w.Title, w.Department,
				w.Mailing.Street, w.Mailing.City, w.Mailing.State, w.Mailing.PostalCode, w.Mailing.Country}
			for j := range exp {
				if got[j] != exp[j] {
					t.Errorf("row %d column %d: got %q, want %q", i, j, got[j], exp[j])
				}
			}
		}
	}

	t.Run("query by city matches the golden and its order", func(t *testing.T) {
		rows, err := repo.QueryByCity(ctx, want.City, 50)
		if err != nil {
			t.Fatal(err)
		}
		check(t, rows)
	})
	t.Run("city is case-insensitive", func(t *testing.T) {
		rows, err := repo.QueryByCity(ctx, "sAINT lOUIS", 50)
		if err != nil {
			t.Fatal(err)
		}
		check(t, rows)
	})
	t.Run("limit is honoured", func(t *testing.T) {
		rows, err := repo.QueryByCity(ctx, want.City, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || str(rows[0].LastName) != want.Contacts[0].LastName {
			t.Errorf("got %d rows, first %q", len(rows), str(rows[0].LastName))
		}
	})
	t.Run("unknown city is empty", func(t *testing.T) {
		rows, err := repo.QueryByCity(ctx, "Nowhere", 50)
		if err != nil || len(rows) != 0 {
			t.Errorf("got %v, %v", rows, err)
		}
	})
	t.Run("a cancelled context is an error", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := repo.QueryByCity(cancelled, want.City, 50); err == nil {
			t.Error("expected an error")
		}
	})
}

// countRows counts the contacts table through a raw connection, independently of the repository.
func countRows(t testing.TB, driver, dsn string) int {
	t.Helper()
	sqlDB, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var n int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM contacts").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSQLiteRepository(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "contacts.db")
	repo, err := db.Open(ctx, config.SQLite, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer repo.Close()
	if repo.Profile() != config.SQLite {
		t.Errorf("profile = %v", repo.Profile())
	}

	RepositoryContract(t, repo)

	if n := countRows(t, "sqlite", path); n != 100 {
		t.Errorf("table holds %d rows, want 100", n)
	}

	t.Run("opening again re-applies the idempotent schema and keeps the rows", func(t *testing.T) {
		again, err := db.Open(ctx, config.SQLite, path)
		if err != nil {
			t.Fatal(err)
		}
		again.Close()
		if n := countRows(t, "sqlite", path); n != 100 {
			t.Errorf("table holds %d rows after reopening, want 100", n)
		}
	})

	t.Run("stored representations follow MAPPING.md deviation 7", func(t *testing.T) {
		sqlDB, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer sqlDB.Close()
		var birthdate, created, modified, kind string
		var mayEdit, doNotCall int64
		row := sqlDB.QueryRow("SELECT birthdate, created_date, last_modified_date, may_edit, do_not_call, typeof(may_edit) FROM contacts WHERE external_id = '1'")
		if err := row.Scan(&birthdate, &created, &modified, &mayEdit, &doNotCall, &kind); err != nil {
			t.Fatal(err)
		}
		if birthdate != "1976-12-18" || created != "2007-09-26" || modified != "2025-05-01" || mayEdit != 1 || doNotCall != 0 || kind != "integer" {
			t.Errorf("stored as %q %q %q %d %d (%s)", birthdate, created, modified, mayEdit, doNotCall, kind)
		}
	})

	t.Run("errors are *db.Error", func(t *testing.T) {
		closed, err := db.Open(ctx, config.SQLite, filepath.Join(t.TempDir(), "x.db"))
		if err != nil {
			t.Fatal(err)
		}
		closed.Close()
		_, err = closed.QueryByCity(ctx, "x", 1)
		var dbErr *db.Error
		if !errors.As(err, &dbErr) || dbErr.Unwrap() == nil {
			t.Errorf("QueryByCity on a closed repository: %v", err)
		}
		err = closed.BulkInsert(ctx, loadContacts(t, "contact-data-100.csv")[:1])
		if !errors.As(err, &dbErr) {
			t.Errorf("BulkInsert on a closed repository: %v", err)
		}
	})
}

func TestOpenErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := db.Open(ctx, config.SQLite, ""); err == nil {
		t.Error("empty DSN must be rejected")
	}
	if _, err := db.Open(ctx, config.Profile("oracle"), "x"); err == nil {
		t.Error("unknown profile must be rejected")
	}
	if _, err := db.Open(ctx, config.SQLite, filepath.Join(os.DevNull, "x", "y.db")); err == nil {
		t.Error("an uncreatable directory must be an error")
	}
	_, err := db.Open(ctx, config.Postgres, "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	var dbErr *db.Error
	if !errors.As(err, &dbErr) {
		t.Errorf("unreachable Postgres: got %v, want *db.Error", err)
	}
}

// Bind is what ends up in the INSERT; the SQLite spellings must be exactly these for the rows to be
// byte-identical to the C# port.
func TestBind(t *testing.T) {
	first := loadContacts(t, "contact-data-100.csv")[0]
	var nilContact contacts.Contact

	sqlite, err := db.Open(context.Background(), config.SQLite, filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlite.Close()
	params := sqlite.Bind(first)
	if len(params) != len(contacts.Columns) {
		t.Fatalf("%d parameters for %d columns", len(params), len(contacts.Columns))
	}
	index := func(column string) int {
		for i, c := range contacts.Columns {
			if c == column {
				return i
			}
		}
		t.Fatalf("no column %s", column)
		return -1
	}
	want := map[string]any{
		"account_id":                "001Hu00003RhROAIA3",
		"active_tracker_count":      int64(0),
		"birthdate":                 "1976-12-18",
		"can_allow_portal_self_reg": int64(0),
		"created_date":              "2007-09-26",
		"fax":                       "",
		"last_modified_date":        "2025-05-01", // `as Date` on 2025-05-01T11:39:15.257Z, as in Mule
		"may_edit":                  int64(1),
		"system_mod_stamp":          "2025-05-01",
	}
	for column, w := range want {
		if got := params[index(column)]; got != w {
			t.Errorf("sqlite %s = %#v, want %#v", column, got, w)
		}
	}
	for i, p := range sqlite.Bind(nilContact) {
		if p != nil {
			t.Errorf("nil field %s must bind NULL, got %#v", contacts.Columns[i], p)
		}
	}

	// The Postgres spellings are checked without a server: Bind only needs the profile.
	pg := postgresRepositoryForBind(t)
	if pg == nil {
		return
	}
	params = pg.Bind(first)
	if got := params[index("may_edit")]; got != true {
		t.Errorf("postgres may_edit = %#v, want true", got)
	}
	if got, ok := params[index("birthdate")].(time.Time); !ok || !got.Equal(time.Date(1976, 12, 18, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("postgres birthdate = %#v", params[index("birthdate")])
	}
	if got, ok := params[index("last_modified_date")].(time.Time); !ok || !got.Equal(time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("postgres last_modified_date = %#v, want midnight UTC of the date", params[index("last_modified_date")])
	}
	if got, ok := params[index("created_date")].(time.Time); !ok || !got.Equal(time.Date(2007, 9, 26, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("postgres created_date = %#v, want midnight UTC as a native timestamp", params[index("created_date")])
	}
}

// TIMESTAMP_PRECISION=instant: the full timestamp lands in SQLite as yyyy-MM-ddTHH:mm:ss.fffZ text,
// a plain date in the input still as yyyy-MM-dd; Postgres would get the instant.
func TestInstantPrecisionStoresTheFullTimestampInSQLite(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "instant.db")
	repo, err := db.Open(ctx, config.SQLite, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	f, err := os.Open(testutil.Fixture(t, "contact-data-100.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var first contacts.Contact
	for row, err := range contacts.ReadRows(f) {
		if err != nil {
			t.Fatal(err)
		}
		if first, err = contacts.FromRow(row, contacts.PrecisionInstant); err != nil {
			t.Fatal(err)
		}
		break
	}
	if err := repo.BulkInsert(ctx, []contacts.Contact{first}); err != nil {
		t.Fatal(err)
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var created, modified, stamp string
	if err := sqlDB.QueryRow("SELECT created_date, last_modified_date, system_mod_stamp FROM contacts WHERE external_id = '1'").Scan(&created, &modified, &stamp); err != nil {
		t.Fatal(err)
	}
	if created != "2007-09-26" || modified != "2025-05-01T11:39:15.257Z" || stamp != "2025-05-01T11:39:15.257Z" {
		t.Errorf("stored as %q %q %q", created, modified, stamp)
	}
	params := repo.Bind(first)
	if got := params[index("last_modified_date")]; got != "2025-05-01T11:39:15.257Z" {
		t.Errorf("sqlite instant binding = %#v", got)
	}
	if pg := postgresRepositoryForBind(t); pg != nil {
		if got, ok := pg.Bind(first)[index("last_modified_date")].(time.Time); !ok || !got.Equal(time.Date(2025, 5, 1, 11, 39, 15, 257_000_000, time.UTC)) {
			t.Errorf("postgres instant binding = %#v", pg.Bind(first)[index("last_modified_date")])
		}
	}
}

// index is the position of a column in contacts.Columns.
func index(column string) int {
	for i, c := range contacts.Columns {
		if c == column {
			return i
		}
	}
	return -1
}
