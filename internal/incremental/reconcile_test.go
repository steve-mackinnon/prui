package incremental

import (
	"context"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/testutil"
	"reflect"
	"testing"
)

func fixture(t *testing.T) (*testutil.Repo, inventory.Inventory, inventory.Inventory, *Bundle) {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Write("stable", "before\ncontext\n")
	r.Write("changed", "before\ncontext\n")
	base := r.Commit()
	r.Write("stable", "after\ncontext\n")
	r.Write("changed", "after\ncontext\n")
	old := r.Commit()
	r.Write("changed", "again\n")
	next := r.Commit()
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: old}
	a, err := inventory.Build(context.Background(), v, source.PinnedComparison{Metadata: meta, MergeBaseSHA: base}, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	nextMeta := meta
	nextMeta.HeadSHA = next
	b, err := inventory.Build(context.Background(), v, source.PinnedComparison{Metadata: nextMeta, MergeBaseSHA: base}, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	captured, err := Capture(context.Background(), v, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "ref", meta, nextMeta, nil, source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return r, a, b, captured
}

func TestCarryRequiresEntireRawContentProof(t *testing.T) {
	_, old, next, b := fixture(t)
	ids := []string{old.Files[0].ID, old.Files[1].ID}
	got := Carry(old, next, ids, b)
	var want []string
	for _, f := range next.Files {
		if string(f.NewPath) == "stable" {
			want = append(want, f.ID)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("carry %v want %v", got, want)
	}
	next.Comparison.Metadata.BaseSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if got := Carry(old, next, ids, b); len(got) != 0 {
		t.Fatal("capture pins mismatch carried progress")
	}
}
func TestUnavailableNeverCarriesAndDraftKeepsOriginalTarget(t *testing.T) {
	_, old, next, b := fixture(t)
	b.Status = "unavailable"
	b.Diff = nil
	if got := Carry(old, next, []string{old.Files[0].ID}, b); len(got) != 0 {
		t.Fatal("unavailable carried")
	}
	target := source.ReviewCommentTarget{Identity: old.Comparison.Metadata.Identity, CommitID: old.Comparison.Metadata.HeadSHA, Path: "stable", Side: "RIGHT", Line: 1}
	saved := target
	outcome := Assess(old, next, target, false)
	if outcome.Kind != AnchorUnchanged || outcome.Proposed == nil || outcome.Proposed.CommitID != next.Comparison.Metadata.HeadSHA || target != saved {
		t.Fatalf("outcome %#v", outcome)
	}
	if got := Assess(old, next, target, true); got.Kind != AnchorUncertain || got.Proposed != nil {
		t.Fatalf("uncertain %#v", got)
	}
}

func TestExactPathWinsRegardlessOfDuplicateContentOrder(t *testing.T) {
	_, old, next, _ := fixture(t)
	var stable inventory.FileChange
	for _, f := range next.Files {
		if string(f.NewPath) == "stable" {
			stable = f
		}
	}
	copyFile := stable
	copyFile.ID = "copy"
	copyFile.OldPath = []byte("another")
	copyFile.NewPath = []byte("another")
	copyFile.Status = "A"
	rename := stable
	rename.ID = "rename"
	rename.OldPath = []byte("stable")
	rename.NewPath = []byte("renamed")
	rename.Status = "R100"
	baseTarget := source.ReviewCommentTarget{Identity: old.Comparison.Metadata.Identity, CommitID: old.Comparison.Metadata.HeadSHA, Path: "stable", Side: "RIGHT", Line: 1}
	targets := []source.ReviewCommentTarget{baseTarget}
	left := baseTarget
	left.Side = "LEFT"
	targets = append(targets, left)
	file := baseTarget
	file.SubjectType = "file"
	file.Side = ""
	file.Line = 0
	targets = append(targets, file)
	for _, side := range []string{"LEFT", "RIGHT"} {
		r := baseTarget
		r.Side = side
		r.StartSide = side
		r.StartLine = 1
		r.Line = 2
		targets = append(targets, r)
	}
	for _, other := range []inventory.FileChange{copyFile, rename} {
		for _, first := range []bool{true, false} {
			candidate := next
			if first {
				candidate.Files = append([]inventory.FileChange{other}, next.Files...)
			} else {
				candidate.Files = append(append([]inventory.FileChange{}, next.Files...), other)
			}
			for _, target := range targets {
				o := Assess(old, candidate, target, false)
				if o.Kind != AnchorUnchanged || o.Proposed == nil || o.Proposed.Path != target.Path || o.Proposed.StartLine != target.StartLine || o.Proposed.StartSide != target.StartSide || o.Proposed.Line != target.Line || o.Proposed.SubjectType != target.SubjectType {
					t.Fatalf("first=%v shape=%#v: %#v", first, target, o)
				}
			}
		}
	}
	// A same-blob file never constitutes rename evidence if the actual path is gone.
	candidate := next
	candidate.Files = []inventory.FileChange{copyFile}
	if got := Assess(old, candidate, baseTarget, false); got.Kind != AnchorRemoved {
		t.Fatalf("copy treated as rename %#v", got)
	}
	candidate.Files = []inventory.FileChange{rename}
	if got := Assess(old, candidate, baseTarget, false); got.Kind != AnchorRenamed || got.Proposed != nil {
		t.Fatalf("rename %#v", got)
	}
}

func TestProofRejectsPathTextOnlyModesPartialAndPolicyChanges(t *testing.T) {
	_, old, next, b := fixture(t)
	var stable inventory.FileChange
	for _, f := range old.Files {
		if string(f.NewPath) == "stable" {
			stable = f
		}
	}
	for _, mode := range []string{"partial", "old oid", "new oid", "mode", "settings", "unavailable unit"} {
		t.Run(mode, func(t *testing.T) {
			candidate := next
			candidate.Files = append([]inventory.FileChange{}, next.Files...)
			candidate.Units = append([]inventory.ReviewUnit{}, next.Units...)
			for i := range candidate.Files {
				f := &candidate.Files[i]
				if string(f.NewPath) != "stable" {
					continue
				}
				switch mode {
				case "old oid":
					f.OldOID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				case "new oid":
					f.NewOID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
				case "mode":
					f.NewMode = "100755"
				}
			}
			switch mode {
			case "partial":
				candidate.Complete = false
			case "settings":
				candidate.Comparison.DiffSettings = "different"
			case "unavailable unit":
				for i := range candidate.Units {
					if candidate.Units[i].FileChangeID == stable.ID {
						candidate.Units[i].Kind = inventory.Unavailable
						break
					}
				}
			}
			if got := Carry(old, candidate, []string{stable.ID}, b); len(got) != 0 {
				t.Fatalf("%s carried %v", mode, got)
			}
		})
	}
}

func TestExistingRenameDoesNotCarryOnLaterPush(t *testing.T) {
	_, old, next, b := fixture(t)
	// An already-renamed record remains unchanged on both sides at a later
	// revision. The conservative policy still requires explicit review.
	for i := range old.Files {
		if string(old.Files[i].NewPath) == "stable" {
			old.Files[i].Status = "R100"
			old.Files[i].OldPath = []byte("original")
		}
	}
	for i := range next.Files {
		if string(next.Files[i].NewPath) == "stable" {
			next.Files[i].Status = "R100"
			next.Files[i].OldPath = []byte("original")
		}
	}
	var stable inventory.FileChange
	for _, f := range old.Files {
		if string(f.NewPath) == "stable" {
			stable = f
		}
	}
	if got := Carry(old, next, []string{stable.ID}, b); len(got) != 0 {
		t.Fatalf("rename carried %v", got)
	}
	target := source.ReviewCommentTarget{Identity: old.Comparison.Metadata.Identity, CommitID: old.Comparison.Metadata.HeadSHA, Path: "stable", SubjectType: "file"}
	if got := Assess(old, next, target, false); got.Kind == AnchorUnchanged || got.Proposed != nil {
		t.Fatalf("rename auto-proposal %#v", got)
	}
}
