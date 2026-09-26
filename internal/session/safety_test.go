package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRejectsUnownedRootsAndSymlinks(t *testing.T) {
	parent := t.TempDir()
	unrelated := filepath.Join(parent, "unrelated")
	if err := os.Mkdir(unrelated, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(unrelated, "keep")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(unrelated); err == nil {
		store.Close()
		t.Fatal("unrelated nonempty directory adopted")
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "untouched" {
		t.Fatal("unrelated data modified")
	}
	path := filepath.Join(parent, "storage")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "linked")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(link); err == nil {
		store.Close()
		t.Fatal("symlink root accepted")
	}
	dbPath := filepath.Join(path, "store.sqlite3")
	if err := os.Rename(dbPath, filepath.Join(parent, "saved.sqlite3")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, dbPath); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(path); err == nil {
		store.Close()
		t.Fatal("symlink database accepted")
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "untouched" {
		t.Fatal("external target modified")
	}
}

func TestStoreRejectsSymlinkControlAndJournalFiles(t *testing.T) {
	for _, name := range []string{".sqlite-owner", ".sqlite-init.lock", "store.sqlite3-journal"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			path := filepath.Join(parent, "storage")
			store, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(parent, "untouched")
			if err := os.WriteFile(target, []byte("sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			control := filepath.Join(path, name)
			if err := os.Remove(control); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.Symlink(target, control); err != nil {
				t.Fatal(err)
			}
			if got, err := Open(path); err == nil {
				_ = got.Close()
				t.Fatal("writable open followed symlink")
			}
			if got, err := OpenReadOnly(path); err == nil {
				_ = got.Close()
				t.Fatal("read-only open followed symlink")
			}
			if b, err := os.ReadFile(target); err != nil || string(b) != "sentinel" {
				t.Fatalf("target changed: %q %v", b, err)
			}
		})
	}
}
