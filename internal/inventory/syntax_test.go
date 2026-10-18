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

func TestSyntaxBudgetReportsReasons(t *testing.T) {
	f := FileChange{OldPath: []byte("x.ts"), NewPath: []byte("x.ts")}
	for _, tc := range []struct {
		name   string
		budget syntaxBudget
		want   syntax.Status
	}{
		{"time", syntaxBudget{}, syntax.InventoryTimeLimit},
		{"bytes", syntaxBudget{remaining: SyntaxDuration, bytes: 4 << 20}, syntax.InventoryByteLimit},
		{"spans", syntaxBudget{remaining: SyntaxDuration, spans: 32000}, syntax.InventorySpanLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cached := syntax.WithCache(context.Background())
			for _, text := range []string{"const x=1;\n", "const x=2;\n"} {
				if _, status := syntax.TokenizeWithStatus(cached, "x.ts", []byte(text)); status != syntax.Ready {
					t.Fatalf("cache warmup unavailable: %v", status)
				}
			}
			for _, ctx := range []context.Context{context.Background(), cached} {
				budget := tc.budget
				old, new, status := budget.capture(ctx, f, []byte("const x=1;\n"), []byte("const x=2;\n"))
				if old != nil || new != nil || status.Old != tc.want || status.New != tc.want {
					t.Fatalf("silent budget omission: %#v", status)
				}
			}
		})
	}
}

func TestCaptureSyntaxFailureDoesNotHideSourceOrLaterFile(t *testing.T) {
	r := testutil.NewRepo(t)
	large := strings.Repeat("// unchanged\n", 22000)
	r.Write("a.ts", large+"export const x = 1;\n")
	r.Write("b.ts", "export const x = 1;\n")
	base := r.Commit()
	r.Write("a.ts", large+"export const x = 2;\n")
	r.Write("b.ts", "export const x = 2;\n")
	head := r.Commit()
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	inv, err := BuildCommit(context.Background(), v, CommitComparison{base, head}, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if !inv.Complete || len(inv.Problems) > 0 {
		t.Fatal("optional syntax failure changed source completeness")
	}
	for _, f := range inv.Files {
		status := inv.SyntaxStatus[f.ID]
		switch string(f.NewPath) {
		case "a.ts":
			if status.Old != syntax.ByteLimit || status.New != syntax.ByteLimit {
				t.Fatalf("missing size reason: %#v", status)
			}
		case "b.ts":
			if status.Old != syntax.Ready || status.New != syntax.Ready {
				t.Fatalf("later file starved: %#v", status)
			}
		}
	}
	for _, u := range inv.Units {
		if u.Kind != TextHunk {
			continue
		}
		if len(inv.Patches[u.PatchReference]) == 0 {
			t.Fatal("source lost")
		}
	}
}

func TestLegacySyntaxStorageOmissionIsDiagnosed(t *testing.T) {
	units := []ReviewUnit{{ID: "u", FileChangeID: "f", Kind: TextHunk}}
	tokens := map[string]syntax.Patch{"u": {}}
	got := SyntaxStorageStatuses(units, tokens, nil)
	if got["f"].Old != syntax.StorageLimit || got["f"].New != syntax.StorageLimit {
		t.Fatalf("silent legacy omission: %#v", got)
	}
	if tokens["u"] == nil {
		t.Fatal("mutated original metadata")
	}
}
