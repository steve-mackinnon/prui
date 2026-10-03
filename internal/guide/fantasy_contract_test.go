package guide

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openai"
	"github.com/openai/openai-go/v3/option"
)

// legacyGuideSchema is the exact strict schema sent by the former adapter.
// The contract test compares Fantasy's generated wire schema against it.
func legacyGuideSchema() map[string]any {
	str := map[string]any{"type": "string"}
	section := legacyObject([]string{"title", "description", "unit_ids"}, map[string]any{
		"title": str, "description": str, "unit_ids": map[string]any{"type": "array", "items": str},
	})
	guide := legacyObject([]string{"title", "description", "sections"}, map[string]any{
		"title": str, "description": str, "sections": map[string]any{"type": "array", "items": section},
	})
	return legacyObject([]string{"guides"}, map[string]any{"guides": map[string]any{"type": "array", "items": guide}})
}

func legacyObject(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}

// This fixture pins the wire contract we rely on before replacing the
// existing guide adapter. It deliberately uses the public provider API.
func TestFantasyOpenAIContract(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fake-key" {
			t.Errorf("authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if got := body["model"]; got != "gpt-6.1-sol" {
			t.Errorf("model = %v", got)
		}
		if got := body["store"]; got != false {
			t.Errorf("store = %v", got)
		}
		text, _ := body["text"].(map[string]any)
		format, _ := text["format"].(map[string]any)
		if format["type"] != "json_schema" || format["name"] != SchemaName || format["strict"] != true {
			t.Errorf("unexpected format: %#v", format)
		}
		root, _ := format["schema"].(map[string]any)
		encoded, err := json.Marshal(legacyGuideSchema())
		if err != nil {
			t.Fatal(err)
		}
		var wanted map[string]any
		if err := json.Unmarshal(encoded, &wanted); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(root, wanted) {
			t.Errorf("guide schema differs from existing strict schema:\n got: %#v\nwant: %#v", root, wanted)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_test","object":"response","model":"gpt-6.1-sol","status":"completed","output":[{"type":"message","id":"msg_test","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"guides\":[]}"}]}]}`))
	}))
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	provider, err := openai.New(
		openai.WithAPIKey("fake-key"),
		openai.WithBaseURL(server.URL+"/v1"),
		openai.WithHTTPClient(client),
		openai.WithUseResponsesAPI(),
		openai.WithResponsesAPIFunc(func(string) bool { return true }),
		openai.WithSDKOptions(option.WithJSONSet("text.format.strict", true)),
	)
	if err != nil {
		t.Fatal(err)
	}
	model, err := provider.LanguageModel(context.Background(), "gpt-6.1-sol")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(legacyGuideSchema())
	if err != nil {
		t.Fatal(err)
	}
	var guideSchema fantasy.Schema
	if err := json.Unmarshal(encoded, &guideSchema); err != nil {
		t.Fatal(err)
	}
	result, err := model.GenerateObject(context.Background(), fantasy.ObjectCall{
		Prompt:     fantasy.Prompt{fantasy.NewUserMessage("synthetic source")},
		SchemaName: SchemaName,
		Schema:     guideSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Errorf("requests = %d, want 1", calls.Load())
	}
	if result.RawText != `{"guides":[]}` {
		t.Errorf("raw text = %q", result.RawText)
	}
}

func TestFantasyRetryAndRedirectContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		location string
	}{
		{"server-error", http.StatusServiceUnavailable, ""},
		{"redirect", http.StatusTemporaryRedirect, "https://elsewhere.example/v1/responses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if tc.location != "" {
					w.Header().Set("Location", tc.location)
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()
			client := server.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			provider, err := openai.New(openai.WithAPIKey("fake-key"), openai.WithBaseURL(server.URL+"/v1"), openai.WithHTTPClient(client), openai.WithUseResponsesAPI(), openai.WithResponsesAPIFunc(func(string) bool { return true }), openai.WithSDKOptions(option.WithJSONSet("text.format.strict", true)))
			if err != nil {
				t.Fatal(err)
			}
			model, err := provider.LanguageModel(context.Background(), "gpt-6.1-sol")
			if err != nil {
				t.Fatal(err)
			}
			_, err = model.GenerateObject(context.Background(), fantasy.ObjectCall{Prompt: fantasy.Prompt{fantasy.NewUserMessage("synthetic source")}, Schema: fantasy.Schema{Type: "object"}})
			if err == nil {
				t.Fatal("expected provider error")
			}
			if calls.Load() != 1 {
				t.Errorf("requests = %d, want 1", calls.Load())
			}
			if strings.Contains(err.Error(), "fake-key") || strings.Contains(err.Error(), "synthetic source") {
				t.Errorf("provider error exposed secret or source: %v", err)
			}
		})
	}
}
