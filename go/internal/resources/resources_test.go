package resources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootAndFiles(t *testing.T) {
	root, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "MAPPING.md")); err != nil {
		t.Errorf("%s is not the repository root: %v", root, err)
	}
	again, _ := Root()
	if again != root {
		t.Error("Root must be stable")
	}

	sqlite, err := SQLiteSchema()
	if err != nil || !strings.Contains(string(sqlite), "CREATE TABLE IF NOT EXISTS contacts") {
		t.Errorf("SQLiteSchema: %v", err)
	}
	postgres, err := PostgresSchema()
	if err != nil || !strings.Contains(string(postgres), "CREATE TABLE public.contacts") {
		t.Errorf("PostgresSchema: %v", err)
	}
	template, err := ReportTemplate()
	if err != nil || !strings.Contains(string(template), "#[now()") {
		t.Errorf("ReportTemplate: %v", err)
	}
	if _, err := read("does/not/exist"); err == nil {
		t.Error("a missing resource must be an error")
	}
}

func TestLocateFailsOutsideTheRepository(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	// The executable (the test binary) lives in a temporary build directory, so neither start finds the marker.
	if _, err := locate(); err == nil {
		t.Error("expected an error outside the repository")
	}
	t.Chdir(wd)
}
