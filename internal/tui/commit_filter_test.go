package tui

import (
	tea "charm.land/bubbletea/v2"
	"crypto/sha1"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"image"
	"prui/internal/commits"
	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestCommitFilterSelectionAndCanonicalState(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	dedicatedSHA := m.commit.selectedSHA
	original := m.Session
	m.Selected = 1
	m.Scroll[1] = 7
	key(m, 'C')
	if !m.commitFilter.open || m.commitFilter.subset {
		t.Fatal("opening changed selection")
	}
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	if !m.commitFilter.subset || len(m.commitFilter.selected) != 1 {
		t.Fatal("first commit not selected")
	}
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	if m.commitFilter.subset {
		t.Fatal("complete membership did not reset")
	}
	if m.Session != original || m.Selected != 1 || m.Scroll[1] != 7 {
		t.Fatal("canonical state changed")
	}
	key(m, 'C')
	key(m, '4')
	if m.commit.selectedSHA != dedicatedSHA {
		t.Fatal("dedicated commit selection changed")
	}
}

func TestCommitFilterOverlaysExistingReview(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	m.Width, m.Height = 120, 20
	before := ansi.Strip(m.View().Content)
	key(m, 'C')
	opened := ansi.Strip(m.View().Content)
	if !strings.Contains(opened, "╭") || !strings.Contains(opened, "example/review #42") || !strings.Contains(opened, "Overview [1]") {
		t.Fatalf("picker replaced review instead of overlaying it:\n%s", opened)
	}
	key(m, 'C')
	if got := ansi.Strip(m.View().Content); got != before {
		t.Fatal("closing picker changed the review")
	}
}

func TestCommitFilterModalMouseCoordinates(t *testing.T) {
	for _, size := range [][2]int{{120, 20}, {60, 10}, {40, 6}} {
		m := commitModel(t)
		key(m, '2')
		m.Width, m.Height = size[0], size[1]
		key(m, 'C')
		m.commitFilterKey("end")
		card, choices := m.commitFilterPickerBounds()
		generation := m.commitFilter.generation
		// Background clicks and the card's footer cannot select a commit.
		for _, point := range []image.Point{{0, 3}, {card.Min.X + 2, card.Max.Y - 2}} {
			m.Update(tea.MouseClickMsg{X: point.X, Y: point.Y, Button: tea.MouseLeft})
		}
		if m.commitFilter.generation != generation {
			t.Fatal("background or footer click selected a commit")
		}
		index := m.commitFilter.focus - m.commitFilter.offset
		m.Update(tea.MouseClickMsg{X: choices.Min.X, Y: choices.Min.Y + index, Button: tea.MouseLeft})
		if !m.commitFilter.subset || !m.commitFilter.selected[strings.Repeat("b", 40)] {
			t.Fatalf("modal click missed selected row at %v", size)
		}
	}
}
func TestCommitFilterEmptyAndStaleResult(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	key(m, 'C')
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	generation := m.commitFilter.generation
	namedKey(m, tea.KeyEnter)
	if !m.commitFilter.subset || len(m.commitFilter.selected) != 0 {
		t.Fatal("empty selection reset")
	}
	m.Update(commitFilterResult{state: m.reviewTabState, session: m.Session, generation: generation, inventory: inventory.Inventory{Files: []inventory.FileChange{{ID: "stale"}}}})
	if len(m.commitFilter.inventory.Files) != 0 {
		t.Fatal("stale result accepted")
	}
	key(m, 'C')
	if !strings.Contains(ansi.Strip(m.View().Content), "No commits selected") {
		t.Fatal("empty surface missing")
	}
}

func TestCommitFilterCompositionReadonlyAndRestore(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	canonical := m.Session
	selected := m.Selected
	m.Focus = paneDiff
	m.Scroll[0] = 3
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	data := []byte("selected content\n")
	oid := fmt.Sprintf("%x", sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...)))
	file := inventory.FileChange{ID: "selected", NewPath: []byte("historical.go"), NewMode: "100644", NewOID: oid, Status: "A"}
	tree := []commits.TreeEntry{{Path: file.NewPath, Mode: file.NewMode, OID: oid}}
	canonical.Commits = &commits.Bundle{Status: commits.Captured, Complete: true, Entries: []commits.Entry{
		{SHA: a, Subject: "Add historical file", Status: commits.Captured, Diff: &commits.Diff{Complete: true, Files: []inventory.FileChange{file}}},
		{SHA: b, Subject: "Empty follow-up", Parents: []string{a}, Status: commits.Captured, Diff: &commits.Diff{Complete: true}},
	}, Composition: &commits.Composition{Status: commits.Captured, Trees: map[string][]commits.TreeEntry{commits.EmptyTreeSHA: nil, a: tree, b: tree}, Blobs: map[string][]byte{oid: data}}}
	key(m, 'C')
	key(m, 'j')
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no composition command")
	}
	m.Update(cmd())
	key(m, 'C')
	if m.commitFilter.err != nil {
		t.Fatal(m.commitFilter.err)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "historical.go") || !strings.Contains(view, "+selected content") {
		t.Fatal(view)
	}
	for _, row := range m.filteredRows() {
		if row.target != nil || row.commentID > 0 {
			t.Fatal("filtered row commentable")
		}
	}
	for _, k := range []rune{'m', 'e', 'i', 'z', 'j'} {
		key(m, k)
	}
	ctrlKey(m, 'l')
	namedKey(m, tea.KeyEnter)
	key(m, '3')
	m.View()
	if canonical != m.Session || m.Composer != nil || m.Selected != selected || m.Scroll[0] != 3 || m.Focus != paneDiff || len(m.Session.ReviewedSliceIDs) > 0 {
		t.Fatal("canonical review changed")
	}
	key(m, 'C')
	namedKey(m, tea.KeyHome)
	namedKey(m, tea.KeyEnter)
	key(m, 'C')
	if m.commitFilter.subset || m.Scroll[0] != 3 {
		t.Fatal("restore failed")
	}
}
func TestCommitFilterGroupedGuideAndPickerMouseBounds(t *testing.T) {
	m := commitModel(t)
	key(m, '3')
	s := m.Session
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Original explanation", Sections: []guide.Section{{UnitIDs: []string{s.Inventory.Units[0].ID}}}}}}
	f := &m.commitFilter
	f.subset = true
	f.selected = map[string]bool{"a": true}
	f.inventory = inventory.Inventory{Files: []inventory.FileChange{s.Inventory.Files[0], {ID: "other", NewPath: []byte("outside.go")}}}
	list := m.filteredList()
	seen := map[int]int{}
	for _, r := range list {
		if r.row >= 0 {
			seen[r.row]++
		}
	}
	if seen[0] != 1 || seen[1] != 1 || len(list) != 4 || !strings.Contains(list[0].text, "Full PR guide") || !strings.Contains(list[2].text, "outside guide") {
		t.Fatal(list)
	}
	key(m, 'C')
	before := f.generation
	m.Update(tea.MouseClickMsg{X: 1, Y: 5, Button: tea.MouseLeft})
	if f.generation != before {
		t.Fatal("footer click selected invisible row")
	}
}
func TestCommitFilterPRIsolationAndScreens(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	first := m.reviewTabState
	key(m, 'C')
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	key(m, 'C')
	other := screenSession()
	other.Inventory.Comparison.Metadata.Identity.Number = 43
	m.openReviewTab(other)
	if m.commitFilter.subset {
		t.Fatal("selection leaked across PR")
	}
	m.activateTab(0)
	if m.reviewTabState != first || !m.commitFilter.subset {
		t.Fatal("selection not retained")
	}
	for _, dims := range [][2]int{{120, 12}, {60, 10}, {20, 6}} {
		m.Width, m.Height = dims[0], dims[1]
		key(m, 'C')
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "Commits [C]") {
			t.Fatal(view)
		}
		for _, line := range strings.Split(view, "\n") {
			if visibleWidth(line) > dims[0] {
				t.Fatal("width exceeded")
			}
		}
		key(m, 'C')
	}
}

