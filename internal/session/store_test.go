package session

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
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
