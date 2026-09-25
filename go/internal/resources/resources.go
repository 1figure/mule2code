// Package resources locates the upstream files the port reuses at run time: the two schema
// scripts under docker/ and the e-mail template of the Mule application. They live outside the
// Go module, so they are found by walking up from the working directory (or the executable) to
// the repository root.
package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const marker = "docker/sqlite/contacts.sqlite.sql"

var (
	once    sync.Once
	root    string
	rootErr error
)

// Root returns the repository root: the nearest ancestor of the working directory (then of the
// executable) that contains docker/sqlite/contacts.sqlite.sql.
func Root() (string, error) {
	once.Do(func() { root, rootErr = locate() })
	return root, rootErr
}

// SQLiteSchema returns the content of docker/sqlite/contacts.sqlite.sql.
func SQLiteSchema() ([]byte, error) {
	return read("docker/sqlite/contacts.sqlite.sql")
}

// PostgresSchema returns the content of docker/init/01-contacts.sql.
func PostgresSchema() ([]byte, error) {
	return read("docker/init/01-contacts.sql")
}

// ReportTemplate returns the upstream parse-template/contacts-batch-report-email.template.
func ReportTemplate() ([]byte, error) {
	return read("mule/batch-contacts-csv-to-db/src/main/resources/parse-template/contacts-batch-report-email.template")
}

func read(rel string) ([]byte, error) {
	dir, err := Root()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("reading resource %s: %w", rel, err)
	}
	return data, nil
}

func locate() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		for dir := start; ; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(marker))); err == nil {
				return dir, nil
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return "", fmt.Errorf("repository root not found: no %s above %s", marker, strings.Join(starts, " or "))
}
