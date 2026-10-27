package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
	"prui/internal/theme"
)

func TestDiffCommentMarkdownSurvivesSelectedCardPainting(t *testing.T) {
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		for _, themeName := range []string{theme.CatppuccinMocha, "gh-light"} {
			t.Run(map[diffLayout]string{diffLayoutUnified: "unified", diffLayoutSideBySide: "split"}[layout]+"/"+themeName, func(t *testing.T) {
				m := rangeTestModel()
				m.Width, m.Height = 160, 35
				t.Cleanup(m.Close)
				m.layout, m.Focus = layout, paneDiff
				palette, err := theme.Resolve(themeName, nil)
				if err != nil {
					t.Fatal(err)
				}
				m.SetTheme(palette)
				m.colorProfile = colorprofile.TrueColor
				var target source.ReviewCommentTarget
				for _, line := range m.baseDetail() {
					if line.target != nil {
						target = *line.target
						break
					}
				}
				if target.Path == "" {
					t.Fatal("fixture has no source target")
				}
				m.Comments = []source.ReviewComment{{ID: 42, Target: target, Author: "reviewer", Body: "ordinary **emphasis** and `inline_code` [document](https://example.com/read)"}}
				for i, line := range m.displayDetail() {
					if line.commentID == 42 && strings.Contains(ansi.Strip(line.Text), "emphasis") {
						m.setCursor(i)
						m.cursorActive = true
						break
					}
				}
				for _, horizontal := range []int{0, 3} {
					m.Horizontal = horizontal
					view := m.View().Content
					if strings.Contains(ansi.Strip(view), "**emphasis**") || horizontal == 0 && (!strings.Contains(view, "┌") || !strings.Contains(view, "└")) {
						t.Fatal("selected card lost Markdown rendering or solid border")
					}
					cells := themeCanvasBuffer(view, m.Width, m.Height)
					found := false
					for y, row := range strings.Split(ansi.Strip(view), "\n") {
						if !strings.Contains(row, "ordinary") || !strings.Contains(row, "emphasis") || !strings.Contains(row, "inline_code") {
							continue
						}
						found = true
						plainX := visibleWidth(row[:strings.Index(row, "ordinary")])
						boldX := visibleWidth(row[:strings.Index(row, "emphasis")])
						codeX := visibleWidth(row[:strings.Index(row, "inline_code")])
						linkX := visibleWidth(row[:strings.Index(row, "document")])
						if cells.CellAt(linkX, y).Link.URL != "https://example.com/read" {
							t.Fatal("final painter lost Markdown hyperlink")
						}
						if cells.CellAt(boldX, y).Style.Attrs&uv.AttrBold == 0 {
							t.Fatal("final painter removed Markdown bold")
						}
						if sameCanvasColor(cells.CellAt(plainX, y).Style.Fg, cells.CellAt(codeX, y).Style.Fg) {
							t.Fatal("final painter removed inline code color")
						}
						for _, x := range []int{plainX, boldX, codeX} {
							if cells.CellAt(x, y).Style.Attrs&uv.AttrReverse != 0 || !sameCanvasColor(cells.CellAt(x, y).Style.Bg, modalBackground(palette)) {
								t.Fatal("selection changed Markdown body surface")
							}
						}
					}
					if !found {
						t.Fatal("formatted body missing from final View")
					}
				}
			})
		}
	}
}

