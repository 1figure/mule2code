package pipeline

import (
	"fmt"

	"muletocode/internal/batch"
	"muletocode/internal/contacts"
)

// ErrorRecord is one entry of the <stem>.<timestamp>.errors.json file.
type ErrorRecord struct {
	// Error is Batch::getFirstException().message.
	Error string `json:"Error"`
	// Record is the failed row re-serialised as a single CSV line without header.
	Record string `json:"Record"`
}

// NewErrorRecord is <ee:transform doc:name="Create Error Record">:
// {Error: Batch::getFirstException().message, Record: write(payload, "application/csv", {header: false, lineSeparator: ""})}.
func NewErrorRecord(r *batch.Record[contacts.Row]) ErrorRecord {
	message := ""
	if r.Err != nil {
		message = r.Err.Error()
	}
	return ErrorRecord{Error: message, Record: contacts.RowToCSVLine(r.Payload)}
}

// FileError reports that processing one input file failed; the file was moved to failed/ and the
// cause is wrapped.
type FileError struct {
	Filename string
	Err      error
}

func (e *FileError) Error() string {
	return fmt.Sprintf("Failed processing file %s: %v", e.Filename, e.Err)
}

// Unwrap returns the cause.
func (e *FileError) Unwrap() error { return e.Err }
