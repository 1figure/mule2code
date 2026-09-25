// Package config reads the settings of both applications from environment variables.
// It replaces <global-property name="env"> + properties/mule-props-${env}.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"muletocode/internal/contacts"
)

// Profile is the database profile selected by DB_PROFILE.
type Profile string

const (
	// SQLite is a file database; the schema is applied on every start.
	SQLite Profile = "sqlite"
	// Postgres is the profile the original Mule applications were written against.
	Postgres Profile = "postgres"
)

// Config holds the settings of both applications. Every field has a default, so nothing is required.
type Config struct {
	// DBProfile is DB_PROFILE: sqlite (default) or postgres.
	DBProfile Profile
	// SQLitePath is SQLITE_PATH, the SQLite database file (default <DATA_DIR>/contacts.db).
	SQLitePath string
	// PostgresDSN is POSTGRES_DSN (default matches docker/docker-compose.yml).
	PostgresDSN string
	// DataDir is DATA_DIR, the root of inbox/, processed/, failed/ and reports/ (default data).
	DataDir string
	// BatchBlockSize is BATCH_BLOCK_SIZE → ${batch.job.block_size} (default 10000).
	BatchBlockSize int
	// BatchAggregatorSize is BATCH_AGGREGATOR_SIZE → ${batch.aggregator.main.size} (default 1000).
	BatchAggregatorSize int
	// PollSeconds is POLL_SECONDS, the interval of the inbox watcher (the listener's fixed-frequency scheduler; default 10).
	PollSeconds int
	// HTTPAddr is HTTP_ADDR, the listen address of the API (default 0.0.0.0:8082, as ${http.host}:${http.port}).
	HTTPAddr string
	// ZippopotamBaseURL is ZIPPOPOTAM_BASE_URL → <http:request-config name="Zippopotam_Request_config"> (default https://api.zippopotam.us).
	ZippopotamBaseURL string
	// NormalizeMaxContacts is NORMALIZE_MAX_CONTACTS → ${normalize.max_contacts} (default 50).
	NormalizeMaxContacts int
	// TimestampPrecision is TIMESTAMP_PRECISION: how the three timestamp columns are coerced,
	// date (Mule's `as Date`, default) or instant (MAPPING.md deviation 4).
	TimestampPrecision contacts.Precision
}

// Default returns the settings used when no variable is set; SQLitePath is derived from DataDir.
func Default() Config {
	return Config{
		DBProfile:            SQLite,
		SQLitePath:           filepath.Join("data", "contacts.db"),
		PostgresDSN:          "postgres://contacts:contacts@localhost:5432/contacts",
		DataDir:              "data",
		BatchBlockSize:       10000,
		BatchAggregatorSize:  1000,
		PollSeconds:          10,
		HTTPAddr:             "0.0.0.0:8082",
		ZippopotamBaseURL:    "https://api.zippopotam.us",
		NormalizeMaxContacts: 50,
		TimestampPrecision:   contacts.PrecisionDate,
	}
}

// FromEnv reads the settings from the process environment, falling back to Default.
func FromEnv() (Config, error) {
	return FromLookup(os.LookupEnv)
}

// FromLookup reads the settings through lookup, which reports whether a variable is set.
// A set but empty variable counts as unset.
func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	cfg := Default()
	get := func(name string) string {
		v, _ := lookup(name)
		return v
	}

	profile, err := parseProfile(get("DB_PROFILE"))
	if err != nil {
		return Config{}, err
	}
	cfg.DBProfile = profile
	precision, err := contacts.ParsePrecision(get("TIMESTAMP_PRECISION"))
	if err != nil {
		return Config{}, err
	}
	cfg.TimestampPrecision = precision

	if v := get("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	cfg.SQLitePath = filepath.Join(cfg.DataDir, "contacts.db")
	if v := get("SQLITE_PATH"); v != "" {
		cfg.SQLitePath = v
	}
	if v := get("POSTGRES_DSN"); v != "" {
		cfg.PostgresDSN = v
	}
	if v := get("HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := get("ZIPPOPOTAM_BASE_URL"); v != "" {
		cfg.ZippopotamBaseURL = v
	}

	ints := []struct {
		name string
		dst  *int
	}{
		{"BATCH_BLOCK_SIZE", &cfg.BatchBlockSize},
		{"BATCH_AGGREGATOR_SIZE", &cfg.BatchAggregatorSize},
		{"POLL_SECONDS", &cfg.PollSeconds},
		{"NORMALIZE_MAX_CONTACTS", &cfg.NormalizeMaxContacts},
	}
	for _, v := range ints {
		raw := get(v.name)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be an integer, got %q: %w", v.name, raw, err)
		}
		*v.dst = n
	}
	return cfg, nil
}

// DSN is the connection string of the selected profile: the SQLite file path or the Postgres URL.
func (c Config) DSN() string {
	if c.DBProfile == Postgres {
		return c.PostgresDSN
	}
	return c.SQLitePath
}

// InboxDir is the directory the batch listener polls (${sftp.new_dir}).
func (c Config) InboxDir() string { return filepath.Join(c.DataDir, "inbox") }

// ProcessedDir is where successful files and errors files go (${sftp.processed_dir}).
func (c Config) ProcessedDir() string { return filepath.Join(c.DataDir, "processed") }

// FailedDir is where failed files are moved to (${sftp.failed_dir}).
func (c Config) FailedDir() string { return filepath.Join(c.DataDir, "failed") }

// ReportsDir is where the file report sink writes (replaces the e-mail).
func (c Config) ReportsDir() string { return filepath.Join(c.DataDir, "reports") }

func parseProfile(value string) (Profile, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "sqlite":
		return SQLite, nil
	case "postgres", "postgresql":
		return Postgres, nil
	default:
		return "", fmt.Errorf("DB_PROFILE must be 'sqlite' or 'postgres', got %q", value)
	}
}
