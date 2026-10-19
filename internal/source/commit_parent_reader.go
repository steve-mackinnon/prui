package source

import (
	"context"
	"io"
	"slices"
	"sync"
)

type commitParentResult struct {
	parents []string
	err     error
}

// CommitParentReader consumes parent validations once, in requested order.
// Close cancels and joins speculative work; the caller continues to own the view.
type CommitParentReader struct {
	ctx          context.Context
	cancel       context.CancelFunc
	ids          []string
	next, window int
	err          error
	read         func(context.Context, string) ([]string, error)
	jobs         chan int
	results      []chan commitParentResult
	workers      sync.WaitGroup
}

// ReadCommitParents preserves every CommitParents check while reading at most
// four entries ahead. Reserve one runner slot for capture's foreground Git work.
// Serial/custom runners without parallel capability remain lazy and serial.
func (v *View) ReadCommitParents(ctx context.Context, head string, ids []string) *CommitParentReader {
	parallelism := 1
	if runner, ok := v.runner.(interface{ MaxParallelism() int }); ok {
		parallelism = max(1, min(8, runner.MaxParallelism()))
	}
	window := min(4, parallelism-1, len(ids))
	return newCommitParentReader(ctx, ids, window, func(c context.Context, id string) ([]string, error) { return v.CommitParents(c, id, head) })
}

func newCommitParentReader(parent context.Context, ids []string, window int, read func(context.Context, string) ([]string, error)) *CommitParentReader {
	ctx, cancel := context.WithCancel(parent)
	r := &CommitParentReader{ctx: ctx, cancel: cancel, read: read}
	if len(ids) > 100 {
		r.err = ErrLimit
		return r
	}
	r.ids = slices.Clone(ids)
	r.window = max(0, min(4, window, len(ids)))
	if r.window == 0 || ctx.Err() != nil {
		return r
	}
	r.jobs = make(chan int, r.window)
	r.results = make([]chan commitParentResult, len(ids))
	for i := range r.results {
		r.results[i] = make(chan commitParentResult, 1)
	}
	for i := range r.window {
		r.jobs <- i
	}
	for range r.window {
		r.workers.Go(func() {
			for {
				select {
				case <-ctx.Done():
					return
				case i := <-r.jobs:
					parents, err := read(ctx, r.ids[i])
					r.results[i] <- commitParentResult{parents, err}
				}
			}
		})
	}
	return r
}

// Next never retries a failed validation. Call it sequentially, once per entry.
func (r *CommitParentReader) Next() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.next >= len(r.ids) {
		return nil, io.EOF
	}
	i := r.next
	r.next++
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if r.window == 0 {
		return r.read(r.ctx, r.ids[i])
	}
	select {
	case <-r.ctx.Done():
		return nil, r.ctx.Err()
	case result := <-r.results[i]:
		if next := i + r.window; next < len(r.ids) {
			select {
			case r.jobs <- next:
			case <-r.ctx.Done():
				return nil, r.ctx.Err()
			}
		}
		return result.parents, result.err
	}
}

func (r *CommitParentReader) Close() { r.cancel(); r.workers.Wait() }
