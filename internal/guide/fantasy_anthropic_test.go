package guide

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/privacy"
)

func TestFantasyAnthropicGeneratesOneStructuredRequest(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "unrelated-env-key")
	t.Setenv("ANTHROPIC_BASE_URL", "https://unrelated.invalid")
	inv := twoUnits(t)
	var requests int
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if req.Method != http.MethodPost || req.URL.Path != "/v1/messages" {
			t.Errorf("unexpected operation %s %s", req.Method, req.URL.Path)
		}
		if req.Header.Get("X-Api-Key") != testKey {
			t.Error("Anthropic API key was not sent")
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		anthropicGuideAnswer(w)
	}))
	t.Cleanup(server.Close)

	a, err := NewFantasyAnthropic(FantasyOptions{APIKey: testKey, Model: "claude-test", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	b := run(t, a, inv, Defaults)
	if b.Status != Generated || b.Provider != "anthropic" || b.Model != "claude-test" {
		t.Fatalf("guide was not generated with Anthropic provenance: %+v", b)
	}
	if requests != 1 {
		t.Fatalf("expected one provider request, got %d", requests)
	}
	if body["model"] != "claude-test" {
		t.Fatalf("request named wrong model: %v", body["model"])
	}
	choice, ok := body["tool_choice"].(map[string]any)
	if !ok || choice["type"] != "tool" || choice["name"] != SchemaName {
		t.Fatalf("guide schema was not forced: %v", body["tool_choice"])
	}
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected only the guide schema tool: %v", body["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["name"] != SchemaName {
		t.Fatalf("unexpected tool: %v", tools[0])
	}
	covered(t, b, inv)
}

func TestFantasyAnthropicErrorsDoNotLeakCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"bad ` + testKey + `"}}`))
	}))
	t.Cleanup(server.Close)
	a, err := NewFantasyAnthropic(FantasyOptions{APIKey: testKey, BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	b := run(t, a, twoUnits(t), Defaults)
	if b.Status != Unavailable || strings.Contains(b.Reason, testKey) || b.Provider != "" || len(b.Items) != 0 {
		t.Fatalf("provider failure leaked data or retained guides: %+v", b)
	}
}

func TestFantasyAnthropicCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Keep the response pending past the client deadline. A bounded wait
		// lets httptest close even when the server has not observed disconnect.
		time.Sleep(100 * time.Millisecond)
		anthropicGuideAnswer(w)
	}))
	t.Cleanup(server.Close)
	a, err := NewFantasyAnthropic(FantasyOptions{APIKey: testKey, BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	inv := twoUnits(t)
	in := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Defaults)
	_, err = a.Analyze(ctx, in)
	if err == nil || ctx.Err() == nil {
		t.Fatalf("cancelled request did not return context error: %v", err)
	}
}

func TestFantasyAnthropicRequiresSafeConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  FantasyOptions
		valid bool
	}{
		{"missing key", FantasyOptions{}, false},
		{"default endpoint", FantasyOptions{APIKey: testKey}, true},
		{"remote http", FantasyOptions{APIKey: testKey, BaseURL: "http://example.com"}, false},
		{"embedded credentials", FantasyOptions{APIKey: testKey, BaseURL: "https://user:pass@example.com"}, false},
		{"query string", FantasyOptions{APIKey: testKey, BaseURL: "https://example.com?key=value"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := NewFantasyAnthropic(tc.opts)
			if (err == nil) != tc.valid || (err == nil && a == nil) {
				t.Fatalf("valid=%v analyzer=%v error=%v", tc.valid, a, err)
			}
			if err != nil && strings.Contains(err.Error(), testKey) {
				t.Fatal("configuration error leaked API key")
			}
		})
	}
}

func anthropicGuideAnswer(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-test",
		"content": []any{map[string]any{
			"type": "tool_use", "id": "toolu_test", "name": SchemaName,
			"input": map[string]any{"guides": []any{map[string]any{
				"title": "Authentication flow", "description": "Adds a login endpoint.",
				"sections": []any{map[string]any{
					"title": "Add login endpoint", "description": "Handles the request.", "unit_ids": []string{"u1", "u2"},
				}},
			}}},
		}},
		"stop_reason": "tool_use", "usage": map[string]any{"input_tokens": 10, "output_tokens": 10},
	})
}