func TestCommitFilterScreens(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		picker        bool
		state         string
	}{
		{"commit_filter_picker_wide", 120, 12, true, ""},
		{"commit_filter_picker_narrow", 60, 10, true, ""},
		{"commit_filter_picker_short", 40, 6, true, ""},
		{"commit_filter_empty", 60, 10, false, "empty"},
		{"commit_filter_unavailable", 120, 12, false, "unavailable"},
		{"commit_filter_net_wide", 120, 12, false, "net"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := commitModel(t)
			key(m, '2')
			m.Width, m.Height = tc.width, tc.height
			if tc.picker {
				key(m, 'C')
			} else {
				m.commitFilter.subset = true
				m.commitFilter.selected = map[string]bool{}
				if tc.state != "empty" {
					m.commitFilter.selected["a"] = true
				}
				if tc.state == "unavailable" {
					m.commitFilter.err = fmt.Errorf("conflict in commit aaaaaa, path main.go")
				}
				if tc.state == "net" {
					m.commitFilter.inventory = m.Session.Inventory
				}
			}
			got := ansi.Strip(m.View().Content)
			lines := strings.Split(got, "\n")
			if len(lines) > tc.height {
				t.Fatal("height exceeded")
			}
			for i, line := range lines {
				if visibleWidth(line) > tc.width {
					t.Fatal("width exceeded")
				}
				lines[i] = strings.TrimRight(line, " ")
			}
			checkScreen(t, tc.name, strings.Join(lines, "\n")+"\n")
		})
	}
}

