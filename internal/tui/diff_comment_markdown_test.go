package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

const diffCommentMarkdownFixture = `<!-- devin-review-comment {"id":"opaque-machine-id"} -->
🟡 **Valid room builds fail after upload**

When a build contains ` + "`.nova/`" + ` files, ` + "`buildRefArchive`" + ` rejects it after upload.

<details><summary>Learn more</summary>

The upload validator accepts every safe path. **Example:** An agent uploads files, then the archive builder throws.

</details>
<!-- devin-review-badge-begin -->
<a href="https://example.com/review/973"><picture><source media="(prefers-color-scheme: dark)" srcset="https://example.com/dark.svg"><img src="https://example.com/light.svg" alt="Devin Review"></picture></a>
<!-- devin-review-badge-end -->`

func TestDiffCommentMarkdownReadableAtCardWidth(t *testing.T) {
	for _, width := range []int{64, 160} {
		for _, indent := range []int{0, 4, 8} {
			t.Run(fmt.Sprintf("width%d/indent%d", width, indent), func(t *testing.T) {
				m := largeModel(screenSession(), width, 40)
				defer m.Close()
				comment := source.ReviewComment{ID: 42, Author: "reviewer", Body: diffCommentMarkdownFixture}
				m.Comments = []source.ReviewComment{comment}
				rows := m.reviewCommentLinesAt(comment, indent)
				var text strings.Builder
				for _, row := range rows {
					if row.commentID != comment.ID {
						t.Fatalf("card row lost comment identity: %+v", row)
					}
					if got, want := visibleWidth(row.Text), 2+indent+m.overlayInnerWidth(indent)+4; got != want {
						t.Fatalf("row width %d, want %d: %q", got, want, ansi.Strip(row.Text))
					}
					text.WriteString(ansi.Strip(row.Text))
					text.WriteByte('\n')
				}
				out := text.String()
				for _, want := range []string{"Valid room", "buildRefArchive", "Learn more", "The upload validator", "Devin Review"} {
					if !strings.Contains(out, want) {
						t.Errorf("missing %q in card:\n%s", want, out)
					}
				}
				for _, bad := range []string{"opaque-machine-id", "<details>", "<summary>", "<picture>", "srcset", "**", "`buildRefArchive`", "Enter: expand"} {
					if strings.Contains(out, bad) {
						t.Errorf("card leaked %q:\n%s", bad, out)
					}
				}
				if m.Comments[0].Body != diffCommentMarkdownFixture {
					t.Fatal("rendering changed raw body")
				}
			})
		}
	}
}

func TestDiffCommentMarkdownPreservesCodeHTML(t *testing.T) {
	m := largeModel(screenSession(), 160, 40)
	defer m.Close()
	comment := source.ReviewComment{ID: 42, Body: "Inline `<details>`.\n\n```html\n<!-- fenced -->\n<img src=\"example\">\n```\n\n    <!-- indented -->\n"}
	var text strings.Builder
	for _, row := range m.reviewCommentLines(comment) {
		text.WriteString(ansi.Strip(row.Text))
		text.WriteByte('\n')
	}
	for _, want := range []string{"<details>", "<!-- fenced -->", `<img src="example">`, "<!-- indented -->"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("code lost literal %q:\n%s", want, text.String())
		}
	}
}

func TestDiffCommentMarkdownHorizontalCropPreservesStyle(t *testing.T) {
	m := largeModel(screenSession(), 160, 40)
	defer m.Close()
	row := diffLine{styledLine: styledLine{Class: classPlain, Text: "  | \x1b[1mBold\x1b[0m <literal> |"}, commentID: 42}
	got := m.syntaxText(row, 5, 30, "› ")
	if want := "› old <literal> |"; ansi.Strip(got) != want {
		t.Fatalf("cropped card = %q, want %q", ansi.Strip(got), want)
	}
	canvas := themeCanvasBuffer(got, visibleWidth(got), 1)
	if canvas.CellAt(2, 0).Style.Attrs == 0 {
		t.Fatal("cropping lost trusted emphasis")
	}
}
