package tui

import (
	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/source"
	"reflect"
	"testing"
)

func TestSharedPatchRowsKeepCoordinatesSeparateFromPresentation(t *testing.T) {
	s := kindsSession()
	f := inventory.FileChange{ID: "f", OldPath: []byte("old.go"), NewPath: []byte("new.go")}
	u := inventory.ReviewUnit{FileChangeID: "f", Kind: inventory.TextHunk, PatchReference: "p"}
	patch := []byte("diff --git a/old.go b/new.go\n--- a/old.go\n+++ b/new.go\n@@ -4,2 +7,2 @@\n-old\n+new\n context\n\\ No newline at end of file\n@@ -20 +30 @@\n+\tlast\n")
	s.Inventory.Files[0], s.Inventory.Units[1], s.Inventory.Patches["p"] = f, u, patch
	metadata := s.Inventory.Comparison.Metadata
	main := unitLines(s, 1)
	commit := commitDiffRowsFor(&commits.Diff{Files: []inventory.FileChange{f}, Units: []inventory.ReviewUnit{u}, Patches: map[string][]byte{"p": patch}}, metadata.Identity, metadata.HeadSHA)[1:]
	if !reflect.DeepEqual(main, commit) {
		t.Fatalf("main and commit source rows differ:\n%+v\n%+v", main, commit)
	}
	want := []string{"@@ -4,2 +7,2 @@", "   4      -old", "        7 +new", "   5    8  context", `\ No newline at end of file`, "@@ -20 +30 @@", `       30 +\tlast`, ""}
	for i, row := range commit {
		if got := numberedPatchText(row); got != want[i] {
			t.Errorf("row %d = %q, want %q", i, got, want[i])
		}
	}
	wantAnchors := map[int]source.ReviewCommentTarget{
		1: {Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "old.go", Side: "LEFT", Line: 4},
		2: {Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "new.go", Side: "RIGHT", Line: 7},
		3: {Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "new.go", Side: "RIGHT", Line: 8},
		6: {Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "new.go", Side: "RIGHT", Line: 30},
	}
	for i, row := range commit {
		want, anchored := wantAnchors[i]
		if anchored {
			if row.target == nil || *row.target != want {
				t.Errorf("row %d anchor = %+v, want %+v", i, row.target, want)
			}
		} else if row.target != nil {
			t.Errorf("structural row %d has anchor %+v", i, row.target)
		}
	}
	split := projectSideBySideRows(commit)
	if split[1].old == nil || split[1].new == nil || split[1].old.number != 4 || split[1].new.number != 7 {
		t.Fatalf("split alignment lost: %+v", split[1])
	}
}

func TestCommitSourceRowsRejectUnsafeAnchors(t *testing.T) {
	for _, path := range [][]byte{nil, []byte("bad\npath"), {0xff}} {
		d := &commits.Diff{Files: []inventory.FileChange{{ID: "f", NewPath: path}}, Units: []inventory.ReviewUnit{{FileChangeID: "f", Kind: inventory.TextHunk, PatchReference: "p"}}, Patches: map[string][]byte{"p": []byte("@@ -0,0 +1 @@\n+new\n")}}
		for _, sha := range []string{"", "sha"} {
			for _, row := range commitDiffRowsFor(d, source.Identity{}, sha) {
				if row.target != nil {
					t.Fatalf("unsafe target: %+v", row.target)
				}
			}
		}
	}
}