func TestCommitFilterMouseDerivedOnlyNarrow(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	m.Width, m.Height = 60, 10
	f := &m.commitFilter
	f.subset = true
	f.selected = map[string]bool{"a": true}
	f.inventory = m.Session.Inventory
	m.Session.Inventory.Units = nil
	m.Update(tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	if f.readingFocus != paneList {
		t.Fatal("derived list not focusable")
	}
	namedKey(m, tea.KeyEnter)
	if f.readingFocus != paneDiff {
		t.Fatal("derived detail focus failed")
	}
	m.View()
	m.Update(tea.MouseWheelMsg{X: 3, Y: 4, Button: tea.MouseWheelDown})
	if m.Focus != paneList || m.Composer != nil {
		t.Fatal("canonical focus or editor changed")
	}
	f.open = false
	m.Update(tea.MouseClickMsg{X: -1, Y: 2, Button: tea.MouseLeft})
	if f.open {
		t.Fatal("out of bounds control click")
	}
	m.Update(tea.MouseClickMsg{X: 3, Y: 2, Button: tea.MouseLeft})
	if !f.open {
		t.Fatal("visible control not clickable")
	}
}

func TestCommitFilterControlHitRange(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	m.Update(tea.MouseClickMsg{X: 33, Y: 2, Button: tea.MouseLeft})
	if m.commitFilter.open {
		t.Fatal("Files label opened filter")
	}
	header := strings.Split(ansi.Strip(m.View().Content), "\n")[2]
	start := strings.Index(header, "Commits [C]")
	start = visibleWidth(header[:start])
	m.Update(tea.MouseClickMsg{X: start + 8, Y: 2, Button: tea.MouseLeft})
	if !m.commitFilter.open {
		t.Fatal("Commits label missed")
	}
	key(m, 'C')
	m.Update(tea.MouseClickMsg{X: start + visibleWidth("Commits [C] · All changes") - 1, Y: 2, Button: tea.MouseLeft})
	if !m.commitFilter.open {
		t.Fatal("All changes label missed")
	}
}
func TestCommitFilterGuideNavigationOrderAndInterpretation(t *testing.T) {
	m := commitModel(t)
	key(m, '3')
	s := largeSession(3, 3)
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{
		{Title: "A", Description: "Original narrative", Sections: []guide.Section{{Title: "Section A", Description: "Original section narrative", UnitIDs: []string{s.Inventory.Units[0].ID, s.Inventory.Units[2].ID}}}},
		{Title: "B", Sections: []guide.Section{{UnitIDs: []string{s.Inventory.Units[1].ID}}}}}}
	m.Session = s
	f := &m.commitFilter
	f.subset = true
	f.selected = map[string]bool{"a": true}
	f.inventory = s.Inventory
	m.filteredReadingKey("j")
	if f.file != 2 {
		t.Fatal("navigation ignored grouped visible order", f.file)
	}
	rows := m.filteredRows()
	var text string
	for _, r := range rows {
		text += r.Text + "\n"
	}
	if !strings.Contains(text, "Original narrative") || !strings.Contains(text, "Original section narrative") {
		t.Fatal(text)
	}
	m.filteredReadingKey("j")
	if f.file != 1 {
		t.Fatal("second group unreachable")
	}
}

