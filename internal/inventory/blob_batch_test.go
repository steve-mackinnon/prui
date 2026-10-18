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
	if err != nil || !reflect.DeepEqual(got, want) || objects.batches != 1 {
		t.Fatalf("batch inventory differs from serial: err=%v batches=%d", err, objects.batches)
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
	v, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	o := &batchInventoryObjects{View: v, fail: true}
	comparison := source.PinnedComparison{MergeBaseSHA: base, Metadata: source.Metadata{BaseSHA: base, HeadSHA: head}}
	got, err := Build(context.Background(), o, comparison, source.Defaults())
	if err != nil || got.Complete || len(got.Problems) != 3 || o.reads != 0 || o.batches != 1 {
		t.Fatalf("batch failure retried or lost: err=%v complete=%t problems=%d reads=%d batches=%d", err, got.Complete, len(got.Problems), o.reads, o.batches)
	}
}
