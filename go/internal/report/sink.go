package report

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Sink is where a rendered report goes; it replaces <email:send doc:name="Contacts Batch Report Email"> (MAPPING.md deviation 2).
// Real SMTP would be one net/smtp call behind this interface.
type Sink interface {
	// Send delivers one report.
	Send(ctx context.Context, subject, html string) error
}

// FileSink writes each report to <Dir>/<yyyyMMddTHHmmssSSS>.html.
type FileSink struct {
	// Dir is created on first use.
	Dir string
	// Now names the file; defaults to time.Now.
	Now func() time.Time
}

// NewFileSink creates a sink writing into dir.
func NewFileSink(dir string) *FileSink {
	return &FileSink{Dir: dir, Now: time.Now}
}

// Send writes the report, with the subject as a leading HTML comment.
func (s *FileSink) Send(_ context.Context, subject, html string) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("creating reports directory: %w", err)
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	name := strings.Replace(now().Format("20060102T150405.000"), ".", "", 1) + ".html"
	path := filepath.Join(s.Dir, name)
	if err := os.WriteFile(path, []byte("<!-- "+subject+" -->\n"+html), 0o644); err != nil {
		return fmt.Errorf("writing report %s: %w", path, err)
	}
	return nil
}

// StdoutSink prints each report to a writer (stdout in one-shot mode).
type StdoutSink struct {
	W io.Writer
}

// NewStdoutSink creates a sink printing to w, or to os.Stdout when w is nil.
func NewStdoutSink(w io.Writer) *StdoutSink {
	if w == nil {
		w = os.Stdout
	}
	return &StdoutSink{W: w}
}

// Send prints "Subject: ..." followed by the HTML.
func (s *StdoutSink) Send(_ context.Context, subject, html string) error {
	if _, err := fmt.Fprintf(s.W, "Subject: %s\n%s\n", subject, html); err != nil {
		return fmt.Errorf("printing report: %w", err)
	}
	return nil
}
