package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCommitEditorRemainsVisibleAfterResize(t *testing.T) {
	for _, tc := range []struct {
		name  string
		draft string
	}{
		{"short", "resize draft"},
		{"tall", strings.Repeat("draft line\n", 12) + "active cursor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := commitModel(t)
			entry := &m.Session.Commits.Entries[0]
			entry.SHA = strings.Repeat("d", 40)
			m.commit.selectedSHA = entry.SHA
			m.commit.cache = commitRenderCache{}
			entry.Parents = []string{strings.Repeat("c", 40)}
			entry.Diff.Files[0].Status = "A"
			m.commit.focus = paneDiff
			m.commitCursor()
			for i, row := range m.commitRows() {
				if row.target != nil && row.target.Side == "RIGHT" {
					m.commit.cursors[m.commit.selectedSHA] = i
				}
			}
			m.openCommitComposer()
			if m.Composer == nil {
				t.Fatalf("could not open commit composer: %v", m.ActionError)
			}
			m.Composer.Draft = tc.draft
			m.Composer.Cursor = len([]rune(tc.draft))
			m.ensureCommitEditorVisible()
			m.Scroll[0] = 7
			m.Scroll[1] = 11

			for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 10}, {Width: 120, Height: 30}} {
				m.Update(size)
				rows := m.commitRows()
				first, last := -1, -1
				for i, row := range rows {
					if row.editor {
						if first < 0 {
							first = i
						}
						last = i
					}
				}
				if first < 0 {
					t.Fatal("commit editor disappeared")
				}
				offset, height := m.commitOffset(), m.bodyHeight()
				if last-first+1 <= height {
					if first < offset || last >= offset+height {
						t.Errorf("size %dx%d: editor rows %d–%d outside viewport %d–%d", size.Width, size.Height, first, last, offset, offset+height-1)
					}
				} else {
					cursorLine := first + 1 + strings.Count(tc.draft, "\n")
					if cursorLine < offset || cursorLine >= offset+height {
						t.Errorf("size %dx%d: cursor row %d outside viewport %d–%d", size.Width, size.Height, cursorLine, offset, offset+height-1)
					}
				}
				if m.Scroll[0] != 7 || m.Scroll[1] != 11 {
					t.Fatal("commit resize changed main-review scroll")
				}
			}
		})
	}
}
