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
