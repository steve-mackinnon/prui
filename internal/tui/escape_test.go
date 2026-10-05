package tui

import (
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/syntax"
	"testing"
)

func TestEscapePreservesSourceAndSanitizesTerminalInput(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"directive", `"use client";`, `"use client";`},
		{"source escapes", `const s = "a\"b\\c";`, `const s = "a\"b\\c";`},
		{"unicode", "α café 🙂", "α café 🙂"},
		{"controls", "\t\n\r\x1b]52;c;payload\a", `\t\n\r\x1b]52;c;payload\a`},
		{"invalid bytes", string([]byte{0xff, 0xfe}), `\xff\xfe`},
		{"nonprinting unicode", "a\u202eb", `a\u202eb`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Escape(tc.input); got != tc.want {
				t.Fatalf("Escape(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDiffQuotesAndSyntaxOffsetsRemainSourceAccurate(t *testing.T) {
	raw := `const s = "a\b";`
	f := inventory.FileChange{NewPath: []byte("example.ts")}
	u := inventory.ReviewUnit{NewRange: inventory.Range{Start: 1, Count: 1}}
	tokens := syntax.Patch{1: {New: []syntax.Span{{Start: 10, End: 15, Kind: syntax.String}}}}
	rows := textHunkLines(f, u, []byte("@@ -0,0 +1 @@\n+"+raw+"\n"), source.Identity{}, "", tokens)
	if got := rows[1].Text; got != "+"+raw {
		t.Fatalf("diff source changed: %q", got)
	}
	span := rows[1].syntax[0]
	if got := rows[1].Text[span.Start:span.End]; got != `"a\b"` {
		t.Fatalf("syntax span points to %q", got)
	}
	if rows[1].target.Line != 1 || rows[1].target.Side != "RIGHT" {
		t.Fatalf("source target changed: %+v", rows[1].target)
	}
}
