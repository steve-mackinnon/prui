package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptions(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		valid bool
	}{
		{[]string{"open", "https://github.com/o/r/pull/1", "--plain"}, true},
		{[]string{}, true},
		{[]string{"open", "1", "--github-repo", "o/r"}, true},
		{[]string{"open", "1"}, false},
		{[]string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r"}, false},
		{[]string{"open", "https://github.com/o/r/pull/1"}, true},
		{[]string{"open", "1", "--github-repo", "o/r"}, true},
		{[]string{"prs", "o/r", "--plain"}, true},
		{[]string{"prs", "--plain"}, true},
		{[]string{"prs", "../r"}, false},
		{[]string{"prs", "o/r", "extra"}, false},
		{[]string{"sessions"}, true}, {[]string{"open", "1", "--github-repo", "o/r", "extra"}, false},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--offline", "--plain"}, true},
		{[]string{"eval-guides", "0123456789abcdef0123456789abcdef"}, true},
		{[]string{"eval-guides", "0123456789abcdef0123456789abcdef", "--plain"}, false},
		{[]string{"eval-guides", "not-a-session"}, false},
		{[]string{"delete", "../outside"}, false},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--offline", "--new"}, false},
		{[]string{"sessions", "extra"}, false},
		{[]string{"resume"}, false},
		{[]string{"open", "1", "--github-repo", "o/r", "--model", "gpt-6-astra"}, false},
		{[]string{"open", "1", "--github-repo", "o/r", "--send-source-to-openai"}, false},
		{[]string{"verify", "https://github.com/o/r/pull/1", "--artifacts", "/tmp/artifacts"}, true},
		{[]string{"verify", "https://github.com/o/r/pull/1"}, false},
		{[]string{"verify", "1", "--github-repo", "o/r", "--artifacts", "/tmp/artifacts", "--measure-runs", "3"}, true},
		{[]string{"verify", "1", "--github-repo", "o/r", "--artifacts", "/tmp/artifacts", "--open-timeout", "90s"}, true},
		{[]string{"verify", "1", "--github-repo", "o/r", "--artifacts", "/tmp/artifacts", "--open-timeout", "-1s"}, false},
		{[]string{"verify", "1", "--github-repo", "o/r", "--artifacts", "/tmp/artifacts", "--measure-runs", "0"}, false},
	} {
		_, e := parseOptions(tc.args)
		if (e == nil) != tc.valid {
			t.Errorf("%v: %v", tc.args, e)
		}
	}
}

func TestCheckoutFromDirectoryRequiresRepositoryRoot(t *testing.T) {
	dir := t.TempDir()
	if _, err := checkoutFromDirectory(dir); err == nil || !strings.Contains(err.Error(), "repository root") {
		t.Fatalf("non-repository directory error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := checkoutFromDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, err := canonicalPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("checkout = %q, want %q", got, want)
	}
}
