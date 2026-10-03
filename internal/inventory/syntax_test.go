package inventory

import (
	"context"
	"prui/internal/source"
	"prui/internal/syntax"
	"prui/internal/testutil"
	"strings"
	"testing"
)

func TestCaptureSyntaxUsesFullPinnedBlobs(t *testing.T) {
	r := testutil.NewRepo(t)
	prefix := "package p\n/*\n" + strings.Repeat("unchanged\n", 12)
	r.Write("x.go", prefix+"old words\n*/\n")
	base := r.Commit()
	r.Write("x.go", prefix+"new words\n*/\n")
	head := r.Commit()
	r.Write("x.go", "dirty checkout must never be lexed")
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	inv, err := BuildCommit(context.Background(), v, CommitComparison{base, head}, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, u := range inv.Units {
		if u.Kind != TextHunk {
			continue
		}
		patch := strings.Split(string(inv.Patches[u.PatchReference]), "\n")
		for i, row := range patch {
			if row != "-old words" && row != "+new words" {
				continue
			}
			spans := inv.Syntax[u.ID][i].New
			if row[0] == '-' {
				spans = inv.Syntax[u.ID][i].Old
			}
			if len(spans) != 1 || spans[0].Kind != syntax.Comment {
				t.Fatalf("lost full-file context for %q: %#v", row, spans)
			}
			found++
		}
	}
	if found != 2 {
		t.Fatalf("checked %d lines", found)
	}
}
