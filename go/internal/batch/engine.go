// Package batch is a small generic reproduction of the Mule <batch:job>:
//
//   - the input is loaded in blocks of Job.BlockSize records;
//   - steps run in order, and a step finishes for every record before the next step starts;
//   - a step accepts records by its AcceptPolicy, runs its processors per record, then hands the
//     records that passed to its aggregator in blocks of Step.AggregatorSize (0 = all);
//   - a failing processor fails the record, a failing aggregator fails every record of its block;
//   - the job stops once more than Job.MaxFailedRecords records have failed (-1 = unlimited);
//   - Job.OnComplete receives the Statistics.
//
// Unlike Mule the job runs synchronously on the caller (MAPPING.md deviation 3).
package batch

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"time"
)

// AcceptPolicy is the acceptPolicy attribute of a <batch:step>.
type AcceptPolicy int

const (
	// NoFailures accepts only records that no previous step has failed (the default).
	NoFailures AcceptPolicy = iota
	// OnlyFailures accepts only records that a previous step has failed.
	OnlyFailures
	// All accepts every record.
	All
)

// String returns the Mule spelling of the policy.
func (p AcceptPolicy) String() string {
	switch p {
	case NoFailures:
		return "NO_FAILURES"
	case OnlyFailures:
		return "ONLY_FAILURES"
	case All:
		return "ALL"
	default:
		return fmt.Sprintf("AcceptPolicy(%d)", int(p))
	}
}

// Record is one record travelling through a Job. Like a Mule batch record it keeps its payload
// and, once it has failed, the first error that failed it (Batch::getFirstException()).
type Record[T any] struct {
	// Payload is the record; a processor may replace it (a Mule set-payload inside a step).
	Payload T
	// Index is the 0-based position in the input.
	Index int64
	// Err is the first error that failed the record, or nil while it is successful. Use Fail to set it.
	Err error
}

// Failed reports whether any step has failed the record.
func (r *Record[T]) Failed() bool { return r.Err != nil }

// Fail marks the record failed; only the first error is kept.
func (r *Record[T]) Fail(err error) {
	if r.Err == nil {
		r.Err = err
	}
}

// Processor is a per-record processor of a step; returning an error fails the record.
type Processor[T any] func(ctx context.Context, r *Record[T]) error

// Aggregator is the body of a <batch:aggregator>: it receives one block of records that passed
// the step's processors. Returning an error fails every record of the block.
type Aggregator[T any] func(ctx context.Context, block []*Record[T]) error

// Step is a <batch:step>: an accept policy, per-record processors and an optional aggregator.
type Step[T any] struct {
	// Name is the name attribute, for logs.
	Name string
	// Accept is the acceptPolicy attribute; the zero value is NoFailures.
	Accept AcceptPolicy
	// Processors run for each accepted record, in order, before the aggregator.
	Processors []Processor[T]
	// Aggregator is the aggregator body, or nil when the step has none.
	Aggregator Aggregator[T]
	// AggregatorSize is the aggregator's size attribute; 0 means streaming="true", all records of the step in one call.
	AggregatorSize int
}

// Accepts reports whether r enters the step under its accept policy.
func (s *Step[T]) Accepts(r *Record[T]) bool {
	switch s.Accept {
	case NoFailures:
		return !r.Failed()
	case OnlyFailures:
		return r.Failed()
	default:
		return true
	}
}

// Statistics is the payload of <batch:on-complete>. The field names follow the Mule
// BatchJobResult (see examples/batch-job-statistics-example.json); the *PhaseException fields
// are not reproduced (MAPPING.md deviation 8).
type Statistics struct {
	BatchJobInstanceID  string `json:"batchJobInstanceId"`
	TotalRecords        int64  `json:"totalRecords"`
	LoadedRecords       int64  `json:"loadedRecords"`
	ProcessedRecords    int64  `json:"processedRecords"`
	SuccessfulRecords   int64  `json:"successfulRecords"`
	FailedRecords       int64  `json:"failedRecords"`
	ElapsedTimeInMillis int64  `json:"elapsedTimeInMillis"`
	FailedOnInputPhase  bool   `json:"failedOnInputPhase"`
	// FailedOnCompletePhase reports that the <batch:on-complete> body failed; the job itself still completed.
	FailedOnCompletePhase bool `json:"failedOnCompletePhase"`
}

// InputError reports that reading the job input failed before any record was processed.
type InputError struct {
	Job string
	Err error
}

func (e *InputError) Error() string {
	return fmt.Sprintf("batch job %q failed reading its input: %v", e.Job, e.Err)
}

// Unwrap returns the input error.
func (e *InputError) Unwrap() error { return e.Err }

// Job is a <batch:job>.
type Job[T any] struct {
	// Name is the jobName attribute.
	Name string
	// MaxFailedRecords is the maxFailedRecords attribute; -1 means unlimited.
	MaxFailedRecords int
	// BlockSize is the blockSize attribute: how many records are pulled from the input at a time. Must be positive.
	BlockSize int
	// Steps are the <batch:process-records> steps, in order.
	Steps []Step[T]
	// OnComplete is the <batch:on-complete> body; it receives the statistics. Optional.
	OnComplete func(ctx context.Context, stats Statistics) error
	// Logger receives the job's trace messages at debug level; nil discards them.
	Logger *slog.Logger
}

