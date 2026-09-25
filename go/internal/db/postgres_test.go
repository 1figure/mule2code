package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"muletocode/internal/config"
	"muletocode/internal/db"
)

// postgresOptedIn reports whether the PostgreSQL tests should run: they are opt-in with RUN_POSTGRES=1.
func postgresOptedIn() bool { return os.Getenv("RUN_POSTGRES") == "1" }

// startPostgres starts a throwaway PostgreSQL through Testcontainers and returns its DSN. The test
// is skipped unless RUN_POSTGRES=1 is set, and when Docker is not available.
func startPostgres(t testing.TB) string {
	t.Helper()
	if !postgresOptedIn() {
		t.Skip("set RUN_POSTGRES=1 to run the PostgreSQL container test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("contacts"),
		postgres.WithUsername("contacts"),
		postgres.WithPassword("contacts"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		t.Skipf("PostgreSQL container not available (is Docker running?): %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("terminating container: %v", err)
		}
	})
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}

// postgresRepositoryForBind gives TestBind a Postgres-profile repository, or nil when skipped.
func postgresRepositoryForBind(t testing.TB) *db.Repository {
	t.Helper()
	if !postgresOptedIn() {
		return nil
	}
	dsn := startPostgres(t)
	repo, err := db.Open(context.Background(), config.Postgres, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func TestPostgresRepository(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	repo, err := db.Open(ctx, config.Postgres, dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer repo.Close()
	if repo.Profile() != config.Postgres {
		t.Errorf("profile = %v", repo.Profile())
	}

	RepositoryContract(t, repo)

	if n := countRows(t, "pgx", dsn); n != 100 {
		t.Errorf("table holds %d rows, want 100", n)
	}

	t.Run("opening again does not re-apply the schema", func(t *testing.T) {
		again, err := db.Open(ctx, config.Postgres, dsn)
		if err != nil {
			t.Fatalf("second Open must find the existing table: %v", err)
		}
		again.Close()
		if n := countRows(t, "pgx", dsn); n != 100 {
			t.Errorf("table holds %d rows after reopening, want 100", n)
		}
	})
}
