package guideconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   PathInputs
		want string
	}{
		{"XDG", PathInputs{Home: "/home/user", XDGConfigHome: "/settings"}, "/settings/pr-review/config.json"},
		{"XDG without home", PathInputs{XDGConfigHome: "/settings"}, "/settings/pr-review/config.json"},
		{"fallback", PathInputs{Home: "/home/user"}, "/home/user/.config/pr-review/config.json"},
		{"relative XDG", PathInputs{Home: "/home/user", XDGConfigHome: "settings"}, "/home/user/.config/pr-review/config.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConfigPath(tc.in)
			if err != nil || got != filepath.FromSlash(tc.want) {
				t.Fatalf("ConfigPath() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if _, err := ConfigPath(PathInputs{}); err == nil {
		t.Fatal("missing home must fail")
	}
}

func TestMissingFilePreservesOpenAISelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.json")
	defaultSelection, err := Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if defaultSelection.Provider != "openai" || defaultSelection.Model != defaultModel || defaultSelection.BaseURL != defaultOpenAIBaseURL || defaultSelection.APIKeyEnv != "OPENAI_API_KEY" || defaultSelection.Destination != "https://api.openai.com" || !defaultSelection.LegacyDefault {
		t.Fatalf("unexpected default selection: %+v", defaultSelection)
	}
	legacyOverride, err := Load(path, "https://proxy.example/v1/")
	if err != nil {
		t.Fatal(err)
	}
	if legacyOverride.BaseURL != "https://proxy.example/v1" || legacyOverride.Destination != "https://proxy.example" || legacyOverride.LegacyDefault {
		t.Fatalf("unexpected legacy override: %+v", legacyOverride)
	}
	legacyOrigin, err := Load(path, "https://proxy.example")
	if err != nil || legacyOrigin.BaseURL != "https://proxy.example/v1" {
		t.Fatalf("legacy origin normalization: %+v, %v", legacyOrigin, err)
	}
}

func TestLoadConfiguredProviders(t *testing.T) {
	for _, tc := range []struct {
		name, json, provider, base, keyEnv, destination string
	}{
		{"OpenAI", `{"guide":{"provider":"openai","model":"gpt-custom"}}`, "openai", defaultOpenAIBaseURL, "OPENAI_API_KEY", "https://api.openai.com"},
		{"Anthropic", `{"guide":{"provider":"anthropic","model":"claude-custom"}}`, "anthropic", "https://api.anthropic.com", "ANTHROPIC_API_KEY", "https://api.anthropic.com"},
		{"Google", `{"guide":{"provider":"google","model":"gemini-custom"}}`, "google", "https://generativelanguage.googleapis.com", "GEMINI_API_KEY", "https://generativelanguage.googleapis.com"},
		{"Compatible", `{"guide":{"provider":"openai-compatible","model":"local","base_url":"http://127.0.0.1:11434/v1","api_key_env":"LOCAL_KEY"}}`, "openai-compatible", "http://127.0.0.1:11434/v1", "LOCAL_KEY", "http://127.0.0.1:11434"},
		{"Unauthenticated loopback", `{"guide":{"provider":"openai-compatible","model":"local","base_url":"http://localhost:11434/v1"}}`, "openai-compatible", "http://localhost:11434/v1", "", "http://localhost:11434"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selection, err := loadText(t, tc.json)
			if err != nil {
				t.Fatal(err)
			}
			if selection.Provider != tc.provider || selection.Model == "" || selection.BaseURL != tc.base || selection.APIKeyEnv != tc.keyEnv || selection.Destination != tc.destination || selection.LegacyDefault {
				t.Fatalf("unexpected selection: %+v", selection)
			}
		})
	}
}

func TestRejectMalformedAndUnsafeConfig(t *testing.T) {
	for _, tc := range []struct{ name, json string }{
		{"malformed", `{"guide":`},
		{"null root", `null`},
		{"missing guide", `{}`},
		{"unknown root", `{"guide":{"provider":"openai","model":"m"},"other":1}`},
		{"unknown guide field", `{"guide":{"provider":"openai","model":"m","api_key":"secret"}}`},
		{"duplicate root", `{"guide":{"provider":"openai","model":"m"},"guide":{"provider":"google","model":"m"}}`},
		{"duplicate nested", `{"guide":{"provider":"openai","provider":"google","model":"m"}}`},
		{"provider", `{"guide":{"provider":"unknown","model":"m"}}`},
		{"model", `{"guide":{"provider":"openai","model":""}}`},
		{"model control", "{\"guide\":{\"provider\":\"openai\",\"model\":\"m\\nother\"}}"},
		{"empty env name", `{"guide":{"provider":"openai","model":"m","api_key_env":""}}`},
		{"invalid env name", `{"guide":{"provider":"openai","model":"m","api_key_env":"A-B"}}`},
		{"native override", `{"guide":{"provider":"anthropic","model":"m","base_url":"https://other.example"}}`},
		{"compatible missing URL", `{"guide":{"provider":"openai-compatible","model":"m","api_key_env":"KEY"}}`},
		{"compatible remote missing key", `{"guide":{"provider":"openai-compatible","model":"m","base_url":"https://other.example/v1"}}`},
		{"remote HTTP", `{"guide":{"provider":"openai-compatible","model":"m","base_url":"http://other.example/v1","api_key_env":"KEY"}}`},
		{"URL credentials", `{"guide":{"provider":"openai-compatible","model":"m","base_url":"https://secret@other.example/v1","api_key_env":"KEY"}}`},
		{"URL query", `{"guide":{"provider":"openai-compatible","model":"m","base_url":"https://other.example/v1?secret=1","api_key_env":"KEY"}}`},
		{"URL fragment", `{"guide":{"provider":"openai-compatible","model":"m","base_url":"https://other.example/v1#fragment","api_key_env":"KEY"}}`},
		{"encoded URL path", `{"guide":{"provider":"openai-compatible","model":"m","base_url":"https://other.example/proxy%2Fv1","api_key_env":"KEY"}}`},
		{"extra JSON", `{"guide":{"provider":"openai","model":"m"}} {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadText(t, tc.json)
			var invalid *InvalidConfigError
			if !errors.As(err, &invalid) || err.Error() != "invalid guide configuration" {
				t.Fatalf("want fixed invalid configuration error, got %v", err)
			}
		})
	}
}

func TestFingerprintIsDeterministicAndNonSecret(t *testing.T) {
	selection, err := loadText(t, `{"guide":{"provider":"openai-compatible","model":"m","base_url":"https://models.example/v1","api_key_env":"SECRET_ENV"}}`)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := selection.Fingerprint("prompt-1", "schema-1")
	if fingerprint == "" || strings.Contains(fingerprint, "SECRET_ENV") || fingerprint != selection.Fingerprint("prompt-1", "schema-1") {
		t.Fatalf("invalid fingerprint %q", fingerprint)
	}
	selection.APIKeyEnv = "OTHER_KEY"
	if fingerprint != selection.Fingerprint("prompt-1", "schema-1") {
		t.Fatal("credential variable must not affect cache identity")
	}
	selection.Model = "other"
	if fingerprint == selection.Fingerprint("prompt-1", "schema-1") {
		t.Fatal("model change must affect cache identity")
	}
}

func loadText(t *testing.T, content string) (Selection, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return Load(path, "ignored legacy override")
}
