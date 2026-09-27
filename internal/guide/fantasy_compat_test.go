package guide

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFantasyCompatibleSendsOneForcedSchemaTool(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "unrelated-env-key")
	t.Setenv("OPENAI_CUSTOM_HEADERS", `{"X-Injected-Header":"should-not-leave"}`)
	inv := twoUnits(t)
	var requests int
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected operation %s %s", req.Method, req.URL.Path)
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Errorf("keyless loopback used an authorization header: %q", got)
		}
		if got := req.Header.Get("X-Injected-Header"); got != "" {
			t.Errorf("SDK environment header was sent: %q", got)
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		compatibleGuideAnswer(w)
	}))
	t.Cleanup(server.Close)

	a, err := NewFantasyCompatible(FantasyOptions{Model: "local-guide-model", BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	b := run(t, a, inv, Defaults)
	if b.Status != Generated || b.Provider != "openai-compatible" || b.Model != "local-guide-model" || b.SchemaName != SchemaName || b.PromptVersion != PromptVersion {
		t.Fatalf("guide lost its selected provenance: %+v", b)
	}
	if requests != 1 {
		t.Fatalf("expected one provider request, got %d", requests)
	}
	if body["model"] != "local-guide-model" {
		t.Fatalf("request named wrong model: %v", body["model"])
	}
	choice, ok := body["tool_choice"].(map[string]any)
	if !ok || choice["type"] != "function" {
		t.Fatalf("guide schema was not forced: %v", body["tool_choice"])
	}
	chosen, ok := choice["function"].(map[string]any)
	if !ok || chosen["name"] != SchemaName {
		t.Fatalf("wrong forced tool: %v", choice)
	}
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected only the guide schema tool: %v", body["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["type"] != "function" {
		t.Fatalf("unexpected tool: %v", tools[0])
	}
	definition, ok := tool["function"].(map[string]any)
	if !ok || definition["name"] != SchemaName {
		t.Fatalf("wrong schema tool: %v", tool)
	}
	parameters, ok := definition["parameters"].(map[string]any)
	if !ok || parameters["type"] != "object" || parameters["properties"] == nil {
		t.Fatalf("schema parameters missing: %v", definition)
	}
	covered(t, b, inv)
}

func TestFantasyCompatibleUnsupportedSchemaDoesNotRetry(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "unrelated-env-key")
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if got := req.Header.Get("Authorization"); got != "Bearer "+testKey {
			t.Errorf("selected credential was not sent: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"unsupported tool schema with ` + testKey + `","type":"invalid_request_error"}}`))
	}))
	t.Cleanup(server.Close)
	a, err := NewFantasyCompatible(FantasyOptions{APIKey: testKey, Model: "local-guide-model", BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	b := run(t, a, twoUnits(t), Defaults)
	if requests != 1 {
		t.Fatalf("unsupported schema caused %d requests", requests)
	}
	if b.Status != Unavailable || len(b.Items) != 0 || b.Provider != "" || strings.Contains(b.Reason, testKey) || strings.Contains(b.Reason, "unsupported tool schema") {
		t.Fatalf("failure leaked provider details or retained guides: %+v", b)
	}
}

func TestFantasyCompatibleRejectsOversizedResponse(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"padding":"` + strings.Repeat("x", fantasyResponseByteLimit) + `"}`))
	}))
	t.Cleanup(server.Close)
	a, err := NewFantasyCompatible(FantasyOptions{Model: "local-guide-model", BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	b := run(t, a, twoUnits(t), Defaults)
	if requests != 1 || b.Status != Unavailable || len(b.Items) != 0 || b.Provider != "" {
		t.Fatalf("oversized response was accepted or retried: requests=%d bundle=%+v", requests, b)
	}
}

func TestFantasyCompatibleRequiresSafeEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  FantasyOptions
		valid bool
	}{
		{"missing model", FantasyOptions{BaseURL: "http://localhost:1234/v1"}, false},
		{"missing endpoint", FantasyOptions{Model: "local-model"}, false},
		{"keyless loopback", FantasyOptions{Model: "local-model", BaseURL: "http://127.0.0.1:1234/v1"}, true},
		{"keyless remote", FantasyOptions{Model: "remote-model", BaseURL: "https://example.com/v1"}, false},
		{"remote plaintext", FantasyOptions{APIKey: testKey, Model: "remote-model", BaseURL: "http://example.com/v1"}, false},
		{"credential in URL", FantasyOptions{APIKey: testKey, Model: "remote-model", BaseURL: "https://user:pass@example.com/v1"}, false},
		{"query in URL", FantasyOptions{APIKey: testKey, Model: "remote-model", BaseURL: "https://example.com/v1?key=secret"}, false},
		{"remote HTTPS with key", FantasyOptions{APIKey: testKey, Model: "remote-model", BaseURL: "https://example.com/v1"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := NewFantasyCompatible(tc.opts)
			if (err == nil) != tc.valid || (err == nil && a == nil) {
				t.Fatalf("valid=%v analyzer=%v error=%v", tc.valid, a, err)
			}
			if err != nil && strings.Contains(err.Error(), testKey) {
				t.Fatal("configuration error leaked API key")
			}
		})
	}
}

func compatibleGuideAnswer(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "chatcmpl_test", "object": "chat.completion", "model": "local-guide-model",
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "tool_calls",
			"message": map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []any{map[string]any{
					"id": "call_test", "type": "function",
					"function": map[string]any{
						"name":      SchemaName,
						"arguments": `{"guides":[{"title":"Authentication flow","description":"Adds a login endpoint.","sections":[{"title":"Add login endpoint","description":"Handles the request.","unit_ids":["u1","u2"]}]}]}`,
					},
				}},
			},
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
	})
}
