package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/session"
	"prui/internal/source"
)

// TestPTYSmoke runs the shipped executable, not run() with substituted streams.
// Python's standard library supplies the same PTY API on Linux and macOS.
func TestPTYSmoke(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("PTY smoke tests support Linux and macOS")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("PTY smoke tests require Python 3 (standard library only):", err)
	}
	root := t.TempDir()
	buildHome := os.Getenv("HOME")
	storePath := temporaryDefaultStore(t, root)
	store, err := session.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	snapshot := session.Snapshot{
		Inventory: inventory.Inventory{
			Comparison: source.PinnedComparison{InventoryID: "pty-inventory", Metadata: source.Metadata{
				Identity: source.Identity{Repository: "owner/alpha", Number: 42}, BaseRepository: "owner/alpha", HeadRepository: "owner/alpha", BaseSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}},
			Complete: true,
		},
	}
	for _, name := range []string{"a.go", "b.go"} {
		i := len(snapshot.Slices)
		snapshot.Inventory.Files = append(snapshot.Inventory.Files, inventory.FileChange{ID: name, NewPath: []byte(name), Status: "A", NewMode: "100644"})
		snapshot.Inventory.Units = append(snapshot.Inventory.Units, inventory.ReviewUnit{InventoryID: "pty-inventory", ID: "unit-" + name, FileChangeID: name, Kind: inventory.FileMetadata})
		snapshot.Slices = append(snapshot.Slices, session.Slice{FileID: name, Units: []int{i}})
		snapshot.UnitFiles = append(snapshot.UnitFiles, i)
	}
	snapshot.Commits = &commits.Bundle{BaseSHA: snapshot.Inventory.Comparison.Metadata.BaseSHA,
		HeadSHA: snapshot.Inventory.Comparison.Metadata.HeadSHA, Status: commits.Captured, Complete: true}
	for i, sha := range []string{"cccccccccccccccccccccccccccccccccccccccc", snapshot.Commits.HeadSHA} {
		patch := []byte(fmt.Sprintf("@@ -1 +1 @@\n-before\n+commit-change-%d\n", i+1))
		ref := fmt.Sprintf("%x", sha256.Sum256(patch))
		snapshot.Commits.Entries = append(snapshot.Commits.Entries, commits.Entry{SHA: sha,
			Subject: fmt.Sprintf("Commit fixture %d", i+1), Author: "Fixture", Parents: []string{snapshot.Commits.BaseSHA}, Status: commits.Captured,
			Diff: &commits.Diff{Complete: true, Files: []inventory.FileChange{{ID: "a.go", OldPath: []byte("a.go"), NewPath: []byte("a.go"), Status: "M", OldMode: "100644", NewMode: "100644", OldOID: snapshot.Commits.BaseSHA, NewOID: snapshot.Commits.HeadSHA}},
				Units: []inventory.ReviewUnit{{InventoryID: "commit-fixture", ID: "commit-unit", FileChangeID: "a.go", Kind: inventory.TextHunk,
					OldRange: inventory.Range{Start: 1, Count: 1}, NewRange: inventory.Range{Start: 1, Count: 1}, PatchReference: ref}}, Patches: map[string][]byte{ref: patch}}})
	}
	saved, err := store.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{"owner/alpha", "owner/beta"} {
		if err := store.RememberRepository(repo, filepath.Join(root, "checkout")); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// On Linux, root/prui is the XDG data directory created above. Build
	// outside the fixture home so go build cannot treat -o as that directory.
	binary := filepath.Join(t.TempDir(), "prui")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	build.Env = append(os.Environ(), "HOME="+buildHome)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build PTY executable: %v\n%s", err, out)
	}
	cmd := exec.CommandContext(ctx, python, "testdata/pty_smoke.py", binary, root, saved.ID)
	// Give the harness a chance to kill its terminal process groups on timeout.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PTY smoke: %v\n%s", err, out)
	} else {
		t.Logf("%s", out)
	}
	// Reopen through the production reader: a matching screen alone does not
	// prove progress was written to a valid, durable session.
	store, err = session.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	restored, err := store.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.ReviewedSliceIDs) != 1 || restored.ReviewedSliceIDs[0] != "b.go" {
		t.Fatalf("keyboard marking did not survive restart: %v", restored.ReviewedSliceIDs)
	}
}
