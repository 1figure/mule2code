// Command api is the host of contacts-api: GET /contacts and POST /contacts/normalize on
// HTTP_ADDR. Exit codes: 0 clean shutdown, 2 bad arguments, configuration, database or listen failure.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"muletocode/internal/api"
	"muletocode/internal/config"
	"muletocode/internal/db"
	"muletocode/internal/zippo"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stderr))
}

// run is the program with its inputs made explicit: it serves until ctx is done and returns the exit code.
func run(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.SetOutput(stderr)
	verbose := fs.Bool("verbose", false, "log at debug level")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return 2
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	// <global-property name="env"> + properties file → environment variables.
	cfg, err := config.FromEnv()
	if err != nil {
		logger.Error(err.Error())
		return 2
	}
	// <db:config name="Contacts_Database_Config">; fail fast on a database that cannot be reached,
	// as the Mule app would at deploy time.
	repo, err := db.Open(ctx, cfg.DBProfile, cfg.DSN())
	if err != nil {
		logger.Error(err.Error())
		return 2
	}
	defer repo.Close()

	// <http:request-config name="Zippopotam_Request_config">
	lookup := zippo.NewClient(cfg.ZippopotamBaseURL, &http.Client{Timeout: 30 * time.Second})

	// <http:listener-config name="HTTP_Listener_config"><http:listener-connection host= port=>
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewHandler(repo, lookup, cfg.NormalizeMaxContacts, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errs := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.HTTPAddr, "profile", cfg.DBProfile)
		errs <- server.ListenAndServe()
	}()

	select {
	case err := <-errs:
		logger.Error(err.Error())
		return 2
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("shutdown failed", "error", err)
		return 2
	}
	logger.Info("stopped")
	return 0
}
