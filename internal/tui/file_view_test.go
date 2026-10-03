package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"prui/internal/inventory"
)

func TestFileViewConcatenatesEveryHunkOnce(t *testing.T) {
	s := largeSession(2, 4)
	s.Inventory.Patches = map[string][]byte{}
	for i := range s.Inventory.Units {
		ref := s.Inventory.Units[i].ID
		s.Inventory.Units[i].Kind = inventory.TextHunk
		s.Inventory.Units[i].PatchReference = ref
		s.Inventory.Patches[ref] = []byte("@@ -1 +1 @@\n-removed " + ref + "\n+added " + ref + "\n")
	}
	m := largeModel(s, 120, 6)
	lines := m.baseDetail()
	var text strings.Builder
	for _, line := range lines {
		text.WriteString(line.Text)
		text.WriteByte('\n')
	}
	got := text.String()
	for i := range s.Inventory.Units {
		ref := s.Inventory.Units[i].ID
		if strings.Count(got, "added "+ref) != 1 {
			t.Fatalf("hunk %s missing or duplicated in file view:\n%s", ref, got)
		}
	}
	for _, file := range s.Inventory.Files {
		if strings.Count(got, fileDivider(file)) != 1 {
			t.Fatalf("file %s missing or duplicated boundary", file.ID)
		}
	}
}

func TestFileViewNavigationHasOneStopPerFile(t *testing.T) {
	m := largeModel(largeTextSession(3, 6), 120, 8)
	m.Focus = paneList
	for file := 1; file < 3; file++ {
		key(m, 'j')
		if got := m.Session.UnitFiles[m.Selected]; got != file {
			t.Fatalf("j selected file %d, want %d", got, file)
		}
		if got := m.offset(); got != m.fileOffset(file) {
			t.Fatalf("j positioned diff at %d, want file boundary %d", got, m.fileOffset(file))
		}
	}
	key(m, 'j')
	if got := m.Session.UnitFiles[m.Selected]; got != 2 {
		t.Fatalf("j advanced beyond final file to %d", got)
	}
	key(m, 'k')
	if got := m.Session.UnitFiles[m.Selected]; got != 1 {
		t.Fatalf("k selected file %d, want 1", got)
	}
	key(m, 'J')
	if got := m.offset(); got <= m.fileOffset(1) {
		t.Fatalf("J did not scroll within continuous diff: %d", got)
	}
}

func TestFileViewJKMovesCursorBeforeScrollingFocusedDiff(t *testing.T) {
	m := largeModel(largeTextSession(2, 4), 120, 12)
	m.Focus = paneDiff
	m.cursorActive = true
	m.setOffset(m.cursor())
	firstFile := m.Selected
	firstCursor, firstOffset := m.cursor(), m.offset()
	key(m, 'j')
	if m.cursor() <= firstCursor || m.offset() != firstOffset {
		t.Fatalf("j moved cursor/offset to %d/%d, want cursor past %d with offset %d", m.cursor(), m.offset(), firstCursor, firstOffset)
	}
	if m.Selected != firstFile {
		t.Fatalf("j jumped from file %d to %d", firstFile, m.Selected)
	}
	for m.cursor() < firstOffset+m.bodyHeight() {
		key(m, 'j')
	}
	if m.offset() == firstOffset {
		t.Fatal("j did not scroll after cursor passed the bottom visible row")
	}
	boundaryOffset := m.offset()
	key(m, 'k')
	if m.offset() != boundaryOffset {
		t.Fatalf("k scrolled before cursor passed the top visible row: %d", m.offset())
	}
	for m.cursor() >= boundaryOffset {
		key(m, 'k')
	}
	if m.offset() >= boundaryOffset {
		t.Fatal("k did not scroll after cursor passed the top visible row")
	}
}

func TestFileViewScrollingContinuesIntoNextFile(t *testing.T) {
	m := largeModel(largeTextSession(2, 4), 120, 8)
	m.Focus = paneDiff
	boundary := m.fileOffset(1)
	for m.offset() < boundary {
		key(m, 'J')
	}
	if got := m.Session.UnitFiles[m.Selected]; got != 1 {
		t.Fatalf("scroll reached second file but selected file %d", got)
	}
	if !strings.Contains(m.View().Content, fileDivider(m.Session.Inventory.Files[1])) {
		t.Fatal("second file diff was not visible after scrolling")
	}
}

