package tui

import (
	"context"
	"strings"
	"testing"
)

func TestInlineEditorWrapsWithoutChangingDraft(t *testing.T) {
	for _, indent := range []int{0, 2} {
		for _, draft := range []string{strings.Repeat("long comment ", 12), strings.Repeat("x", 160), strings.Repeat("界é", 60) + "\nlast line"} {
			m := New(context.Background(), nil)
			m.Width = 50
			m.editorCursorVisible = true
			for _, cursor := range []int{0, len([]rune(draft)) / 2, len([]rune(draft))} {
				lines := m.inlineEditorLinesFor(draft, cursor, indent)
				prefix := strings.Repeat(" ", 2+indent) + "| "
				var content strings.Builder
				for _, line := range lines[1 : len(lines)-1] {
					if visibleWidth(line.Text) > visibleWidth(lines[0].Text) {
						t.Fatalf("row exceeds editor width: %q", line.Text)
					}
					content.WriteString(strings.TrimSuffix(strings.TrimPrefix(line.Text, prefix), " |"))
				}
				got := content.String()
				if strings.Count(got, "▏") != 1 {
					t.Fatalf("caret lost: %q", got)
				}
				compact := func(s string) string { return strings.NewReplacer(" ", "", "\n", "", "▏", "").Replace(s) }
				if compact(got) != compact(draft) {
					t.Fatalf("wrapped editor lost text: %q", got)
				}
				if len(lines) <= strings.Count(draft, "\n")+3 {
					t.Fatal("long draft did not wrap")
				}
			}
		}
	}
}
