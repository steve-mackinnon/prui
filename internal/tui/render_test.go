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
	m.Stack = []page{pageReview, pageCommentComposer}

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{
		"Leave line comment",
		"path: dir/unsafe\\x1b[file.go | side: RIGHT | line: 42",
		"first line",
		"second\\x1b[31m line",
		"enter: newline | ctrl+enter: submit | esc: discard",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("composer view missing %q:\n%s", want, view)
		}
	}
}

func TestCommentBindingsAdvertiseComposerEntryAndControls(t *testing.T) {
	help := renderHealth()
	for _, want := range []string{
		"enter: focus the diff; on a commentable line, open a comment composer",
		"enter: newline in comment composer",
		"ctrl+enter: submit comment composer",
		"esc: discard comment composer",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}