func TestSideBySideFileViewJumpUsesProjectedBoundary(t *testing.T) {
	m := largeModel(largeTextSession(2, 4), 160, 8)
	m.Focus = paneList
	key(m, 'S')
	key(m, 'j')
	if got, want := m.offset(), m.fileOffset(1); got != want {
		t.Fatalf("side-by-side file jump offset = %d, want %d", got, want)
	}
	if !strings.Contains(m.View().Content, fileDivider(m.Session.Inventory.Files[1])) {
		t.Fatal("side-by-side view did not show second file boundary")
	}
}

func TestFileViewCommentCursorUpdatesSelectedFile(t *testing.T) {
	m := largeModel(largeTextSession(2, 2), 120, 8)
	m.Focus = paneDiff
	m.cursorActive = true
	for i := 0; i < len(m.displayDetail()); i++ {
		if m.Session.UnitFiles[m.Selected] == 1 {
			if target := m.displayDetail()[m.cursor()].target; target == nil || target.Path != "file-1" {
				t.Fatalf("selected file followed a different cursor target: %#v", target)
			}
			return
		}
		key(m, 'n')
	}
	t.Fatal("comment cursor reached the next file without updating selection")
}

func TestFilePickerResetsCursorWithinSelectedFile(t *testing.T) {
	for _, width := range []int{120, 180} {
		m := largeModel(largeTextSession(3, 3), width, 1000)
		if width == 180 {
			key(m, 'S')
		}
		m.Focus, m.cursorActive = paneList, true
		for i, line := range m.displayDetail() {
			if i > m.fileOffset(2) && line.target != nil {
				m.setCursor(i)
				break
			}
		}
		m.file(1)
		if got := m.cursor(); got < m.fileOffset(1) || got >= m.fileOffset(2) {
			t.Fatalf("width %d: selected file 1 but cursor is at %d", width, got)
		}
	}
}

func TestActiveFileHeaderFollowsPickerAndCursor(t *testing.T) {
	for _, width := range []int{120, 180} {
		m := largeModel(largeTextSession(3, 3), width, 1000)
		if width == 180 {
			key(m, 'S')
		}
		m.styles = map[lineClass]lipgloss.Style{classSelection: lipgloss.NewStyle().Transform(func(s string) string { return "ACTIVE:" + s })}
		m.Focus = paneList
		m.file(1)
		assertHeader := func(file int) {
			t.Helper()
			got := m.View().Content
			for i, f := range m.Session.Inventory.Files {
				highlighted := false
				for _, line := range strings.Split(got, "\n") {
					cells := strings.Split(line, "│")
					if len(cells) > 2 && strings.Contains(cells[2], "ACTIVE:") && strings.Contains(cells[2], fileDivider(f)) {
						highlighted = true
					}
				}
				if highlighted != (i == file) {
					t.Fatalf("width %d: header %d highlighted=%v, active file=%d", width, i, highlighted, file)
				}
			}
		}
		assertHeader(1)
		m.Focus, m.cursorActive = paneDiff, true
		for i, line := range m.displayDetail() {
			if i > m.fileOffset(2) && line.target != nil {
				m.setCursor(i)
				break
			}
		}
		assertHeader(2)
		m.Focus = paneList
		assertHeader(1)
	}
}

func TestFilenameStaysAtTopWhileScrollingDiff(t *testing.T) {
	for _, width := range []int{120, 180} {
		m := largeModel(largeTextSession(2, 10), width, 14)
		if width == 180 {
			key(m, 'S')
		}
		m.Focus, m.cursorActive = paneDiff, true
		for file := 0; file < 2; file++ {
			m.setOffset(m.fileOffset(file) + 3)
			m.setCursor(m.offset())
			m.ensureCursorVisible()
			view := strings.Split(ansi.Strip(m.View().Content), "\n")
			if !strings.Contains(view[3], fileDivider(m.Session.Inventory.Files[file])) {
				t.Fatalf("width %d: filename missing from top diff row: %s", width, view[3])
			}
			right := strings.Split(view[4], "│")
			if len(right) < 3 || !strings.Contains(strings.Join(right[2:], "│"), cursorMarker(true)) {
				t.Fatalf("width %d: sticky header hid cursor: %s", width, view[4])
			}
		}
	}
}
