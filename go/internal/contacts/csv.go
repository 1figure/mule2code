package contacts

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
)

// Row is one CSV data row keyed by the header names: the element type of `payload as Iterator`.
type Row map[string]string

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ReadRows is <ee:transform doc:name="CSV to Java">: `payload as Iterator` on an
// `application/csv; header=true` payload. It yields one Row per data row, lazily; missing trailing
// cells are empty strings and extra cells are ignored. A missing header or a malformed row ends
// the sequence with an error, which fails the whole file like a DataWeave read error would.
func ReadRows(r io.Reader) iter.Seq2[Row, error] {
	return func(yield func(Row, error) bool) {
		br := bufio.NewReader(r)
		if lead, err := br.Peek(len(utf8BOM)); err == nil && bytes.Equal(lead, utf8BOM) {
			_, _ = br.Discard(len(utf8BOM))
		}
		cr := csv.NewReader(br)
		cr.FieldsPerRecord = -1
		cr.LazyQuotes = true

		header, err := cr.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = errors.New("CSV input has no header row")
			} else {
				err = fmt.Errorf("reading CSV header: %w", err)
			}
			yield(nil, err)
			return
		}
		for {
			cells, err := cr.Read()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(nil, fmt.Errorf("reading CSV: %w", err))
				return
			}
			row := make(Row, len(header))
			for i, name := range header {
				if i < len(cells) {
					row[name] = cells[i]
				} else {
					row[name] = ""
				}
			}
			if !yield(row, nil) {
				return
			}
		}
	}
}

// RowToCSVLine is DataWeave write(payload, "application/csv", {header: false, lineSeparator: ""}):
// the row as a single CSV line without a line terminator, cells in Columns order, quoted only when
// a cell contains a separator, a quote or a line break (MAPPING.md deviation 5).
func RowToCSVLine(row Row) string {
	var sb strings.Builder
	for i, column := range Columns {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(quoteIfNeeded(row[column]))
	}
	return sb.String()
}

func quoteIfNeeded(value string) string {
	if !strings.ContainsAny(value, ",\"\n\r") {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
