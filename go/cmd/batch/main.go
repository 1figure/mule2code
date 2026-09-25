// Command batch is the host of batch-contacts-csv-to-db. Without arguments it watches
// <DATA_DIR>/inbox like the SFTP listener did; -once processes the inbox once; -file <path>
// processes one file. In the one-shot modes the report goes to stdout, otherwise to
// <DATA_DIR>/reports. Exit codes: 0 success, 1 a file failed, 2 bad arguments, configuration or database.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"muletocode/internal/config"
	"muletocode/internal/db"
	"muletocode/internal/pipeline"
	"muletocode/internal/report"
)

const usage = "usage: batch [-once | -file <path>] [-verbose]"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// run is the program with its inputs made explicit: args are the command-line flags, stdout
// receives the one-shot report, stderr the log. It returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("batch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	once := fs.Bool("once", false, "process every *.csv in the inbox once, then exit")
	file := fs.String("file", "", "process one `path`, then exit")
	verbose := fs.Bool("verbose", false, "log the flow's TRACE loggers (debug level)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(stderr, usage)
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
	// <db:config name="Postgres_Database_Config">
	repo, err := db.Open(ctx, cfg.DBProfile, cfg.DSN())
	if err != nil {
		logger.Error(err.Error())
		return 2
	}
	defer repo.Close()

	// <email:smtp-config name="Email_Gmail_Config"> → a report sink (MAPPING.md deviation 2).
	var sink report.Sink
	if *once || *file != "" {
		sink = report.NewStdoutSink(stdout)
	} else {
		sink = report.NewFileSink(cfg.ReportsDir())
	}
	p := pipeline.New(cfg, repo, sink, logger)

	switch {
	case *file != "":
		_, err = p.ProcessFile(ctx, *file)
	case *once:
		_, err = p.ProcessInbox(ctx, false)
	default:
		logger.Info("watching inbox", "inbox", cfg.InboxDir(), "pollSeconds", cfg.PollSeconds, "profile", cfg.DBProfile)
		err = p.Watch(ctx)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Info("stopped")
			return 0
		}
		// A failed file has been logged with its cause by the pipeline and sits in failed/.
		logger.Error(err.Error())
		return 1
	}
	return 0
}
