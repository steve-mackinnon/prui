package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/guide"
	"pr-review/internal/guideconfig"
)

type guideProviderRoundTrip func(*http.Request) (*http.Response, error)

func (f guideProviderRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise the user's configured selection through the actual g/Enter flow.
// The transport intercepts native provider endpoints, so no provider account
// or network access is needed.
func TestConfiguredGuideProvidersFromInteractiveConsent(t *testing.T) {
	for _, tc := range []struct {
		provider, model, config, recipient, path, keyEnv string
	}{
		{"openai", "gpt-test", `{"guide":{"provider":"openai","model":"gpt-test"}}`, "https://api.openai.com", "/v1/responses", "OPENAI_API_KEY"},
		{"anthropic", "claude-test", `{"guide":{"provider":"anthropic","model":"claude-test"}}`, "https://api.anthropic.com", "/v1/messages", "ANTHROPIC_API_KEY"},
		{"google", "gemini-test", `{"guide":{"provider":"google","model":"gemini-test"}}`, "https://generativelanguage.googleapis.com", "/models/gemini-test:generateContent", "GEMINI_API_KEY"},
		{"openai-compatible", "local-test", `{"guide":{"provider":"openai-compatible","model":"local-test","base_url":"http://127.0.0.1:1234/v1"}}`, "http://127.0.0.1:1234", "/v1/chat/completions", ""},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			app, original := wiringFixture(t)
			if tc.keyEnv != "" {
				t.Setenv(tc.keyEnv, "fixture-key")
			}
			configDir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "pr-review")
			if err := os.MkdirAll(configDir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(original.Inventory.Units))
			for _, unit := range original.Inventory.Units {
				ids = append(ids, unit.ID)
			}
			if len(ids) == 0 {
				t.Fatal("fixture has no review units")
			}
			answer, err := json.Marshal(map[string]any{"guides": []any{map[string]any{
				"title": "Review change", "description": "Fixture guide",
				"sections": []any{map[string]any{"title": "Inspect patch", "description": "Check changed code", "unit_ids": ids}},
			}}})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			app.guideClient = &http.Client{Transport: guideProviderRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, tc.path) || r.URL.Scheme+"://"+r.URL.Host != tc.recipient {
					t.Errorf("unexpected provider request: %s %s", r.Method, r.URL)
				}
				if tc.keyEnv == "" && r.Header.Get("Authorization") != "" {
					t.Errorf("keyless local provider received Authorization header")
				}
				return guideProviderResponse(t, tc.provider, tc.model, answer), nil
			})}

			m := app.model(context.Background(), options{Command: "resume", SessionID: original.ID})
			defer m.Close()
			completeModelAction(t, m, m.Init())
			modelKey(m, 'g')
			consent := m.View().Content
			for _, want := range []string{"Provider: " + tc.provider, "Model: " + tc.model, "Recipient: " + tc.recipient} {
				if !strings.Contains(consent, want) {
					t.Fatalf("consent omits %q: %s", want, consent)
				}
			}
			if calls != 0 {
				t.Fatalf("provider called before consent: %d", calls)
			}
			modelKey(m, tea.KeyEnter)
			if m.ActionError != nil || calls != 1 || m.Session == nil || m.Session.DerivedFrom != original.ID || m.Session.Guides == nil {
				t.Fatalf("guide action: calls=%d error=%v session=%#v", calls, m.ActionError, m.Session)
			}
			b := m.Session.Guides
			if b.Status != guide.Generated || b.Provider != tc.provider || b.Model != tc.model || len(b.Items) != 1 {
				t.Fatalf("guide provenance/content: %#v", b)
			}
			wantFingerprint := app.guideSelection.Fingerprint(guide.PromptVersion, guide.SchemaName)
			if b.SelectionFingerprint != wantFingerprint || wantFingerprint == "" {
				t.Fatalf("selection fingerprint = %q, want %q", b.SelectionFingerprint, wantFingerprint)
			}
			persisted, err := app.store.Load(m.Session.ID)
			if err != nil || persisted.Guides == nil || persisted.Guides.SelectionFingerprint != wantFingerprint || persisted.Guides.Provider != tc.provider || persisted.Guides.Model != tc.model {
				t.Fatalf("saved guide provenance: session=%#v error=%v", persisted, err)
			}
		})
	}
}

