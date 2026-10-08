package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"prui/internal/source"
)

func fileScrollModel(width int) *Model {
	m := largeModel(largeTextSession(1, 50), width, 40)
	m.Files, m.Focus, m.cursorActive = true, paneDiff, true
	m.setCursor(30)
	m.setOffset(20)
	if width >= sideBySideMinimumWidth {
		m.layout = diffLayoutSideBySide
	}
	return m
}

func TestFilesPaneScrollKeepsDiffCursorVisible(t *testing.T) {
	for _, width := range []int{120, 180} {
		for _, input := range []rune{'J', 'K'} {
			t.Run(fmt.Sprintf("width=%d/key=%c", width, input), func(t *testing.T) {
				m := fileScrollModel(width)
				defer m.Close()
				m.Focus = paneList
				if input == 'J' {
					m.setCursor(m.offset() + 1)
				} else {
					m.setCursor(m.offset() + m.bodyHeight() - 1)
				}
				before := m.offset()
				key(m, input)
				if m.offset() == before {
					t.Fatal("fixture did not scroll")
				}
				start := m.offset()
				if stickyFileHeader(m.displayDetail(), start) >= 0 {
					start++
				}
				if cursor := m.cursor(); cursor < start || cursor >= m.offset()+m.bodyHeight() {
					t.Fatalf("cursor %d outside visible diff [%d, %d)", cursor, start, m.offset()+m.bodyHeight())
				}
				offset, cursor := m.offset(), m.cursor()
				key(m, 'l')
				if m.Focus != paneDiff || !m.cursorActive || m.offset() != offset || m.cursor() != cursor {
					t.Fatal("focusing diff changed the reading position or lost the cursor")
				}
				m.ensureCursorVisible()
				if m.offset() != offset {
					t.Fatal("revealing cursor jumped the diff scroll")
				}
			})
		}
	}
}

