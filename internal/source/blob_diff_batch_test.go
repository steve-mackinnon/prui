package source

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type parallelFixtureRunner struct {
	active, maximum atomic.Int32
	calls           atomic.Int32
	limit           int
}

func (r *parallelFixtureRunner) MaxParallelism() int { return r.limit }

func (r *parallelFixtureRunner) Run(ctx context.Context, q Request) ([]byte, error) {
	r.calls.Add(1)
	active := r.active.Add(1)
	defer r.active.Add(-1)
	for previous := r.maximum.Load(); active > previous; previous = r.maximum.Load() {
		if r.maximum.CompareAndSwap(previous, active) {
			break
		}
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}
	return []byte(q.Args[len(q.Args)-3]), nil
}

func TestBlobDiffBatchBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	runner := &parallelFixtureRunner{limit: 100}
	v := &View{runner: runner, limits: Defaults()}
	var requests []BlobDiffRequest
	for i := 0; i < 20; i++ {
		requests = append(requests, BlobDiffRequest{OldOID: fmt.Sprintf("%040x", i+1), NewOID: fmt.Sprintf("%040x", i+100), Limit: 100})
	}
	got := v.BlobDiffBatch(context.Background(), requests)
	for i, result := range got {
		if result.Err != nil || string(result.Data) != requests[i].OldOID {
			t.Fatalf("result %d lost origin: %q %v", i, result.Data, result.Err)
		}
	}
	if len(got) != len(requests) || runner.maximum.Load() <= 1 || runner.maximum.Load() > 8 || runner.active.Load() != 0 {
		t.Fatalf("unbounded or leaked workers: results=%d maximum=%d active=%d", len(got), runner.maximum.Load(), runner.active.Load())
	}
}

func TestBlobDiffBatchKeepsUnadvertisedRunnerSerial(t *testing.T) {
	runner := &parallelFixtureRunner{limit: 100}
	// Embedding only Runner deliberately hides the optional capability.
	v := &View{limits: Defaults(), runner: struct{ Runner }{runner}}
	requests := []BlobDiffRequest{
		{OldOID: fmt.Sprintf("%040x", 1), NewOID: fmt.Sprintf("%040x", 2), Limit: 100},
		{OldOID: fmt.Sprintf("%040x", 3), NewOID: fmt.Sprintf("%040x", 4), Limit: 100},
	}
	got := v.BlobDiffBatch(context.Background(), requests)
	if len(got) != 2 || got[0].Err != nil || got[1].Err != nil || runner.maximum.Load() != 1 {
		t.Fatal(got, runner.maximum.Load())
	}
}

func TestBlobDiffBatchRejectsUnreservedOutputAndInvalidObjects(t *testing.T) {
	runner := &parallelFixtureRunner{limit: 8}
	limits := Defaults()
	limits.ContentBytes = 100
	v := &View{limits: limits, runner: runner}
	requests := []BlobDiffRequest{
		{OldOID: fmt.Sprintf("%040x", 1), NewOID: fmt.Sprintf("%040x", 2), Limit: 60},
		{OldOID: fmt.Sprintf("%040x", 3), NewOID: fmt.Sprintf("%040x", 4), Limit: 60},
		{OldOID: "HEAD", NewOID: fmt.Sprintf("%040x", 5), Limit: 10},
	}
	got := v.BlobDiffBatch(context.Background(), requests)
	if len(got) != 3 || got[0].Err != nil || !errors.Is(got[1].Err, ErrLimit) || !errors.Is(got[2].Err, ErrCommand) || runner.calls.Load() != 1 {
		t.Fatal("unreserved or invalid job ran", got, runner.calls.Load())
	}
}

type canceledDiffRunner struct {
	started chan struct{}
	once    sync.Once
	active  atomic.Int32
}

func (*canceledDiffRunner) MaxParallelism() int { return 4 }

func (r *canceledDiffRunner) Run(ctx context.Context, _ Request) ([]byte, error) {
	r.active.Add(1)
	defer r.active.Add(-1)
	r.once.Do(func() { close(r.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestBlobDiffBatchCancellationJoinsWorkers(t *testing.T) {
	runner := &canceledDiffRunner{started: make(chan struct{})}
	v := &View{limits: Defaults(), runner: runner}
	var requests []BlobDiffRequest
	for i := 0; i < 20; i++ {
		requests = append(requests, BlobDiffRequest{OldOID: fmt.Sprintf("%040x", i+1), NewOID: fmt.Sprintf("%040x", i+100), Limit: 100})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan []BlobResult, 1)
	go func() { done <- v.BlobDiffBatch(ctx, requests) }()
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("diff workers did not start")
	}
	cancel()
	select {
	case results := <-done:
		for _, result := range results {
			if result.Data != nil || !errors.Is(result.Err, context.Canceled) {
				t.Fatal("cancellation returned partial or empty-success data", result)
			}
		}
		if runner.active.Load() != 0 {
			t.Fatal("batch returned before workers stopped", runner.active.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("diff workers did not stop on cancellation")
	}
}
