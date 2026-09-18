package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReadOnlyLoadsExistingStoreWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions")
	writable, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record, err := writable.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Close()
	before, err := os.ReadFile(filepath.Join(path, ".format"))
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
	after, err := os.ReadFile(filepath.Join(path, ".format"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("read-only open changed marker: %q, %v", after, err)
	}
	if _, err := store.Create(fixture()); err == nil {
		t.Fatal("read-only store accepted Create")
	}
}

func TestOpenReadOnlyRequiresExistingAppOwnedStore(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := OpenReadOnly(missing); err == nil {
		t.Fatal("read-only open created a missing store")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing store was created: %v", err)
	}
	unowned := filepath.Join(t.TempDir(), "unowned")
	if err := os.Mkdir(unowned, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenReadOnly(unowned); err == nil {
		t.Fatal("read-only open accepted a store without the format marker")
	}
}
