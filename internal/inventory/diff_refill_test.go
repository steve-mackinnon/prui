package inventory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"prui/internal/source"
	"prui/internal/testutil"
)

func TestDiffWindowsRefillAfterAccountedOutput(t *testing.T) {
	repo := testutil.NewRepo(t)
	tail := strings.Repeat("func value() string { return \"example\" }\n", 1000)
	for i := range 12 {
		repo.Write(fmt.Sprintf("file-%02d.go", i), fmt.Sprintf("package p\nconst Value%d = 1\n", i)+tail)
	}
	base := repo.Commit()
	for i := range 12 {
		repo.Write(fmt.Sprintf("file-%02d.go", i), fmt.Sprintf("package p\nconst Value%d = 2\n", i)+tail)
	}
	head := repo.Commit()
	before := repo.Snapshot()
	limits := source.Defaults()
	limits.DiffLines = 40000
	a, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), limits)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), limits)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	p := source.PinnedComparison{MergeBaseSHA: base, Metadata: source.Metadata{BaseSHA: base, HeadSHA: head}}
	want, err := Build(context.Background(), serialBlobObjects{a}, p, limits)
	if err != nil || !want.Complete {
		t.Fatal(err)
	}
	objects := &batchInventoryObjects{View: b}
	got, err := Build(context.Background(), objects, p, limits)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("refill changed bounded inventory", err)
	}
	if objects.treeBatches < 2 || objects.diffFallbacks != 0 {
		t.Fatalf("did not refill: tree=%d serial=%d", objects.treeBatches, objects.diffFallbacks)
	}
	if !reflect.DeepEqual(before, repo.Snapshot()) {
		t.Fatal("diff refill changed checkout")
	}
}

type windowObjects struct {
	calls, fallbacks int
	result           source.BlobResult
	treeStatuses     []string
}

