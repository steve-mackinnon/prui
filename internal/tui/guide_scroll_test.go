package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"prui/internal/guide"
	"prui/internal/source"
)

func guideScrollModel(hunks, width int) *Model {
	s := largeTextSession(1, hunks)
	ids := make([]string, hunks)
	for i := range ids {
		ids[i] = s.Inventory.Units[i].ID
	}
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Scrolling", Sections: []guide.Section{{Title: "Changes", UnitIDs: ids}}}}}
	m := largeModel(s, width, 40)
	m.Files, m.Focus = false, paneDiff
	if width >= sideBySideMinimumWidth {
		m.layout = diffLayoutSideBySide
	}
	m.setCursor(30)
	m.setOffset(20)
	return m
}

func TestGuideRowNavigationScrollsDiff(t *testing.T) {
	for _, width := range []int{120, 180} {
		for _, files := range []int{1, 2} {
			t.Run(fmt.Sprintf("width=%d/files=%d", width, files), func(t *testing.T) {
				s := largeTextSession(files, 2)
				s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Changes", Sections: []guide.Section{
					{Title: "First", UnitIDs: []string{s.Inventory.Units[0].ID}},
					{Title: "Second", UnitIDs: []string{s.Inventory.Units[1].ID}},
				}}}}
				m := loaded(t, s, width, 12)
				if width >= sideBySideMinimumWidth {
					m.layout = diffLayoutSideBySide
				}
				rows := m.rows()
				// Each section has one file portion. The second section starts
				// after the first unit, even when both belong to the same file.
				want := len(unitLines(s, 0)) + 1
				if m.sideBySideEnabled() {
					want = len(projectSideBySideDetail(unitLines(s, 0))) + 1
				}
				for range 3 {
					key(m, 'j')
				}
				if rows[m.Row].kind != sectionRow || rows[m.Row].section != 1 || m.offset() != want || m.Focus != paneList {
					t.Fatalf("j selected row %d, offset %d, focus %v; want second section, offset %d, list focus", m.Row, m.offset(), m.Focus, want)
				}
				key(m, 'j') // its file portion uses the same section occurrence
				if m.offset() != want {
					t.Fatalf("file portion offset = %d, want %d", m.offset(), want)
				}
				key(m, 'k')
				key(m, 'k')
				if m.offset() != 0 || m.Focus != paneList {
					t.Fatalf("k returned offset %d, focus %v; want 0, list focus", m.offset(), m.Focus)
				}
				key(m, 'k')
				m.setOffset(7)
				key(m, 'k') // guide rows retain their saved reading position
				if m.offset() != 7 {
					t.Fatalf("guide row changed saved offset to %d", m.offset())
				}
			})
		}
	}
}

// Showing a cursor must not multiply whole-guide allocations by viewport height.
func TestGuideCursorRenderAllocationGrowth(t *testing.T) {
	m := guideScrollModel(10, 120)
	defer m.Close()
	plain := testing.AllocsPerRun(3, func() { _ = m.View() })
	m.cursorActive = true
	focused := testing.AllocsPerRun(3, func() { _ = m.View() })
	if focused > plain*3 {
		t.Fatalf("cursor render allocations %.0f exceed 3x unfocused %.0f", focused, plain)
	}
}

func BenchmarkGuideScroll(b *testing.B) {
	for _, hunks := range []int{10, 50} {
		for _, width := range []int{120, 180} {
			b.Run(fmt.Sprintf("hunks=%d/width=%d", hunks, width), func(b *testing.B) {
				m := guideScrollModel(hunks, width)
				defer m.Close()
				m.cursorActive = true
				_ = m.View()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if i%2 == 0 {
						key(m, 'j')
					} else {
						key(m, 'k')
					}
					_ = m.View()
				}
			})
		}
	}
}

