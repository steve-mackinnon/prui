package context

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"prui/internal/inventory"
	"prui/internal/privacy"
	"prui/internal/source"
)

func TestChangedPathIndexPreservesRankingAndDeletedSides(t *testing.T) {
	inv := inventory.Inventory{Files: []inventory.FileChange{
		{OldPath: []byte("old/a.go"), NewPath: []byte("new/a.go")},
		{OldPath: []byte("deleted/a.go")},
		{NewPath: []byte("added/a.go")},
		{OldPath: []byte("root.go"), NewPath: []byte("root.go")},
		{OldPath: []byte("odd/../normalized/a.go"), NewPath: []byte("odd/\xff/a.go")},
	}}
	index := indexChangedPaths(inv)
	for _, path := range []string{"old/test.go", "new/README.md", "deleted/a.go", "added/a.go", "go.mod", "other/file.go", "other/test.go", "normalized/a.go", "odd/\xff/b.go", ""} {
		p := []byte(path)
		wantRank := 1
		if k := classify(p); k == Manifest || k == Documentation || k == NearbyTest {
			wantRank = 0
		}
		near, deleted := false, false
		for _, f := range inv.Files {
			for _, q := range [][]byte{f.OldPath, f.NewPath} {
				near = near || len(q) > 0 && filepath.Dir(string(q)) == filepath.Dir(path)
			}
			deleted = deleted || string(f.OldPath) == path && len(f.NewPath) == 0
		}
		if !near {
			wantRank++
		}
		if got := index.rank(p); got != wantRank {
			t.Errorf("rank %q = %d, want %d", path, got, wantRank)
		}
		if got := index.deleted[path]; got != deleted {
			t.Errorf("deleted %q = %t, want %t", path, got, deleted)
		}
	}
}

// Exercise tree ranking independently of subprocess and network latency. The
// trees and changed paths deliberately span many directories, as in a monorepo.
func BenchmarkRetrieveLargeTree(b *testing.B) {
	const sha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var tree []byte
	inv := inventory.Inventory{}
	for i := 0; i < 1000; i++ {
		p := []byte(fmt.Sprintf("pkg/%04d/changed.go", i))
		inv.Files = append(inv.Files, inventory.FileChange{OldPath: p, NewPath: p})
	}
	for i := 0; i < 10000; i++ {
		tree = fmt.Appendf(tree, "100644 blob %s\tpkg/%04d/file.go%c", sha, i, 0)
	}
	p := source.PinnedComparison{MergeBaseSHA: sha, Metadata: source.Metadata{HeadSHA: sha}}
	objects := fakeObjects{tree: tree, blobs: map[string][]byte{sha: []byte("package fixture\n")}}
	limits := Limits{Files: 100, ExcerptBytes: 1024, Bytes: 256 << 10, Duration: time.Minute}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := Retrieve(context.Background(), objects, p, inv, privacy.Policy{}, limits)
		if len(got.ExhaustedBudgets) != 0 {
			b.Fatal(got.ExhaustedBudgets)
		}
	}
}