func (o *windowObjects) Git(context.Context, int, ...string) ([]byte, error) {
	o.fallbacks++
	return []byte("serial"), nil
}
func (o *windowObjects) Blob(context.Context, string, int) ([]byte, error) {
	panic("unexpected blob read")
}
func (o *windowObjects) BlobDiffBatch(_ context.Context, requests []source.BlobDiffRequest) []source.BlobResult {
	o.calls += len(requests)
	out := make([]source.BlobResult, len(requests))
	for i := range out {
		out[i] = o.result
	}
	return out
}
func (o *windowObjects) TreeDiffBatch(_ context.Context, _, _ string, requests []source.TreeDiffRequest) map[int]source.BlobResult {
	for _, r := range requests {
		o.treeStatuses = append(o.treeStatuses, r.Status)
	}
	return nil
}
func windowFixture(count int) ([]FileChange, map[string]source.BlobResult) {
	var files []FileChange
	blobs := map[string]source.BlobResult{}
	for i := range count {
		old, next := fmt.Sprintf("%040x", i*2+1), fmt.Sprintf("%040x", i*2+2)
		files = append(files, FileChange{Status: "M", OldOID: old, NewOID: next, OldMode: "100644", NewMode: "100644", OldPath: []byte(fmt.Sprint(i)), NewPath: []byte(fmt.Sprint(i))})
		blobs[old] = source.BlobResult{Data: []byte("o\n")}
		blobs[next] = source.BlobResult{Data: []byte("n\n")}
	}
	return files, blobs
}
func TestDiffWindowsBoundRetainedDataAndNeverRetryFailures(t *testing.T) {
	files, blobs := windowFixture(8)
	limits := source.Defaults()
	limits.ContentBytes = 4096
	o := &windowObjects{result: source.BlobResult{Data: []byte(strings.Repeat("x", 1000))}}
	reads := newDiffReads(o, files, blobs, "empty", "old", "new", limits)
	for i, f := range files {
		_, _ = reads.read(context.Background(), i, fileBlobPair(f, "empty"), limits.ContentBytes, limits.DiffLines)
	}
	if reads.retained > limits.ContentBytes || o.fallbacks == 0 {
		t.Fatalf("retention %d fallback %d", reads.retained, o.fallbacks)
	}
	o = &windowObjects{result: source.BlobResult{Err: source.ErrLimit}}
	files = append(files, files[0])
	reads = newDiffReads(o, files, blobs, "empty", "old", "new", limits)
	for i, f := range files {
		_, err := reads.read(context.Background(), i, fileBlobPair(f, "empty"), limits.ContentBytes, limits.DiffLines)
		if !errors.Is(err, source.ErrLimit) {
			t.Fatal(err)
		}
	}
	if o.calls != 8 || o.fallbacks != 0 || reads.retained != 0 {
		t.Fatalf("failed request retried: calls %d fallback %d retained %d", o.calls, o.fallbacks, reads.retained)
	}
}
func TestDiffWindowsRejectOversizePreparedResults(t *testing.T) {
	files, blobs := windowFixture(2)
	limits := source.Defaults()
	limits.ContentBytes = 4096
	o := &windowObjects{result: source.BlobResult{Data: make([]byte, 5000)}}
	reads := newDiffReads(o, files, blobs, "empty", "old", "new", limits)
	for i, f := range files {
		_, err := reads.read(context.Background(), i, fileBlobPair(f, "empty"), 4096, limits.DiffLines)
		if !errors.Is(err, source.ErrLimit) {
			t.Fatal(err)
		}
	}
	if reads.retained != 0 || o.fallbacks != 0 {
		t.Fatal("oversize result retained or retried")
	}
}
func TestDiffWindowRetainsWholeComparisonPathCollisionGuard(t *testing.T) {
	files, blobs := windowFixture(2)
	files[0].OldPath = []byte("swap/child")
	files[0].NewPath = nil
	files[1].Status = "A"
	files[1].OldMode = "000000"
	files[1].OldOID = ""
	files[1].NewPath = []byte("swap")
	empty := "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"
	blobs[empty] = source.BlobResult{Data: []byte{}}
	o := &windowObjects{result: source.BlobResult{Data: []byte("patch")}}
	reads := newDiffReads(o, files, blobs, empty, "old", "new", source.Defaults())
	_, through := reads.prepare(context.Background(), 1, source.Defaults())
	if through != len(files) || len(o.treeStatuses) != 1 || o.treeStatuses[0] != "" {
		t.Fatal("lost earlier changed descendant", o.treeStatuses)
	}
}

func TestDiffWindowsOwnRetainedBuffers(t *testing.T) {
	files, blobs := windowFixture(1)
	limits := source.Defaults()
	limits.ContentBytes = 4096
	raw := make([]byte, 1, 1<<20)
	raw[0] = 'x'
	o := &windowObjects{result: source.BlobResult{Data: raw}}
	reads := newDiffReads(o, files, blobs, "empty", "old", "new", limits)
	pair := fileBlobPair(files[0], "empty")
	got, err := reads.read(context.Background(), 0, pair, 4096, limits.DiffLines)
	if err != nil || len(got) != 1 || cap(got) != 1 {
		t.Fatal("unbounded retained backing buffer", err, len(got), cap(got))
	}
	raw[0] = 'y'
	if got[0] != 'x' || reads.retained != 1 {
		t.Fatal("backend buffer retained")
	}
}

func TestDiffWindowsDoNotSpeculatePastUnknownBudgetConsumer(t *testing.T) {
	files, blobs := windowFixture(2)
	delete(blobs, files[0].OldOID)
	o := &windowObjects{result: source.BlobResult{Data: []byte("patch")}}
	reads := newDiffReads(o, files, blobs, "empty", "old", "new", source.Defaults())
	results, through := reads.prepare(context.Background(), 0, source.Defaults())
	if len(results) != 0 || through != 0 || o.calls != 0 {
		t.Fatal("speculated past unknown blob")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reads.read(ctx, 0, fileBlobPair(files[0], "empty"), 4096, 100); !errors.Is(err, context.Canceled) || o.calls != 0 || o.fallbacks != 0 {
		t.Fatal("read after cancellation", err)
	}
}
