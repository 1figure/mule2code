// Package testutil gives the tests access to the shared fixtures in <repo>/testdata by path.
// The fixtures are never copied into the Go tree; a test that must hand a file to the pipeline
// (which moves it) copies it into its own temporary directory.
package testutil

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"muletocode/internal/resources"
)

// FixturePath returns the absolute path of <repo>/testdata/<rel>, whether or not it exists.
func FixturePath(t testing.TB, rel string) string {
	t.Helper()
	root, err := resources.Root()
	if err != nil {
		t.Fatalf("locating repository root: %v", err)
	}
	return filepath.Join(root, "testdata", filepath.FromSlash(rel))
}

// Fixture returns the absolute path of <repo>/testdata/<rel> and fails the test if it is missing.
func Fixture(t testing.TB, rel string) string {
	t.Helper()
	path := FixturePath(t, rel)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture %s: %v", rel, err)
	}
	return path
}

// ReadFixture returns the content of <repo>/testdata/<rel>.
func ReadFixture(t testing.TB, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(Fixture(t, rel))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", rel, err)
	}
	return data
}

// ReadJSON decodes the JSON fixture <repo>/testdata/<rel> into v.
func ReadJSON(t testing.TB, rel string, v any) {
	t.Helper()
	if err := json.Unmarshal(ReadFixture(t, rel), v); err != nil {
		t.Fatalf("decoding fixture %s: %v", rel, err)
	}
}

// Canonical re-encodes a JSON document with sorted keys and no insignificant whitespace, so two
// documents can be compared byte for byte regardless of formatting.
func Canonical(t testing.TB, data []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decoding JSON: %v\n%s", err, data)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encoding JSON: %v", err)
	}
	return string(out)
}

// CopyFixture copies <repo>/testdata/<rel> to dst, creating dst's directory.
func CopyFixture(t testing.TB, rel, dst string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, ReadFixture(t, rel), 0o644); err != nil {
		t.Fatalf("writing %s: %v", dst, err)
	}
}

// AssertJSONEqual fails when the two JSON documents differ after canonicalisation.
func AssertJSONEqual(t testing.TB, want, got []byte) {
	t.Helper()
	w, g := Canonical(t, want), Canonical(t, got)
	if w != g {
		t.Errorf("JSON differs\nwant: %s\ngot:  %s", w, g)
	}
}

// Equal is bytes.Equal, re-exported so callers do not need the import for one call.
func Equal(a, b []byte) bool { return bytes.Equal(a, b) }