func TestModalSelectionUsesAndRemembersChosenProvider(t *testing.T) {
	app, original := wiringFixture(t)
	ids := make([]string, 0, len(original.Inventory.Units))
	for _, unit := range original.Inventory.Units {
		ids = append(ids, unit.ID)
	}
	answer, err := json.Marshal(map[string]any{"guides": []any{map[string]any{
		"title": "Review change", "description": "Fixture guide",
		"sections": []any{map[string]any{"title": "Inspect patch", "description": "Check changed code", "unit_ids": ids}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	app.guideClient = &http.Client{Transport: guideProviderRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if r.URL.Host != "api.anthropic.com" || !strings.Contains(string(body), "claude-ui") {
			t.Errorf("modal choice did not reach provider: %s %s", r.URL, body)
		}
		return guideProviderResponse(t, "anthropic", "claude-ui", answer), nil
	})}
	m := app.model(context.Background(), options{Command: "resume", SessionID: original.ID})
	defer m.Close()
	completeModelAction(t, m, m.Init())
	modelKey(m, 'g')
	modelKey(m, tea.KeyTab)
	modelKey(m, tea.KeyRight)
	modelKey(m, tea.KeyTab)
	for _, ch := range "claude-ui" {
		modelKey(m, ch)
	}
	modelKey(m, tea.KeyTab)
	if view := m.View().Content; !strings.Contains(view, "Provider: anthropic") || !strings.Contains(view, "Model: claude-ui") {
		t.Fatalf("modal did not show chosen selection: %s", view)
	}
	modelKey(m, tea.KeyEnter)
	if m.ActionError != nil || calls != 1 || m.Session.Guides == nil || m.Session.Guides.Provider != "anthropic" || m.Session.Guides.Model != "claude-ui" {
		t.Fatalf("guide result used wrong choice: calls=%d, error=%v, guide=%#v", calls, m.ActionError, m.Session.Guides)
	}
	if want := app.guideSelection.Fingerprint(guide.PromptVersion, guide.SchemaName); m.Session.Guides.SelectionFingerprint != want {
		t.Fatalf("fingerprint = %q, want %q", m.Session.Guides.SelectionFingerprint, want)
	}
	remembered, err := guideconfig.LoadRemembered(guideconfig.RememberedPath(app.guideConfigPath))
	if err != nil || remembered.Provider != "anthropic" || remembered.Model != "claude-ui" {
		t.Fatalf("remembered selection = %+v, %v", remembered, err)
	}
	restarted := &application{store: app.store}
	restarted.resolveGuideSelection()
	_, selected, err := restarted.interactiveGuideChoices()
	if err != nil || selected.Provider != "anthropic" || selected.Model != "claude-ui" {
		t.Fatalf("restarted selection = %+v, %v", selected, err)
	}
}

func guideProviderResponse(t *testing.T, provider, model string, answer []byte) *http.Response {
	t.Helper()
	var payload any
	switch provider {
	case "openai":
		payload = map[string]any{"id": "resp_test", "object": "response", "model": model, "status": "completed", "output": []any{map[string]any{
			"type": "message", "id": "msg_test", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": string(answer)}},
		}}}
	case "anthropic":
		var object any
		if err := json.Unmarshal(answer, &object); err != nil {
			t.Fatal(err)
		}
		payload = map[string]any{"id": "msg_test", "type": "message", "role": "assistant", "model": model, "content": []any{map[string]any{
			"type": "tool_use", "id": "toolu_test", "name": guide.SchemaName, "input": object,
		}}, "stop_reason": "tool_use", "usage": map[string]any{"input_tokens": 10, "output_tokens": 10}}
	case "google":
		payload = map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": string(answer)}}}, "finishReason": "STOP"}}, "usageMetadata": map[string]any{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2}}
	case "openai-compatible":
		payload = map[string]any{"id": "chatcmpl_test", "object": "chat.completion", "model": model, "choices": []any{map[string]any{
			"index": 0, "finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"id": "call_test", "type": "function", "function": map[string]any{"name": guide.SchemaName, "arguments": string(answer)},
			}}},
		}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}}
	default:
		t.Fatalf("unsupported fixture provider %q", provider)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(encoded)))}
}
