package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"prui/internal/inventory"
)

func progressFixture(count int) (Snapshot, []string) {
	snapshot := fixture()
	snapshot.Inventory.Files = nil
	snapshot.Inventory.Units = nil
	snapshot.Inventory.Patches = map[string][]byte{}
	snapshot.Slices, snapshot.UnitFiles = nil, nil
	var ids []string
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("file-%05d", i)
		ids = append(ids, id)
		snapshot.Inventory.Files = append(snapshot.Inventory.Files, inventory.FileChange{ID: id, NewPath: []byte(id)})
		snapshot.Inventory.Units = append(snapshot.Inventory.Units, inventory.ReviewUnit{ID: fmt.Sprintf("unit-%05d", i), InventoryID: "inventory", FileChangeID: id, Kind: inventory.FileMetadata})
		snapshot.Slices = append(snapshot.Slices, Slice{FileID: id, Units: []int{i}})
		snapshot.UnitFiles = append(snapshot.UnitFiles, i)
	}
	return snapshot, ids
}

func TestSQLiteFreshnessKeepsUnchangedProgressRows(t *testing.T) {
	s := openSQLiteTestStore(t)
	snapshot, ids := progressFixture(3)
	r, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: ids, RevisionStatus: Current})
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	// Deterministically count writes instead of asserting elapsed time.
	for _, sql := range []string{
		`CREATE TEMP TABLE progress_audit(event TEXT)`,
		`CREATE TEMP TRIGGER progress_delete AFTER DELETE ON progress BEGIN INSERT INTO progress_audit VALUES('delete'); END`,
		`CREATE TEMP TRIGGER progress_insert AFTER INSERT ON progress BEGIN INSERT INTO progress_audit VALUES('insert'); END`,
	} {
		if _, err := db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	next, err := s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: ids, RevisionStatus: Unchecked})
	if err != nil {
		t.Fatal(err)
	}
	var writes int
	if err := db.QueryRow(`SELECT count(*) FROM progress_audit`).Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatalf("freshness-only update rewrote %d progress rows", writes)
	}
	loaded, err := s.Load(r.ID)
	if err != nil || loaded.Generation != state.Generation+1 || loaded.RevisionStatus != Unchecked || !reflect.DeepEqual(loaded.ReviewedSliceIDs, ids) || !loaded.UpdatedAt.Equal(next.UpdatedAt) {
		t.Fatalf("state not durably advanced: %#v, %v", loaded, err)
	}
	if _, err := s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: ids, RevisionStatus: Current}); !errors.Is(err, ErrStateConflict) {
		t.Fatal("stale writer accepted", err)
	}
}

func TestSQLiteFreshnessDoesNotTrustInvalidExistingProgress(t *testing.T) {
	s := openSQLiteTestStore(t)
	snapshot, ids := progressFixture(2)
	r, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: ids, RevisionStatus: Current})
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE progress SET ordinal=ordinal+10 WHERE session_id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	state, err = s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: ids, RevisionStatus: Unchecked})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(r.ID); err != nil {
		t.Fatal("progress ordinals were not repaired", err)
	}
	if _, err := db.Exec(`UPDATE progress SET file_id='invented' WHERE session_id=? AND ordinal=0`, r.ID); err != nil {
		t.Fatal(err)
	}
	invalid := []string{"invented", ids[1]}
	if _, err := s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: invalid, RevisionStatus: Current}); !errors.Is(err, ErrInvalidStateUpdate) {
		t.Fatal("unknown unchanged mark accepted", err)
	}
	// The failed CAS transaction must roll back, permitting a valid repair.
	reordered := []string{ids[1], ids[0]}
	state, err = s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: reordered, RevisionStatus: Current})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(r.ID)
	if err != nil || !reflect.DeepEqual(loaded.ReviewedSliceIDs, reordered) || loaded.Generation != state.Generation {
		t.Fatal("valid progress repair failed", err)
	}
}

func TestSQLiteFreshnessEmptyProgressStillAdvancesAndCanClearMarks(t *testing.T) {
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{RevisionStatus: Current})
	if err != nil || state.Generation != r.Generation+1 || len(state.ReviewedSliceIDs) != 0 {
		t.Fatalf("empty update = %#v, %v", state, err)
	}
	state, err = s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current})
	if err != nil {
		t.Fatal(err)
	}
	state, err = s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{RevisionStatus: Unchecked})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(r.ID)
	if err != nil || loaded.Generation != r.Generation+3 || state.Generation != loaded.Generation || len(loaded.ReviewedSliceIDs) != 0 || loaded.RevisionStatus != Unchecked {
		t.Fatal("clearing progress did not persist", err)
	}
}

func TestSQLiteFreshnessRepairsOversizedExistingProgress(t *testing.T) {
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current})
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	oversized := strings.Repeat("x", 1<<20)
	if _, err := db.Exec(`UPDATE progress SET file_id=? WHERE session_id=?`, oversized, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{oversized}, RevisionStatus: Current}); !errors.Is(err, ErrInvalidStateUpdate) {
		t.Fatal("oversized unknown mark accepted", err)
	}
	state, err = s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Unchecked})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(r.ID)
	if err != nil || !reflect.DeepEqual(loaded.ReviewedSliceIDs, []string{"file"}) || loaded.Generation != state.Generation {
		t.Fatal("oversized row was not repaired", err)
	}
}

func BenchmarkSQLiteFreshnessProgress(b *testing.B) {
	for _, count := range []int{0, 1, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			s, err := Open(filepath.Join(b.TempDir(), "storage"))
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			snapshot, ids := progressFixture(max(1, count))
			ids = ids[:count]
			r, err := s.Create(snapshot)
			if err != nil {
				b.Fatal(err)
			}
			state, err := s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: ids, RevisionStatus: Current})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				state, err = s.UpdateState(context.Background(), r.ID, state.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: ids, RevisionStatus: Unchecked})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
