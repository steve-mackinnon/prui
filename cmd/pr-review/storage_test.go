package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageCannotMutateReviewedCheckout(t *testing.T) {
	checkout := t.TempDir()
	if err := os.Mkdir(filepath.Join(checkout, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(checkout)
	store := filepath.Join(checkout, "new", "sessions")
	args := []string{"open", "https://github.com/o/r/pull/1", "--store", store, "--plain"}
	if code := run(args); code != 1 {
		t.Fatalf("unsafe store exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(checkout, "new")); !os.IsNotExist(err) {
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
