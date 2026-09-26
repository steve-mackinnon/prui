package session

import (
	"context"
	"path/filepath"
	"testing"
)

func BenchmarkLatestComparisonHistory(b *testing.B) {
	store, err := Open(filepath.Join(b.TempDir(), "storage"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	snapshot := fixture()
	snapshot.Context.Evidence[0].Excerpt = make([]byte, 256<<10)
	for i := 0; i < 50; i++ {
		if _, err := store.Create(snapshot); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := store.LatestComparison(snapshot.Inventory.Comparison.Metadata.Identity)
		if err != nil || got == nil {
			b.Fatalf("lookup: %v", err)
		}
	}
}

func TestLatestComparisonOrdersStateAndSkipsCorruptSource(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "storage"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	firstSource := fixture()
	description := "first"
	firstSource.PullRequestDescription = &description
	first, err := store.Create(firstSource)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := store.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	id := first.Inventory.Comparison.Metadata.Identity
	if got, err := store.LatestComparison(id); err != nil || got.ID != newer.ID {
		t.Fatalf("newest = %#v, %v", got, err)
	}
	state, err := store.UpdateState(context.Background(), first.ID, first.Generation, first.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current})
	if err != nil || state.Generation != 2 {
		t.Fatalf("updated = %#v, %v", state, err)
	}
	if got, err := store.LatestComparison(id); err != nil || got.ID != first.ID {
		t.Fatalf("updated newest = %#v, %v", got, err)
	}
	db, _ := store.db.SQL()
	if _, err := db.Exec(`UPDATE snapshots SET payload=x'7b' WHERE digest=(SELECT snapshot_digest FROM sessions WHERE id=?)`, first.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LatestComparison(id); err != nil || got.ID != newer.ID {
		t.Fatalf("corrupt candidate was reused: %#v, %v", got, err)
	}
}
