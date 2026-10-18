package source

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

func TestBorrowObjectsPreservesIsolatedHardLinks(t *testing.T) {
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprint(workers), func(t *testing.T) {
			objects, borrowed := t.TempDir(), t.TempDir()
			paths := []string{"ab/01234567890123456789012345678901234567", "cd/01234567890123456789012345678901234567", "pack/pack-data.pack", "pack/pack-data.idx"}
			for _, path := range append(append([]string{}, paths...), "info/alternates", "pack/pack-data.promisor", "pack/pack-data.bitmap", "pack/pack-data.keep", "maintenance.lock") {
				full := filepath.Join(objects, path)
				if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(path), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// Even a symlink inside ignored metadata must never be followed.
			if err := os.Symlink(t.TempDir(), filepath.Join(objects, "info", "untrusted")); err != nil {
				t.Fatal(err)
			}
			if err := borrowObjects(context.Background(), objects, borrowed, workers); err != nil {
				t.Fatal(err)
			}
			var got []string
			if err := filepath.WalkDir(borrowed, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() {
					rel, err := filepath.Rel(borrowed, path)
					if err != nil {
						return err
					}
					got = append(got, filepath.ToSlash(rel))
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			sort.Strings(paths)
			if !reflect.DeepEqual(got, paths) {
				t.Fatalf("borrowed metadata or lost data: got %v want %v", got, paths)
			}
			for _, path := range paths {
				original, err := os.Stat(filepath.Join(objects, path))
				if err != nil {
					t.Fatal(err)
				}
				linked, err := os.Stat(filepath.Join(borrowed, path))
				if err != nil || !os.SameFile(original, linked) {
					t.Fatal("object lifetime is not retained by a hard link", path, err)
				}
			}
			if err := os.RemoveAll(objects); err != nil {
				t.Fatal(err)
			}
			for _, path := range paths {
				data, err := os.ReadFile(filepath.Join(borrowed, path))
				if err != nil || string(data) != path {
					t.Fatal("source deletion invalidated borrowed object", path, err)
				}
			}
		})
	}
}

func TestBorrowObjectsRejectsSymlinksAndCancellation(t *testing.T) {
	for _, workers := range []int{1, 4} {
		for _, shape := range []string{"directory-link", "file-link", "canceled"} {
			t.Run(fmt.Sprintf("%d/%s", workers, shape), func(t *testing.T) {
				objects, borrowed := t.TempDir(), t.TempDir()
				for i := range 100 {
					if err := os.WriteFile(filepath.Join(objects, fmt.Sprintf("%04d", i)), []byte("object"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				ctx := context.Background()
				if shape == "canceled" {
					canceled, cancel := context.WithCancel(ctx)
					cancel()
					ctx = canceled
				} else {
					target := t.TempDir()
					if shape == "file-link" {
						target = filepath.Join(objects, "0000")
					}
					if err := os.Symlink(target, filepath.Join(objects, "0099-link")); err != nil {
						t.Fatal(err)
					}
				}
				err := borrowObjects(ctx, objects, borrowed, workers)
				if err == nil || shape == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatal("borrow ignored failed storage or cancellation", err)
				}
				if err := os.RemoveAll(borrowed); err != nil {
					t.Fatal("borrow workers prevented cleanup", err)
				}
			})
		}
	}
}

func TestNewViewCleansFailedConcurrentBorrow(t *testing.T) {
	checkout := t.TempDir()
	objects := filepath.Join(checkout, ".git", "objects")
	if err := os.MkdirAll(objects, 0700); err != nil {
		t.Fatal(err)
	}
	for i := range 100 {
		if err := os.WriteFile(filepath.Join(objects, fmt.Sprintf("%04d", i)), []byte("object"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(objects, "9999-link")); err != nil {
		t.Fatal(err)
	}
	private := t.TempDir()
	t.Setenv("TMPDIR", private)
	if view, err := NewView(context.Background(), checkout, NewRunner(), Defaults()); view != nil || err == nil {
		t.Fatal("failed borrow returned a usable view", view, err)
	}
	entries, err := os.ReadDir(private)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed borrow left private data after return", entries, err)
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
