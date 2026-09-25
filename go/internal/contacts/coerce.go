package contacts

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CoercionError reports a DataWeave coercion (`as Number`, `as Boolean`, `as Date`) that failed
// for one field. It fails the record and, being raised inside the aggregator, its whole block.
type CoercionError struct {
	Field  string
	Value  string
	Target string
	Err    error
}

func (e *CoercionError) Error() string {
	// DataWeave's own wording: it reaches the errors file as Batch::getFirstException().message.
	return fmt.Sprintf("Cannot coerce String (%s) to %s", e.Value, e.Target)
}

// Unwrap returns the parse error, if any.
func (e *CoercionError) Unwrap() error { return e.Err }

// The coercions below take the cell as *string: nil (column absent) and "" (empty cell) both
// coerce to nil, as DataWeave does.

// parseInt is DataWeave `as Number` for an integer column.
func parseInt(value *string, field string) (*int64, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(*value), 10, 32)
	if err != nil {
		return nil, &CoercionError{Field: field, Value: *value, Target: "Number", Err: err}
	}
	return &n, nil
}

// parseBool is DataWeave `as Boolean`: "true" / "false", case-insensitive.
func parseBool(value *string, field string) (*bool, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	var b bool
	switch strings.ToLower(*value) {
	case "true":
		b = true
	case "false":
		b = false
	default:
		return nil, &CoercionError{Field: field, Value: *value, Target: "Boolean"}
	}
	return &b, nil
}

// parseDate is DataWeave `as Date` on a yyyy-MM-dd value.
// A `yyyy-MM-dd` date, or an ISO-8601 date-time (a missing offset means UTC). The first layout is
// the date-only one so a caller can tell the two apart.
var dateLayouts = []string{
	"2006-01-02",
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
}

// parseISO parses value with dateLayouts and reports whether it was a plain date. The returned
// time keeps the offset as written, so its Year/Month/Day are the calendar date of the input.
func parseISO(value, field string) (t time.Time, dateOnly bool, err error) {
	var lastErr error
	for i, layout := range dateLayouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, i == 0, nil
		}
		lastErr = err
	}
	return time.Time{}, false, lastErr
}

// parseDate is DataWeave `as Date`: a Date has no time and no zone, so an ISO date-time such as
// 2025-05-01T11:39:15.257Z becomes 2025-05-01 — the calendar date as written, never converted to
// UTC first (2025-05-01T23:59:59+02:00 stays 2025-05-01).
func parseDate(value *string, field string) (*Date, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	t, _, err := parseISO(*value, field)
	if err != nil {
		return nil, &CoercionError{Field: field, Value: *value, Target: "Date", Err: err}
	}
	return &Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}, nil
}

// parseTimestamp coerces one of the three timestamp columns under precision (MAPPING.md
// deviation 4). PrecisionDate is Mule's `as Date`, exactly parseDate. PrecisionInstant keeps the
// full timestamp, normalised to UTC; a plain date in the input stays a date.
func parseTimestamp(value *string, field string, precision Precision) (*Timestamp, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	if precision != PrecisionInstant {
		d, err := parseDate(value, field)
		if err != nil {
			return nil, err
		}
		return &Timestamp{Date: *d, DateOnly: true}, nil
	}
	t, dateOnly, err := parseISO(*value, field)
	if err != nil {
		return nil, &CoercionError{Field: field, Value: *value, Target: "DateTime", Err: err}
	}
	ts := &Timestamp{Date: Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}, DateOnly: dateOnly}
	if !dateOnly {
		ts.Instant = t.UTC()
	}
	return ts, nil
}
