// Package pipeline is contacts-batch-process-flow and the flows it calls: it watches the inbox,
// runs file-processing-job on each CSV file, writes the errors file, sends the report and moves
// the file to processed/ or failed/. The SFTP directories became local ones (MAPPING.md
// deviation 1) and the job runs inline (deviation 3).
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"muletocode/internal/batch"
	"muletocode/internal/config"
	"muletocode/internal/contacts"
	"muletocode/internal/report"
)

// Repository is the target of the bulk inserts; *db.Repository implements it.
type Repository interface {
	// BulkInsert is <db:bulk-insert doc:name="Contact Data">.
	BulkInsert(ctx context.Context, rows []contacts.Contact) error
}

// Pipeline processes CSV files from the inbox through the batch job.
type Pipeline struct {
	cfg  config.Config
	repo Repository
	sink report.Sink
	log  *slog.Logger
	// Now is the source of now() for vars.startTime and the report; tests may replace it.
	Now func() time.Time
}

// New creates the pipeline over an opened repository and a report sink. A nil logger discards the
// flow's TRACE loggers, which are otherwise logged at debug level.
func New(cfg config.Config, repo Repository, sink report.Sink, logger *slog.Logger) *Pipeline {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Pipeline{cfg: cfg, repo: repo, sink: sink, log: logger, Now: time.Now}
}

// Watch is <sftp:listener doc:name="On New or Updated File"> with its fixed-frequency scheduler: it polls inbox/ every
// PollSeconds until ctx is done and returns ctx.Err(). A file that fails is moved to failed/ and
// the watcher keeps going.
func (p *Pipeline) Watch(ctx context.Context) error {
	interval := time.Duration(max(1, p.cfg.PollSeconds)) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := p.ProcessInbox(ctx, true); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// ProcessInbox is one pass of the listener: it processes every *.csv in inbox/ in name order and
// returns the statistics per file. Other files are ignored. With continueOnError a failing file
// is skipped (it has been logged and moved to failed/), otherwise the pass stops with a *FileError.
func (p *Pipeline) ProcessInbox(ctx context.Context, continueOnError bool) ([]batch.Statistics, error) {
	inbox := p.cfg.InboxDir()
	if err := os.MkdirAll(inbox, 0o755); err != nil {
		return nil, fmt.Errorf("creating inbox %s: %w", inbox, err)
	}
	entries, err := os.ReadDir(inbox)
	if err != nil {
		return nil, fmt.Errorf("listing inbox %s: %w", inbox, err)
	}
	var results []batch.Statistics
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".csv") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return results, err
		}
		stats, err := p.ProcessFile(ctx, filepath.Join(inbox, entry.Name()))
		if err != nil {
			var fileErr *FileError
			if continueOnError && errors.As(err, &fileErr) {
				continue
			}
			return results, err
		}
		results = append(results, stats)
	}
	return results, nil
}

// ProcessFile is contacts-batch-process-flow for one file: initialization, CSV to records, the
// batch job, then the file is moved to processed/<newFilename>. On any error the file goes to
// failed/ instead and a *FileError carries the cause; a cancelled ctx leaves the file in place.
func (p *Pipeline) ProcessFile(ctx context.Context, path string) (batch.Statistics, error) {
	p.log.Debug("Flow starting")
	p.log.Debug("Batch configurations", "blockSize", p.cfg.BatchBlockSize, "mainAggregatorSize", p.cfg.BatchAggregatorSize)

	// <flow-ref name="initialization-flow">
	vars := initialization(filepath.Base(path), p.Now())
	stats, err := p.processFile(ctx, path, vars)
	if err != nil {
		if ctx.Err() != nil {
			return stats, err
		}
		// <error-handler><on-error-propagate type="ANY">
		p.log.Error("Failed processing file", "currentFilename", vars.currentFilename, "error", err)
		p.moveToFailed(path, vars.currentFilename)
		return stats, &FileError{Filename: vars.currentFilename, Err: err}
	}
	return stats, nil
}

