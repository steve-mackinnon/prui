package session

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"pr-review/internal/session/storage"
)

func newSQLiteStore(db *storage.DB) *Store { return &Store{db: db} }

func openSQLiteTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "storage"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newSQLiteStore(db)
}

func TestSQLiteSessionSourceDedupAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openSQLiteTestStore(t)
	first := fixture()
	first.Checkout = []byte("/first")
	created, err := s.Create(first)
	if err != nil {
		t.Fatal(err)
	}
	second := fixture()
	second.Checkout = []byte("/second")
	other, err := s.Create(second)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == other.ID || created.SnapshotReference == other.SnapshotReference {
		t.Fatal("independent sessions must have separate IDs and logical references")
	}
	sqlDB, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	var sources int
	if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM snapshots").Scan(&sources); err != nil || sources != 1 {
		t.Fatalf("source count = %d, %v", sources, err)
	}
	got, err := s.Load(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Inventory.Files[0].NewPath) != string(first.Inventory.Files[0].NewPath) || string(got.Inventory.Patches[first.Inventory.Units[0].PatchReference]) != string(first.Inventory.Patches[first.Inventory.Units[0].PatchReference]) || string(got.Checkout) != "/first" {
		t.Fatalf("binary source or checkout changed: %#v", got)
	}
}

func TestSQLiteSnapshotDescriptionNilAndEmptyAreDistinct(t *testing.T) {
	s := openSQLiteTestStore(t)
	nilDescription, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	snapshot := fixture()
	snapshot.PullRequestDescription = &empty
	emptyDescription, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	gotNil, err := s.Load(nilDescription.ID)
	if err != nil || gotNil.PullRequestDescription != nil {
		t.Fatalf("nil description = %#v, %v", gotNil, err)
	}
	gotEmpty, err := s.Load(emptyDescription.ID)
	if err != nil || gotEmpty.PullRequestDescription == nil || *gotEmpty.PullRequestDescription != "" {
		t.Fatalf("empty description = %#v, %v", gotEmpty, err)
	}
}

func TestSQLiteDerivedRequiresLiveMatchingParent(t *testing.T) {
	ctx := context.Background()
	s := openSQLiteTestStore(t)
	parent, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	child := fixture()
	child.DerivedFrom = parent.ID
	created, err := s.Create(child)
	if err != nil {
		t.Fatal(err)
	}
	changed := fixture()
	changed.DerivedFrom = parent.ID
	newDescription := "changed"
	changed.PullRequestDescription = &newDescription
	if _, err := s.Create(changed); err == nil {
		t.Fatal("derived session accepted changed source")
	}
	sqlDB, _ := s.db.SQL()
	if _, err := sqlDB.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(created.ID); err != nil {
		t.Fatalf("child lost after parent deletion: %v", err)
	}
	if _, err := s.Create(child); err == nil {
		t.Fatal("derived session accepted deleted parent")
	}
}

func TestSQLiteSessionPreservesGuideProvenance(t *testing.T) {
	s := openSQLiteTestStore(t)
	parent, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := fixture()
	snapshot.DerivedFrom = parent.ID
	bundle := generatedGuide()
	snapshot.Guides = &bundle
	child, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(child.ID)
	if err != nil || got.DerivedFrom != parent.ID || got.Guides == nil || got.Guides.Model != bundle.Model {
		t.Fatalf("guide provenance = %#v, %v", got, err)
	}
	sqlDB, _ := s.db.SQL()
	var sources, bundles int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM snapshots`).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`SELECT count(*) FROM guide_bundles`).Scan(&bundles); err != nil {
		t.Fatal(err)
	}
	if sources != 1 || bundles != 1 {
		t.Fatalf("source rows=%d guide bundles=%d", sources, bundles)
	}
}

func TestSQLitePayloadIndexedIdentityTamperFailsLoad(t *testing.T) {
	s := openSQLiteTestStore(t)
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := s.db.SQL()
	if _, err := sqlDB.Exec(`UPDATE snapshots SET inventory_id='wrong'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(r.ID); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("tampered indexed identity = %v", err)
	}
}

func TestSQLiteDerivedRejectsCorruptLiveParent(t *testing.T) {
	s := openSQLiteTestStore(t)
	parent, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	db, _ := s.db.SQL()
	if _, err := db.Exec(`UPDATE snapshots SET payload=x'7b'`); err != nil {
		t.Fatal(err)
	}
	child := fixture()
	child.DerivedFrom = parent.ID
	if _, err := s.Create(child); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("corrupt live parent accepted: %v", err)
	}
}

func TestSQLiteSessionCreateLeavesCorruptMembershipUnchanged(t *testing.T) {
	s := openSQLiteTestStore(t)
	if _, err := s.Create(fixture()); err != nil {
		t.Fatal(err)
	}
	db, _ := s.db.SQL()
	if _, err := db.Exec(`DELETE FROM snapshot_files`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(fixture()); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("missing source membership was repaired: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM snapshot_files`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("corrupt source was changed: count=%d, %v", count, err)
	}
}

func TestSQLiteSessionRejectsInvalidPinnedSHAs(t *testing.T) {
	store := openSQLiteTestStore(t)
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) {
			s.Inventory.Comparison.Metadata.BaseSHA = "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
		},
		func(s *Snapshot) {
			s.Inventory.Comparison.Metadata.HeadSHA = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
		},
	} {
		snapshot := fixture()
		mutate(&snapshot)
		if _, err := store.Create(snapshot); err == nil {
			t.Fatal("invalid pinned SHA accepted")
		}
	}
}
