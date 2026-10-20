package source

import (
	"context"
	"sync"
	"sync/atomic"
)

type BlobDiffRequest struct {
	OldOID, NewOID string
	Limit          int
}

// BlobDiffBatch runs canonical pinned-blob diffs with at most eight workers.
// Results retain request order. Reserved output caps may not exceed the view's
// content budget in total; failures remain explicit and are never retried.
func (v *View) BlobDiffBatch(ctx context.Context, requests []BlobDiffRequest) []BlobResult {
	results := make([]BlobResult, len(requests))
	remaining := v.limits.ContentBytes
	var jobs []int
	for i, request := range requests {
		switch {
		case !shaPattern.MatchString(request.OldOID) || !shaPattern.MatchString(request.NewOID):
			results[i].Err = ErrCommand
		case request.Limit <= 0 || request.Limit > remaining || len(jobs) >= v.limits.Entries:
			results[i].Err = ErrLimit
		default:
			remaining -= request.Limit
			jobs = append(jobs, i)
		}
	}
	parallelism := 1
	if runner, ok := v.runner.(interface{ MaxParallelism() int }); ok {
		parallelism = max(1, min(8, runner.MaxParallelism()))
	}
	var next atomic.Int64
	var workers sync.WaitGroup
	for worker := 0; worker < min(parallelism, len(jobs)); worker++ {
		workers.Go(func() {
			for {
				n := int(next.Add(1)) - 1
				if n >= len(jobs) {
					return
				}
				i := jobs[n]
				request := requests[i]
				if err := ctx.Err(); err != nil {
					results[i].Err = err
					continue
				}
				results[i].Data, results[i].Err = v.Git(ctx, request.Limit, BlobDiffArgs(request.OldOID, request.NewOID)...)
			}
		})
	}
	workers.Wait()
	return results
}
