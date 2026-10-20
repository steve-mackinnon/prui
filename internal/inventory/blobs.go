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
		pair := fileBlobPair(f, empty)
		ids = append(ids, pair.old, pair.next)
	}
	return reader.BlobBatch(ctx, ids, limit)
}
