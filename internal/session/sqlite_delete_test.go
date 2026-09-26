package session

import "testing"

func TestSQLiteDeleteReclaimsOnlyUnreferencedData(t *testing.T) {
	s := lookupStore(t)
	parent, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	snap := parent.Snapshot
	snap.DerivedFrom = parent.ID
	child, err := s.Create(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(child.ID); err != nil {
		t.Fatalf("child after parent deletion: %v", err)
	}
	db, _ := s.db.SQL()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM snapshots").Scan(&count); err != nil || count != 1 {
		t.Fatalf("shared snapshot count %d: %v", count, err)
	}
	if err := s.Delete(child.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"sessions", "progress", "snapshots", "snapshot_files"} {
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count %d: %v", table, count, err)
		}
	}
	if err := s.Delete(child.ID); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}

func TestSQLiteDeleteKeepsReusableGuidesAndRollsBackFailure(t *testing.T) {
	s := lookupStore(t)
	snap := fixture()
	bundle := generatedGuide()
	snap.Guides = &bundle
	r, err := s.Create(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveGeneratedGuide(guideCacheKey(), bundle, snap.Inventory); err != nil {
		t.Fatal(err)
	}
	if err := s.RememberRepository("owner/repo", "/checkout"); err != nil {
		t.Fatal(err)
	}
	db, _ := s.db.SQL()
	if _, err := db.Exec(`CREATE TRIGGER prevent_reclaim BEFORE DELETE ON snapshots BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(r.ID); err == nil {
		t.Fatal("expected deletion failure")
	}
	if _, err := s.Load(r.ID); err != nil {
		t.Fatalf("failed deletion changed record: %v", err)
	}
	if _, err := db.Exec("DROP TRIGGER prevent_reclaim"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(r.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadGeneratedGuide(guideCacheKey(), snap.Inventory); err != nil || got == nil {
		t.Fatalf("cached guide lost: %v", err)
	}
	if got, err := s.LookupRepository("owner/repo"); err != nil || got != "/checkout" {
		t.Fatalf("registry lost: %v", err)
	}
}
