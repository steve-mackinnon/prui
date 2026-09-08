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
	path := filepath.Join(parent, "sessions")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	link := filepath.Join(parent, "linked")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(link); err == nil {
		store.Close()
		t.Fatal("symlink root accepted")
	}
	registry := filepath.Join(path, "repositories.json")
	if err := os.Symlink(sentinel, registry); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LookupRepository("owner/repo"); err == nil {
		t.Fatal("external registry followed")
	}
	if err := os.Remove(registry); err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	if err := os.Symlink(unrelated, filepath.Join(path, id)); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(id); err == nil {
		t.Fatal("symlink session deleted")
	}
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(path, r.ID, "state.json")
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, state); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(r.ID); err == nil {
		t.Fatal("external state followed")
	}
	if err := s.Delete(r.ID); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(sentinel); string(b) != "untouched" {
		t.Fatal("deletion followed internal symlink")
	}
}
