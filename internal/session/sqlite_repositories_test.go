package session

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"prui/internal/session/storage"
)

func TestSQLiteRepositoryOrderReplacementAndReadOnlyReopen(t *testing.T) {
	s, path := sqliteGuideStore(t)
	a, b, replacement := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b"), filepath.Join(t.TempDir(), "replacement")
	if err := s.RememberRepository("Owner/Repo", a); err != nil {
		t.Fatal(err)
	}
	if err := s.RememberRepository("other/repo", b); err != nil {
		t.Fatal(err)
	}
	if err := s.RememberRepository("owner/repo", replacement); err != nil {
		t.Fatal(err)
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
	list, err := ro.ListRepositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0] != (Repository{"owner/repo", replacement}) || list[1] != (Repository{"other/repo", b}) {
		t.Fatalf("repository order = %#v", list)
	}
	if got, err := ro.LookupRepository("OWNER/REPO"); err != nil || got != replacement {
		t.Fatalf("lookup = %q, %v", got, err)
	}
	if err := ro.RememberRepository("new/repo", a); !errors.Is(err, storage.ErrReadOnly) {
		t.Fatalf("read-only write = %v", err)
	}
}

func TestSQLiteRepositoryRejectsInvalidInputAndRows(t *testing.T) {
	s, _ := sqliteGuideStore(t)
	if _, err := s.LookupRepository("owner/repo"); !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("missing lookup = %v", err)
	}
	for _, pair := range [][2]string{{"bad repo", "/tmp/checkout"}, {"owner/repo", "relative"}, {"owner/repo", "/tmp/../tmp/checkout"}} {
		if err := s.RememberRepository(pair[0], pair[1]); err == nil {
			t.Fatalf("accepted %#v", pair)
		}
	}
	conn, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("INSERT INTO repositories(repository,checkout,ordinal) VALUES('bad repo', ?, 0)", []byte("relative")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListRepositories(); err == nil {
		t.Fatal("invalid row hidden in list")
	}
	if err := s.RememberRepository("owner/repo", filepath.Join(t.TempDir(), "checkout")); err == nil {
		t.Fatal("invalid row overwritten")
	}
	if _, err := conn.Exec("DROP TABLE repositories"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListRepositories(); err == nil {
		t.Fatal("database error hidden")
	}
}
