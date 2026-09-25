package tui

import (
	"strings"
	"testing"

	"pr-review/internal/inventory"
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
	m.Focus = paneDiff
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
	m.Focus = paneDiff
	key(m, 'S')
	key(m, 'j')
	if got, want := m.offset(), m.fileOffset(1); got != want {
		t.Fatalf("side-by-side file jump offset = %d, want %d", got, want)
	}
	if !strings.Contains(m.View().Content, fileDivider(m.Session.Inventory.Files[1])) {
		t.Fatal("side-by-side view did not show second file boundary")
	}
}
