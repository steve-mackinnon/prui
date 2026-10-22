package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestInlineEditorCaretDoesNotInsertSpaceInDraft(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width = 50
	for _, visible := range []bool{false, true} {
		m.editorCursorVisible = visible
		for _, draft := range []string{"the vercel cdn", strings.Repeat("x", m.overlayInnerWidth(0)), "世界 café", "e\u0301clair"} {
			for cursor := 0; cursor < len([]rune(draft)); cursor++ {
				rows := m.inlineEditorLinesFor(draft, cursor, 0)
				want := "  | " + draft + strings.Repeat(" ", m.overlayInnerWidth(0)-visibleWidth(draft)) + " |"
				if len(rows) != 3 || rows[1].Text != want {
					t.Fatalf("visible=%v cursor=%d draft=%q: editor added spacing or wrapping: %#v", visible, cursor, draft, rows)
				}
			}
		}
	}
}

func TestInlineEditorCaretPresentationPreservesText(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width = 50
	for _, draft := range []string{"the vercel cdn", "世界 café", "e\u0301clair", "safe\x1b[31m text", strings.Repeat("x", 80)} {
		for cursor := 0; cursor < len([]rune(draft)); cursor++ {
			m.editorCursorVisible = true
			shown := m.inlineEditorLinesFor(draft, cursor, 0)
			m.editorCursorVisible = false
			hidden := m.inlineEditorLinesFor(draft, cursor, 0)
			highlights := 0
			for i, row := range shown {
				// Exercise the unified and split presentation paths, including panning.
				for _, pan := range []int{0, 3} {
					a := m.presentUnifiedDiffLine(row, pan, 48, "  ").Text
					b := m.presentUnifiedDiffLine(hidden[i], pan, 48, "  ").Text
					if ansi.Strip(a) != ansi.Strip(b) {
						t.Fatalf("blink changed unified text: %q vs %q", a, b)
					}
				}
				a := m.renderSideBySideViewport([]diffLine{row}, 48, 0, -1, nil)[0].Text
				b := m.renderSideBySideViewport([]diffLine{hidden[i]}, 48, 0, -1, nil)[0].Text
				if ansi.Strip(a) != ansi.Strip(b) {
					t.Fatalf("blink changed split text: %q vs %q", a, b)
				}
				highlights += strings.Count(a, "\x1b[7m")
			}
			if highlights != 1 {
				t.Fatalf("cursor=%d draft=%q: got %d cursor highlights", cursor, draft, highlights)
			}
		}
	}
}

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
				carets := strings.Count(got, "▏")
				for _, line := range lines {
					if line.editorCaret != nil {
						carets++
					}
				}
				if carets != 1 {
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

func TestInlineEditorBlinkKeepsTextAndWrappingStationary(t *testing.T) {
	for _, indent := range []int{0, 2} {
		m := New(context.Background(), nil)
		m.Width = 50
		inner := m.overlayInnerWidth(indent)
		for _, draft := range []string{
			"here is adjacent text",
			strings.Repeat("x", inner),
			strings.Repeat("long comment ", 12),
			strings.Repeat("界é", 60) + "\nlast line",
			"first\n\nlast\n",
			"",
		} {
			for cursor := 0; cursor <= len([]rune(draft)); cursor++ {
				m.editorCursorVisible = true
				shown := m.inlineEditorLinesFor(draft, cursor, indent)
				m.editorCursorVisible = false
				hidden := m.inlineEditorLinesFor(draft, cursor, indent)
				if len(shown) != len(hidden) {
					t.Fatalf("indent=%d cursor=%d draft=%q: blink changed row count from %d to %d", indent, cursor, draft, len(shown), len(hidden))
				}
				for i := range shown {
					want := strings.ReplaceAll(shown[i].Text, "▏", " ")
					if hidden[i].Text != want {
						t.Fatalf("indent=%d cursor=%d draft=%q row=%d: blink moved text\nshown:  %q\nhidden: %q", indent, cursor, draft, i, shown[i].Text, hidden[i].Text)
					}
				}
			}
		}
	}
}
