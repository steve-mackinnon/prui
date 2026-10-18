package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"prui/internal/inventory"
	"prui/internal/privacy"
	"prui/internal/source"
	"prui/internal/testutil"
)

type evidenceBatchObjects struct {
	fakeObjects
	batches    [][]string
	fail, omit bool
}

func (o *evidenceBatchObjects) BlobBatch(ctx context.Context, ids []string, limit int) map[string]source.BlobResult {
	o.batches = append(o.batches, append([]string{}, ids...))
	out := map[string]source.BlobResult{}
	for i, id := range ids {
		if o.fail {
			out[id] = source.BlobResult{Err: source.ErrLimit}
			continue
		}
		if o.omit && i%2 == 1 {
			continue
		}
		data, err := o.Blob(ctx, id, limit)
		out[id] = source.BlobResult{Data: data, Err: err}
	}
	return out
}

func evidenceFixture(n int) (fakeObjects, source.PinnedComparison) {
	var tree strings.Builder
	blobs := map[string][]byte{}
	for i := range n {
		oid := fmt.Sprintf("%040x", i+1)
		fmt.Fprintf(&tree, "100644 blob %s\tfile%04d.go\x00", oid, i)
		blobs[oid] = []byte("data")
	}
	return fakeObjects{tree: []byte(tree.String()), blobs: blobs}, source.PinnedComparison{InventoryID: "comparison", MergeBaseSHA: strings.Repeat("a", 40), Metadata: source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, HeadSHA: strings.Repeat("b", 40)}}
}

func TestEvidenceBatchPreservesBudgetsAndOrdering(t *testing.T) {
	for _, limits := range []Limits{
		{Files: 20, ExcerptBytes: 4, Bytes: 80, Duration: time.Second},
		{Files: 1, ExcerptBytes: 4, Bytes: 80, Duration: time.Second},
		{Files: 20, ExcerptBytes: 4, Bytes: 8, Duration: time.Second},
		{Files: 20, ExcerptBytes: 4, Bytes: 5, Duration: time.Second},
	} {
		t.Run(fmt.Sprintf("%d/%d", limits.Files, limits.Bytes), func(t *testing.T) {
			plain, p := evidenceFixture(12)
			serialCalls, batchedCalls := 0, 0
			plain.blobCalls = &serialCalls
			want := Retrieve(context.Background(), plain, p, inventory.Inventory{}, privacy.Policy{}, limits)
			plain.blobCalls = &batchedCalls
			batch := &evidenceBatchObjects{fakeObjects: plain}
			got := Retrieve(context.Background(), batch, p, inventory.Inventory{}, privacy.Policy{}, limits)
			if !reflect.DeepEqual(got, want) || serialCalls != batchedCalls {
				t.Fatal("evidence budget, order, or read scope changed", serialCalls, batchedCalls)
			}
			if limits.Files == 20 && limits.Bytes == 80 && len(batch.batches) == 0 {
				t.Fatal("eligible evidence did not batch")
			}
		})
	}
}

func TestEvidenceBatchReservesRepeatedOIDsPerFile(t *testing.T) {
	plain, p := evidenceFixture(0)
	for i, id := range []int{1, 1, 2, 3} {
		oid := fmt.Sprintf("%040x", id)
		plain.tree = append(plain.tree, []byte(fmt.Sprintf("100644 blob %s\tfile%d.go\x00", oid, i))...)
		plain.blobs[oid] = []byte("data")
	}
	for _, files := range []int{2, 3} {
		limits := Limits{Files: files, ExcerptBytes: 4, Bytes: files * 4, Duration: time.Second}
		want := Retrieve(context.Background(), plain, p, inventory.Inventory{}, privacy.Policy{}, limits)
		batch := &evidenceBatchObjects{fakeObjects: plain}
		got := Retrieve(context.Background(), batch, p, inventory.Inventory{}, privacy.Policy{}, limits)
		if !reflect.DeepEqual(got, want) {
			t.Fatal("repeated OID changed file budget", files)
		}
		for _, ids := range batch.batches {
			for _, id := range ids {
				if id == fmt.Sprintf("%040x", 3) {
					t.Fatal("read beyond repeated file reservations")
				}
			}
		}
	}
}

func TestEvidenceBatchDoesNotRetryFailuresAndFallsBackOnlyForUnreadIDs(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			plain, p := evidenceFixture(4)
			calls := 0
			plain.blobCalls = &calls
			batch := &evidenceBatchObjects{fakeObjects: plain, fail: fail, omit: !fail}
			got := Retrieve(context.Background(), batch, p, inventory.Inventory{}, privacy.Policy{}, Limits{Files: 4, ExcerptBytes: 4, Bytes: 16, Duration: time.Second})
			if len(batch.batches) != 1 {
				t.Fatal("missing or repeated evidence batch", len(batch.batches))
			}
			if fail {
				if calls != 0 || len(got.Evidence) != 0 || len(got.OmittedPaths) != 4 {
					t.Fatal("failed evidence read retried or hidden", calls, got)
				}
			} else if calls != 4 || len(got.Evidence) != 4 {
				t.Fatal("unread IDs lost or reread", calls, got)
			}
		})
	}
}

