package guideconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAvailableSelections(t *testing.T) {
	configured, err := loadText(t, `{"guide":{"provider":"anthropic","model":"claude-custom","api_key_env":"CUSTOM_ANTHROPIC"}}`)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{"OPENAI_API_KEY": "openai-secret", "CUSTOM_ANTHROPIC": "anthropic-secret", "GEMINI_API_KEY": "google-secret"}
	got := AvailableSelections(configured, func(name string) string { return keys[name] })
	if len(got) != 3 {
		t.Fatalf("got %d choices: %+v", len(got), got)
	}
	if got[0].Provider != "openai" || got[0].Model != defaultModel || got[0].BaseURL != defaultOpenAIBaseURL {
		t.Fatalf("OpenAI default: %+v", got[0])
	}
	if got[1] != configured {
		t.Fatalf("configured override: %+v", got[1])
	}
	if got[2].Provider != "google" || got[2].Model != "" || got[2].APIKeyEnv != "GEMINI_API_KEY" {
		t.Fatalf("Google choice: %+v", got[2])
	}
	for _, choice := range got {
		if strings.Contains(fmt.Sprintf("%+v", choice), "secret") {
			t.Fatal("key value leaked")
		}
	}
	delete(keys, "CUSTOM_ANTHROPIC")
	got = AvailableSelections(configured, func(name string) string { return keys[name] })
	if len(got) != 2 {
		t.Fatalf("missing configured key should hide Anthropic: %+v", got)
	}
}

func TestValidModel(t *testing.T) {
	for _, model := range []string{"gpt-5.6-terra", "publisher/model:latest"} {
		if !ValidModel(model) {
			t.Fatalf("rejected valid model %q", model)
		}
	}
	for _, model := range []string{"", " leading", "space inside", "tab\tinside", "newline\n", "nonbreaking\u00a0space"} {
		if ValidModel(model) {
			t.Fatalf("accepted invalid model %q", model)
		}
	}
}

func TestAvailableCompatibleSelections(t *testing.T) {
	for _, tc := range []struct {
		config string
		keys   map[string]string
		want   bool
	}{
		{`{"guide":{"provider":"openai-compatible","model":"local","base_url":"http://localhost:11434/v1"}}`, nil, true},
		{`{"guide":{"provider":"openai-compatible","model":"remote","base_url":"https://models.example/v1","api_key_env":"MODEL_KEY"}}`, map[string]string{"MODEL_KEY": "secret"}, true},
		{`{"guide":{"provider":"openai-compatible","model":"remote","base_url":"https://models.example/v1","api_key_env":"MODEL_KEY"}}`, nil, false},
	} {
		configured, err := loadText(t, tc.config)
		if err != nil {
			t.Fatal(err)
		}
		got := AvailableSelections(configured, func(name string) string { return tc.keys[name] })
		found := false
		for _, choice := range got {
			if choice.Provider == "openai-compatible" {
				found = true
				if choice != configured {
					t.Fatalf("compatible choice lost config: %+v", choice)
				}
			}
		}
		if found != tc.want {
			t.Fatalf("found compatible=%t, want %t: %+v", found, tc.want, got)
		}
	}
}

func TestRememberedSelectionPersistence(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "prui", "config.json")
	path := RememberedPath(configPath)
	if path != filepath.Join(filepath.Dir(configPath), "guide-last.json") {
		t.Fatalf("path=%q", path)
	}
	got, err := LoadRemembered(path)
	if err != nil || got != (RememberedSelection{}) {
		t.Fatalf("missing preference = %+v, %v", got, err)
	}
	want := RememberedSelection{Provider: "anthropic", Model: "claude-custom"}
	if err := PersistRemembered(path, want); err != nil {
		t.Fatal(err)
	}
	got, err = LoadRemembered(path)
	if err != nil || got != want {
		t.Fatalf("persisted preference = %+v, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v", info.Mode().Perm())
	}
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), "secret") {
		t.Fatalf("unexpected preference contents: %s, %v", data, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".guide-last-*.tmp")); len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestRememberedSelectionRejectsMalformedWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guide-last.json")
	bad := []byte(`{"provider":"openai","model":"ok","extra":"secret"}`)
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRemembered(path); err == nil {
		t.Fatal("malformed preference accepted")
	}
	if err := PersistRemembered(path, RememberedSelection{Provider: "google", Model: "gemini"}); err == nil {
		t.Fatal("malformed preference overwritten")
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != string(bad) {
		t.Fatalf("preference modified: %q, %v", contents, err)
	}
	for _, choice := range []RememberedSelection{{Provider: "unknown", Model: "m"}, {Provider: "openai", Model: ""}, {Provider: "google", Model: "bad\nmodel"}} {
		if err := PersistRemembered(filepath.Join(t.TempDir(), "choice.json"), choice); err == nil {
			t.Fatalf("invalid choice accepted: %+v", choice)
		}
	}
	if _, err := LoadRemembered(filepath.Join(t.TempDir(), "missing")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
