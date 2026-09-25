package contacts

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"muletocode/internal/testutil"
)

// readAll returns every row of a CSV fixture.
func readAll(t *testing.T, rel string) []Row {
	t.Helper()
	f, err := os.Open(testutil.Fixture(t, rel))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []Row
	for row, err := range ReadRows(f) {
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		rows = append(rows, row)
	}
	return rows
}

// Golden: <ee:transform doc:name="CSV to SQL"> on the first record of contact-data-100.csv.
func TestFromRow_FirstRecordGolden(t *testing.T) {
	rows := readAll(t, "contact-data-100.csv")
	if len(rows) != 100 {
		t.Fatalf("contact-data-100.csv has %d rows, want 100", len(rows))
	}
	contact, err := FromRow(rows[0], PrecisionDate)
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	gotBytes, err := json.Marshal(contact)
	if err != nil {
		t.Fatal(err)
	}

	// The three timestamp columns go through `as Date` like the original: the golden holds
	// "2007-09-26" for a date cell and "2025-05-01" for 2025-05-01T11:39:15.257Z.
	testutil.AssertJSONEqual(t, testutil.ReadFixture(t, "expected/first-record.json"), gotBytes)

	// Typed fields, spot-checked directly so a JSON-only difference cannot hide a wrong type.
	if contact.ActiveTrackerCount == nil || *contact.ActiveTrackerCount != 0 {
		t.Errorf("active_tracker_count = %v, want 0", contact.ActiveTrackerCount)
	}
	if contact.Birthdate == nil || contact.Birthdate.String() != "1976-12-18" {
		t.Errorf("birthdate = %v, want 1976-12-18", contact.Birthdate)
	}
	if contact.MayEdit == nil || !*contact.MayEdit {
		t.Errorf("may_edit = %v, want true", contact.MayEdit)
	}
	if contact.LastModifiedDate == nil || contact.LastModifiedDate.String() != "2025-05-01" {
		t.Errorf("last_modified_date = %v, want 2025-05-01 (as Date drops the time)", contact.LastModifiedDate)
	}
	if contact.CreatedDate == nil || contact.CreatedDate.String() != "2007-09-26" {
		t.Errorf("created_date = %v, want 2007-09-26", contact.CreatedDate)
	}
	if contact.Fax == nil || *contact.Fax != "" {
		t.Errorf("fax = %v, want an empty string (deviation 6)", contact.Fax)
	}
}

func TestFromRow_CoercionErrors(t *testing.T) {
	base := readAll(t, "contact-data-100.csv")[0]
	cases := []struct {
		column, value, want string
	}{
		{"active_tracker_count", "abc", "Cannot coerce String (abc) to Number"},
		{"active_tracker_count", "1.5", "Cannot coerce String (1.5) to Number"},
		{"do_not_call", "yes", "Cannot coerce String (yes) to Boolean"},
		{"birthdate", "1976-13-18", "Cannot coerce String (1976-13-18) to Date"},
		{"birthdate", "18/12/1976", "Cannot coerce String (18/12/1976) to Date"},
		{"created_date", "yesterday", "Cannot coerce String (yesterday) to Date"},
		{"created_date", "2025-05-01T25:00:00Z", "Cannot coerce String (2025-05-01T25:00:00Z) to Date"},
	}
	for _, tc := range cases {
		row := make(Row, len(base))
		for k, v := range base {
			row[k] = v
		}
		row[tc.column] = tc.value
		_, err := FromRow(row, PrecisionDate)
		var ce *CoercionError
		if !errors.As(err, &ce) {
			t.Errorf("%s=%q: got %v, want *CoercionError", tc.column, tc.value, err)
			continue
		}
		if err.Error() != tc.want || ce.Field != tc.column || ce.Value != tc.value {
			t.Errorf("%s=%q: got %q, want %q", tc.column, tc.value, err.Error(), tc.want)
		}
	}
}