func TestCommitFilterPendingEditReturnsToCanonicalSurface(t *testing.T) {
	m := commitModel(t)
	m.Session = kindsSession()
	m.Selected = 1
	key(m, '2')
	var target source.ReviewCommentTarget
	for _, row := range m.baseDetail() {
		if row.target != nil {
			target = *row.target
			break
		}
	}
	if target.Path == "" {
		t.Fatal("fixture has no target")
	}
	m.Pending = []source.ReviewComment{{Target: target, Body: "retained draft"}}
	f := &m.commitFilter
	f.subset = true
	f.selected = map[string]bool{"a": true}
	key(m, 'R')
	namedKey(m, tea.KeyEscape)
	if !f.subset {
		t.Fatal("cancel review form cleared filter")
	}
	key(m, 'R')
	namedKey(m, tea.KeyTab)
	namedKey(m, tea.KeyTab)
	namedKey(m, tea.KeyEnter)
	if f.subset || m.Composer == nil || m.Composer.Target != target {
		t.Fatal("pending editor did not restore canonical target")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "retained draft") {
		t.Fatal("pending editor invisible", view)
	}
}

func TestCommitFilterDividerResize(t *testing.T) {
	for _, view := range []rune{'2', '3'} {
		t.Run(string(view), func(t *testing.T) {
			m := commitModel(t)
			m.Width, m.Height = 160, 30
			key(m, view)
			m.commitFilter.subset = true
			m.commitFilter.inventory = m.Session.Inventory
			original := m.listWidth()
			key(m, ']')
			if m.listWidth() != original+2 {
				t.Fatal("filtered review swallowed divider hotkey")
			}
			key(m, '[')
			if m.listWidth() != original {
				t.Fatal("divider shrink hotkey failed")
			}
			d := m.filteredGeometry().Divider
			m.Update(tea.MouseClickMsg{X: d.Min.X, Y: d.Min.Y, Button: tea.MouseLeft})
			m.Update(tea.MouseMotionMsg{X: d.Min.X + 5, Y: d.Min.Y, Button: tea.MouseLeft})
			m.Update(tea.MouseReleaseMsg{X: d.Min.X + 5, Y: d.Min.Y, Button: tea.MouseLeft})
			if m.listWidth() != original+5 || m.drag.active {
				t.Fatal("filtered review swallowed divider drag or release")
			}
			key(m, 'C')
			key(m, ']')
			d = m.filteredGeometry().Divider
			m.Update(tea.MouseClickMsg{X: d.Min.X, Y: d.Min.Y, Button: tea.MouseLeft})
			m.Update(tea.MouseMotionMsg{X: d.Min.X + 5, Y: d.Min.Y, Button: tea.MouseLeft})
			if m.listWidth() != original+5 || m.drag.active {
				t.Fatal("modal allowed background resize")
			}
		})
	}
}
