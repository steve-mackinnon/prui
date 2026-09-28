package session

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"prui/internal/guide"
	"prui/internal/session/storage"
)

func sqliteGuideStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "storage")
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newSQLiteStore(db), path
}

func TestSQLiteGuideCachePersistsExactComparisonAndInventory(t *testing.T) {
	s, path := sqliteGuideStore(t)
	key, inv, bundle := guideCacheKey(), fixture().Inventory, generatedGuide()
	if err := s.SaveGeneratedGuide(key, bundle, inv); err != nil {
		t.Fatal(err)
	}
	upper := key
	upper.Repository = "OWNER/REPO"
	if got, err := s.LoadGeneratedGuide(upper, inv); err != nil || got == nil || got.Model != bundle.Model {
		t.Fatalf("cache hit = %#v, %v", got, err)
	}
	other := inv
	other.Comparison.InventoryID = "different-inventory"
	if got, err := s.LoadGeneratedGuide(key, other); err != nil || got != nil {
		t.Fatalf("different inventory hit = %#v, %v", got, err)
	}
	other = inv
	other.Units = append(other.Units[:0:0], inv.Units...)
	other.Units[0].ID = "different-unit"
	if got, err := s.LoadGeneratedGuide(key, other); err != nil || got != nil {
		t.Fatalf("invalid for inventory hit = %#v, %v", got, err)
	}
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := storage.OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ro := newSQLiteStore(db)
	if got, err := ro.LoadGeneratedGuide(key, inv); err != nil || got == nil {
		t.Fatalf("reopen cache hit = %#v, %v", got, err)
	}
	if err := ro.SaveGeneratedGuide(key, bundle, inv); !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("read-only save = %v", err)
	}
}

func TestSQLiteGuideRejectsUnavailableInvalidAndOldPrompt(t *testing.T) {
	s, _ := sqliteGuideStore(t)
	key, inv := guideCacheKey(), fixture().Inventory
	for _, bundle := range []guide.Bundle{guide.Fallback("unavailable"), func() guide.Bundle {
		b := generatedGuide()
		b.Items[0].Sections[0].UnitIDs = []string{"unknown"}
		return b
	}(), func() guide.Bundle { b := generatedGuide(); b.PromptVersion = "guides-v1"; return b }()} {
		if err := s.SaveGeneratedGuide(key, bundle, inv); err == nil {
			t.Fatalf("accepted invalid bundle: %#v", bundle)
		}
	}
	if got, err := s.LoadGeneratedGuide(key, inv); err != nil || got != nil {
		t.Fatalf("invalid cache hit = %#v, %v", got, err)
	}
}

func TestSQLiteGuideInvalidPayloadMissesButDatabaseErrorsPropagate(t *testing.T) {
	s, _ := sqliteGuideStore(t)
	key, inv := guideCacheKey(), fixture().Inventory
	if err := s.SaveGeneratedGuide(key, generatedGuide(), inv); err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("UPDATE guide_bundles SET payload = ?", []byte("{")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadGeneratedGuide(key, inv); err != nil || got != nil {
		t.Fatalf("invalid payload = %#v, %v", got, err)
	}
	if _, err := conn.Exec("DROP TABLE guide_cache"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadGeneratedGuide(key, inv); err == nil {
		t.Fatal("database failure became cache miss")
	}
}

func TestSQLiteGuideBundleCodecChecksContentDigest(t *testing.T) {
	b := generatedGuide()
	digest, payload, err := encodeGuideBundle(b)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeGuideBundle(digest, payload)
	if err != nil || got == nil || got.Model != b.Model {
		t.Fatalf("decoded bundle = %#v, %v", got, err)
	}
	payload[len(payload)-2] ^= 1
	if _, err := decodeGuideBundle(digest, payload); err == nil {
		t.Fatal("tampered bundle accepted")
	}
}

func TestSQLiteGuideReplacementChangesOnlyCacheReference(t *testing.T) {
	s, _ := sqliteGuideStore(t)
	key, inv := guideCacheKey(), fixture().Inventory
	first := generatedGuide()
	if err := s.SaveGeneratedGuide(key, first, inv); err != nil {
		t.Fatal(err)
	}
	second := generatedGuide()
	second.Model = "replacement-model"
	if err := s.SaveGeneratedGuide(key, second, inv); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadGeneratedGuide(key, inv)
	if err != nil || got == nil || got.Model != second.Model {
		t.Fatalf("replacement = %#v, %v", got, err)
	}
	conn, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	var cacheRows, bundles int
	if err := conn.QueryRow("SELECT count(*) FROM guide_cache").Scan(&cacheRows); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow("SELECT count(*) FROM guide_bundles").Scan(&bundles); err != nil {
		t.Fatal(err)
	}
	if cacheRows != 1 || bundles != 1 {
		t.Fatalf("replacement retained %d cache rows and %d bundles", cacheRows, bundles)
	}
}

func TestSQLiteGuideWriteRejectsDamagedExistingBundle(t *testing.T) {
	s, _ := sqliteGuideStore(t)
	key, inv := guideCacheKey(), fixture().Inventory
	bundle := generatedGuide()
	if err := s.SaveGeneratedGuide(key, bundle, inv); err != nil {
		t.Fatal(err)
	}
	conn, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("UPDATE guide_bundles SET payload=?", []byte("damaged")); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveGeneratedGuide(key, bundle, inv); err == nil {
		t.Fatal("damaged content-addressed bundle reused")
	}
	var payload []byte
	if err := conn.QueryRow("SELECT payload FROM guide_bundles").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if string(payload) != "damaged" {
		t.Fatalf("damaged artifact was changed: %q", payload)
	}
}
