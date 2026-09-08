package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/plan"
)

func fixture() Snapshot {
	patch := []byte("@@ -1 +1 @@\n-old\n+new\xff\n")
	ref := fmt.Sprintf("%x", sha256.Sum256(patch))
	i := inventory.Inventory{Complete: true, Files: []inventory.FileChange{{ID: "file", NewPath: []byte{'a', 0xff}}}, Units: []inventory.ReviewUnit{{ID: "unit", InventoryID: "inventory", FileChangeID: "file", Kind: inventory.TextHunk, PatchReference: ref}}, Patches: map[string][]byte{ref: patch}}
	i.Comparison.InventoryID = "inventory"
	return Snapshot{Inventory: i, PlanVersion: "file-v1", Slices: []Slice{{FileID: "file", Units: []int{0}}}, UnitFiles: []int{0}, Context: reviewcontext.ContextBundle{ComparisonID: "inventory", Evidence: []reviewcontext.Evidence{{EvidenceID: "evidence", CommitSHA: "commit", Path: []byte("README.md"), Excerpt: []byte("docs")}}, OmittedPaths: []reviewcontext.Omitted{{Path: []byte(".env"), Reason: "credential-like filename"}}}}
}

func TestStoreRestartFrozenBytesAndProgress(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	r.ReviewedSliceIDs = []string{"file"}
	if err = s.Save(r); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Load(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ReviewedSliceIDs) != 1 || !bytes.Equal(got.Inventory.Files[0].NewPath, r.Inventory.Files[0].NewPath) {
		t.Fatal("lost progress or path bytes")
	}
	for ref, patch := range r.Inventory.Patches {
		if !bytes.Equal(got.Inventory.Patches[ref], patch) {
			t.Fatal("lost patch bytes")
		}
	}
	if len(got.Context.Evidence) != 1 || len(got.Context.OmittedPaths) != 1 {
		t.Fatal("lost persisted evidence scope")
	}
	for _, path := range []string{dir, filepath.Join(dir, r.ID), filepath.Join(dir, r.ID, "snapshot.json"), filepath.Join(dir, r.ID, "state.json")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatalf("unsafe permissions: %s %v", path, err)
		}
	}
}

func TestStoreLockImmutabilityAndDeletion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if other, err := Open(dir); err == nil {
		other.Close()
		t.Fatal("second writer accepted")
	}
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	old, _ := s.Load(r.ID)
	r.ReviewedSliceIDs = []string{"file"}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(old); err == nil {
		t.Fatal("outdated writer accepted")
	}
	r.PlanVersion = "changed"
	if err := s.Save(r); err == nil {
		t.Fatal("snapshot mutated")
	}
	if err := s.Delete("../outside"); err == nil {
		t.Fatal("unsafe ID accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, r.ID, ".write-interrupted"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, r.ID)); !os.IsNotExist(err) {
		t.Fatal("session artifacts remain")
	}
}