func (p *Pipeline) processFile(ctx context.Context, path string, vars runVars) (batch.Statistics, error) {
	if err := os.MkdirAll(p.cfg.ProcessedDir(), 0o755); err != nil {
		return batch.Statistics{}, fmt.Errorf("creating processed directory: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return batch.Statistics{}, fmt.Errorf("opening input file: %w", err)
	}
	p.log.Debug("Transforming input data to Java")
	rows := contacts.ReadRows(f)
	p.log.Debug("Staging Batch Job")
	stats, err := p.buildJob(vars).Run(ctx, rows)
	closeErr := f.Close()
	if err != nil {
		return stats, err
	}
	if closeErr != nil {
		return stats, fmt.Errorf("closing input file: %w", closeErr)
	}
	p.log.Debug("Batch job successfully started")

	// <sftp:listener autoDelete="true" moveToDirectory=processed renameTo=#[vars.newFilename]>,
	// after the job instead of concurrently with it (deviation 3).
	target := filepath.Join(p.cfg.ProcessedDir(), vars.newFilename)
	if err := os.Rename(path, target); err != nil {
		return stats, fmt.Errorf("moving input file to %s: %w", target, err)
	}
	p.log.Debug("Flow ending")
	return stats, nil
}

// buildJob is <batch:job jobName="file-processing-job" maxFailedRecords="-1" blockSize="${batch.job.block_size}">.
func (p *Pipeline) buildJob(vars runVars) *batch.Job[contacts.Row] {
	return &batch.Job[contacts.Row]{
		Name:             "file-processing-job",
		MaxFailedRecords: -1,
		BlockSize:        p.cfg.BatchBlockSize,
		Logger:           p.log,
		Steps: []batch.Step[contacts.Row]{
			// <batch:step name="main-processing-step">
			{
				Name:   "main-processing-step",
				Accept: batch.NoFailures,
				Processors: []batch.Processor[contacts.Row]{
					// <validation:is-email doc:name="Is email" email="#[payload.email]" message="Missing or invalid email">
					func(_ context.Context, r *batch.Record[contacts.Row]) error {
						return contacts.ValidateEmail(r.Payload)
					},
				},
				// <batch:aggregator doc:name="main-records-aggregator" size="${batch.aggregator.main.size}">
				AggregatorSize: p.cfg.BatchAggregatorSize,
				Aggregator:     p.insertBlock,
			},
			// <batch:step name="failed-records-processing-step" acceptPolicy="ONLY_FAILURES">
			{
				Name:   "failed-records-processing-step",
				Accept: batch.OnlyFailures,
				// <batch:aggregator doc:name="failed-records-aggregator" streaming="true">
				AggregatorSize: 0,
				Aggregator: func(ctx context.Context, block []*batch.Record[contacts.Row]) error {
					return p.writeErrorsFile(ctx, vars, block)
				},
			},
		},
		// <batch:on-complete>. In Mule this phase runs after contacts-batch-process-flow has ended,
		// so a failure here never reaches the flow's <on-error-propagate> and never moves the file:
		// the job records it as failedOnCompletePhase (MAPPING.md deviation 3).
		OnComplete: func(ctx context.Context, stats batch.Statistics) error {
			if data, err := json.Marshal(stats); err == nil {
				p.log.Debug("Batch job statistics", "statistics", string(data))
			}
			if err := p.sendReport(ctx, vars, stats); err != nil {
				return err
			}
			p.log.Debug("Batch job completed")
			return nil
		},
	}
}

// insertBlock is the body of main-records-aggregator: <ee:transform doc:name="CSV to SQL"> for each
// record of the block, then <db:bulk-insert doc:name="Contact Data">. A coercion error is returned as
// is, so its message reaches the errors file unchanged.
func (p *Pipeline) insertBlock(ctx context.Context, block []*batch.Record[contacts.Row]) error {
	rows := make([]contacts.Contact, 0, len(block))
	for _, r := range block {
		c, err := contacts.FromRow(r.Payload, p.cfg.TimestampPrecision)
		if err != nil {
			return err
		}
		rows = append(rows, c)
	}
	return p.repo.BulkInsert(ctx, rows)
}

// writeErrorsFile is the body of failed-records-aggregator: <ee:transform doc:name="Create Error Record">
// per record (a processor in the XML; folded into the aggregator here so the step's payload type
// stays the CSV row), then <sftp:write doc:name="Error Records" path='#[p("sftp.processed_dir") ++ vars.errorsFilename]'> as JSON.
func (p *Pipeline) writeErrorsFile(_ context.Context, vars runVars, block []*batch.Record[contacts.Row]) error {
	p.log.Debug("Writing errors/failed records to file")
	records := make([]ErrorRecord, 0, len(block))
	for _, r := range block {
		records = append(records, NewErrorRecord(r))
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(records); err != nil {
		return fmt.Errorf("encoding errors file: %w", err)
	}
	path := filepath.Join(p.cfg.ProcessedDir(), vars.errorsFilename)
	if err := os.WriteFile(path, bytes.TrimRight(buf.Bytes(), "\n"), 0o644); err != nil {
		return fmt.Errorf("writing errors file: %w", err)
	}
	return nil
}

// sendReport is <flow name="send-email-report-flow">.
func (p *Pipeline) sendReport(ctx context.Context, vars runVars, stats batch.Statistics) error {
	p.log.Debug("Sending contacts batch report email")
	now := p.Now()
	items := report.KeyStatistics(vars.currentFilename, now.Sub(vars.startTime), stats)
	html, err := report.Render(items, now)
	if err != nil {
		return err
	}
	if err := p.sink.Send(ctx, report.Subject, html); err != nil {
		return fmt.Errorf("sending report: %w", err)
	}
	p.log.Debug("Contacts batch report email sent successfully")
	return nil
}

// moveToFailed is <sftp:move doc:name="File to Failed Directory" sourcePath='#[p("sftp.new_dir") ++ vars.currentFilename]' targetPath='#[p("sftp.failed_dir")]'>.
func (p *Pipeline) moveToFailed(path, filename string) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	failed := p.cfg.FailedDir()
	if err := os.MkdirAll(failed, 0o755); err != nil {
		p.log.Error("Could not create failed directory", "failedDir", failed, "error", err)
		return
	}
	if err := os.Rename(path, filepath.Join(failed, filename)); err != nil {
		p.log.Error("Could not move file to failed directory", "currentFilename", filename, "failedDir", failed, "error", err)
	}
}
