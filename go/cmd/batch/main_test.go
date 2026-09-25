package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // row counts

	"muletocode/internal/testutil"
)

// env points the host at a fresh DATA_DIR; the tests set the process environment, so none runs in parallel.
func env(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)
	t.Setenv("DB_PROFILE", "sqlite")
	t.Setenv("SQLITE_PATH", "")
	return dir
}

func rows(t *testing.T, dir string) int {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", filepath.Join(dir, "contacts.db"))
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

func TestRun_Arguments(t *testing.T) {
	env(t)
	var stderr strings.Builder
	if code := run(context.Background(), []string{"-bogus"}, &strings.Builder{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Errorf("unknown flag: code %d, stderr %q", code, stderr.String())
	}
	stderr.Reset()
	if code := run(context.Background(), []string{"extra"}, &strings.Builder{}, &stderr); code != 2 || !strings.Contains(stderr.String(), usage) {
		t.Errorf("positional argument: code %d, stderr %q", code, stderr.String())
	}
	if code := run(context.Background(), []string{"-h"}, &strings.Builder{}, &stderr); code != 2 {
		t.Errorf("-h: code %d", code)
	}
}

func TestRun_ConfigurationAndDatabaseErrors(t *testing.T) {
	env(t)
	t.Setenv("DB_PROFILE", "oracle")
	var stderr strings.Builder
	if code := run(context.Background(), []string{"-once"}, &strings.Builder{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "DB_PROFILE") {
		t.Errorf("bad profile: code %d, stderr %q", code, stderr.String())
	}
	t.Setenv("DB_PROFILE", "postgres")
	t.Setenv("POSTGRES_DSN", "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	stderr.Reset()
	if code := run(context.Background(), []string{"-once"}, &strings.Builder{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "Cannot open the postgres database") {
		t.Errorf("unreachable database: code %d, stderr %q", code, stderr.String())
	}
}

func TestRun_File(t *testing.T) {
	dir := env(t)
	src := filepath.Join(dir, "in", "contact-data-100.csv")
	testutil.CopyFixture(t, "contact-data-100.csv", src)
	var stdout, stderr strings.Builder
	if code := run(context.Background(), []string{"-verbose", "-file", src}, &stdout, &stderr); code != 0 {
		t.Fatalf("code %d, stderr:\n%s", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "Subject: Mule Contacts Batch Job Report\n") || !strings.Contains(stdout.String(), "<td>Records read:</td><td>100</td>") {
		t.Errorf("report on stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "level=DEBUG") {
		t.Error("-verbose must log the TRACE loggers")
	}
	if n := rows(t, dir); n != 100 {
		t.Errorf("%d rows, want 100", n)
	}

	// A broken file: exit 1, nothing on stdout.
	empty := filepath.Join(dir, "in", "empty.csv")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"-file", empty}, &stdout, &stderr); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Failed processing file empty.csv") {
		t.Errorf("empty file: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestRun_Once(t *testing.T) {
	dir := env(t)
	testutil.CopyFixture(t, "contact-data-100-with-errors.csv", filepath.Join(dir, "inbox", "a.csv"))
	var stdout strings.Builder
	if code := run(context.Background(), []string{"-once"}, &stdout, &strings.Builder{}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if n := rows(t, dir); n != 98 {
		t.Errorf("%d rows, want 98", n)
	}
	if code := run(context.Background(), []string{"-once"}, &stdout, &strings.Builder{}); code != 0 {
		t.Errorf("empty inbox: code %d", code)
	}
}

// TIMESTAMP_PRECISION reaches the pipeline: instant keeps the time part, an invalid value is rejected at start-up.
func TestRun_TimestampPrecision(t *testing.T) {
	dir := env(t)
	t.Setenv("TIMESTAMP_PRECISION", "instant")
	src := filepath.Join(dir, "in", "contact-data-100.csv")
	testutil.CopyFixture(t, "contact-data-100.csv", src)
	if code := run(context.Background(), []string{"-file", src}, &strings.Builder{}, &strings.Builder{}); code != 0 {
		t.Fatalf("code %d", code)
	}
	sqlDB, err := sql.Open("sqlite", filepath.Join(dir, "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var created, modified string
	if err := sqlDB.QueryRow("SELECT created_date, last_modified_date FROM contacts WHERE external_id = '1'").Scan(&created, &modified); err != nil {
		t.Fatal(err)
	}
	if created != "2007-09-26" || modified != "2025-05-01T11:39:15.257Z" {
		t.Errorf("stored as %q %q", created, modified)
	}

	t.Setenv("TIMESTAMP_PRECISION", "nanos")
	var stderr strings.Builder
	if code := run(context.Background(), []string{"-once"}, &strings.Builder{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "TIMESTAMP_PRECISION") {
		t.Errorf("invalid precision: code %d, stderr %q", code, stderr.String())
	}
}

func TestRun_WatchUntilCancelled(t *testing.T) {
	dir := env(t)
	t.Setenv("POLL_SECONDS", "1")
	testutil.CopyFixture(t, "contact-data-100.csv", filepath.Join(dir, "inbox", "a.csv"))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if entries, _ := os.ReadDir(filepath.Join(dir, "reports")); len(entries) > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}()
	var stdout, stderr strings.Builder
	if code := run(ctx, nil, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "stopped") {
		t.Errorf("code %d, stderr %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Error("in watch mode the report goes to the file sink, not stdout")
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "reports")); len(entries) != 1 {
		t.Errorf("reports/ holds %d files, want 1", len(entries))
	}
}
