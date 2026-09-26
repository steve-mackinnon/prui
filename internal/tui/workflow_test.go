package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"pr-review/internal/inventory"
	"pr-review/internal/review"
)

func largeSession(files, units int) *review.Session {
	s := &review.Session{}
	s.Inventory.Complete = true
	s.Inventory.Comparison.InventoryID = "large"
	s.Inventory.Files = make([]inventory.FileChange, files)
	s.Inventory.Units = make([]inventory.ReviewUnit, units)
	s.Slices = make([]review.Slice, files)
	s.UnitFiles = make([]int, units)
	for i := range s.Inventory.Files {
		id := fmt.Sprintf("file-%d", i)
		s.Inventory.Files[i] = inventory.FileChange{ID: id, NewPath: []byte(id)}
		s.Slices[i] = review.Slice{FileID: id}
	}
	for i := range s.Inventory.Units {
		file := i % files
		s.Inventory.Units[i] = inventory.ReviewUnit{ID: fmt.Sprintf("unit-%d", i), InventoryID: "large", FileChangeID: s.Inventory.Files[file].ID, Kind: inventory.FileMetadata}
		s.UnitFiles[i] = file
		s.Slices[file].Units = append(s.Slices[file].Units, i)
	}
	return s
}

// largeTextSession is the styling-heavy shape: every unit is a text hunk, so
// each rendered line is classified by Git-diff grammar rather than by kind.
func largeTextSession(files, units int) *review.Session {
	s := largeSession(files, units)
	var patch strings.Builder
	patch.WriteString("diff --git a/f b/f\nindex 1111111..2222222 100644\n--- a/f\n+++ b/f\n@@ -1,20 +1,20 @@\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&patch, " context %d\n-old %d\n+new %d\n", i, i, i)
	}
	s.Inventory.Patches = map[string][]byte{"p": []byte(patch.String())}
	for i := range s.Inventory.Units {
		s.Inventory.Units[i].Kind = inventory.TextHunk
		s.Inventory.Units[i].PatchReference = "p"
	}
	return s
}

// largeModel loads a fixture straight into the review screen. Without clearing
// the loading flag the viewport would measure the loading notice instead.
func largeModel(s *review.Session, w, h int) *Model {
	m := New(context.Background(), nil)
	m.Session, m.Width, m.Height, m.Loading = s, w, h, false
	return m
}

func BenchmarkLargeReview(b *testing.B) {
	s := largeSession(1000, 50000)
	m := largeModel(s, 120, 30)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Selected = (i * 97) % len(s.Inventory.Units)
		_ = m.View().Content
	}
}

func BenchmarkViewportRender(b *testing.B) {
	s := largeSession(1000, 50000)
	m := largeModel(s, 80, 20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Selected = i % len(s.Inventory.Units)
		_ = m.View().Content
	}
}

// BenchmarkLargeTextHunkReview exercises per-line classification and styling on
// the same 1,000-file / 50,000-unit fixture.
func BenchmarkLargeTextHunkReview(b *testing.B) {
	s := largeTextSession(1000, 50000)
	m := largeModel(s, 120, 30)
	m.Focus = paneDiff
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Selected = (i * 97) % len(s.Inventory.Units)
		_ = m.View().Content
	}
}

func TestWorkflowLargeViewport(t *testing.T) {
	for _, tc := range []struct {
		name      string
		session   *review.Session
		inventory bool
	}{
		{"metadata inventory", largeSession(1000, 50000), true},
		{"text inventory", largeTextSession(1000, 50000), true},
		{"files", largeSession(1000, 50000), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := largeModel(tc.session, 80, 20)
			// Unit-by-unit navigation belongs to the inventory view. Files mode
			// navigates whole files and renders their continuous diff instead.
			m.Inventory = tc.inventory
			for i := 0; i < 100; i++ {
				m.move(1)
				want := i + 1
				if !tc.inventory {
					want = tc.session.Slices[i+1].Units[0]
				}
				if m.Selected != want {
					t.Fatalf("move %d selected unit %d, want %d", i+1, m.Selected, want)
				}
				content := m.View().Content
				if len(content) == 0 {
					t.Fatal("large review rendered empty")
				}
				lines := strings.Split(content, "\n")
				if len(lines) > m.Height {
					t.Fatalf("large review rendered %d lines, height %d", len(lines), m.Height)
				}
				for _, line := range lines {
					if visibleWidth(line) > m.Width {
						t.Fatalf("styled large review exceeded %d: %q", m.Width, line)
					}
				}
			}
		})
	}
}