func BenchmarkFilesScroll(b *testing.B) {
	for _, width := range []int{120, 180} {
		b.Run(fmt.Sprint(width), func(b *testing.B) {
			m := fileScrollModel(width)
			defer m.Close()
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

// Exercise an actual file transition in a large continuous diff. The original
// benchmark alternates within one file and cannot expose transition costs.
func BenchmarkFilesScrollTransition(b *testing.B) {
	for _, width := range []int{120, 180} {
		for _, crossing := range []bool{false, true} {
			b.Run(fmt.Sprintf("width=%d/crossing=%t", width, crossing), func(b *testing.B) {
				m := largeModel(largeTextSession(100, 1000), width, 40)
				defer m.Close()
				m.Files, m.Focus, m.cursorActive = true, paneDiff, true
				if width >= sideBySideMinimumWidth {
					m.layout = diffLayoutSideBySide
				}
				start := m.fileOffset(50) - 1
				detail := m.displayDetail()
				for detail[start].target == nil {
					start--
				}
				if !crossing {
					start -= 5
				}
				m.setCursor(start)
				m.setOffset(start - m.bodyHeight() + 1)
				m.syncFileToLine(start)
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

func TestFilesScrollAllocationBudget(t *testing.T) {
	for _, tc := range []struct {
		width  int
		budget float64
	}{{120, 10000}, {180, 20000}} {
		t.Run(fmt.Sprint(tc.width), func(t *testing.T) {
			m := fileScrollModel(tc.width)
			defer m.Close()
			_ = m.View()
			n := 0
			allocs := testing.AllocsPerRun(3, func() {
				if n%2 == 0 {
					key(m, 'j')
				} else {
					key(m, 'k')
				}
				n++
				_ = m.View()
			})
			if allocs > tc.budget {
				t.Fatalf("files keypress + render: %.0f allocations exceed %.0f", allocs, tc.budget)
			}
		})
	}
}

// Reading an unchanged diff without overlays must reuse the cached stream.
// Otherwise navigation repeatedly copies the entire review before clipping.
func TestUnchangedDiffStreamAllocationBudget(t *testing.T) {
	for _, width := range []int{120, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := fileScrollModel(width)
			defer m.Close()
			_ = m.displayDetail()
			allocs := testing.AllocsPerRun(5, func() { _ = m.displayDetail() })
			if allocs != 0 {
				t.Fatalf("unchanged diff stream allocated %.0f times; want zero", allocs)
			}
		})
	}
}

func TestFilesDetailCacheSourceAndOverlay(t *testing.T) {
	for _, width := range []int{120, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := fileScrollModel(width)
			defer m.Close()
			original := append([]diffLine(nil), m.baseDetail()...)
			var originalSplit []diffLine
			if width >= sideBySideMinimumWidth {
				originalSplit = projectSideBySideDetail(original)
			}
			_ = m.View()
			m.Horizontal = 5
			_ = m.View()
			if !reflect.DeepEqual(m.baseDetail(), original) {
				t.Fatal("renderer mutated cached source")
			}
			if originalSplit != nil && !reflect.DeepEqual(m.cachedFileDetail(true), originalSplit) {
				t.Fatal("renderer mutated cached split projection")
			}
			var target source.ReviewCommentTarget
			for _, line := range m.baseDetail() {
				if line.target != nil {
					target = *line.target
					break
				}
			}
			m.Comments = []source.ReviewComment{{ID: 123, Target: target, Body: "fresh comment"}}
			m.Composer = &commentComposer{Target: target, Draft: "fresh draft", PendingIndex: -1}
			joined := func() string {
				var b strings.Builder
				for _, line := range m.displayDetail() {
					b.WriteString(line.Text)
				}
				return b.String()
			}
			if got := joined(); !strings.Contains(got, "fresh comment") || !strings.Contains(got, "fresh draft") {
				t.Fatal("overlays did not refresh")
			}
			m.Comments, m.Composer = nil, nil
			if got := joined(); strings.Contains(got, "fresh comment") || strings.Contains(got, "fresh draft") {
				t.Fatal("stale overlay")
			}
			m.Pending = []source.ReviewComment{{Target: target, Body: "pending draft"}}
			if !strings.Contains(joined(), "pending draft") {
				t.Fatal("pending-only overlay did not refresh")
			}
			m.Pending = nil
			if strings.Contains(joined(), "pending draft") {
				t.Fatal("stale pending overlay")
			}
			if !reflect.DeepEqual(m.baseDetail(), original) {
				t.Fatal("overlay mutated cached source")
			}
		})
	}
}

func TestFilesDetailCacheInvalidatesSession(t *testing.T) {
	m := fileScrollModel(120)
	defer m.Close()
	first := m.Session
	before := m.baseDetail()
	replacement := largeTextSession(1, 50)
	replacement.Inventory.Patches["p"] = []byte("@@ -1 +1 @@\n-old\n+replacement\n")
	m.Session = replacement
	if reflect.DeepEqual(m.baseDetail(), before) {
		t.Fatal("different snapshot reused cached source")
	}
	m.Session = first
	if !reflect.DeepEqual(m.baseDetail(), before) {
		t.Fatal("original snapshot did not restore")
	}
	m.Session = nil
	if got := m.cachedFileDetail(false); got != nil || m.fileCache.session != nil {
		t.Fatal("clearing the session retained cached source")
	}
}

func TestFilesJKMovesOneDisplayRow(t *testing.T) {
	for _, width := range []int{120, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := fileScrollModel(width)
			defer m.Close()
			m.setOffset(0)
			m.setCursor(0)
			rows := m.displayDetail()
			if m.navigableLine(rows[0]) {
				t.Fatal("fixture must begin with a noncommentable header")
			}
			for i := 1; i < min(len(rows), 80); i++ {
				before := m.offset()
				key(m, 'j')
				if i >= m.bodyHeight() && m.offset() != before+1 {
					t.Fatalf("j scrolled from %d to %d, want one row", before, m.offset())
				}
				if got := m.cursor(); got != i {
					t.Fatalf("j cursor = %d, want display row %d", got, i)
				}
			}
			for i := min(len(rows), 80) - 2; i >= 0; i-- {
				key(m, 'k')
				if got := m.cursor(); got != i {
					t.Fatalf("k cursor = %d, want display row %d", got, i)
				}
			}
			key(m, 'k')
			if m.cursor() != 0 {
				t.Fatal("k moved before the first display row")
			}
			if target, comment := m.cursorAnchor(); target != nil || comment != 0 {
				t.Fatal("header acquired a comment target")
			}
			key(m, 'n')
			if !m.navigableLine(m.displayDetail()[m.cursor()]) {
				t.Fatal("n did not find a comment target from the header")
			}
		})
	}
}