func TestFromRow_EmptyAndAbsentCells(t *testing.T) {
	row := Row{"active_tracker_count": "", "do_not_call": "", "birthdate": "", "created_date": "", "fax": ""}
	c, err := FromRow(row, PrecisionDate)
	if err != nil {
		t.Fatal(err)
	}
	if c.ActiveTrackerCount != nil || c.DoNotCall != nil || c.Birthdate != nil || c.CreatedDate != nil {
		t.Error("empty typed cells must coerce to nil")
	}
	if c.Fax == nil || *c.Fax != "" {
		t.Error("an empty text cell must stay \"\"")
	}
	if c.Title != nil {
		t.Error("a column absent from the CSV must be nil (NULL)")
	}
}

func TestCoercions_Formats(t *testing.T) {
	s := func(v string) *string { return &v }
	b, err := parseBool(s("TRUE"), "x")
	if err != nil || b == nil || !*b {
		t.Errorf("parseBool(TRUE) = %v, %v", b, err)
	}
	// `as Date` on a date-time keeps the calendar date as written and drops time and zone:
	// 2025-05-01T23:30:00-05:00 is 2025-05-02T04:30Z, but the Date is still 2025-05-01.
	// The +02:00 and -05:00 cases mirror the C# AsDateTakesTheCalendarDateAsWritten test: neither
	// 2025-05-01T23:59:59+02:00 (2025-05-01T21:59:59Z) nor 2025-05-01T23:30:00-05:00 (2025-05-02T04:30Z)
	// may drift to another day.
	for _, in := range []string{"2025-05-01", "2025-05-01T11:39:15.257Z", "2025-05-01T11:39:15.257+00:00", "2025-05-01T13:39:15.257+02:00", "2025-05-01T23:59:59+02:00", "2025-05-01T00:00:01+02:00", "2025-05-01T11:39:15.257", "2025-05-01T11:39:15", "2025-05-01T23:30:00-05:00"} {
		d, err := parseDate(s(in), "x")
		if err != nil || d.String() != "2025-05-01" {
			t.Errorf("parseDate(%s) = %v, %v; want 2025-05-01", in, d, err)
		}
	}
	d, err := parseDate(s("1976-12-18"), "x")
	if err != nil || d.Time() != time.Date(1976, 12, 18, 0, 0, 0, 0, time.UTC) {
		t.Errorf("parseDate = %v, %v", d, err)
	}
	if got, _ := json.Marshal(d); string(got) != `"1976-12-18"` {
		t.Errorf("Date JSON = %s", got)
	}
}

// Golden: the two "Record" lines of errors-100-with-errors.json are the failing rows of
// contact-data-100-with-errors.csv re-serialised by write(payload, "application/csv", ...).
func TestRowToCSVLine_ErrorRecordsGolden(t *testing.T) {
	rows := readAll(t, "contact-data-100-with-errors.csv")
	var want []struct{ Error, Record string }
	testutil.ReadJSON(t, "expected/errors-100-with-errors.json", &want)

	var failing []int
	for i, row := range rows {
		if ValidateEmail(row) != nil {
			failing = append(failing, i)
		}
	}
	if len(failing) != len(want) {
		t.Fatalf("%d rows fail the e-mail validation, golden has %d", len(failing), len(want))
	}
	if failing[0] != 2 || failing[1] != 6 {
		t.Errorf("failing rows are %v, want rows 3 and 7 (0-based 2 and 6)", failing)
	}
	for i, idx := range failing {
		if got := RowToCSVLine(rows[idx]); got != want[i].Record {
			t.Errorf("Record %d:\n got %s\nwant %s", i, got, want[i].Record)
		}
		if got := ValidateEmail(rows[idx]).Error(); got != want[i].Error {
			t.Errorf("Error %d: got %q, want %q", i, got, want[i].Error)
		}
	}
}

