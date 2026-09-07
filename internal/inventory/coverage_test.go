package inventory

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

func TestInventoryEmptyDeletedAndSeparatedHunks(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("deleted", "delete me\n")
	r.Write("empty", "")
	r.Write("hunks", "old\n"+strings.Repeat("context\n", 20)+"old\n")
	base := r.Commit()
	if e := os.Remove(filepath.Join(r.Dir, "deleted")); e != nil {
		t.Fatal(e)
	}
	r.Write("hunks", "new\n"+strings.Repeat("context\n", 20)+"new\n")
	r.Write("added-empty", "")
	head := r.Commit()
	v, e := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	p := source.PinnedComparison{MergeBaseSHA: base, Metadata: source.Metadata{BaseSHA: base, HeadSHA: head}}
	inv, e := Build(context.Background(), v, p, source.Defaults())
	if e != nil || !inv.Complete || len(inv.Files) != 3 {
		t.Fatal("incorrect inventory", e)
	}
	count := 0
	sawDeletion := false
	for _, u := range inv.Units {
		for _, f := range inv.Files {
			if u.FileChangeID == f.ID && string(f.NewPath) == "hunks" && u.Kind == TextHunk {
				count++
			}
			if f.Status == "D" && u.FileChangeID == f.ID && bytes.Contains(inv.Patches[u.PatchReference], []byte("-delete me")) {
				sawDeletion = true
			}
		}
	}
	if count != 2 || !sawDeletion {
		t.Fatal("hunks/deletion not represented", count, sawDeletion)
	}
	p.MergeBaseSHA = head
	inv, e = Build(context.Background(), v, p, source.Defaults())
	if e != nil || !inv.Complete || len(inv.Files) != 0 || len(inv.Units) != 0 {
		t.Fatal("empty comparison incorrect", e)
	}
}
