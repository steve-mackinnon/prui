package session

import (
	"context"
	"path/filepath"
	"testing"
)

func BenchmarkSQLiteStateUpdate(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "storage"))
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	snap := fixture()
	snap.Context.Evidence[0].Excerpt = make([]byte, 256<<10)
	r, err := s.Create(snap)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, err := s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current})
		if err != nil {
			b.Fatal(err)
		}
		r.State = next
	}
}

func BenchmarkSQLiteListHistory(b *testing.B) {
	s, err := Open(filepath.Join(b.TempDir(), "storage"))
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	snap := fixture()
	snap.Context.Evidence[0].Excerpt = make([]byte, 256<<10)
	for i := 0; i < 50; i++ {
		if _, err := s.Create(snap); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if entries, err := s.List(); err != nil || len(entries) != 50 {
			b.Fatalf("List count/error: %d %v", len(entries), err)
		}
	}
}
