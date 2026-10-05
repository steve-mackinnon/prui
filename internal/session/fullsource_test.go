package session

import (
	"bytes"
	"path/filepath"
	"prui/internal/inventory"
	"testing"
)

func TestFullSourceOfflineRestartIntegrityAndDeletion(t *testing.T) {
	snapshot := fixture()
	const oid = "ce013625030ba8dba906f756967f9e9ca394464a"
	snapshot.Inventory.Files[0].NewOID = oid
	snapshot.Inventory.Files[0].NewMode = "100644"
	snapshot.Inventory.FullSource = &inventory.FullSource{Blobs: map[string][]byte{oid: []byte("hello\n")}}
	path := filepath.Join(t.TempDir(), "storage")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.Load(saved.ID)
	if err != nil || !bytes.Equal(loaded.Inventory.FullSource.Blobs[oid], []byte("hello\n")) {
		t.Fatal("offline restart lost source", err)
	}
	snapshot.Inventory.FullSource.Blobs[oid] = []byte("tampered")
	if _, err = store.Create(snapshot); err == nil {
		t.Fatal("invalid source identity accepted")
	}
	if err = store.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Load(saved.ID); err == nil {
		t.Fatal("deleted session remained")
	}
}
