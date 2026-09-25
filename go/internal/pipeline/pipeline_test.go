package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite" // raw row counts

	"muletocode/internal/batch"
	"muletocode/internal/config"
	"muletocode/internal/contacts"
	"muletocode/internal/db"
	"muletocode/internal/testutil"
)

// captureSink records what the report flow sent.
type captureSink struct {
	mu    sync.Mutex
	sent  []string
	fail  error
	calls int
}

func (s *captureSink) Send(_ context.Context, subject, html string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.fail != nil {
		return s.fail
	}
	s.sent = append(s.sent, subject+"\n"+html)
	return nil
}

// fixedClock is the run's now(): every call returns the same instant, so file names are predictable.
var fixedClock = time.Date(2026, time.September, 25, 14, 3, 7, 123_000_000, time.UTC)

const fixedTimestamp = "20260925T140307123"

type harness struct {
	p    *Pipeline
	cfg  config.Config
	sink *captureSink
}

func newHarness(t *testing.T, aggregatorSize int) *harness {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.SQLitePath = filepath.Join(dir, "contacts.db")
	cfg.BatchAggregatorSize = aggregatorSize
	repo, err := db.Open(context.Background(), config.SQLite, cfg.SQLitePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	sink := &captureSink{}
	p := New(cfg, repo, sink, nil)
	p.Now = func() time.Time { return fixedClock }
	return &harness{p: p, cfg: cfg, sink: sink}
}

func (h *harness) inbox(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(h.cfg.InboxDir(), name)
}

func (h *harness) rowCount(t *testing.T) int {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", h.cfg.SQLitePath)
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

func (h *harness) processedFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(h.cfg.ProcessedDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// expectedStatistics is the subset of the statistics object the golden files carry.
type expectedStatistics struct {
	TotalRecords      int64 `json:"totalRecords"`
	LoadedRecords     int64 `json:"loadedRecords"`
	ProcessedRecords  int64 `json:"processedRecords"`
	SuccessfulRecords int64 `json:"successfulRecords"`
	FailedRecords     int64 `json:"failedRecords"`
}

func assertStatistics(t *testing.T, golden string, stats batch.Statistics) {
	t.Helper()
	var want expectedStatistics
	testutil.ReadJSON(t, golden, &want)
	got := expectedStatistics{stats.TotalRecords, stats.LoadedRecords, stats.ProcessedRecords, stats.SuccessfulRecords, stats.FailedRecords}
	if got != want {
		t.Errorf("statistics = %+v, want %+v (%s)", got, want, golden)
	}
	if stats.BatchJobInstanceID == "" || stats.FailedOnInputPhase {
		t.Errorf("statistics = %+v", stats)
	}
}

var renamedPattern = regexp.MustCompile(`^(.+)\.(\d{8}T\d{9})\.csv$`)

func TestProcessFile_CleanFile(t *testing.T) {
	h := newHarness(t, 1000)
	src := h.inbox(t, "contact-data-100.csv")
	testutil.CopyFixture(t, "contact-data-100.csv", src)

	stats, err := h.p.ProcessFile(context.Background(), src)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}
	assertStatistics(t, "expected/batch-report-100.json", stats)
	if n := h.rowCount(t); n != 100 {
		t.Errorf("%d rows inserted, want 100", n)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("the input file must have left the inbox")
	}
	files := h.processedFiles(t)
	if len(files) != 1 {
		t.Fatalf("processed/ holds %v, want the renamed file only (no errors file for a clean run)", files)
	}
	m := renamedPattern.FindStringSubmatch(files[0])
	if m == nil || m[1] != "contact-data-100" || m[2] != fixedTimestamp {
		t.Errorf("renamed to %q, want contact-data-100.%s.csv", files[0], fixedTimestamp)
	}
	if len(h.sink.sent) != 1 {
		t.Fatalf("report sent %d times, want 1", len(h.sink.sent))
	}
	report := h.sink.sent[0]
	for _, want := range []string{"Mule Contacts Batch Job Report", "<td>Processed file:</td><td>contact-data-100.csv</td>", "<td>Records read:</td><td>100</td>", "<td>Successful records:</td><td>100</td>", "<td>Failed records:</td><td>0</td>", "Friday, September 25, 2026 at 14:03:07.123"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

func TestProcessFile_WithErrors(t *testing.T) {
	h := newHarness(t, 1000)
	src := h.inbox(t, "contact-data-100-with-errors.csv")
	testutil.CopyFixture(t, "contact-data-100-with-errors.csv", src)

	stats, err := h.p.ProcessFile(context.Background(), src)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}
	assertStatistics(t, "expected/batch-report-100-with-errors.json", stats)
	if n := h.rowCount(t); n != 98 {
		t.Errorf("%d rows inserted, want 98", n)
	}

	files := h.processedFiles(t)
	csvName := "contact-data-100-with-errors." + fixedTimestamp + ".csv"
	errName := "contact-data-100-with-errors." + fixedTimestamp + ".errors.json"
	if len(files) != 2 || files[0] != csvName || files[1] != errName {
		t.Fatalf("processed/ holds %v, want [%s %s]", files, csvName, errName)
	}
	got, err := os.ReadFile(filepath.Join(h.cfg.ProcessedDir(), errName))
	if err != nil {
		t.Fatal(err)
	}
	testutil.AssertJSONEqual(t, testutil.ReadFixture(t, "expected/errors-100-with-errors.json"), got)
	if !testutil.Equal(got, testutil.ReadFixture(t, "expected/errors-100-with-errors.json")) {
		t.Log("errors file is JSON-equal but not byte-identical to the golden")
	}
	if len(h.sink.sent) != 1 || !strings.Contains(h.sink.sent[0], "<td>Failed records:</td><td>2</td>") {
		t.Errorf("report: %v", h.sink.sent)
	}
}

// A coercion error is raised inside the aggregator, so it fails the whole block, not one record.
func TestProcessFile_CoercionErrorFailsTheBlock(t *testing.T) {
	const blockSize, badRow = 10, 14 // 0-based row 14 sits in the second block (rows 10-19)
	h := newHarness(t, blockSize)

	// Derive the input from the fixture: one cell of one row made non-numeric.
	f, err := os.Open(testutil.Fixture(t, "contact-data-100.csv"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []contacts.Row
	for row, err := range contacts.ReadRows(f) {
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	f.Close()
	rows[badRow]["active_tracker_count"] = "many"
	var sb strings.Builder
	sb.WriteString(strings.Join(contacts.Columns, ",") + "\n")
	for _, row := range rows {
		sb.WriteString(contacts.RowToCSVLine(row) + "\n")
	}
	src := h.inbox(t, "coercion.csv")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	stats, err := h.p.ProcessFile(context.Background(), src)
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}
	if stats.FailedRecords != blockSize || stats.SuccessfulRecords != 100-blockSize {
		t.Errorf("statistics = %+v, want %d failed", stats, blockSize)
	}
	if n := h.rowCount(t); n != 100-blockSize {
		t.Errorf("%d rows inserted, want %d", n, 100-blockSize)
	}
	data, err := os.ReadFile(filepath.Join(h.cfg.ProcessedDir(), "coercion."+fixedTimestamp+".errors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var records []ErrorRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != blockSize {
		t.Fatalf("errors file holds %d records, want the block of %d", len(records), blockSize)
	}
	for i, r := range records {
		if r.Error != "Cannot coerce String (many) to Number" {
			t.Errorf("record %d error = %q", i, r.Error)
		}
		if want := contacts.RowToCSVLine(rows[10+i]); r.Record != want {
			t.Errorf("record %d is not row %d of the block", i, 10+i)
		}
	}
}

// The errors file writes ', <, >, & and non-ASCII literally, as DataWeave's JSON writer does.
func TestProcessFile_ErrorsFileEscaping(t *testing.T) {
	h := newHarness(t, 1000)
	row := contacts.Row{}
	for _, c := range contacts.Columns {
		row[c] = ""
	}
	row["contact_name"] = `O'Brien <&> Ñandú`
	row["email"] = "not-an-email"
	src := h.inbox(t, "esc.csv")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(strings.Join(contacts.Columns, ",")+"\n"+contacts.RowToCSVLine(row)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.p.ProcessFile(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(h.cfg.ProcessedDir(), "esc."+fixedTimestamp+".errors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `O'Brien <&> Ñandú`) || strings.Contains(string(data), `\u`) {
		t.Errorf("errors file must not escape these characters:\n%s", data)
	}
}

func TestProcessFile_EmptyFileGoesToFailed(t *testing.T) {
	h := newHarness(t, 1000)
	src := h.inbox(t, "empty.csv")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := h.p.ProcessFile(context.Background(), src)
	var fileErr *FileError
	var inputErr *batch.InputError
	if !errors.As(err, &fileErr) || !errors.As(err, &inputErr) || fileErr.Filename != "empty.csv" {
		t.Fatalf("got %v, want *FileError wrapping *batch.InputError", err)
	}
	if !strings.HasPrefix(err.Error(), "Failed processing file empty.csv") {
		t.Errorf("message = %q", err)
	}
	if _, err := os.Stat(filepath.Join(h.cfg.FailedDir(), "empty.csv")); err != nil {
		t.Errorf("file not in failed/: %v", err)
	}
	if files := h.processedFiles(t); len(files) != 0 {
		t.Errorf("processed/ must stay empty, holds %v", files)
	}
	if h.sink.calls != 0 {
		t.Error("no report may be sent for a failed file")
	}
	if n := h.rowCount(t); n != 0 {
		t.Errorf("%d rows inserted, want 0", n)
	}
}

// <batch:on-complete> runs after the flow has ended in Mule, so a failing report is logged and
// never moves the file: the rows are committed and the file lands in processed/ as usual.
func TestProcessFile_ReportFailure(t *testing.T) {
	h := newHarness(t, 1000)
	h.sink.fail = errors.New("smtp down")
	src := h.inbox(t, "contact-data-100.csv")
	testutil.CopyFixture(t, "contact-data-100.csv", src)
	stats, err := h.p.ProcessFile(context.Background(), src)
	if err != nil {
		t.Fatalf("an on-complete failure must not fail the flow: %v", err)
	}
	assertStatistics(t, "expected/batch-report-100.json", stats)
	if !stats.FailedOnCompletePhase {
		t.Error("failedOnCompletePhase must be set")
	}
	if h.sink.calls != 1 {
		t.Errorf("report attempted %d times, want 1", h.sink.calls)
	}
	if _, err := os.Stat(filepath.Join(h.cfg.ProcessedDir(), "contact-data-100."+fixedTimestamp+".csv")); err != nil {
		t.Errorf("on-complete failure must not move the file away from processed/: %v", err)
	}
	if _, err := os.Stat(h.cfg.FailedDir()); !os.IsNotExist(err) {
		t.Error("failed/ must not be created")
	}
	if n := h.rowCount(t); n != 100 {
		t.Errorf("%d rows, want 100", n)
	}
}

func TestProcessFile_MissingFile(t *testing.T) {
	h := newHarness(t, 1000)
	_, err := h.p.ProcessFile(context.Background(), h.inbox(t, "ghost.csv"))
	var fileErr *FileError
	if !errors.As(err, &fileErr) {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(h.cfg.FailedDir()); !os.IsNotExist(err) {
		t.Error("nothing to move: failed/ must not be created")
	}
}

func TestProcessFile_Cancelled(t *testing.T) {
	h := newHarness(t, 1000)
	src := h.inbox(t, "contact-data-100.csv")
	testutil.CopyFixture(t, "contact-data-100.csv", src)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.p.ProcessFile(ctx, src)
	var fileErr *FileError
	if !errors.Is(err, context.Canceled) || errors.As(err, &fileErr) {
		t.Fatalf("got %v, want context.Canceled and no *FileError", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Error("a cancelled run must leave the file in the inbox")
	}
}

func TestProcessInbox(t *testing.T) {
	h := newHarness(t, 1000)
	testutil.CopyFixture(t, "contact-data-100.csv", h.inbox(t, "b-clean.csv"))
	testutil.CopyFixture(t, "contact-data-100-with-errors.csv", h.inbox(t, "a-errors.CSV"))
	if err := os.WriteFile(h.inbox(t, "notes.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.inbox(t, "c-empty.csv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(h.inbox(t, "sub.csv"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("continueOnError=false stops at the failing file", func(t *testing.T) {
		results, err := h.p.ProcessInbox(context.Background(), false)
		var fileErr *FileError
		if !errors.As(err, &fileErr) || fileErr.Filename != "c-empty.csv" {
			t.Fatalf("got %v", err)
		}
		if len(results) != 2 || results[0].FailedRecords != 2 || results[1].FailedRecords != 0 {
			t.Errorf("files must be processed in name order, case-insensitive on the extension: %+v", results)
		}
		if n := h.rowCount(t); n != 198 {
			t.Errorf("%d rows, want 198", n)
		}
		if _, err := os.Stat(h.inbox(t, "notes.txt")); err != nil {
			t.Error("non-CSV files must stay untouched in the inbox")
		}
		if _, err := os.Stat(filepath.Join(h.cfg.FailedDir(), "c-empty.csv")); err != nil {
			t.Error("the empty file must be in failed/")
		}
	})

	t.Run("continueOnError=true skips the failing file", func(t *testing.T) {
		if err := os.WriteFile(h.inbox(t, "a-empty.csv"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		testutil.CopyFixture(t, "contact-data-100.csv", h.inbox(t, "b-clean.csv"))
		results, err := h.p.ProcessInbox(context.Background(), true)
		if err != nil || len(results) != 1 {
			t.Fatalf("got %v, %v", results, err)
		}
	})

	t.Run("empty inbox is created and yields nothing", func(t *testing.T) {
		fresh := newHarness(t, 1000)
		results, err := fresh.p.ProcessInbox(context.Background(), false)
		if err != nil || len(results) != 0 {
			t.Errorf("got %v, %v", results, err)
		}
		if _, err := os.Stat(fresh.cfg.InboxDir()); err != nil {
			t.Error("inbox must be created")
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		testutil.CopyFixture(t, "contact-data-100.csv", h.inbox(t, "z.csv"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := h.p.ProcessInbox(ctx, true); !errors.Is(err, context.Canceled) {
			t.Errorf("got %v", err)
		}
	})
}

func TestWatch(t *testing.T) {
	h := newHarness(t, 1000)
	h.cfg.PollSeconds = 0 // clamped to one second
	h.p = New(h.cfg, h.p.repo, h.sink, nil)
	testutil.CopyFixture(t, "contact-data-100.csv", h.inbox(t, "first.csv"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.p.Watch(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	for len(h.processedFiles(t)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the watcher did not process the file")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A file dropped while watching is picked up on the next tick, and a failing one does not stop the watcher.
	if err := os.WriteFile(h.inbox(t, "broken.csv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.CopyFixture(t, "contact-data-100.csv", h.inbox(t, "second.csv"))
	for len(h.processedFiles(t)) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("the watcher did not pick up the second file")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Watch returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not stop")
	}
	if _, err := os.Stat(filepath.Join(h.cfg.FailedDir(), "broken.csv")); err != nil {
		t.Error("the broken file must be in failed/")
	}
}

func TestInitialization(t *testing.T) {
	vars := initialization("contact-data-100.csv", fixedClock)
	if vars.timestamp != fixedTimestamp || vars.currentFilename != "contact-data-100.csv" || !vars.startTime.Equal(fixedClock) {
		t.Errorf("vars = %+v", vars)
	}
	if vars.newFilename != "contact-data-100."+fixedTimestamp+".csv" || vars.errorsFilename != "contact-data-100."+fixedTimestamp+".errors.json" {
		t.Errorf("names = %q %q", vars.newFilename, vars.errorsFilename)
	}
	// splitBy(".") semantics: the second segment is the extension, later ones are dropped.
	if v := initialization("a.b.c", fixedClock); v.newFilename != "a."+fixedTimestamp+".b" {
		t.Errorf("a.b.c → %q", v.newFilename)
	}
	// A name without a dot is not asserted: ProcessInbox only picks *.csv, and in Mule
	// `(vars.currentFilename splitBy("."))[1]` is null there, so `++ null` fails the flow.
}

func TestNewErrorRecord(t *testing.T) {
	r := &batch.Record[contacts.Row]{Payload: contacts.Row{"account_id": "x"}}
	if got := NewErrorRecord(r); got.Error != "" || !strings.HasPrefix(got.Record, "x,") {
		t.Errorf("unfailed record → %+v", got)
	}
	r.Fail(contacts.ErrInvalidEmail)
	if got := NewErrorRecord(r); got.Error != "Missing or invalid email" {
		t.Errorf("failed record → %+v", got)
	}
}
