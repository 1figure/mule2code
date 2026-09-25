package batch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"testing"
)

// ints yields 0..n-1 as the job input.
func ints(n int) iter.Seq2[int, error] {
	return func(yield func(int, error) bool) {
		for i := 0; i < n; i++ {
			if !yield(i, nil) {
				return
			}
		}
	}
}

// failOdd is a processor that fails records with an odd payload.
func failOdd(_ context.Context, r *Record[int]) error {
	if r.Payload%2 == 1 {
		return fmt.Errorf("odd %d", r.Payload)
	}
	return nil
}

// payloads returns the payloads of a block.
func payloads(block []*Record[int]) []int {
	out := make([]int, len(block))
	for i, r := range block {
		out[i] = r.Payload
	}
	return out
}

// collector is an aggregator that remembers the blocks it received.
type collector struct{ blocks [][]int }

func (c *collector) aggregate(_ context.Context, block []*Record[int]) error {
	c.blocks = append(c.blocks, payloads(block))
	return nil
}

func run(t *testing.T, job *Job[int], n int) Statistics {
	t.Helper()
	if job.BlockSize == 0 {
		job.BlockSize = 100
	}
	if job.MaxFailedRecords == 0 {
		job.MaxFailedRecords = -1
	}
	stats, err := job.Run(context.Background(), ints(n))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return stats
}

func TestStepsRunInOrder_EachFinishesBeforeTheNext(t *testing.T) {
	var log []string
	step := func(name string) Step[int] {
		return Step[int]{
			Name: name,
			Processors: []Processor[int]{func(_ context.Context, r *Record[int]) error {
				log = append(log, fmt.Sprintf("%s:p%d", name, r.Payload))
				return nil
			}},
			Aggregator: func(_ context.Context, block []*Record[int]) error {
				log = append(log, fmt.Sprintf("%s:agg%v", name, payloads(block)))
				return nil
			},
		}
	}
	run(t, &Job[int]{Name: "j", Steps: []Step[int]{step("s1"), step("s2")}}, 3)
	want := "s1:p0 s1:p1 s1:p2 s1:agg[0 1 2] s2:p0 s2:p1 s2:p2 s2:agg[0 1 2]"
	if got := strings.Join(log, " "); got != want {
		t.Errorf("order:\n got %s\nwant %s", got, want)
	}
}

func TestAcceptPolicies(t *testing.T) {
	var noFailures, onlyFailures, all collector
	job := &Job[int]{Name: "j", Steps: []Step[int]{
		{Name: "fail-odd", Processors: []Processor[int]{failOdd}},
		{Name: "no-failures", Accept: NoFailures, Aggregator: noFailures.aggregate},
		{Name: "only-failures", Accept: OnlyFailures, Aggregator: onlyFailures.aggregate},
		{Name: "all", Accept: All, Aggregator: all.aggregate},
	}}
	stats := run(t, job, 6)
	if got := noFailures.blocks; !slices.Equal(got[0], []int{0, 2, 4}) {
		t.Errorf("NO_FAILURES saw %v", got)
	}
	if got := onlyFailures.blocks; !slices.Equal(got[0], []int{1, 3, 5}) {
		t.Errorf("ONLY_FAILURES saw %v", got)
	}
	if got := all.blocks; !slices.Equal(got[0], []int{0, 1, 2, 3, 4, 5}) {
		t.Errorf("ALL saw %v", got)
	}
	if stats.FailedRecords != 3 || stats.SuccessfulRecords != 3 {
		t.Errorf("stats = %+v", stats)
	}
	for _, p := range []AcceptPolicy{NoFailures, OnlyFailures, All, AcceptPolicy(9)} {
		if p.String() == "" {
			t.Errorf("empty String() for %d", int(p))
		}
	}
}

