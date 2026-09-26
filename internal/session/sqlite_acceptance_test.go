package session

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"pr-review/internal/session/storage"
)

func TestSQLiteHundredReopensShareSource(t *testing.T) {
	s := lookupStore(t)
	snap := fixture()
	parent, err := s.Create(snap)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 100; i++ {
		copy := snap
		copy.Checkout = []byte(fmt.Sprintf("/checkout/%d", i))
		if i%2 == 1 {
			bundle := generatedGuide()
			copy.Guides = &bundle
			copy.DerivedFrom = parent.ID
		}
		r, err := s.Create(copy)
		if err != nil {
			t.Fatal(err)
		}
		if i%3 == 0 {
			if _, err := s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current}); err != nil {
				t.Fatal(err)
			}
		}
	}
	db, _ := s.db.SQL()
	for table, want := range map[string]int{"sessions": 100, "snapshots": 1, "guide_bundles": 1, "progress": 33} {
		var got int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil || got != want {
			t.Fatalf("%s=%d want %d: %v", table, got, want, err)
		}
	}
	r, err := s.Load(parent.ID)
	if err != nil || r.Generation != 1 || len(r.ReviewedSliceIDs) != 0 {
		t.Fatalf("parent state changed: %#v %v", r, err)
	}
	changed := snap
	body := "changed description"
	changed.PullRequestDescription = &body
	if _, err := s.Create(changed); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM snapshots").Scan(&count); err != nil || count != 2 {
		t.Fatalf("changed source deduplicated: %d %v", count, err)
	}
}

func TestSQLiteDiskFullRollsBackProgress(t *testing.T) {
	s := lookupStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.db.SQL()
	if _, err := db.Exec("CREATE TABLE test_space(payload BLOB)"); err != nil {
		t.Fatal(err)
	}
	var pages int
	if err := db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", pages)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TEMP TRIGGER exhaust_space BEFORE UPDATE ON sessions BEGIN INSERT INTO test_space VALUES(zeroblob(1048576)); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current}); !errors.Is(err, storage.ErrStorage) {
		t.Fatalf("disk full error: %v", err)
	}
	got, err := s.Load(r.ID)
	if err != nil || got.Generation != 1 || len(got.ReviewedSliceIDs) != 0 {
		t.Fatalf("partial state after full: %#v %v", got, err)
	}
}

func TestSQLiteCommitContentionIsBoundedAndExplicit(t *testing.T) {
	s := lookupStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	other, err := Open(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	db, _ := other.db.SQL()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var generation int
	if err := tx.QueryRow("SELECT generation FROM sessions WHERE id=?", r.ID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = s.UpdateState(context.Background(), r.ID, r.Generation, r.SnapshotReference, StateUpdate{RevisionStatus: Current})
	if !errors.Is(err, ErrCommitUncertain) || !errors.Is(err, storage.ErrBusy) {
		t.Fatalf("commit contention: %v", err)
	}
	if time.Since(start) > 12*time.Second {
		t.Fatalf("unbounded commit wait: %v", time.Since(start))
	}
	_ = tx.Rollback()
	got, err := s.Load(r.ID)
	if err != nil || got.Generation != 1 {
		t.Fatalf("partial commit: %#v %v", got, err)
	}
}
