package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pr-review/internal/theme"
)

func TestThemeResolutionPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(`{"theme":"light","colors":{"added":"#112233"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	got, warning := resolveThemeConfig("dark", path)
	if warning != "" {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if got.Name != theme.Dark || got.Syntax(theme.Added) != "#112233" {
		t.Fatalf("CLI theme did not win while retaining overrides: %#v", got)
	}
}

func TestThemeResolutionDefaultsWhenConfigIsMissing(t *testing.T) {
	got, warning := resolveThemeConfig("", filepath.Join(t.TempDir(), "missing.json"))
	if warning != "" || got.Name != theme.Terminal {
		t.Fatalf("missing config = %q, warning %q", got.Name, warning)
	}
}

func TestThemeResolutionFallsBackForInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(`{"theme":"not-a-theme"}`), 0600); err != nil {
		t.Fatal(err)
	}

	got, warning := resolveThemeConfig("", path)
	if got.Name != theme.Terminal {
		t.Fatalf("theme = %q, want terminal", got.Name)
	}
	if !strings.Contains(warning, "Theme configuration ignored") || strings.Contains(warning, "not-a-theme") {
		t.Fatalf("warning is not safe: %q", warning)
	}
	if _, _, savable := resolveThemeConfigDetails("", path); savable {
		t.Fatal("malformed configuration must not receive a persistence saver")
	}
}

func TestThemeResolutionAllowsSaverForMissingOrValidConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	if _, _, savable := resolveThemeConfigDetails("", missing); !savable {
		t.Fatal("missing configuration should allow first-time persistence")
	}
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, savable := resolveThemeConfigDetails("", path); !savable {
		t.Fatal("valid configuration should allow persistence")
	}
}

func TestThemeResolutionDoesNotReadConfigurationForNonInteractiveInvocation(t *testing.T) {
	called := false
	got, warning := resolveThemeForInvocation(false, "dark", func() (string, error) {
		called = true
		return "", errors.New("must not be called")
	})
	if called || warning != "" || got.Name != theme.Terminal {
		t.Fatalf("non-interactive resolution = %q, warning %q, config read %t", got.Name, warning, called)
	}
}
