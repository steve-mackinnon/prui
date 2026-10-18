package inventory

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"prui/internal/source"
	"prui/internal/testutil"
)

// Hide the optional batch capability while retaining authoritative attributes.
type serialBlobObjects struct{ Objects }

func (o serialBlobObjects) Attributes(ctx context.Context, sha string, names []string, paths [][]byte) (map[string]map[string]string, error) {
	return o.Objects.(attributeObjects).Attributes(ctx, sha, names, paths)
}

type batchInventoryObjects struct {
	*source.View
	batches, reads int
	fail           bool
	diffBatches    int
	treeBatches    int
	diffFallbacks  int
	failDiffs      bool
}

func (o *batchInventoryObjects) TreeDiffBatch(ctx context.Context, old, next string, requests []source.TreeDiffRequest) map[int]source.BlobResult {
	o.treeBatches++
	if !o.failDiffs {
		return o.View.TreeDiffBatch(ctx, old, next, requests)
	}
	results := map[int]source.BlobResult{}
	for i := range requests {
		results[i] = source.BlobResult{Err: source.ErrLimit}
	}
	return results
}

func (o *batchInventoryObjects) Git(ctx context.Context, limit int, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "diff" {
		o.diffFallbacks++
	}
	return o.View.Git(ctx, limit, args...)
}

func (o *batchInventoryObjects) BlobDiffBatch(ctx context.Context, requests []source.BlobDiffRequest) []source.BlobResult {
	o.diffBatches++
	if !o.failDiffs {
		return o.View.BlobDiffBatch(ctx, requests)
	}
	results := make([]source.BlobResult, len(requests))
	for i := range results {
		results[i].Err = source.ErrLimit
	}
	return results
}

func (o *batchInventoryObjects) Blob(ctx context.Context, oid string, limit int) ([]byte, error) {
	o.reads++
	return o.View.Blob(ctx, oid, limit)
}

func (o *batchInventoryObjects) BlobBatch(ctx context.Context, ids []string, limit int) map[string]source.BlobResult {
	o.batches++
	if !o.fail {
		return o.View.BlobBatch(ctx, ids, limit)
	}
	results := map[string]source.BlobResult{}
	for _, id := range ids {
		results[id] = source.BlobResult{Err: source.ErrLimit}
	}
	return results
}

func TestBatchInventoryMatchesSerialSource(t *testing.T) {
	v, comparison, repo := fixture(t)
	want, err := Build(context.Background(), serialBlobObjects{v}, comparison, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	batched, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer batched.Close()
	objects := &batchInventoryObjects{View: batched}
	got, err := Build(context.Background(), objects, comparison, source.Defaults())
	if err != nil || !reflect.DeepEqual(got, want) || objects.batches != 1 || objects.diffBatches != 1 || objects.treeBatches != 1 {
		t.Fatalf("batch inventory differs from serial: err=%v batches=%d diff batches=%d tree batches=%d", err, objects.batches, objects.diffBatches, objects.treeBatches)
	}
}

func TestBatchInventoryDoesNotRetryResourceFailures(t *testing.T) {
	repo := testutil.NewRepo(t)
	for i := 0; i < 3; i++ {
		repo.Write(fmt.Sprintf("file%d.txt", i), fmt.Sprintf("old %d\n", i))
	}
	base := repo.Commit()
	for i := 0; i < 3; i++ {
		repo.Write(fmt.Sprintf("file%d.txt", i), fmt.Sprintf("new %d\n", i))
	}
	head := repo.Commit()
	comparison := source.PinnedComparison{MergeBaseSHA: base, Metadata: source.Metadata{BaseSHA: base, HeadSHA: head}}
	for _, phase := range []string{"blobs", "diffs"} {
		t.Run(phase, func(t *testing.T) {
			v, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), source.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			o := &batchInventoryObjects{View: v, fail: phase == "blobs", failDiffs: phase == "diffs"}
			got, err := Build(context.Background(), o, comparison, source.Defaults())
			if err != nil || got.Complete || len(got.Problems) != 3 || o.reads != 0 || o.batches != 1 || o.diffFallbacks != 0 {
				t.Fatalf("batch failure retried or lost: err=%v complete=%t problems=%d reads=%d batches=%d fallback diffs=%d", err, got.Complete, len(got.Problems), o.reads, o.batches, o.diffFallbacks)
			}
			if phase == "diffs" && o.diffBatches+o.treeBatches != 1 {
				t.Fatal("fixture did not exercise a failed diff batch")
			}
		})
	}
}
