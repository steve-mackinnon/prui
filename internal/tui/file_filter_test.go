package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"strings"
	"testing"
)

func TestFileFilterMatchesPathsAndNavigatesResults(t *testing.T) {
	m := largeModel(largeTextSession(3, 3), 120, 12)
	m.Focus = paneList
	m.Session.Inventory.Files[0].NewPath = []byte("src/main.go")
	m.Session.Inventory.Files[1].NewPath = []byte("test/helper.go")
	m.Session.Inventory.Files[2].NewPath = []byte("src/other.go")
	key(m, '/')
	for _, r := range "SRC/" {
		key(m, r)
	}
	lines, _ := m.reviewListPresentation()
	for _, line := range lines {
		if strings.Contains(line.text, "helper.go") {
			t.Fatal("nonmatching file visible")
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	key(m, 'j')
	if m.Session.UnitFiles[m.Selected] != 2 {
		t.Fatal("navigation did not skip filtered file")
	}
	key(m, '/')
	key(m, 'q')
	if !strings.Contains(m.View().Content, "No matching files") {
		t.Fatal("missing empty state")
	}
}

func TestFileRailSeparatesFilenameAndDirectory(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 12)
	m.Session.Inventory.Files[0].NewPath = []byte("internal/tui/model.go")
	lines, _ := m.reviewListPresentation()
	if len(lines) != 1 || !strings.Contains(lines[0].text, "model.go  internal/tui") || lines[0].mutedFrom <= 0 {
		t.Fatalf("filename and muted directory not inline: %#v", lines)
	}
}

func TestFileFilterAcceptsShortcutsAsTextAndClears(t *testing.T) {
	m := largeModel(largeTextSession(3, 3), 120, 12)
	m.Session.Inventory.Files[1].OldPath = []byte("old/日本.go")
	m.Session.Inventory.Files[1].NewPath = []byte("new/renamed.go")
	key(m, '/')
	for _, r := range "日本" {
		key(m, r)
	}
	if m.Session.UnitFiles[m.Selected] != 1 {
		t.Fatal("old rename path did not match")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.fileFilter != "日" {
		t.Fatal("backspace split a Unicode rune")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.fileFilter != "" || m.fileFilterEditing {
		t.Fatal("Escape did not clear filter")
	}
	key(m, '/')
	key(m, 'q')
	if m.fileFilter != "q" {
		t.Fatal("quit shortcut was not entered as filter text")
	}
}

func TestFileFilterMouseSelectsDisplayedIdentity(t *testing.T) {
	m := largeModel(largeTextSession(3, 3), 120, 12)
	m.Session.Inventory.Files[0].NewPath = []byte("src/a.go")
	m.Session.Inventory.Files[1].NewPath = []byte("test/b.go")
	m.Session.Inventory.Files[2].NewPath = []byte("src/c.go")
	key(m, '/')
	for _, r := range "src" {
		key(m, r)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	lines, _ := m.reviewListPresentation()
	for i, line := range lines {
		if line.row == 2 {
			m.mouseReviewClick(3, 3+i)
			if m.Session.UnitFiles[m.Selected] != 2 {
				t.Fatal("click selected unfiltered index")
			}
			return
		}
	}
	t.Fatal("matching file not visible")
}

func TestInlineFilePathUsesMutedStyleWhileSelected(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 12)
	m.Session.Inventory.Files[0].NewPath = []byte("internal/tui/model.go")
	m.Focus = paneList
	m.styles = map[lineClass]lipgloss.Style{
		classSelectionFocused: lipgloss.NewStyle().Transform(func(s string) string { return "PRIMARY:" + s }),
		classMetadata:         lipgloss.NewStyle().Transform(func(s string) string { return "MUTED:" + s }),
	}
	if got := m.View().Content; !strings.Contains(got, "PRIMARY:› model.go  MUTED:internal/tui") {
		t.Fatalf("missing distinct path style: %s", got)
	}
}

func TestFileFilterHeaderClickOpensInput(t *testing.T) {
	for _, width := range []int{60, 120} {
		m := largeModel(largeTextSession(3, 3), width, 12)
		m.Focus = paneDiff
		if !strings.Contains(m.View().Content, "Filter (/)") {
			t.Fatal("missing filter control")
		}
		// Click the hotkey end of the control, including when the narrow UI shows the diff.
		header := strings.Split(m.View().Content, "\n")[2]
		prefix, _, _ := strings.Cut(header, "Filter")
		x := visibleWidth(prefix)
		m.mouseReviewClick(x, 2)
		if !m.fileFilterEditing || m.Focus != paneList {
			t.Fatal("header click did not focus filter")
		}
		key(m, 'q')
		if m.fileFilter != "q" {
			t.Fatal("click did not route typing to filter")
		}
	}
}
