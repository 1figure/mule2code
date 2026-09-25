package pipeline

import (
	"strings"
	"time"
)

// runVars are the flow variables set by initialization-flow, one set per processed file.
type runVars struct {
	startTime       time.Time // vars.startTime
	currentFilename string    // vars.currentFilename
	timestamp       string    // vars.timestamp: startTime as uuuuMMdd'T'HHmmssSSS
	newFilename     string    // vars.newFilename: <stem>.<timestamp>.<ext>, the name in processed/
	errorsFilename  string    // vars.errorsFilename: <stem>.<timestamp>.errors.json
}

// The DataWeave uuuuMMdd'T'HHmmssSSS pattern: Go has no millisecond verb without a separator,
// so the dot of .000 is removed after formatting.
const timestampLayout = "20060102T150405.000"

// initialization is <flow name="initialization-flow">: it derives the variables from the file
// name and the start time. The name is split at dots exactly as splitBy(".") did: the stem is the
// part before the first dot, the extension the part after it.
func initialization(filename string, startTime time.Time) runVars {
	timestamp := strings.Replace(startTime.Format(timestampLayout), ".", "", 1)
	parts := strings.Split(filename, ".")
	stem := parts[0]
	extension := ""
	if len(parts) > 1 {
		extension = parts[1]
	}
	return runVars{
		startTime:       startTime,
		currentFilename: filename,
		timestamp:       timestamp,
		newFilename:     stem + "." + timestamp + "." + extension,
		errorsFilename:  stem + "." + timestamp + ".errors.json",
	}
}
