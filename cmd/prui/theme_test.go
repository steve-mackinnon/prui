package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prui/internal/theme"
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

func TestThemeResolutionNewPresetsRetainsBaseOverrideProvenance(t *testing.T) {
	for _, name := range theme.BuiltInNames() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "theme.json")
			input := `{"theme":"` + name + `","colors":{"foreground":"#cdd6f4","background":"default","added":"green"}}`
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			for _, cli := range []string{"", theme.Terminal, theme.OneDark} {
				got, warning, savable := resolveThemeConfigDetails(cli, path)
				wantName := name
				if cli != "" {
					wantName = cli
				}
				if got.Name != wantName || warning != "" || !savable {
					t.Fatalf("CLI %q: resolved %q, warning %q, savable %v", cli, got.Name, warning, savable)
				}
				wantOverrides := map[theme.Token]string{theme.Foreground: "#cdd6f4", theme.Background: "default", theme.Added: "green"}
				if len(got.Overrides()) != len(wantOverrides) {
					t.Fatalf("override provenance = %v", got.Overrides())
				}
				for token, syntax := range wantOverrides {
					if got.Syntax(token) != syntax || got.Overrides()[token] != syntax {
						t.Errorf("CLI %q: %s value/provenance = %q/%q, want %q", cli, token, got.Syntax(token), got.Overrides()[token], syntax)
					}
				}
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != input {
				t.Fatal("resolution changed persisted preference")
			}
		})
	}
}

func TestThemeResolutionNewPresetWithInvalidBaseConfigRemainsReadOnly(t *testing.T) {
	for _, name := range theme.BuiltInNames() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "theme.json")
			input := `{"theme":"dark","colors":{"background":"\u001b[31m"}}`
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			got, warning, saverPath := resolveThemeForInteractiveInvocation(true, name, func() (string, error) { return path, nil })
			if got.Name != name || warning == "" || saverPath != "" || len(got.Overrides()) != 0 {
				t.Fatalf("invalid config resolution: %q, warning %q, saver path %q, overrides %v", got.Name, warning, saverPath, got.Overrides())
			}
			if strings.Contains(warning, "31m") || strings.Contains(warning, path) {
				t.Fatalf("warning leaked config contents/path: %q", warning)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != input {
				t.Fatal("invalid configuration rewritten")
			}
		})
	}
}

func TestThemeResolutionPlainNewPresetsNeverConsultConfig(t *testing.T) {
	for _, name := range theme.BuiltInNames() {
		got, warning, path := resolveThemeForInteractiveInvocation(false, name, func() (string, error) { t.Fatal("plain invocation consulted theme configuration"); return "", nil })
		if got.Name != theme.Terminal || warning != "" || path != "" || len(got.Overrides()) != 0 {
			t.Fatalf("plain %q: theme %q, warning %q, path %q", name, got.Name, warning, path)
		}
	}
}

func TestThemeCatalogPersistenceRetainsOverridesAcrossLaunch(t *testing.T) {
	for _, name := range theme.BuiltInNames() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "theme.json")
			if err := os.WriteFile(path, []byte(`{"theme":"terminal","colors":{"foreground":"default","background":"#112233","added":"green"}}`), 0600); err != nil {
				t.Fatal(err)
			}
			before, warning, saverPath := resolveThemeForInteractiveInvocation(true, name, func() (string, error) { return path, nil })
			if before.Name != name || warning != "" || saverPath != path {
				t.Fatalf("interactive configuration=%q,%q,%q", before.Name, warning, saverPath)
			}
			if _, err := theme.PersistSelection(saverPath, name); err != nil {
				t.Fatal(err)
			}
			after, warning := resolveThemeConfig("", path)
			if after.Name != name || warning != "" {
				t.Fatalf("next launch theme=%q warning=%q", after.Name, warning)
			}
			for token, want := range map[theme.Token]string{theme.Foreground: "default", theme.Background: "#112233", theme.Added: "green"} {
				if after.Syntax(token) != want || after.Overrides()[token] != want {
					t.Fatalf("persisted %s value/provenance=%q/%q", token, after.Syntax(token), after.Overrides()[token])
				}
			}
		})
	}
}
