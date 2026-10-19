package inventory

import (
	"context"

	"prui/internal/source"
)

// Each window reserves worst-case output before reading. Refill only after the
// earlier window has been consumed in file order, using the actual remaining
// materialization budget. Retain prepared results to prevent failure retries;
// their raw bytes have a separate cap even if malformed patches consume none
// of the final inventory budget.
type diffReads struct {
	objects               Objects
	files                 []FileChange
	blobs                 map[string]source.BlobResult
	empty, oldSHA, newSHA string
	limits                source.Limits
	changedPaths          []string
	prepared              map[blobPair]source.BlobResult
	through, retained     int
}

func newDiffReads(objects Objects, files []FileChange, blobs map[string]source.BlobResult, empty, oldSHA, newSHA string, limits source.Limits) *diffReads {
	return &diffReads{objects: objects, files: files, blobs: blobs, empty: empty, oldSHA: oldSHA, newSHA: newSHA, limits: limits}
}

func (d *diffReads) read(ctx context.Context, index int, pair blobPair, byteLimit, lineLimit int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, known := d.prepared[pair]
	if !known && index >= d.through && d.retained < d.limits.ContentBytes {
		limits := d.limits
		limits.ContentBytes = min(byteLimit, d.limits.ContentBytes-d.retained)
		limits.DiffLines = lineLimit
		prepared, through := d.prepare(ctx, index, limits)
		d.through = through
		if len(prepared) > 0 && d.prepared == nil {
			d.prepared = map[blobPair]source.BlobResult{}
		}
		for pair, result := range prepared {
			if _, exists := d.prepared[pair]; !exists {
				// Do not retain a small subslice's potentially much larger backing
				// buffer or a buffer the backend could reuse on the next window.
				data := make([]byte, len(result.Data))
				copy(data, result.Data)
				result.Data = data
				d.prepared[pair] = result
				d.retained += len(result.Data)
			}
		}
		result, known = d.prepared[pair]
	}
	if known {
		if result.Err == nil && len(result.Data) > byteLimit {
			return nil, source.ErrLimit
		}
		return result.Data, result.Err
	}
	return d.objects.Git(ctx, byteLimit, source.BlobDiffArgs(pair.old, pair.next)...)
}
