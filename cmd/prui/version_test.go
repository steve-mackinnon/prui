package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionWithoutCheckoutOrStorage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", "")
	// A file where the data directory would be makes storage access fail.
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", blocked)
	out, code := captureStdout(t, func() int { return run([]string{"--version"}) })
	if code != 0 || out != "prui dev (commit unknown)\n" {
		t.Fatalf("--version = %q, exit %d", out, code)
	}
}