func TestAggregatorBlockSize(t *testing.T) {
	var sized, streaming collector
	job := &Job[int]{Name: "j", Steps: []Step[int]{
		{Name: "sized", AggregatorSize: 3, Aggregator: sized.aggregate},
		{Name: "streaming", AggregatorSize: 0, Aggregator: streaming.aggregate},
	}}
	run(t, job, 10)
	want := [][]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {9}}
	if !slices.EqualFunc(sized.blocks, want, slices.Equal) {
		t.Errorf("size=3 blocks = %v, want %v", sized.blocks, want)
	}
	if len(streaming.blocks) != 1 || len(streaming.blocks[0]) != 10 {
		t.Errorf("size=0 must receive everything in one block, got %v", streaming.blocks)
	}
	// No records: the aggregator is not called at all.
	var empty collector
	run(t, &Job[int]{Name: "j", Steps: []Step[int]{{Name: "s", Aggregator: empty.aggregate}}}, 0)
	if len(empty.blocks) != 0 {
		t.Errorf("aggregator called on an empty input: %v", empty.blocks)
	}
}

func TestFailingAggregatorFailsExactlyItsBlock(t *testing.T) {
	boom := errors.New("insert failed")
	var next collector
	job := &Job[int]{Name: "j", Steps: []Step[int]{
		{Name: "insert", AggregatorSize: 3, Aggregator: func(_ context.Context, block []*Record[int]) error {
			if slices.Contains(payloads(block), 4) {
				return boom
			}
			return nil
		}},
		{Name: "failed", Accept: OnlyFailures, Aggregator: next.aggregate},
	}}
	stats := run(t, job, 10)
	if !slices.Equal(next.blocks[0], []int{3, 4, 5}) {
		t.Errorf("failed records = %v, want the block [3 4 5]", next.blocks)
	}
	if stats.FailedRecords != 3 || stats.SuccessfulRecords != 7 {
		t.Errorf("stats = %+v", stats)
	}
	// Every record of the block carries the aggregator's error as its first exception.
	for _, r := range next.blocks {
		_ = r
	}
	var seen []error
	job.Steps[1].Aggregator = func(_ context.Context, block []*Record[int]) error {
		for _, r := range block {
			seen = append(seen, r.Err)
		}
		return nil
	}
	run(t, job, 10)
	for _, err := range seen {
		if !errors.Is(err, boom) {
			t.Errorf("record error = %v, want %v", err, boom)
		}
	}
}

