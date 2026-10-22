package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func TestDiffCommentMarkdownScreenSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		layout diffLayout
	}{
		{"diff_comment_markdown_narrow", 64, diffLayoutUnified},
		{"diff_comment_markdown_split", 160, diffLayoutSideBySide},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := rangeTestModel()
			m.Width, m.Height = tc.width, 36
			t.Cleanup(m.Close)
			m.layout, m.Focus, m.cursorActive = tc.layout, paneDiff, true
			var target source.ReviewCommentTarget
			for _, row := range m.baseDetail() {
				if row.target != nil {
					target = *row.target
					break
				}
			}
			if target.Path == "" {
				t.Fatal("fixture has no comment anchor")
			}
			m.Comments = []source.ReviewComment{
				{ID: 42, Target: target, Author: "review-bot", Body: diffCommentMarkdownFixture},
				{ID: 43, ParentID: 42, Target: target, Author: "reviewer", Body: "**Confirmed.** Keep `<details>` literal in code."},
			}
			for i, row := range m.displayDetail() {
				if row.commentID == 42 {
					m.setCursor(i)
					break
				}
			}
			view := ansi.Strip(m.View().Content)
			if !strings.Contains(view, "Learn more") || !strings.Contains(view, "Devin Review") || !strings.Contains(view, "Confirmed.") {
				t.Fatal("screen lost comment content or nested reply", view)
			}
			lines := strings.Split(view, "\n")
			if len(lines) > m.Height {
				t.Fatal("screen exceeds terminal height")
			}
			for i, line := range lines {
				if visibleWidth(line) > tc.width {
					t.Fatalf("screen exceeds terminal width: %q", line)
				}
				lines[i] = strings.TrimRight(line, " ")
			}
			checkScreen(t, tc.name, strings.Join(lines, "\n")+"\n")
		})
	}
}
