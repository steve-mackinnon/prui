package theme

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPathUsesInjectedPlatformInputs(t *testing.T) {
	tests := []struct {
		name string
		in   ConfigPathInputs
		want string
	}{
		{"macOS", ConfigPathInputs{GOOS: "darwin", Home: "/users/reviewer"}, "/users/reviewer/Library/Application Support/pr-review/theme.json"},
		{"Linux XDG", ConfigPathInputs{GOOS: "linux", XDGConfigHome: "/var/config"}, "/var/config/pr-review/theme.json"},
		{"Linux ignores relative XDG", ConfigPathInputs{GOOS: "linux", Home: "/home/reviewer", XDGConfigHome: "config"}, "/home/reviewer/.config/pr-review/theme.json"},
		{"Linux fallback", ConfigPathInputs{GOOS: "linux", Home: "/home/reviewer"}, "/home/reviewer/.config/pr-review/theme.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConfigPath(tt.in)
			if err != nil {
				t.Fatalf("ConfigPath() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ConfigPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigPathRejectsUnsupportedPlatformAndMissingHome(t *testing.T) {
	for _, in := range []ConfigPathInputs{
		{GOOS: "windows", Home: "/users/reviewer"},
		{GOOS: "darwin"},
		{GOOS: "linux"},
	} {
		if _, err := ConfigPath(in); err == nil {
			t.Fatalf("ConfigPath(%+v) succeeded", in)
		}
	}
}

func TestConfigLoadMissingIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "theme.json")
	config, exists, err := LoadConfig(path)
	if err != nil || exists {
		t.Fatalf("LoadConfig() = (%+v, %v, %v), want missing without error", config, exists, err)
	}
	if config.Theme != "" || len(config.Colors) != 0 {
		t.Fatalf("missing config = %+v, want zero config", config)
	}
}

func TestConfigLoadValidatesWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	input := "{\n  \"theme\": \"dark\",\n  \"colors\": {\n    \"added\": \"#112233\",\n    \"warning\": \"bright-yellow\"\n  }\n}\n"
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	got, exists, err := LoadConfig(path)
	if err != nil || !exists {
		t.Fatalf("LoadConfig() = (%+v, %v, %v)", got, exists, err)
	}
	if got.Theme != Dark || got.Colors[Added] != "#112233" || got.Colors[Warning] != "bright-yellow" {
		t.Fatalf("loaded config = %+v", got)
	}
}

func TestConfigLoadRejectsMalformedUnknownAndInvalidValues(t *testing.T) {
	tests := []string{
		"{",
		`{"unknown": true}`,
		`{"theme": "solarized"}`,
		`{"colors": {"notAToken": "red"}}`,
		`{"colors": {"added": "not-a-color"}}`,
		`{"theme": 7}`,
		`{"colors": []}`,
		`{"theme": "dark"} trailing`,
		`{"theme": "dark", "theme": "light"}`,
		`{"colors": {"added": "red", "added": "green"}}`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "theme.json")
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, exists, err := LoadConfig(path); err == nil || !exists {
				t.Fatalf("LoadConfig() accepted invalid input: exists=%v err=%v", exists, err)
			}
		})
	}
}

func TestPersistenceCreatesPrivateConfigAndKeepsOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "theme.json")
	if _, err := PersistSelection(path, Dark); err != nil {
		t.Fatalf("PersistSelection() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("file mode = %o, want 600", info.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0700 {
		t.Fatalf("directory mode = %o, want 700", dir.Mode().Perm())
	}

	if err := os.WriteFile(path, []byte(`{"theme":"dark","colors":{"added":"#112233"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PersistSelection(path, Light); err != nil {
		t.Fatalf("PersistSelection() error = %v", err)
	}
	got, exists, err := LoadConfig(path)
	if err != nil || !exists {
		t.Fatalf("LoadConfig() = (%+v, %v, %v)", got, exists, err)
	}
	if got.Theme != Light || got.Colors[Added] != "#112233" {
		t.Fatalf("persisted config = %+v", got)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"theme\": \"light\",\n  \"colors\": {\n    \"added\": \"#112233\"\n  }\n}\n"
	if string(bytes) != want {
		t.Fatalf("config bytes = %q, want %q", bytes, want)
	}
}

func TestPersistenceRefusesToOverwriteInvalidConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	original := []byte(`{"theme":"dark","unknown":true}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := PersistSelection(path, Light)
	if err == nil {
		t.Fatal("PersistSelection() succeeded for malformed config")
	}
	var invalid *InvalidConfigError
	if !errors.As(err, &invalid) {
		t.Fatalf("PersistSelection() error = %T %v, want InvalidConfigError", err, err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("file changed: got %q, want %q", got, original)
	}
}

func TestPersistenceRejectsUnknownThemeWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	_, err := PersistSelection(path, "solarized")
	if err == nil || !strings.Contains(err.Error(), "unknown theme") {
		t.Fatalf("PersistSelection() error = %v", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config was created after rejected selection: %v", statErr)
	}
}

func TestPersistenceReportsPostReplacementDurabilityWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	result, err := persistSelection(path, Light, func(string) error {
		return errors.New("directory sync failed")
	})
	if err != nil {
		t.Fatalf("persistSelection() error = %v; replacement already occurred", err)
	}
	if result.DurabilityWarning == nil {
		t.Fatal("persistSelection() did not report a post-replacement durability warning")
	}
	config, exists, loadErr := LoadConfig(path)
	if loadErr != nil || !exists || config.Theme != Light {
		t.Fatalf("replaced config = (%+v, %v, %v), want persisted light selection", config, exists, loadErr)
	}
}