func TestStoreCorruptionRetained(t *testing.T) {
	for _, data := range []string{"{", `{"schema_version":999}`, `{"schema_version":1}`} {
		t.Run(data, func(t *testing.T) {
			s, err := Open(filepath.Join(t.TempDir(), "sessions"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			r, err := s.Create(fixture())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(s.Path(), r.ID, "state.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Load(r.ID); err == nil {
				t.Fatal("corruption accepted")
			}
			if err := s.Save(r); err == nil {
				t.Fatal("corruption overwritten")
			}
			got, _ := os.ReadFile(path)
			if string(got) != data {
				t.Fatal("old record erased")
			}
			entries, err := s.List()
			if err != nil || len(entries) != 1 || entries[0].Err == nil {
				t.Fatal("corrupt session hidden")
			}
			if err := s.Delete(r.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEditApplyPlanResetsCompletionAndRetainsPrevious(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	r.ReviewedSliceIDs = []string{"file"}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	p := plan.ValidatedPlan{Version: "edited", InventoryID: "inventory", AnalysisStatus: "valid", Slices: []plan.Slice{{SliceID: "edited", Title: "Edited", UnitIDs: []string{"unit"}}}}
	if err := s.ApplyPlan(r, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ReviewedSliceIDs) != 0 || got.CurrentPlan() == nil || got.CurrentPlan().Version != "edited" || got.PreviousPlan == nil {
		t.Fatalf("plan edit state=%+v", got.State)
	}
}

// preGuideSnapshot is the snapshot shape stored before guide analysis existed.
type preGuideSnapshot struct {
	Checkout     []byte
	Inventory    inventory.Inventory
	PlanVersion  string
	Slices       []Slice
	UnitFiles    []int
	Context      reviewcontext.ContextBundle
	AnalysisPlan *plan.ValidatedPlan
}

func TestStoreGuideBundleRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	snapshot := fixture()
	b := guide.Bundle{Status: guide.Generated, Provider: "openai", Model: "test-model", PromptVersion: "guides-v1", SchemaName: "pr_review_guides", InputDigest: "digest", EvidenceIDs: []string{"evidence"}, WithheldPaths: []guide.Omitted{{Path: []byte(".env"), Reason: "credential-like filename"}}, Limits: guide.Limits{Units: 400, UnitBytes: 32 << 10, Bytes: 512 << 10}, Items: []guide.Item{{Title: "Authentication flow", Description: "Adds a login endpoint.", Sections: []guide.Section{{Title: "Add login endpoint", Description: "one hunk", UnitIDs: []string{"unit"}}}}}}
	snapshot.Guides = &b
	r, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Guides == nil || got.Guides.Status != guide.Generated || got.Guides.Model != "test-model" || got.Guides.InputDigest != "digest" {
		t.Fatal("lost guide provenance")
	}
	if len(got.Guides.Items) != 1 || len(got.Guides.Items[0].Sections) != 1 || got.Guides.Items[0].Sections[0].UnitIDs[0] != "unit" {
		t.Fatal("lost guide structure")
	}
	if len(got.Guides.WithheldPaths) != 1 || !bytes.Equal(got.Guides.WithheldPaths[0].Path, []byte(".env")) || got.Guides.Limits.Units != 400 {
		t.Fatal("lost analysis scope")
	}
	got.ReviewedSliceIDs = []string{"file"}
	if err := s.Save(got); err != nil {
		t.Fatal("guide snapshot blocks progress", err)
	}
	got.Guides.Items[0].Sections[0].UnitIDs = []string{"invented"}
	if err := s.Save(got); err == nil {
		t.Fatal("tampered guide accepted")
	}
	if _, err := s.Create(func() Snapshot {
		bad := fixture()
		invented := guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "x", Sections: []guide.Section{{Title: "y", UnitIDs: []string{"invented"}}}}}}
		bad.Guides = &invented
		return bad
	}()); err == nil {
		t.Fatal("guide referencing unknown units stored")
	}
}

func TestStoreDerivedSnapshotRetainsOriginalAndStartsUnread(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	original, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	original.ReviewedSliceIDs = []string{"file"}
	if err := s.Save(original); err != nil {
		t.Fatal(err)
	}
	derived := original.Snapshot
	derived.DerivedFrom = original.ID
	derived.Guides = &guide.Bundle{Status: guide.Unavailable, Reason: "provider unavailable"}
	child, err := s.Create(derived)
	if err != nil {
		t.Fatal(err)
	}
	if child.DerivedFrom != original.ID || len(child.ReviewedSliceIDs) != 0 {
		t.Fatal("derived session lost source link or inherited progress", child)
	}
	stored, err := s.Load(original.ID)
	if err != nil || len(stored.ReviewedSliceIDs) != 1 || stored.DerivedFrom != "" || stored.Guides != nil {
		t.Fatal("derived session changed original", stored, err)
	}
	derived.DerivedFrom = "not-a-session-id"
	if _, err := s.Create(derived); err == nil {
		t.Fatal("invalid derived source accepted")
	}
	derived.DerivedFrom = original.ID
	derived.PlanVersion = "altered"
	if _, err := s.Create(derived); err == nil {
		t.Fatal("derived session changed source evidence")
	}
}

func TestStorePreGuideBytesUnchanged(t *testing.T) {
	snapshot := fixture()
	if snapshot.Guides != nil {
		t.Fatal("fixture already carries guides")
	}
	current, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(preGuideSnapshot{Checkout: snapshot.Checkout, Inventory: snapshot.Inventory, PlanVersion: snapshot.PlanVersion, Slices: snapshot.Slices, UnitFiles: snapshot.UnitFiles, Context: snapshot.Context, AnalysisPlan: snapshot.AnalysisPlan})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, current) {
		t.Fatalf("pre-guide snapshot bytes changed:\n%s\n%s", before, current)
	}
	dir := filepath.Join(t.TempDir(), "sessions")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filepath.Join(dir, r.ID, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, before) || bytes.Contains(stored, []byte("guides")) {
		t.Fatal("guideless session gained guide bytes")
	}
	r.ReviewedSliceIDs = []string{"file"}
	if err := s.Save(r); err != nil {
		t.Fatal("pre-guide session can no longer record progress", err)
	}
}

func TestRepositoryRegistryRoundTripReplacementAndCorruption(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	checkout := filepath.Join(t.TempDir(), "checkout")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LookupRepository("o/r"); !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatal("missing repository lookup", err)
	}
	if err := s.RememberRepository("Owner/Repo", checkout); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(t.TempDir(), "replacement")
	if err := s.RememberRepository("owner/repo", replacement); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LookupRepository("OWNER/REPO"); err != nil || got != replacement {
		t.Fatal("registry replacement lost", got, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, err := s.LookupRepository("owner/repo"); err != nil || got != replacement {
		t.Fatal("registry did not survive restart", got, err)
	}
	registry := filepath.Join(dir, "repositories.json")
	bad := []byte(`{"repositories":[{"repository":"owner/repo","checkout":"relative"}]}`)
	if err := os.WriteFile(registry, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LookupRepository("owner/repo"); err == nil {
		t.Fatal("malformed registry accepted")
	}
	if err := s.RememberRepository("other/repo", checkout); err == nil {
		t.Fatal("malformed registry overwritten")
	}
	if got, err := os.ReadFile(registry); err != nil || !bytes.Equal(got, bad) {
		t.Fatal("malformed registry changed", err)
	}
}
