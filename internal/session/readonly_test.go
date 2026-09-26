package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReadOnlyLoadsExistingStoreWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storage")
	writable, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record, err := writable.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Close()
	owner := filepath.Join(path, ".sqlite-owner")
	before, err := os.ReadFile(owner)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got, err := store.Load(record.ID); err != nil || got.ID != record.ID {
		t.Fatalf("Load() = %#v, %v", got, err)
	}
	after, err := os.ReadFile(owner)
	if err != nil || string(after) != string(before) {
		t.Fatalf("read-only open changed owner marker: %q, %v", after, err)
	}
	if _, err := store.Create(fixture()); err == nil {
		t.Fatal("read-only store accepted Create")
	}
	if _, err := store.UpdateState(context.Background(), record.ID, record.Generation, record.SnapshotReference,
		StateUpdate{RevisionStatus: Current}); err == nil {
		t.Fatal("read-only store accepted UpdateState")
	}
}

func TestOpenReadOnlyRequiresExistingAppOwnedStore(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := OpenReadOnly(missing); err == nil {
		t.Fatal("read-only open created missing store")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing store was created: %v", err)
	}
	unrelated := filepath.Join(t.TempDir(), "unrelated")
	if err := os.Mkdir(unrelated, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenReadOnly(unrelated); err == nil {
		t.Fatal("read-only open accepted unowned directory")
	}
}
