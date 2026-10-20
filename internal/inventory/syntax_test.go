package inventory

import (
	"context"
	"prui/internal/source"
	"prui/internal/syntax"
	"prui/internal/testutil"
	"reflect"
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

func TestSyntaxPrefixRetainsEveryHunkAndChargesFullBytes(t *testing.T) {
	f := FileChange{OldPath: []byte("x.go"), NewPath: []byte("x.go")}
	old := []byte("package p\n/* multi\nline comment\n*/\n" + strings.Repeat("const Value = 1\n", 30))
	new := []byte(strings.ReplaceAll(string(old), "Value = 1", "Value = 2"))
	hunks := []hunk{
		{old: Range{Start: 2, Count: 2}, newRange: Range{Start: 2, Count: 2}, patch: []byte("@@ -2,2 +2,2 @@\n /* multi\n line comment\n")},
		{old: Range{Start: 12, Count: 1}, newRange: Range{Start: 12, Count: 1}, patch: []byte("@@ -12 +12 @@\n-const Value = 1\n+const Value = 2\n")},
	}
	fullBudget, limitedBudget := syntaxBudget{remaining: SyntaxDuration}, syntaxBudget{remaining: SyntaxDuration}
	a, b, status := fullBudget.capture(context.Background(), f, old, new)
	x, y, limitedStatus := limitedBudget.capture(context.Background(), f, old, new, hunks...)
	if status != limitedStatus || limitedStatus.Old != syntax.Ready || limitedStatus.New != syntax.Ready {
		t.Fatal(status, limitedStatus)
	}
	if len(x[12]) == 0 || len(x[13]) != 0 || len(y[13]) != 0 {
		t.Fatal("prefix boundary lost or exceeded")
	}
	for _, h := range hunks {
		if !reflect.DeepEqual(hunkSyntax(h, a, b), hunkSyntax(h, x, y)) {
			t.Fatal("displayed hunk syntax changed")
		}
	}
	if limitedBudget.bytes != len(old)+len(new) {
		t.Fatal("prefix bypassed full-source byte budget")
	}
}

func TestSyntaxPrefixDoesNotMaterializeUnusedTailSpans(t *testing.T) {
	r := testutil.NewRepo(t)
	tail := strings.Repeat("1\n", syntax.MaxSpans+1)
	r.Write("x.ts", "const x = 1;\n"+tail)
	base := r.Commit()
	r.Write("x.ts", "const x = 2;\n"+tail)
	head := r.Commit()
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	inv, err := BuildCommit(context.Background(), v, CommitComparison{base, head}, source.Defaults())
	if err != nil || !inv.Complete || len(inv.Files) != 1 {
		t.Fatal(err)
	}
	status := inv.SyntaxStatus[inv.Files[0].ID]
	if status.Old != syntax.Ready || status.New != syntax.Ready {
		t.Fatal(status)
	}
	if len(inv.Syntax) == 0 {
		t.Fatal("displayed source lost highlighting")
	}
}

func TestSyntaxPrefixUnsafeEndpointsKeepFullCapture(t *testing.T) {
	for _, r := range []Range{{Start: -1, Count: 1}, {Start: 1, Count: -1}, {Start: int(^uint(0) >> 1), Count: 2}} {
		a, b := syntaxHunkEnds([]hunk{{old: r, newRange: Range{Start: 1, Count: 2}}})
		if a != 0 || b != 0 {
			t.Fatal("unsafe endpoint did not keep full capture")
		}
	}
}
