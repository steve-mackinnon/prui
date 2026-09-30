package commits

import (
	"prui/internal/inventory"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestContainsTargetUsesRawCommitPatch(t *testing.T) {
	sha := strings.Repeat("a", 40)
	bundle := &Bundle{Status: Captured, Entries: []Entry{{SHA: sha, Status: Captured, Diff: &Diff{Files: []inventory.FileChange{{ID: "f", OldPath: []byte("old.go"), NewPath: []byte("new.go")}}, Units: []inventory.ReviewUnit{{FileChangeID: "f", Kind: inventory.TextHunk, PatchReference: "p"}}, Patches: map[string][]byte{"p": []byte("@@ -7,2 +9,2 @@\n-old\n+new\n context\n\\ No newline at end of file\n")}}}}}
	target := source.ReviewCommentTarget{CommitID: sha, Path: "new.go", Side: "RIGHT", Line: 9}
	if !ContainsTarget(bundle, target) {
		t.Fatal("addition not found")
	}
	target.Path = "old.go"
	target.Side = "LEFT"
	target.Line = 7
	if !ContainsTarget(bundle, target) {
		t.Fatal("deletion not found")
	}
	target.Path = "new.go"
	target.Side = "RIGHT"
	target.Line = 10
	if !ContainsTarget(bundle, target) {
		t.Fatal("context not found")
	}
	for _, change := range []func(*source.ReviewCommentTarget){func(t *source.ReviewCommentTarget) { t.CommitID = strings.Repeat("b", 40) }, func(t *source.ReviewCommentTarget) { t.Line = 11 }, func(t *source.ReviewCommentTarget) { t.Path = "other.go" }, func(t *source.ReviewCommentTarget) { t.Side = "BOTH" }} {
		forged := target
		change(&forged)
		if ContainsTarget(bundle, forged) {
			t.Fatalf("forged target accepted: %#v", forged)
		}
	}
	bundle.Entries[0].Diff.Units[0].Kind = inventory.Binary
	if ContainsTarget(bundle, target) {
		t.Fatal("binary unit accepted")
	}
}

func TestHistoricalCommentTargetSupportsVerifiedRightCoordinates(t *testing.T) {
	sha := strings.Repeat("a", 40)
	target := source.ReviewCommentTarget{CommitID: sha, Path: "a.go", Side: "RIGHT", Line: 9}
	bundle := &Bundle{Status: Captured, Entries: []Entry{{SHA: sha, Parents: []string{strings.Repeat("b", 40)}, Status: Captured, Diff: &Diff{Complete: true, Files: []inventory.FileChange{{ID: "f", Status: "M", OldPath: []byte("a.go"), NewPath: []byte("a.go")}}, Units: []inventory.ReviewUnit{{FileChangeID: "f", Kind: inventory.TextHunk, PatchReference: "p"}}, Patches: map[string][]byte{"p": []byte("@@ -7,2 +9,2 @@\n-old\n+new\n context\n")}}}}}
	if !HistoricalCommentTarget(bundle, target) {
		t.Fatal("replacement rejected")
	}
	target.Line = 10
	if !HistoricalCommentTarget(bundle, target) {
		t.Fatal("context rejected")
	}
	target.Line = 9
	for _, mutate := range []func(){
		func() { bundle.Entries[0].Parents = nil },
		func() { bundle.Entries[0].Parents = append(bundle.Entries[0].Parents, sha) },
		func() { bundle.Entries[0].Diff.Complete = false },
		func() { bundle.Entries[0].Diff.Files[0].Status = "R100" },
		func() { bundle.Entries[0].Diff.Files[0].Status = "C100" },
		func() { bundle.Entries[0].Diff.Files[0].OldPath = []byte("other.go") },
		func() { target.Side = "LEFT"; target.Line = 7 },
		func() { target.Line = 20 },
	} {
		entry := bundle.Entries[0]
		file := entry.Diff.Files[0]
		originalTarget := target
		mutate()
		if HistoricalCommentTarget(bundle, target) {
			t.Fatal("unsupported or forged coordinates accepted")
		}
		bundle.Entries[0] = entry
		entry.Diff.Complete = true
		entry.Diff.Files[0] = file
		target = originalTarget
	}
	bundle.Entries[0].Diff.Files[0].Status = "A"
	bundle.Entries[0].Diff.Files[0].OldPath = nil
	if !HistoricalCommentTarget(bundle, target) {
		t.Fatal("addition rejected")
	}
}
