package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPinnedIdentity(t *testing.T) {
	for _, tc := range []struct {
		input, repo string
		valid       bool
	}{
		{"https://github.com/owner/repo/pull/42", "", true}, {"42", "owner/repo", true},
		{"42", "", false}, {"https://evil.test/o/r/pull/1", "", false},
		{"https://github.com/o/r/pull/1?x=y", "", false}, {"-1", "o/r", false},
		{"https://user@github.com/o/r/pull/1", "", false}, {"1", "../repo", false},
	} {
		_, err := ParseIdentity(tc.input, tc.repo)
		if (err == nil) != tc.valid {
			t.Errorf("%q: %v", tc.input, err)
		}
	}
}

func TestSafetyProcessBounds(t *testing.T) {
	r := NewRunner()
	_, err := r.Run(context.Background(), Request{Program: "/bin/sh", Args: []string{"-c", "while :; do printf 1234567890; done"}, Limit: 100, Timeout: time.Second})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("output limit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = r.Run(ctx, Request{Program: "/bin/sleep", Args: []string{"10"}, Limit: 100, Timeout: time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	dir := t.TempDir()
	_, err = r.Run(context.Background(), Request{Program: "/bin/sh", Args: []string{"-c", "printf 1234567890 > data; sleep 10"}, Dir: dir, Limit: 100, Timeout: time.Second, StorageDir: dir, StorageLimit: 1})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("storage limit: %v", err)
	}
}

func TestSafetyCredentialEnvironment(t *testing.T) {
	env := gitEnvironment(t.TempDir(), "synthetic-token")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "synthetic-token") || !strings.Contains(joined, "AUTHORIZATION: basic ") {
		t.Fatal("credential transport must be encoded environment-only header")
	}
	if !strings.Contains(joined, "GIT_CONFIG_NOSYSTEM=1") || !strings.Contains(joined, "GIT_NO_LAZY_FETCH=1") {
		t.Fatal("missing isolation")
	}
}

func TestSafetyRejectObjectSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, ".git", "objects", "ab")); err != nil {
		t.Fatal(err)
	}
	_, err := NewView(context.Background(), dir, NewRunner(), Defaults())
	if err == nil {
		t.Fatal("object symlink accepted")
	}
}
