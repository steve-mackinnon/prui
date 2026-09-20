package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/source"
)

func TestCommentComposerViewRendersFrozenTargetEscapedMultilineDraftAndControls(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.Composer = &commentComposer{
		Target: source.ReviewCommentTarget{Path: "dir/unsafe\x1b[file.go", Side: "RIGHT", Line: 42},
		Draft:  "first line\nsecond\x1b[31m line",
		Cursor: len([]rune("first ")),
	}
	m.editorCursorVisible = true
	view := ansi.Strip(strings.Join(func() []string {
		lines := m.inlineEditorLines()
		out := make([]string, len(lines))
		for i := range lines {
			out[i] = lines[i].Text
		}
		return out
	}(), "\n"))
	for _, want := range []string{
		"+",
		"first ▏line",
		"second\\x1b[31m line",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("composer view missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"[comment", "ctrl+enter", "esc: discard"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("editor chrome unexpectedly contains %q:\n%s", unwanted, view)
		}
	}
}

func TestCommentBindingsAdvertiseComposerEntryAndControls(t *testing.T) {
	help := renderHealth()
	for _, want := range []string{
		"enter: focus the diff; on a commentable line, open an inline comment editor",
		"enter: submit inline comment editor",
		"shift+enter: newline in inline comment editor",
		"backspace/delete: delete previous/following rune in comment editor",
		"c: refresh ephemeral inline review comments",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}
