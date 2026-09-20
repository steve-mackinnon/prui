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
	}
	view := ansi.Strip(strings.Join(func() []string {
		lines := m.inlineEditorLines()
		out := make([]string, len(lines))
		for i := range lines {
			out[i] = lines[i].Text
		}
		return out
	}(), "\n"))
	for _, want := range []string{
		"[comment RIGHT dir/unsafe\\x1b[file.go:42]",
		"first line",
		"second\\x1b[31m line",
		"enter: newline · ctrl+enter: submit · esc: discard",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("composer view missing %q:\n%s", want, view)
		}
	}
}

func TestCommentBindingsAdvertiseComposerEntryAndControls(t *testing.T) {
	help := renderHealth()
	for _, want := range []string{
		"enter: focus the diff; on a commentable line, open an inline comment editor",
		"enter: newline in inline comment editor",
		"ctrl+enter: submit inline comment editor",
		"backspace/delete: delete previous/following rune in comment editor",
		"c: refresh ephemeral inline review comments",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}
