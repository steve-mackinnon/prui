package session

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/source"
)

func fixture() Snapshot {
	patch := []byte("@@ -1 +1 @@\n-old\n+new\xff\n")
	ref := fmt.Sprintf("%x", sha256.Sum256(patch))
	i := inventory.Inventory{Complete: true, Files: []inventory.FileChange{{ID: "file", NewPath: []byte{'a', 0xff}}}, Units: []inventory.ReviewUnit{{ID: "unit", InventoryID: "inventory", FileChangeID: "file", Kind: inventory.TextHunk, PatchReference: ref}}, Patches: map[string][]byte{ref: patch}}
	i.Comparison = source.PinnedComparison{Metadata: source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 7}, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}, InventoryID: "inventory"}
	return Snapshot{Inventory: i, Slices: []Slice{{FileID: "file", Units: []int{0}}}, UnitFiles: []int{0}, Context: reviewcontext.ContextBundle{ComparisonID: "inventory", Evidence: []reviewcontext.Evidence{{EvidenceID: "evidence", CommitSHA: "commit", Path: []byte("README.md"), Excerpt: []byte("docs")}}, OmittedPaths: []reviewcontext.Omitted{{Path: []byte(".env"), Reason: "credential-like filename"}}}}
}

func guideCacheKey() GuideCacheKey {
	return GuideCacheKey{Repository: "owner/repo", Number: 7, BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
}

func generatedGuide() guide.Bundle {
	return guide.Bundle{Status: guide.Generated, Provider: "openai", Model: "test-model", PromptVersion: guide.PromptVersion, SchemaName: "pr_review_guides", InputDigest: "digest", Items: []guide.Item{{Title: "Authentication flow", Sections: []guide.Section{{Title: "Add login endpoint", UnitIDs: []string{"unit"}}}}}}
}

func TestStoreRestartFrozenBytesAndProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := store.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.UpdateState(context.Background(), recorded.ID, recorded.Generation, recorded.SnapshotReference,
		StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Load(recorded.ID)
	if err != nil || got.Generation != state.Generation || len(got.ReviewedSliceIDs) != 1 {
		t.Fatalf("reopened state = %#v, %v", got, err)
	}
	for ref, patch := range fixture().Inventory.Patches {
		if !bytes.Equal(got.Inventory.Patches[ref], patch) {
			t.Fatal("frozen patch bytes changed")
		}
	}
}