func TestProcessorFailureNeverReachesTheAggregator(t *testing.T) {
	var agg collector
	var order []string
	job := &Job[int]{Name: "j", Steps: []Step[int]{{
		Name: "s",
		Processors: []Processor[int]{
			func(_ context.Context, r *Record[int]) error {
				order = append(order, fmt.Sprintf("p1:%d", r.Payload))
				if r.Payload == 5 {
					return errors.New("bad five")
				}
				return nil
			},
			func(_ context.Context, r *Record[int]) error {
				order = append(order, fmt.Sprintf("p2:%d", r.Payload))
				return nil
			},
		},
		Aggregator: agg.aggregate,
	}}}
	stats := run(t, job, 8)
	if slices.Contains(agg.blocks[0], 5) {
		t.Errorf("failed record reached the aggregator: %v", agg.blocks)
	}
	if slices.Contains(order, "p2:5") {
		t.Error("a later processor ran on a record an earlier one had failed")
	}
	if stats.FailedRecords != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestMaxFailedRecords(t *testing.T) {
	failAll := func(_ context.Context, r *Record[int]) error { return fmt.Errorf("no %d", r.Payload) }

	t.Run("stops the job once exceeded", func(t *testing.T) {
		var processed int
		var agg, next collector
		job := &Job[int]{Name: "j", MaxFailedRecords: 2, BlockSize: 100, Steps: []Step[int]{
			{Name: "s1", Processors: []Processor[int]{func(ctx context.Context, r *Record[int]) error {
				processed++
				return failAll(ctx, r)
			}}, Aggregator: agg.aggregate},
			{Name: "s2", Accept: All, Aggregator: next.aggregate},
		}}
		stats, err := job.Run(context.Background(), ints(10))
		if err != nil {
			t.Fatal(err)
		}
		if processed != 3 {
			t.Errorf("processed %d records before stopping, want 3 (max 2 + the one that exceeds it)", processed)
		}
		if len(agg.blocks) != 0 || len(next.blocks) != 0 {
			t.Errorf("aggregator / later step must not run after the job stopped: %v %v", agg.blocks, next.blocks)
		}
		if stats.FailedRecords != 3 || stats.SuccessfulRecords != 7 || stats.TotalRecords != 10 {
			t.Errorf("stats = %+v", stats)
		}
	})

	t.Run("-1 never stops", func(t *testing.T) {
		var next collector
		job := &Job[int]{Name: "j", MaxFailedRecords: -1, BlockSize: 100, Steps: []Step[int]{
			{Name: "s1", Processors: []Processor[int]{failAll}},
			{Name: "s2", Accept: OnlyFailures, Aggregator: next.aggregate},
		}}
		stats, err := job.Run(context.Background(), ints(10))
		if err != nil {
			t.Fatal(err)
		}
		if len(next.blocks) != 1 || len(next.blocks[0]) != 10 || stats.FailedRecords != 10 {
			t.Errorf("all 10 failures must be processed: %v, %+v", next.blocks, stats)
		}
	})

	t.Run("an aggregator failure counts too", func(t *testing.T) {
		var next collector
		job := &Job[int]{Name: "j", MaxFailedRecords: 2, BlockSize: 100, Steps: []Step[int]{
			{Name: "s1", AggregatorSize: 3, Aggregator: func(context.Context, []*Record[int]) error { return errors.New("x") }},
			{Name: "s2", Accept: All, Aggregator: next.aggregate},
		}}
		stats, _ := job.Run(context.Background(), ints(10))
		if stats.FailedRecords != 3 || len(next.blocks) != 0 {
			t.Errorf("the first failing block (3 > 2) must stop the job: %+v %v", stats, next.blocks)
		}
	})

	t.Run("exactly max failures does not stop", func(t *testing.T) {
		var next collector
		job := &Job[int]{Name: "j", MaxFailedRecords: 3, BlockSize: 100, Steps: []Step[int]{
			{Name: "s1", Processors: []Processor[int]{failOdd}},
			{Name: "s2", Accept: All, Aggregator: next.aggregate},
		}}
		stats, _ := job.Run(context.Background(), ints(6))
		if stats.FailedRecords != 3 || len(next.blocks) != 1 {
			t.Errorf("3 failures with max 3 must not stop the job: %+v %v", stats, next.blocks)
		}
	})
}

func TestStatisticsAndOnComplete(t *testing.T) {
	var received *Statistics
	job := &Job[int]{Name: "j", BlockSize: 4, MaxFailedRecords: -1,
		Steps: []Step[int]{{Name: "s", Processors: []Processor[int]{failOdd}}},
		OnComplete: func(_ context.Context, s Statistics) error {
			received = &s
			return nil
		},
	}
	stats, err := job.Run(context.Background(), ints(10))
	if err != nil {
		t.Fatal(err)
	}
	if received == nil || *received != stats {
		t.Errorf("OnComplete got %+v, Run returned %+v", received, stats)
	}
	if stats.BatchJobInstanceID == "" || len(stats.BatchJobInstanceID) != 36 {
		t.Errorf("batchJobInstanceId = %q", stats.BatchJobInstanceID)
	}
	if stats.TotalRecords != 10 || stats.LoadedRecords != 10 || stats.ProcessedRecords != 10 ||
		stats.SuccessfulRecords != 5 || stats.FailedRecords != 5 || stats.FailedOnInputPhase || stats.ElapsedTimeInMillis < 0 {
		t.Errorf("stats = %+v", stats)
	}
	other, _ := job.Run(context.Background(), ints(1))
	if other.BatchJobInstanceID == stats.BatchJobInstanceID {
		t.Error("each run must get its own instance id")
	}

	// A failing on-complete does not fail the job: it is recorded on the statistics, as in Mule.
	job.OnComplete = func(context.Context, Statistics) error { return errors.New("mail down") }
	stats, err = job.Run(context.Background(), ints(2))
	if err != nil || !stats.FailedOnCompletePhase || stats.TotalRecords != 2 || stats.FailedRecords != 1 {
		t.Errorf("got %v, %+v", err, stats)
	}
	if data, _ := json.Marshal(stats); !strings.Contains(string(data), `"failedOnCompletePhase":true`) {
		t.Errorf("statistics JSON = %s", data)
	}

	// Unless the failure is the context's: that still ends the run with ctx.Err().
	ctx, cancel := context.WithCancel(context.Background())
	job.OnComplete = func(context.Context, Statistics) error { cancel(); return ctx.Err() }
	if _, err := job.Run(ctx, ints(2)); !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

func TestInputPhaseError(t *testing.T) {
	broken := func(yield func(int, error) bool) {
		if !yield(1, nil) {
			return
		}
		yield(0, errors.New("bad row"))
	}
	var stepRan bool
	var completed bool
	job := &Job[int]{Name: "j", BlockSize: 10, MaxFailedRecords: -1,
		Steps:      []Step[int]{{Name: "s", Processors: []Processor[int]{func(context.Context, *Record[int]) error { stepRan = true; return nil }}}},
		OnComplete: func(context.Context, Statistics) error { completed = true; return nil },
	}
	stats, err := job.Run(context.Background(), broken)
	var inputErr *InputError
	if !errors.As(err, &inputErr) || inputErr.Job != "j" || inputErr.Err.Error() != "bad row" {
		t.Fatalf("got %v, want *InputError", err)
	}
	if !strings.Contains(err.Error(), "bad row") {
		t.Errorf("message = %q", err)
	}
	if stepRan || completed {
		t.Error("no step and no on-complete may run after an input-phase failure")
	}
	if !stats.FailedOnInputPhase || stats.TotalRecords != 0 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestCancellation(t *testing.T) {
	t.Run("from a processor", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var completed bool
		var agg collector
		job := &Job[int]{Name: "j", BlockSize: 10, MaxFailedRecords: -1,
			Steps: []Step[int]{{Name: "s", Processors: []Processor[int]{func(ctx context.Context, r *Record[int]) error {
				if r.Payload == 2 {
					cancel()
					return ctx.Err()
				}
				return nil
			}}, Aggregator: agg.aggregate}},
			OnComplete: func(context.Context, Statistics) error { completed = true; return nil },
		}
		_, err := job.Run(ctx, ints(10))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
		if completed || len(agg.blocks) != 0 {
			t.Error("nothing may run after cancellation")
		}
	})
	t.Run("from an aggregator", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		job := &Job[int]{Name: "j", BlockSize: 10, MaxFailedRecords: -1,
			Steps: []Step[int]{{Name: "s", Aggregator: func(context.Context, []*Record[int]) error {
				cancel()
				return errors.New("interrupted")
			}}},
		}
		if _, err := job.Run(ctx, ints(3)); !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want context.Canceled (not a record failure)", err)
		}
	})
	t.Run("before loading", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		job := &Job[int]{Name: "j", BlockSize: 10, MaxFailedRecords: -1}
		_, err := job.Run(ctx, ints(3))
		var inputErr *InputError
		if !errors.Is(err, context.Canceled) || errors.As(err, &inputErr) {
			t.Errorf("got %v, want a plain context.Canceled", err)
		}
	})
	t.Run("between steps", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var second bool
		job := &Job[int]{Name: "j", BlockSize: 10, MaxFailedRecords: -1, Steps: []Step[int]{
			{Name: "s1", Aggregator: func(context.Context, []*Record[int]) error { cancel(); return nil }},
			{Name: "s2", Processors: []Processor[int]{func(context.Context, *Record[int]) error { second = true; return nil }}},
		}}
		if _, err := job.Run(ctx, ints(3)); !errors.Is(err, context.Canceled) || second {
			t.Errorf("got %v, second step ran: %v", err, second)
		}
	})
}

func TestInvalidBlockSizeAndRecordFail(t *testing.T) {
	job := &Job[int]{Name: "j", BlockSize: 0}
	if _, err := job.Run(context.Background(), ints(1)); err == nil {
		t.Error("block size 0 must be rejected")
	}
	r := &Record[int]{Payload: 1}
	first, second := errors.New("first"), errors.New("second")
	r.Fail(first)
	r.Fail(second)
	if !r.Failed() || r.Err != first {
		t.Errorf("Fail must keep the first error, got %v", r.Err)
	}
}
