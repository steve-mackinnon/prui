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
