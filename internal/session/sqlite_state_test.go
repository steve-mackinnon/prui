package session

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"sync"
	"testing"

	"prui/internal/session/storage"
)

func TestSQLiteStateUpdateCommitsProgressAndRejectsStaleWriter(t *testing.T) {
	ctx := context.Background()
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	otherDB, err := storage.Open(ctx, s.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	other := newSQLiteStore(otherDB)
	updated, err := s.UpdateState(ctx, r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Generation != 2 || len(updated.ReviewedSliceIDs) != 1 || updated.ReviewedSliceIDs[0] != "file" {
		t.Fatalf("returned state = %#v", updated)
	}
	if _, err := other.UpdateState(ctx, r.ID, r.Generation, r.SnapshotReference, StateUpdate{RevisionStatus: Stale}); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("stale writer = %v", err)
	}
	loaded, err := other.Load(r.ID)
	if err != nil || loaded.Generation != 2 || loaded.RevisionStatus != Current || len(loaded.ReviewedSliceIDs) != 1 {
		t.Fatalf("persisted state = %#v, %v", loaded, err)
	}
}

func TestSQLiteStateConcurrentHandlesAcceptExactlyOneWriter(t *testing.T) {
	ctx := context.Background()
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	otherDB, err := storage.Open(ctx, s.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	stores := []*Store{s, newSQLiteStore(otherDB)}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, store := range stores {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			<-start
			_, err := store.UpdateState(ctx, r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current})
			results <- err
		}(store)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrStateConflict):
			conflicts++
		default:
			t.Fatalf("unexpected competing writer error: %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", success, conflicts)
	}
}

func TestSQLiteStateUpdateValidatesMembershipAndLeavesStateUnchanged(t *testing.T) {
	ctx := context.Background()
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateState(ctx, r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"invented"}, RevisionStatus: Current}); !errors.Is(err, ErrInvalidStateUpdate) {
		t.Fatalf("unknown file = %v", err)
	}
	if _, err := s.UpdateState(ctx, r.ID, r.Generation, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", StateUpdate{RevisionStatus: Current}); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("wrong reference = %v", err)
	}
	loaded, err := s.Load(r.ID)
	if err != nil || loaded.Generation != 1 || loaded.RevisionStatus != Unchecked {
		t.Fatalf("state changed after failure: %#v, %v", loaded, err)
	}
}

func TestSQLiteStateUpdateNeverReadsSourcePayload(t *testing.T) {
	ctx := context.Background()
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := s.db.SQL()
	if _, err := sqlDB.ExecContext(ctx, `UPDATE snapshots SET payload = x'7b'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateState(ctx, r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current}); err != nil {
		t.Fatalf("state path read corrupt source payload: %v", err)
	}
	if _, err := s.Load(r.ID); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("corrupt source load = %v", err)
	}
}

func TestSQLiteStateRejectsOverflowAndReadOnly(t *testing.T) {
	ctx := context.Background()
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateState(ctx, r.ID, math.MaxUint64, r.SnapshotReference, StateUpdate{RevisionStatus: Current}); !errors.Is(err, ErrInvalidStateUpdate) {
		t.Fatalf("overflow = %v", err)
	}
	readDB, err := storage.OpenReadOnly(ctx, filepath.Clean(s.Path()))
	if err != nil {
		t.Fatal(err)
	}
	defer readDB.Close()
	if _, err := newSQLiteStore(readDB).UpdateState(ctx, r.ID, r.Generation, r.SnapshotReference, StateUpdate{RevisionStatus: Current}); !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("read-only update = %v", err)
	}
}

func TestSQLiteStateCanceledContextIsExplicit(t *testing.T) {
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.UpdateState(ctx, r.ID, r.Generation, r.SnapshotReference, StateUpdate{RevisionStatus: Current}); !errors.Is(err, storage.ErrBusy) {
		t.Fatalf("canceled update = %v", err)
	}
}