func TestGuideDetailCacheKeepsSourceImmutableAndOverlaysCurrent(t *testing.T) {
	for _, width := range []int{120, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := guideScrollModel(2, width)
			defer m.Close()
			want := detailFor(m.Session, 0)
			m.cursorActive, m.Horizontal = true, 4
			_ = m.View()
			_ = m.View()
			if !reflect.DeepEqual(m.cachedGuideDetail(0), want) {
				t.Fatal("render mutated cached source")
			}
			target := *m.displayDetail()[m.cursor()].target
			m.Comments = []source.ReviewComment{{ID: 123, Target: target, Body: "fresh comment"}}
			m.Composer = &commentComposer{Target: target, Draft: "fresh draft", PendingIndex: -1}
			text := func() string {
				var out strings.Builder
				for _, line := range m.displayDetail() {
					out.WriteString(line.Text)
					out.WriteByte('\n')
				}
				return out.String()
			}
			if got := text(); !strings.Contains(got, "fresh comment") || !strings.Contains(got, "fresh draft") {
				t.Fatal("overlay not added after cache warmed")
			}
			m.Comments, m.Composer = nil, nil
			if got := text(); strings.Contains(got, "fresh comment") || strings.Contains(got, "fresh draft") {
				t.Fatal("stale overlay remained in cache")
			}
		})
	}
}

func TestGuideDetailCacheFollowsGuideBundleAndSession(t *testing.T) {
	m := guideScrollModel(2, 120)
	defer m.Close()
	first := m.Session
	first.Guides.Items = append(first.Guides.Items, guide.Item{Sections: []guide.Section{{UnitIDs: []string{first.Inventory.Units[1].ID}}}})
	check := func(index int) {
		t.Helper()
		if !reflect.DeepEqual(m.cachedGuideDetail(index), detailFor(m.Session, index)) {
			t.Fatal("cached detail belongs to a different selection")
		}
	}
	check(0)
	check(1)
	check(0)
	first.Guides = &guide.Bundle{Status: guide.Generated, Items: first.Guides.Items[1:]}
	check(0)
	other := guideScrollModel(1, 120)
	defer other.Close()
	other.Session.Inventory.Patches["p"] = []byte("@@ -1 +1 @@\n-old\n+different snapshot\n")
	m.Session = other.Session
	check(0)
	m.Session = first
	check(0)
}

func TestSplitViewportPreservesScrolledCellSelection(t *testing.T) {
	m := guideScrollModel(2, 180)
	defer m.Close()
	m.Session.Inventory.Files[0].OldPath = []byte("file-0")
	detail := m.displayDetail()
	for index, line := range detail {
		if index < 20 || line.sideBySide == nil || len(rowTargets(*line.sideBySide)) != 2 {
			continue
		}
		for _, target := range rowTargets(*line.sideBySide) {
			m.cursorActive = true
			m.setCursor(index)
			m.setSelectedDiffTarget(&target)
			offset := index - 5
			whole := m.renderProjectedSideBySideDetail(detail, m.detailWidth(), 3)
			window := m.renderSideBySideViewport(detail[offset:offset+10], m.detailWidth(), 3, index-offset, m.selectedDiffTargetForLine(detail[index]))
			if !reflect.DeepEqual(window, whole[offset:offset+10]) {
				t.Fatal("viewport changed split row selection or formatting")
			}
		}
		return
	}
	t.Fatal("no paired source row")
}

// A generous allocation ceiling catches both patch reconstruction and formatting
// the entire split diff on each repeat, without relying on machine-speed timings.
// The warmed 50-hunk fixture uses about 1,300 unified / 8,200 split allocations;
// the original path used roughly 770,000 / 1,800,000 respectively.
func TestGuideScrollAllocationBudget(t *testing.T) {
	for _, width := range []int{120, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := guideScrollModel(50, width)
			defer m.Close()
			m.cursorActive = true
			repeats := 0
			allocs := testing.AllocsPerRun(3, func() {
				if repeats%2 == 0 {
					key(m, 'j')
				} else {
					key(m, 'k')
				}
				repeats++
				_ = m.View()
			})
			if allocs > 20000 {
				t.Fatalf("guide keypress + render: %.0f allocations exceeds 20,000 budget", allocs)
			}
		})
	}
}
