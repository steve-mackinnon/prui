package tui

import (
	"context"
	"fmt"
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

func BenchmarkLargeReview(b *testing.B) {
	s := largeSession(1000, 50000)
	m := New(context.Background(), nil)
	m.Session, m.Width, m.Height = s, 120, 30
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Selected = (i * 97) % len(s.Inventory.Units)
		_ = m.View().Content
	}
}

func BenchmarkViewportRender(b *testing.B) {
	s := largeSession(1000, 50000)
	m := New(context.Background(), nil)
	m.Session, m.Width, m.Height = s, 80, 20
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.Selected = i % len(s.Inventory.Units)
		_ = m.View().Content
	}
}

func TestWorkflowLargeViewport(t *testing.T) {
	s := largeSession(1000, 50000)
	m := New(context.Background(), nil)
	m.Session, m.Width, m.Height = s, 80, 20
	for i := 0; i < 100; i++ {
		m.move(1)
		if len(m.View().Content) == 0 {
			t.Fatal("large review rendered empty")
		}
	}
}
