package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// WalkDir retains directory entries even when Git removes a maintenance lock
// before the callback runs. Exercise that exact ordering without a timing race.
func TestBorrowEnumeratedMaintenanceLockAndMissingObject(t *testing.T) {
	for _, name := range []string{"maintenance.lock", "ab/01234567890123456789012345678901234567"} {
		t.Run(name, func(t *testing.T) {
			objects, borrowed := t.TempDir(), t.TempDir()
			path := filepath.Join(objects, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(borrowed, name)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			err = borrowObjectEntry(context.Background(), objects, borrowed, path, entries[0], nil)
			if name == "maintenance.lock" {
				if err != nil {
					t.Fatalf("transient metadata must not be borrowed: %v", err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing real object must fail: %v", err)
			}
		})
	}
}

func TestNewViewDoesNotBorrowMaintenanceLock(t *testing.T) {
	checkout := t.TempDir()
	objects := filepath.Join(checkout, ".git", "objects")
	if err := os.MkdirAll(objects, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(objects, "maintenance.lock"), []byte("lock"), 0600); err != nil {
		t.Fatal(err)
	}
	view, err := NewView(context.Background(), checkout, NewRunner(), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	if _, err := os.Stat(filepath.Join(view.dir, "borrowed", "maintenance.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("maintenance metadata was borrowed: %v", err)
	}
}
