package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestReviewArrowOnlyReadingJourney(t *testing.T) {
	for _, width := range []int{60, 120} {
		m := largeModel(largeTextSession(3, 3), width, 18)
		m.selectReviewView(viewFiles)
		namedKey(m, tea.KeyDown)
		if m.Focus != paneList || m.Session.UnitFiles[m.Selected] != 1 {
			t.Fatalf("width %d: Down did not select the second file", width)
		}
		namedKey(m, tea.KeyUp)
		if m.Session.UnitFiles[m.Selected] != 0 {
			t.Fatal("Up did not return to the first file")
		}
		namedKey(m, tea.KeyEnter)
		before := m.offset()
		namedKey(m, tea.KeyDown)
		if m.Focus != paneDiff || m.offset() != before+1 || m.Composer != nil {
			t.Fatal("Enter then Down did not read the diff")
		}
		namedKey(m, tea.KeyUp)
		if m.offset() != before {
			t.Fatal("Up did not restore the diff position")
		}
		namedKey(m, tea.KeyEscape)
		namedKey(m, tea.KeyDown)
		if m.Focus != paneList || m.Session.UnitFiles[m.Selected] != 1 {
			t.Fatal("Escape did not return arrow navigation to the list")
		}
		m.Close()
	}
}

func TestReviewTabSwitchesPanesWithoutMovingReadingPosition(t *testing.T) {
	for _, width := range []int{60, 120} {
		for _, view := range []reviewView{viewFiles, viewGuide} {
			s, _, _ := guidedSession(t, splitAnalyzer{})
			m := loaded(t, s, width, 18)
			m.selectReviewView(view)
			if view == viewGuide {
				for i, r := range m.rows() {
					if r.kind == portionRow {
						m.Row, m.Selected = i, r.units[0]
						break
					}
				}
			}
			m.setOffset(1)
			m.setCursor(1)
			m.Horizontal = 8
			row, selected, count := m.Row, m.Selected, len(m.rows())
			offset, cursor := m.offset(), m.cursor()
			for _, key := range []tea.KeyPressMsg{
				{Code: tea.KeyTab}, {Code: tea.KeyTab},
				{Code: tea.KeyTab, Mod: tea.ModShift}, {Code: tea.KeyTab, Mod: tea.ModShift},
			} {
				before := m.Focus
				m.Update(key)
				if m.Focus == before {
					t.Fatalf("width %d view %v: %s did not switch panes", width, view, key.String())
				}
				if m.Row != row || m.Selected != selected || len(m.rows()) != count || m.offset() != offset || m.cursor() != cursor || m.Horizontal != 8 {
					t.Fatalf("width %d view %v: pane switch changed row %d/%d selected %d/%d count %d/%d offset %d cursor %d horizontal %d", width, view, m.Row, row, m.Selected, selected, len(m.rows()), count, m.offset(), m.cursor(), m.Horizontal)
				}
			}
		}
	}
}

func TestInventoryTabSwitchesPanesWithoutMovingReadingPosition(t *testing.T) {
	for _, width := range []int{60, 120} {
		m := largeModel(largeTextSession(2, 2), width, 18)
		m.selectReviewView(viewFiles)
		m.Inventory = true
		m.setOffset(2)
		m.Horizontal = 8
		for _, key := range []tea.KeyPressMsg{{Code: tea.KeyTab}, {Code: tea.KeyTab, Mod: tea.ModShift}} {
			before := m.Focus
			m.Update(key)
			if m.Focus == before || m.Selected != 0 || m.offset() != 2 || m.Horizontal != 8 {
				t.Fatal("inventory pane switch changed the selected unit or reading position")
			}
		}
		m.Close()
	}
}

func TestGuideArrowOnlyReadingJourney(t *testing.T) {
	for _, width := range []int{60, 120} {
		s, _, _ := guidedSession(t, splitAnalyzer{})
		m := loaded(t, s, width, 8)
		row := m.Row
		namedKey(m, tea.KeyDown)
		if m.Focus != paneList || m.Row <= row {
			t.Fatalf("width %d: Down did not select the next guide row", width)
		}
		namedKey(m, tea.KeyUp)
		if m.Row != row {
			t.Fatal("Up did not restore guide selection")
		}
		namedKey(m, tea.KeyEnter)
		before := m.offset()
		namedKey(m, tea.KeyDown)
		if m.Focus != paneDiff || m.offset() != before+1 || m.Composer != nil {
			t.Fatal("Enter then Down did not read the guide diff")
		}
		namedKey(m, tea.KeyUp)
		if m.offset() != before {
			t.Fatal("Up did not restore guide reading position")
		}
		namedKey(m, tea.KeyEscape)
		namedKey(m, tea.KeyDown)
		if m.Focus != paneList || m.Row <= row {
			t.Fatal("Escape did not return arrow navigation to the guide list")
		}
	}
}