// Run runs the job over input and returns the statistics (the same values OnComplete got).
// An error from input ends the job with an *InputError; a cancelled ctx ends it with ctx.Err();
// an error from OnComplete only sets Statistics.FailedOnCompletePhase.
func (j *Job[T]) Run(ctx context.Context, input iter.Seq2[T, error]) (Statistics, error) {
	if j.BlockSize <= 0 {
		return Statistics{}, fmt.Errorf("batch job %q: block size must be positive, got %d", j.Name, j.BlockSize)
	}
	logger := j.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	start := time.Now()
	stats := Statistics{BatchJobInstanceID: newInstanceID()}
	logger.Debug("batch job starting", "job", j.Name, "instance", stats.BatchJobInstanceID)

	// Input / loading phase. Mule enqueues records in blocks of blockSize; the blocks are then
	// processed step by step, but every step must see all records before the next starts, so
	// the whole input is materialised here.
	records, err := j.load(ctx, input, logger)
	if err != nil {
		stats.ElapsedTimeInMillis = time.Since(start).Milliseconds()
		if ctx.Err() != nil {
			return stats, fmt.Errorf("batch job %q: %w", j.Name, ctx.Err())
		}
		stats.FailedOnInputPhase = true
		logger.Error("batch job input phase failed", "job", j.Name, "error", err)
		return stats, &InputError{Job: j.Name, Err: err}
	}

	st := &runState{max: j.MaxFailedRecords}
	for i := range j.Steps {
		step := &j.Steps[i]
		if err := ctx.Err(); err != nil {
			return stats, fmt.Errorf("batch job %q: %w", j.Name, err)
		}
		logger.Debug("batch step starting", "step", step.Name, "accept", step.Accept)
		var passed []*Record[T]
		for _, r := range records {
			if !step.Accepts(r) {
				continue
			}
			ok, err := j.runProcessors(ctx, step, r, st, logger)
			if err != nil {
				return stats, fmt.Errorf("batch job %q: %w", j.Name, err)
			}
			if ok {
				passed = append(passed, r)
			} else if st.exceeds() {
				break
			}
		}
		if !st.exceeds() && step.Aggregator != nil {
			if err := j.runAggregator(ctx, step, passed, st, logger); err != nil {
				return stats, fmt.Errorf("batch job %q: %w", j.Name, err)
			}
		}
		if st.exceeds() {
			logger.Warn("batch job stopped: too many failed records", "job", j.Name, "maxFailedRecords", j.MaxFailedRecords)
			break
		}
	}

	stats.TotalRecords = int64(len(records))
	stats.LoadedRecords = int64(len(records))
	stats.ProcessedRecords = int64(len(records))
	stats.FailedRecords = st.failed
	stats.SuccessfulRecords = int64(len(records)) - st.failed
	stats.ElapsedTimeInMillis = time.Since(start).Milliseconds()

	// Like Mule, a failure in the on-complete phase does not fail the job or reach the calling
	// flow's error handler; it is recorded on the statistics (failedOnCompletePhase) and logged.
	if j.OnComplete != nil {
		if err := j.OnComplete(ctx, stats); err != nil {
			if ctx.Err() != nil {
				return stats, fmt.Errorf("batch job %q: %w", j.Name, ctx.Err())
			}
			logger.Error("batch job on-complete phase failed", "job", j.Name, "error", err)
			stats.FailedOnCompletePhase = true
		}
	}
	logger.Debug("batch job completed", "job", j.Name, "successful", stats.SuccessfulRecords, "failed", stats.FailedRecords)
	return stats, nil
}

func (j *Job[T]) load(ctx context.Context, input iter.Seq2[T, error], logger *slog.Logger) ([]*Record[T], error) {
	var records []*Record[T]
	for payload, err := range input {
		if err != nil {
			return nil, err
		}
		if len(records)%j.BlockSize == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			logger.Debug("batch job loading block", "job", j.Name, "firstRecord", len(records))
		}
		records = append(records, &Record[T]{Payload: payload, Index: int64(len(records))})
	}
	return records, nil
}

// runProcessors runs the step's processors on r; ok is false when one of them failed the record.
// The error is only ever the context's.
func (j *Job[T]) runProcessors(ctx context.Context, step *Step[T], r *Record[T], st *runState, logger *slog.Logger) (ok bool, err error) {
	for _, p := range step.Processors {
		if err := p(ctx, r); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			logger.Debug("batch step failed a record", "step", step.Name, "index", r.Index, "error", err)
			fail(st, r, err)
			return false, nil
		}
	}
	return true, nil
}

// runAggregator feeds the aggregator block by block; it stops early once maxFailedRecords is exceeded.
func (j *Job[T]) runAggregator(ctx context.Context, step *Step[T], passed []*Record[T], st *runState, logger *slog.Logger) error {
	// size="0" / streaming="true": the whole step's output in a single call.
	size := step.AggregatorSize
	if size <= 0 {
		size = max(len(passed), 1)
	}
	for offset := 0; offset < len(passed); offset += size {
		if err := ctx.Err(); err != nil {
			return err
		}
		block := passed[offset:min(offset+size, len(passed))]
		if err := step.Aggregator(ctx, block); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Warn("batch step aggregator failed a block", "step", step.Name, "records", len(block), "error", err)
			for _, r := range block {
				fail(st, r, err)
			}
			if st.exceeds() {
				return nil
			}
		}
	}
	return nil
}

// runState is the mutable state of one Run: the failure counter and the maxFailedRecords check.
type runState struct {
	failed int64
	max    int
}

func (s *runState) exceeds() bool {
	return s.max >= 0 && s.failed > int64(s.max)
}

// fail records the failure of r; a record already failed by an earlier step is not counted twice.
func fail[T any](s *runState, r *Record[T], err error) {
	if !r.Failed() {
		s.failed++
	}
	r.Fail(err)
}

// newInstanceID returns a random UUID (version 4) for batchJobInstanceId.
func newInstanceID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails since Go 1.24
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