func TestRowToCSVLine_Quoting(t *testing.T) {
	row := Row{"account_id": `plain`, "contact_name": `has, comma`, "title": `say "hi"`, "fax": "two\nlines", "email": " padded "}
	line := RowToCSVLine(row)
	cells := strings.Split(line, "\n")
	if len(cells) != 2 {
		t.Fatalf("a newline inside a quoted cell must be kept: %q", line)
	}
	for _, want := range []string{`plain,`, `"has, comma"`, `"say ""hi"""`, `"two`, `, padded ,`} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q does not contain %q", line, want)
		}
	}
	if strings.Count(line, ",") < len(Columns)-1 {
		t.Errorf("line must have one cell per column: %q", line)
	}
}

func TestReadRows_Edges(t *testing.T) {
	read := func(src string) ([]Row, error) {
		var rows []Row
		for row, err := range ReadRows(strings.NewReader(src)) {
			if err != nil {
				return rows, err
			}
			rows = append(rows, row)
		}
		return rows, nil
	}

	if _, err := read(""); err == nil || !strings.Contains(err.Error(), "no header row") {
		t.Errorf("empty input: got %v", err)
	}
	if rows, err := read("a,b\n"); err != nil || len(rows) != 0 {
		t.Errorf("header only: got %v, %v", rows, err)
	}
	rows, err := read("\xEF\xBB\xBFa,b,c\n1,\"x,y\"\n\n2,\"say \"\"hi\"\"\",3,extra\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (empty line skipped)", len(rows))
	}
	if rows[0]["a"] != "1" || rows[0]["b"] != "x,y" || rows[0]["c"] != "" {
		t.Errorf("BOM / quoting / missing trailing cell: %v", rows[0])
	}
	if rows[1]["b"] != `say "hi"` || rows[1]["c"] != "3" || len(rows[1]) != 3 {
		t.Errorf("escaped quote / extra cell: %v", rows[1])
	}
	// A quote that does not open a cell is literal, as DataWeave's reader treats it.
	if rows, err := read("a,b\nsay \"hi\",2\n"); err != nil || rows[0]["a"] != `say "hi"` {
		t.Errorf("stray quote: got %v, %v", rows, err)
	}
	// Stopping the iteration early must not block or panic.
	for range ReadRows(strings.NewReader("a\n1\n2\n3\n")) {
		break
	}
}

// <validation:is-email>: every address of the clean sample is valid, the two of the with-errors
// sample that were made invalid are not, plus a few synthetic shapes.
func TestValidateEmail(t *testing.T) {
	for i, row := range readAll(t, "contact-data-100.csv") {
		if err := ValidateEmail(row); err != nil {
			t.Errorf("row %d: %q rejected: %v", i, row["email"], err)
		}
	}
	rows := readAll(t, "contact-data-100-with-errors.csv")
	for _, idx := range []int{2, 6} {
		if err := ValidateEmail(rows[idx]); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("row %d (%q): got %v, want ErrInvalidEmail", idx, rows[idx]["email"], err)
		}
	}
	if ErrInvalidEmail.Error() != "Missing or invalid email" {
		t.Errorf("message = %q", ErrInvalidEmail.Error())
	}

	valid := []string{"a@b.co", "first.last+tag@sub.example.org", "x_y-z@ex-ample.com", "o'neil@example.ie"}
	invalid := []string{"", " ", "not-an-email", "a@b", "@example.com", "a@.com", ".a@example.com", "a.@example.com", "a..b@example.com", "a@example.c", "a b@example.com", "a@exa mple.com", "a@-example.com", strings.Repeat("x", 65) + "@example.com", "a@" + strings.Repeat("d", 250) + ".com"}
	for _, e := range valid {
		if !IsEmail(e) {
			t.Errorf("IsEmail(%q) = false, want true", e)
		}
	}
	for _, e := range invalid {
		if IsEmail(e) {
			t.Errorf("IsEmail(%q) = true, want false", e)
		}
	}
	if ValidateEmail(Row{}) == nil {
		t.Error("a row without an email column must fail")
	}
}