func TestDiffCommentMarkdownRowsKeepNestedReplyAndFileTargets(t *testing.T) {
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		for _, fileComment := range []bool{false, true} {
			name := map[diffLayout]string{diffLayoutUnified: "unified", diffLayoutSideBySide: "split"}[layout] + map[bool]string{false: "/line", true: "/file"}[fileComment]
			t.Run(name, func(t *testing.T) {
				m := rangeTestModel()
				t.Cleanup(m.Close)
				m.layout, m.Height = layout, 45
				selectRawTarget(t, m, "RIGHT", 2)
				target := *m.selectedDiffTarget()
				if fileComment {
					target.SubjectType, target.Side, target.Line = "file", "", 0
				}
				body := "<!-- hidden metadata -->\n**root body**\n\n<details><summary>Learn more</summary>\n\nroot explanation\n</details>"
				reply := "<strong>nested reply</strong>\n\nreply explanation"
				m.Comments = []source.ReviewComment{{ID: 7, Target: target, Body: body}, {ID: 8, ParentID: 7, Target: target, Body: reply}}
				m.reactionEmoji = false
				m.CommentReactions = map[int64][]source.ReviewCommentReaction{8: {{Content: "+1"}, {Content: "+1"}}}
				seen, rootText, replyText := map[int64]int{}, "", ""
				for i, row := range m.displayDetail() {
					if row.commentID == 0 {
						continue
					}
					seen[row.commentID]++
					if row.target != nil || row.sideBySide != nil && len(rowTargets(*row.sideBySide)) != 0 {
						t.Fatal("formatted comment row became a source target")
					}
					switch row.commentID {
					case 7:
						rootText += ansi.Strip(row.Text) + "\n"
					case 8:
						replyText += ansi.Strip(row.Text) + "\n"
						if strings.Contains(ansi.Strip(row.Text), "reply explanation") {
							m.setCursor(i)
						}
					}
				}
				if seen[7] == 0 || seen[8] == 0 || !strings.Contains(rootText, "root explanation") || !strings.Contains(replyText, "nested reply") || !strings.Contains(replyText, "[+1 2]") {
					t.Fatalf("formatted thread or reactions missing:\n%s\n%s", rootText, replyText)
				}
				if strings.Contains(rootText+replyText, "hidden metadata") || strings.Contains(rootText+replyText, "<strong>") || strings.Contains(rootText, "<details>") {
					t.Fatal("raw HTML leaked into thread")
				}
				if !m.openCommentActionMenu() || m.CommentMenu.CommentID != 8 || m.CommentMenu.ReplyToID != 7 || m.CommentMenu.Target != target {
					t.Fatalf("formatted reply lost action identity: %#v", m.CommentMenu)
				}
				key(m, 'r')
				if m.CommentMenu.mode != commentActionReply || m.CommentMenu.Target != target {
					t.Fatal("reply editor lost exact target")
				}
				if m.Comments[0].Body != body || m.Comments[1].Body != reply {
					t.Fatal("presentation changed stored source bodies")
				}
			})
		}
	}
}

func TestDiffCommentMarkdownPreservesPublishedSuggestionPreview(t *testing.T) {
	m := rangeTestModel()
	t.Cleanup(m.Close)
	selectRawTarget(t, m, "RIGHT", 2)
	target := *m.selectedDiffTarget()
	body, err := source.SuggestionBody("<strong>literal replacement</strong>")
	if err != nil {
		t.Fatal(err)
	}
	comment := source.ReviewComment{ID: 9, Target: target, Body: body}
	m.Comments = []source.ReviewComment{comment}
	var text strings.Builder
	for _, row := range m.reviewCommentLines(comment) {
		if row.commentID != 9 || row.target != nil {
			t.Fatal("suggestion card lost comment-only identity")
		}
		text.WriteString(ansi.Strip(row.Text) + "\n")
	}
	for _, want := range []string{"Suggestion · Before", "- second", "+ <strong>literal replacement</strong>", "ctrl+a in comment actions: apply suggestion"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("suggestion preview lost %q:\n%s", want, text.String())
		}
	}
	if m.Comments[0].Body != body {
		t.Fatal("Markdown presentation changed application source")
	}
}

func TestDiffCommentMarkdownHistoricalBodyOpensOriginalDiscussion(t *testing.T) {
	s, snapshot := historicalCommentFixture()
	snapshot.Snapshot.Threads[0].Comments[0].Body = "<details><summary>Earlier finding</summary>\n\n**historical explanation**\n</details>"
	m := largeModel(s, 160, 35)
	t.Cleanup(m.Close)
	m.openReviewTab(s)
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: s, Snapshot: snapshot})
	m.Focus = paneDiff
	for i, row := range m.displayDetail() {
		if row.commentID != 7 || !strings.Contains(ansi.Strip(row.Text), "historical explanation") {
			continue
		}
		if strings.Contains(row.Text, "**") || row.target != nil {
			t.Fatal("historical presentation leaked Markdown or source target")
		}
		m.setCursor(i)
		if !m.openCommentActionMenu() || m.top() != pageReview || m.selectedReviewView() != viewDescription || !m.discussions.overviewExpanded[m.discussions.selectedID] || m.discussions.selectedID != "old-thread" || m.CommentMenu != nil {
			t.Fatal("formatted historical row lost original discussion navigation")
		}
		return
	}
	t.Fatal("formatted historical explanation not rendered inline")
}

func TestDiffCommentMarkdownASCIIOutputKeepsReadableContent(t *testing.T) {
	m := rangeTestModel()
	t.Cleanup(m.Close)
	m.colorProfile = colorprofile.Ascii
	var body strings.Builder
	for _, row := range m.reviewCommentLines(source.ReviewComment{ID: 7, Body: "<!-- hidden -->\n<strong>visible body</strong> and [document](https://example.com)"}) {
		if strings.ContainsRune(row.Text, '\x1b') {
			t.Fatal("ASCII comment contains renderer control sequences")
		}
		body.WriteString(row.Text + "\n")
	}
	if !strings.Contains(body.String(), "visible body") || !strings.Contains(body.String(), "document") || strings.Contains(body.String(), "<strong>") || strings.Contains(body.String(), "hidden") {
		t.Fatalf("ASCII card lost readable conversion:\n%s", body.String())
	}
}
