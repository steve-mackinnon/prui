package session

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkLatestComparisonHistory(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "sessions"))
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	snapshot := fixture()
	// Retained evidence represents a moderately sized frozen review.
	snapshot.Context.Evidence[0].Excerpt = make([]byte, 256<<10)
	for i := 0; i < 50; i++ {
		if _, err := s.Create(snapshot); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := s.LatestComparison(snapshot.Inventory.Comparison.Metadata.Identity)
		if err != nil || got == nil {
			b.Fatalf("lookup: %v", err)
		}
	}
}

func TestLatestComparisonOrdersStateAndSkipsCorruptSnapshots(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	first, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	newer, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	other := fixture()
	other.Inventory.Comparison.Metadata.Identity.Number++
	if _, err := s.Create(other); err != nil {
		t.Fatal(err)
	}
	id := first.Inventory.Comparison.Metadata.Identity
	assertLatest := func(want string) {
		t.Helper()
		got, err := s.LatestComparison(id)
		if err != nil || got == nil || got.ID != want {
			t.Fatalf("latest = %v, %v; want %s", got, err, want)
		}
	}
	assertLatest(newer.ID)
	first.ReviewedSliceIDs = []string{"file"}
	if err := s.Save(first); err != nil {
		t.Fatal(err)
	}
	assertLatest(first.ID)
	if err := os.WriteFile(filepath.Join(s.Path(), first.ID, "snapshot.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	assertLatest(newer.ID)
	// Reopen to prove the optimization works with an existing store.
	path := s.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertLatest(newer.ID)
}