func TestCommitsTabSwitchesPanesAndKeepsCommitAnchor(t *testing.T) {
	for _, width := range []int{60, 120} {
		m := commitModel(t)
		m.Width = width
		namedKey(m, tea.KeyDown)
		namedKey(m, tea.KeyEnter)
		namedKey(m, tea.KeyDown)
		sha, offset, cursor := m.commit.selectedSHA, m.commitOffset(), m.commitCursor()
		for _, key := range []tea.KeyPressMsg{
			{Code: tea.KeyTab}, {Code: tea.KeyTab},
			{Code: tea.KeyTab, Mod: tea.ModShift}, {Code: tea.KeyTab, Mod: tea.ModShift},
		} {
			before := m.commit.focus
			m.Update(key)
			if m.commit.focus == before || m.commit.selectedSHA != sha || m.commitOffset() != offset || m.commitCursor() != cursor {
				t.Fatalf("width %d: %s did not switch commit panes while retaining anchor", width, key.String())
			}
		}
		namedKey(m, tea.KeyEscape)
		if m.commit.focus != paneList || m.Composer != nil {
			t.Fatal("Escape did not return to the commit list")
		}
	}
}

func TestPaneSwitchPreservesRawCommentTarget(t *testing.T) {
	for _, width := range []int{60, 180} {
		for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
			m := rangeTestModel()
			m.Width, m.layout = width, layout
			selectRawTarget(t, m, "RIGHT", 2)
			want := *m.selectedDiffTarget()
			namedKey(m, tea.KeyTab)
			m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
			if got := m.selectedDiffTarget(); got == nil || *got != want {
				t.Fatalf("width %d layout %v: pane switch changed raw comment target", width, layout)
			}
			m.Close()
		}
	}
}

func TestEmptyCommitsTabKeepsListFocus(t *testing.T) {
	m := commitModel(t)
	m.Session.Commits.Entries = nil
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyTab}, {Code: tea.KeyTab, Mod: tea.ModShift}} {
		m.Update(key)
		if m.commit.focus != paneList {
			t.Fatal("empty commit list focused an unavailable diff")
		}
	}
}

func TestSelectedCommitsTabSwitchesReadingPanes(t *testing.T) {
	for _, view := range []reviewView{viewFiles, viewGuide} {
		m := commitModel(t)
		m.selectReviewView(view)
		m.commitFilter.subset = true
		m.commitFilter.scroll, m.commitFilter.horizontal = 3, 8
		for _, key := range []tea.KeyPressMsg{{Code: tea.KeyTab}, {Code: tea.KeyTab, Mod: tea.ModShift}} {
			before := m.commitFilter.readingFocus
			m.Update(key)
			if m.commitFilter.readingFocus == before || m.commitFilter.scroll != 3 || m.commitFilter.horizontal != 8 {
				t.Fatal("Tab did not switch the selected-commits reading pane without moving it")
			}
		}
	}
}

func TestNavigationTabsRespectTextInputAndModalOwnership(t *testing.T) {
	for _, input := range []string{"file filter", "search", "review form", "comment editor"} {
		t.Run(input, func(t *testing.T) {
			m := rangeTestModel()
			defer m.Close()
			switch input {
			case "file filter":
				m.openFileFilter()
			case "search":
				searchInput(m, "first")
			case "review form":
				m.openReviewForm()
			case "comment editor":
				selectRawTarget(t, m, "RIGHT", 1)
				namedKey(m, tea.KeyEnter)
			}
			before, selected := m.Focus, m.Selected
			for _, key := range []tea.KeyPressMsg{{Code: tea.KeyTab}, {Code: tea.KeyTab, Mod: tea.ModShift}} {
				m.Update(key)
				if m.Focus != before || m.Selected != selected {
					t.Fatal("pane navigation intercepted input-owned Tab")
				}
			}
		})
	}
}