func TestEvidenceBatchFiltersPathsBeforeReading(t *testing.T) {
	plain, p := evidenceFixture(0)
	for i, path := range []string{".env", "excluded.go", "link.go", "allowed.go", "notes.md", "more.go"} {
		oid := fmt.Sprintf("%040x", i+1)
		mode := "100644"
		if path == "link.go" {
			mode = "120000"
		}
		plain.tree = append(plain.tree, []byte(fmt.Sprintf("%s blob %s\t%s\x00", mode, oid, path))...)
		plain.blobs[oid] = []byte("safe\n")
		if path == "notes.md" {
			plain.blobs[oid] = []byte("api_key=secret\n")
		}
	}
	policy := privacy.Policy{Excluded: []string{"excluded.go"}}
	limits := Limits{Files: 8, ExcerptBytes: 32, Bytes: 256, Duration: time.Second}
	want := Retrieve(context.Background(), plain, p, inventory.Inventory{}, policy, limits)
	batch := &evidenceBatchObjects{fakeObjects: plain}
	got := Retrieve(context.Background(), batch, p, inventory.Inventory{}, policy, limits)
	if !reflect.DeepEqual(got, want) || len(batch.batches) != 1 {
		t.Fatal("privacy or source selection changed", got)
	}
	for _, ids := range batch.batches {
		for _, id := range ids {
			if id == fmt.Sprintf("%040x", 1) || id == fmt.Sprintf("%040x", 2) || id == fmt.Sprintf("%040x", 3) {
				t.Fatal("excluded path read", id)
			}
		}
	}
}

func TestEvidenceBatchFailureRetentionIsBounded(t *testing.T) {
	plain, p := evidenceFixture(evidenceFailedIDs + 2)
	for id := range plain.blobs {
		plain.blobs[id] = []byte("oversized")
	}
	plain.tree = append(plain.tree, []byte(fmt.Sprintf("100644 blob %040x\tzduplicate.go\x00", 1))...)
	calls := 0
	plain.blobCalls = &calls
	batch := &evidenceBatchObjects{fakeObjects: plain, fail: true}
	got := Retrieve(context.Background(), batch, p, inventory.Inventory{}, privacy.Policy{}, Limits{Files: 10, ExcerptBytes: 4, Bytes: 40, Duration: 5 * time.Second})
	requested := 0
	for _, ids := range batch.batches {
		requested += len(ids)
	}
	if requested != evidenceFailedIDs || calls != 2 || len(got.Evidence) != 0 || len(got.OmittedPaths) != evidenceFailedIDs+3 {
		t.Fatal("unbounded failure retention or failed batch retry", requested, calls, len(got.OmittedPaths))
	}
}

func TestEvidenceBatchPreservesDeletedSideAndCancellation(t *testing.T) {
	plain, p := evidenceFixture(8)
	inv := inventory.Inventory{Files: []inventory.FileChange{{OldPath: []byte("file0000.go")}}}
	limits := Limits{Files: 3, ExcerptBytes: 4, Bytes: 12, Duration: time.Second}
	want := Retrieve(context.Background(), plain, p, inv, privacy.Policy{}, limits)
	batch := &evidenceBatchObjects{fakeObjects: plain}
	got := Retrieve(context.Background(), batch, p, inv, privacy.Policy{}, limits)
	if !reflect.DeepEqual(got, want) || len(batch.batches) == 0 || got.Evidence[0].CommitSHA != p.MergeBaseSHA {
		t.Fatal("deleted-side evidence changed", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	plain.blobCalls = &calls
	batch = &evidenceBatchObjects{fakeObjects: plain}
	got = Retrieve(ctx, batch, p, inv, privacy.Policy{}, limits)
	if calls != 0 || len(batch.batches) != 0 || len(got.Evidence) != 0 || len(got.ExhaustedBudgets) == 0 {
		t.Fatal("canceled retrieval read content or hid exhaustion", got, calls)
	}
}

type serialEvidenceObjects struct{ Objects }

func TestEvidenceBatchMatchesRealPinnedSource(t *testing.T) {
	repo := testutil.NewRepo(t)
	for i := range 24 {
		repo.Write(fmt.Sprintf("pkg/context%d_test.go", i), fmt.Sprintf("package pkg\n// independent context %d\n", i))
	}
	repo.Write("pkg/code.go", "package pkg\nconst Value = 1\n")
	repo.Write("gone.go", "package gone\n")
	repo.Write(".env", "api_key=not-readable\n")
	repo.Write("notes_test.go", "api_key=not-evidence\n")
	if err := os.Symlink("/untrusted/never-follow", filepath.Join(repo.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	base := repo.Commit()
	repo.Write("pkg/code.go", "package pkg\nconst Value = 2\n")
	repo.Git("rm", "gone.go")
	head := repo.Commit()
	before := repo.Snapshot()
	p := source.PinnedComparison{MergeBaseSHA: base, Metadata: source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseSHA: base, HeadSHA: head}}
	views := make([]*source.View, 2)
	var inventories [2]inventory.Inventory
	for i := range views {
		v, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), source.Defaults())
		if err != nil {
			t.Fatal(err)
		}
		defer v.Close()
		views[i] = v
		inv, err := inventory.Build(context.Background(), v, p, source.Defaults())
		if err != nil || !inv.Complete {
			t.Fatal("incomplete fixture", err)
		}
		inventories[i] = inv
	}
	limits := Limits{Files: 20, ExcerptBytes: 32 << 10, Bytes: 256 << 10, Duration: 30 * time.Second}
	want := Retrieve(context.Background(), serialEvidenceObjects{views[0]}, inventories[0].Comparison, inventories[0], privacy.Policy{}, limits)
	got := Retrieve(context.Background(), views[1], inventories[1].Comparison, inventories[1], privacy.Policy{}, limits)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("real pinned evidence changed")
	}
	if !reflect.DeepEqual(before, repo.Snapshot()) {
		t.Fatal("evidence retrieval changed checkout")
	}
}
