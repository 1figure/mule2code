package config

import (
	"path/filepath"
	"strings"
	"testing"

	"muletocode/internal/contacts"
)

func lookup(env map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := env[name]
		return v, ok
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := FromLookup(lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Default() {
		t.Errorf("no variables set: got %+v, want %+v", cfg, Default())
	}
	if cfg.DBProfile != SQLite || cfg.DSN() != filepath.Join("data", "contacts.db") || cfg.HTTPAddr != "0.0.0.0:8082" ||
		cfg.BatchBlockSize != 10000 || cfg.BatchAggregatorSize != 1000 || cfg.PollSeconds != 10 || cfg.NormalizeMaxContacts != 50 ||
		cfg.ZippopotamBaseURL != "https://api.zippopotam.us" {
		t.Errorf("defaults = %+v", cfg)
	}
	for _, dir := range []string{cfg.InboxDir(), cfg.ProcessedDir(), cfg.FailedDir(), cfg.ReportsDir()} {
		if !strings.HasPrefix(dir, "data"+string(filepath.Separator)) {
			t.Errorf("%s is not under DATA_DIR", dir)
		}
	}
}

func TestFromLookup(t *testing.T) {
	cfg, err := FromLookup(lookup(map[string]string{
		"DB_PROFILE":             " PostgreSQL ",
		"POSTGRES_DSN":           "postgres://x@h/db",
		"DATA_DIR":               "/var/data",
		"BATCH_BLOCK_SIZE":       "7",
		"BATCH_AGGREGATOR_SIZE":  "3",
		"POLL_SECONDS":           "1",
		"HTTP_ADDR":              ":9999",
		"ZIPPOPOTAM_BASE_URL":    "http://stub",
		"NORMALIZE_MAX_CONTACTS": "2",
		"SQLITE_PATH":            "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBProfile != Postgres || cfg.DSN() != "postgres://x@h/db" || cfg.SQLitePath != filepath.Join("/var/data", "contacts.db") ||
		cfg.BatchBlockSize != 7 || cfg.BatchAggregatorSize != 3 || cfg.PollSeconds != 1 || cfg.HTTPAddr != ":9999" ||
		cfg.ZippopotamBaseURL != "http://stub" || cfg.NormalizeMaxContacts != 2 || cfg.InboxDir() != filepath.Join("/var/data", "inbox") {
		t.Errorf("cfg = %+v", cfg)
	}
	cfg, err = FromLookup(lookup(map[string]string{"SQLITE_PATH": "/tmp/x.db", "DB_PROFILE": "sqlite"}))
	if err != nil || cfg.DSN() != "/tmp/x.db" {
		t.Errorf("SQLITE_PATH: %v %v", cfg.DSN(), err)
	}

	if cfg.TimestampPrecision != contacts.PrecisionDate {
		t.Errorf("TIMESTAMP_PRECISION default = %q, want date", cfg.TimestampPrecision)
	}
	cfg, err = FromLookup(lookup(map[string]string{"TIMESTAMP_PRECISION": "Instant"}))
	if err != nil || cfg.TimestampPrecision != contacts.PrecisionInstant {
		t.Errorf("TIMESTAMP_PRECISION=Instant: %v %v", cfg.TimestampPrecision, err)
	}

	for name, env := range map[string]map[string]string{
		"bad profile":   {"DB_PROFILE": "oracle"},
		"bad int":       {"POLL_SECONDS": "soon"},
		"bad precision": {"TIMESTAMP_PRECISION": "nanos"},
	} {
		if _, err := FromLookup(lookup(env)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := FromEnv(); err != nil {
		t.Errorf("FromEnv: %v", err)
	}
}
