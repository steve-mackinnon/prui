package context

import (
	"context"

	"prui/internal/source"
)

type evidenceBatchReader interface {
	BlobBatch(context.Context, []string, int) map[string]source.BlobResult
}

const evidenceWindow = 8
const evidenceFailedIDs = 2048

// Keep one bounded window of bytes, and a bounded set of failed batch IDs.
// Stop batching before that set fills; returned failures never fall through to
// an ordinary Blob retry. Unread IDs retain the ordinary serial path.
type evidenceReads struct {
	objects      Objects
	batch        evidenceBatchReader
	limit, until int
	prepared     map[string]source.BlobResult
	failed       map[string]bool
}

func newEvidenceReads(objects Objects, limit int) evidenceReads {
	batch, _ := objects.(evidenceBatchReader)
	return evidenceReads{objects: objects, batch: batch, limit: limit}
}

func (r *evidenceReads) beginTree() { r.until = 0; r.prepared = nil }

func (r *evidenceReads) read(ctx context.Context, index int, cs []candidate, files, bytes int, eligible func(candidate) bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	oid := cs[index].oid
	if r.failed[oid] {
		return nil, source.ErrCommand
	}
	if index >= r.until {
		r.until = index + 1
		r.prepared = nil
		if r.batch != nil && r.limit > 0 && files > 1 && bytes > 0 && len(r.failed) < evidenceFailedIDs-1 {
			r.prepare(ctx, index, cs, files, bytes, eligible)
		}
	}
	if r.failed[oid] {
		return nil, source.ErrCommand
	}
	if result, ok := r.prepared[oid]; ok {
		return result.Data, result.Err
	}
	return r.objects.Blob(ctx, oid, r.limit)
}

func (r *evidenceReads) prepare(ctx context.Context, index int, cs []candidate, files, bytes int, eligible func(candidate) bool) {
	// Every potential success reserves one file and its maximum excerpt size.
	// A final partial excerpt uses the serial path, so no speculative read can
	// extend past an earlier file or byte-budget consumer.
	count := min(evidenceWindow, files, max(1, bytes/r.limit), evidenceFailedIDs-len(r.failed))
	selected := 0
	seen := map[string]bool{}
	var ids []string
	for i := index; i < len(cs) && selected < count; i++ {
		r.until = i + 1
		c := cs[i]
		if r.failed[c.oid] || !eligible(c) {
			continue
		}
		selected++ // Repeated OIDs still consume separate possible excerpts.
		if !seen[c.oid] {
			seen[c.oid] = true
			ids = append(ids, c.oid)
		}
	}
	if len(ids) < 2 {
		return
	}
	results := r.batch.BlobBatch(ctx, ids, r.limit)
	r.prepared = make(map[string]source.BlobResult, len(ids))
	for _, id := range ids {
		result, ok := results[id]
		if !ok {
			continue
		}
		if result.Err != nil || len(result.Data) > r.limit {
			if r.failed == nil {
				r.failed = map[string]bool{}
			}
			r.failed[id] = true
			continue
		}
		r.prepared[id] = result
	}
}
