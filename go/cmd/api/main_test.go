package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// env gives the host a fresh SQLite file; the tests set the process environment, so none runs in parallel.
func env(t *testing.T) {
	t.Helper()
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("DB_PROFILE", "sqlite")
	t.Setenv("SQLITE_PATH", "")
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
}

func TestRun_Errors(t *testing.T) {
	env(t)
	var stderr strings.Builder
	if code := run(context.Background(), []string{"-bogus"}, &stderr); code != 2 {
		t.Errorf("unknown flag: code %d", code)
	}
	if code := run(context.Background(), []string{"extra"}, &stderr); code != 2 {
		t.Errorf("positional argument: code %d", code)
	}
	t.Setenv("DB_PROFILE", "oracle")
	stderr.Reset()
	if code := run(context.Background(), nil, &stderr); code != 2 || !strings.Contains(stderr.String(), "DB_PROFILE") {
		t.Errorf("bad profile: code %d, stderr %q", code, stderr.String())
	}
	t.Setenv("DB_PROFILE", "postgres")
	t.Setenv("POSTGRES_DSN", "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	stderr.Reset()
	if code := run(context.Background(), nil, &stderr); code != 2 || !strings.Contains(stderr.String(), "Cannot open the postgres database") {
		t.Errorf("unreachable database: code %d, stderr %q", code, stderr.String())
	}
	t.Setenv("DB_PROFILE", "sqlite")
	t.Setenv("HTTP_ADDR", "999.999.999.999:1")
	stderr.Reset()
	if code := run(context.Background(), nil, &stderr); code != 2 || !strings.Contains(stderr.String(), "listen") {
		t.Errorf("unlistenable address: code %d, stderr %q", code, stderr.String())
	}
}

func TestRun_ServesUntilCancelled(t *testing.T) {
	env(t)
	ctx, cancel := context.WithCancel(context.Background())
	stderr := &syncWriter{}
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) && !strings.Contains(stderr.String(), "listening") {
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}()
	if code := run(ctx, []string{"-verbose"}, stderr); code != 0 {
		t.Errorf("code %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "stopped") {
		t.Errorf("stderr %q", stderr.String())
	}
}

// syncWriter lets the polling goroutine read the log while the host writes it.
type syncWriter struct {
	mu sync.Mutex
	sb strings.Builder
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sb.Write(p)
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sb.String()
}
