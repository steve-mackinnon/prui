package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prui/internal/theme"
)

func TestOptions(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		valid bool
	}{
		{[]string{"open", "https://github.com/o/r/pull/1", "--plain"}, true},
		{[]string{"open", "https://github.com/o/r/pull/1", "--theme", "dark"}, true},
		{[]string{"prs", "--theme", "light"}, true},
		{[]string{"current", "--theme", "high-contrast"}, true},
		{[]string{"--theme", "high-contrast"}, true},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--theme", "terminal"}, true},
		{[]string{"open", "https://github.com/o/r/pull/1", "--theme", "unknown"}, false},
		{[]string{"verify", "https://github.com/o/r/pull/1", "--artifacts", "/tmp/artifacts", "--theme", "dark"}, false},
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
		{[]string{"sessions", "--store", "/tmp/other"}, false},
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

func TestOptionsAcceptNewThemeNamesAcrossInteractiveCommands(t *testing.T) {
	for _, name := range []string{"ayu", "ayu-dark", "gh-dark", "gh-light", "gruvbox-light", "horizon", "material-dark", "material-deep-ocean", "melange", "monokai", "night-owl", "one-light", "poimandres", "rose-pine", "rose-pine-dawn", "vscode-dark", "vscode-light", "wombat"} {
		if _, err := parseOptions([]string{"current", "--theme", name}); err != nil {
			t.Errorf("expanded theme %q rejected: %v", name, err)
		}
	}

	for _, name := range theme.BuiltInNames() {
		t.Run(name, func(t *testing.T) {
			for _, args := range [][]string{
				{"--theme", name},
				{"current", "--theme", name},
				{"open", "https://github.com/o/r/pull/1", "--theme", name},
				{"open", "1", "--github-repo", "o/r", "--plain", "--theme", name},
				{"prs", "--theme", name},
				{"prs", "o/r", "--plain", "--theme", name},
				{"resume", "0123456789abcdef0123456789abcdef", "--theme", name},
				{"resume", "0123456789abcdef0123456789abcdef", "--offline", "--plain", "--theme", name},
			} {
				got, err := parseOptions(args)
				if err != nil {
					t.Fatalf("parseOptions(%v): %v", args, err)
				}
				if got.ThemeName != name {
					t.Fatalf("parseOptions(%v) theme = %q", args, got.ThemeName)
				}
			}
		})
	}
}

func TestOptionsRejectInvalidThemeBeforeSourceParsing(t *testing.T) {
	_, err := parseOptions([]string{"open", "not-a-source", "--theme", "catpuccin"})
	if err == nil || !strings.Contains(err.Error(), "invalid --theme") {
		t.Fatalf("theme validation did not precede source parsing: %v", err)
	}
	for _, name := range theme.BuiltInNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("invalid theme diagnostic omitted %q: %v", name, err)
		}
	}
}

func TestOptionsVerifyRejectsNewPersonalThemes(t *testing.T) {
	for _, name := range theme.BuiltInNames() {
		_, err := parseOptions([]string{"verify", "https://github.com/o/r/pull/1", "--artifacts", "/tmp/artifacts", "--theme", name})
		if err == nil {
			t.Fatalf("verify accepted personal theme %q", name)
		}
	}
}
