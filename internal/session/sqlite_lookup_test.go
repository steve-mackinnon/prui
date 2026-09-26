package session

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"pr-review/internal/session/storage"
	"pr-review/internal/source"
)

func lookupStore(t *testing.T) *Store {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newSQLiteStore(db)
}

func TestSQLiteSummaryDoesNotDecodeSource(t *testing.T) {
	s := lookupStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec("UPDATE snapshots SET payload = ?", []byte("invalid source")); err != nil {
		t.Fatal(err)
	}
	entries, err := s.List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("List = %#v, %v", entries, err)
	}
	got := entries[0]
	if got.ID != r.ID || got.Repository != "owner/repo" || got.Number != 7 || got.SliceCount != 1 || got.ReviewedCount != 0 || got.Err != nil {
		t.Fatalf("summary = %#v", got)
	}
	if _, err := s.Load(r.ID); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("corrupt load = %v", err)
	}
}

func TestSQLiteLookupOrdersMatchingCandidatesAndSkipsBadRecords(t *testing.T) {
	s := lookupStore(t)
	first, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot := fixture()
	description := "newer description"
	secondSnapshot.PullRequestDescription = &description
	second, err := s.Create(secondSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	other := fixture()
	other.Inventory.Comparison.Metadata.Identity.Number++
	if _, err := s.Create(other); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := s.db.SQL()
	if _, err := sqlDB.Exec("UPDATE sessions SET updated_at_ns = CASE id WHEN ? THEN 1 WHEN ? THEN 2 ELSE 3 END", first.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	id := first.Inventory.Comparison.Metadata.Identity
	got, err := s.LatestComparison(id)
	if err != nil || got == nil || got.ID != second.ID {
		t.Fatalf("latest = %#v, %v", got, err)
	}
	if _, err := sqlDB.Exec("UPDATE snapshots SET payload=? WHERE digest=(SELECT snapshot_digest FROM sessions WHERE id=?)", []byte("bad"), second.ID); err != nil {
		t.Fatal(err)
	}
	got, err = s.LatestComparison(id)
	if err != nil || got == nil || got.ID != first.ID {
		t.Fatalf("fallback = %#v, %v", got, err)
	}
	found, err := s.HasComparisonSnapshot(id)
	if err != nil || !found {
		t.Fatalf("HasComparison = %v, %v", found, err)
	}
	found, err = s.HasComparisonSnapshot(source.Identity{Repository: "owner/missing", Number: 1})
	if err != nil || found {
		t.Fatalf("missing = %v, %v", found, err)
	}
}

func TestSQLiteLookupPinnedComparisonAndDescription(t *testing.T) {
	s := lookupStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	metadata := r.Inventory.Comparison.Metadata
	metadata.Identity.Repository = "OWNER/REPO"
	metadata.Description = "fresh body"
	got, err := s.LoadComparisonSnapshot(metadata)
	if err != nil || got == nil || got.PullRequestDescription == nil || *got.PullRequestDescription != metadata.Description {
		t.Fatalf("snapshot = %#v, %v", got, err)
	}
	metadata.HeadSHA = strings.Repeat("c", 40)
	if got, err = s.LoadComparisonSnapshot(metadata); err != nil || got != nil {
		t.Fatalf("changed head = %#v, %v", got, err)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LatestComparison(r.Inventory.Comparison.Metadata.Identity); !errors.Is(err, storage.ErrClosed) {
		t.Fatalf("closed = %v", err)
	}
}

func TestSQLiteLookupQueryUsesRecencyIndex(t *testing.T) {
	s := lookupStore(t)
	sqlDB, _ := s.db.SQL()
	rows, err := sqlDB.Query("EXPLAIN QUERY PLAN SELECT id FROM sessions WHERE repository=? AND pr_number=? ORDER BY updated_at_ns DESC,id ASC", "owner/repo", 7)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "sessions_recent") {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("comparison lookup does not use recency index")
	}
}
