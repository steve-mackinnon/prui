package inventory

import (
	"context"

	"prui/internal/source"
)

type blobBatchReader interface {
	BlobBatch(context.Context, []string, int) map[string]source.BlobResult
}

func batchBlobs(ctx context.Context, objects Objects, files []FileChange, empty string, limit int) map[string]source.BlobResult {
	reader, ok := objects.(blobBatchReader)
	if !ok {
		return nil
	}
	var ids []string
	for _, f := range files {
		// Gitlinks need their special per-side handling in the inventory loop.
		if f.OldOID == f.NewOID || f.OldMode == "160000" || f.NewMode == "160000" {
			continue
		}
		old, next := f.OldOID, f.NewOID
		if f.OldMode == "000000" {
			old = empty
		}
		if f.NewMode == "000000" {
			next = empty
		}
		ids = append(ids, old, next)
	}
	return reader.BlobBatch(ctx, ids, limit)
}