// TIMESTAMP_PRECISION: date is Mule's `as Date` (calendar date as written, no zone conversion);
// instant keeps the full timestamp in UTC with fixed three-digit milliseconds, a plain date stays a date.
func TestParseTimestampFollowsThePrecision(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := []struct{ in, date, instant string }{
		{"2007-09-26", "2007-09-26", "2007-09-26"},
		{"2025-05-01T11:39:15.257Z", "2025-05-01", "2025-05-01T11:39:15.257Z"},
		{"2025-05-01T11:39:15.25Z", "2025-05-01", "2025-05-01T11:39:15.250Z"},
		{"2025-05-01T11:39:15", "2025-05-01", "2025-05-01T11:39:15.000Z"},
		{"2025-05-01T23:59:59+02:00", "2025-05-01", "2025-05-01T21:59:59.000Z"},
		{"2025-05-01T23:30:00-05:00", "2025-05-01", "2025-05-02T04:30:00.000Z"},
	}
	for _, tc := range cases {
		d, err := parseTimestamp(s(tc.in), "x", PrecisionDate)
		if err != nil || !d.DateOnly || d.String() != tc.date {
			t.Errorf("date mode %s = %v, %v; want %s", tc.in, d, err, tc.date)
		}
		if def, err := parseTimestamp(s(tc.in), "x", ""); err != nil || def.String() != tc.date {
			t.Errorf("empty precision must be date mode: %s = %v, %v", tc.in, def, err)
		}
		i, err := parseTimestamp(s(tc.in), "x", PrecisionInstant)
		if err != nil || i.String() != tc.instant {
			t.Errorf("instant mode %s = %v, %v; want %s", tc.in, i, err, tc.instant)
		}
		if i.Date.String() != tc.date {
			t.Errorf("instant mode %s keeps the calendar date as written: got %s, want %s", tc.in, i.Date, tc.date)
		}
		if got, _ := json.Marshal(i); string(got) != `"`+tc.instant+`"` {
			t.Errorf("Timestamp JSON = %s", got)
		}
	}
	if ts, err := parseTimestamp(s(""), "x", PrecisionInstant); ts != nil || err != nil {
		t.Errorf("empty cell → nil, got %v, %v", ts, err)
	}
	if _, err := parseTimestamp(s("soon"), "x", PrecisionInstant); err == nil || err.Error() != "Cannot coerce String (soon) to DateTime" {
		t.Errorf("instant mode error = %v", err)
	}
	if _, err := parseTimestamp(s("soon"), "x", PrecisionDate); err == nil || err.Error() != "Cannot coerce String (soon) to Date" {
		t.Errorf("date mode error = %v", err)
	}
	// Time() feeds the native TIMESTAMPTZ column: midnight UTC of the date, or the instant.
	d, _ := parseTimestamp(s("2025-05-01T23:30:00-05:00"), "x", PrecisionDate)
	i, _ := parseTimestamp(s("2025-05-01T23:30:00-05:00"), "x", PrecisionInstant)
	if !d.Time().Equal(time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)) || !i.Time().Equal(time.Date(2025, 5, 2, 4, 30, 0, 0, time.UTC)) {
		t.Errorf("Time() = %v / %v", d.Time(), i.Time())
	}

	for in, want := range map[string]Precision{"": PrecisionDate, "date": PrecisionDate, " Instant ": PrecisionInstant} {
		if got, err := ParsePrecision(in); err != nil || got != want {
			t.Errorf("ParsePrecision(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParsePrecision("nanos"); err == nil {
		t.Error("an unknown precision must be rejected")
	}

	// FromRow under instant mode on the first sample record.
	c, err := FromRow(readAll(t, "contact-data-100.csv")[0], PrecisionInstant)
	if err != nil || c.CreatedDate.String() != "2007-09-26" || c.LastModifiedDate.String() != "2025-05-01T11:39:15.257Z" {
		t.Errorf("FromRow(instant) = %v %v, %v", c.CreatedDate, c.LastModifiedDate, err)
	}
}
