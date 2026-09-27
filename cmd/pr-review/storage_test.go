package main

import (
	"os"
	"path/filepath"
	"testing"

	"pr-review/internal/session"
)

func temporaryDefaultStore(t *testing.T, home string) string {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
	path, err := session.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStorageCannotMutateReviewedCheckout(t *testing.T) {
	checkout := t.TempDir()
	if err := os.Mkdir(filepath.Join(checkout, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(checkout)
	store := temporaryDefaultStore(t, checkout)
	args := []string{"open", "https://github.com/o/r/pull/1", "--plain"}
	if code := run(args); code != 1 {
		t.Fatalf("unsafe store exit %d", code)
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Fatal("storage mutated checkout before rejecting it")
	}
	alias := filepath.Join(t.TempDir(), "checkout-link")
	if err := os.Symlink(checkout, alias); err != nil {
		t.Fatal(err)
	}
	if err := outsideCheckout(filepath.Join(alias, "new"), checkout); err == nil {
		t.Fatal("symlink bypassed checkout guard")
	}
	if err := outsideCheckout(t.TempDir(), checkout); err != nil {
		t.Fatal(err)
	}
}
