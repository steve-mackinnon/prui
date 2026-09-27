package guide

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
)

type openAIFixtureTransport func(*http.Request) (*http.Response, error)

func (f openAIFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFantasyOpenAIAdapterPreservesGuideContract(t *testing.T) {
	t.Setenv("OPENAI_CUSTOM_HEADERS", "X-Secret: leaked-value")
	calls := 0
	client := &http.Client{Transport: openAIFixtureTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.URL.String() != "https://openai.example/v1/responses" || r.Header.Get("Authorization") != "Bearer fake-key" {
			t.Errorf("request destination, method, or credential mismatch: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("X-Secret") != "" {
			t.Error("unrelated OpenAI SDK environment header was sent")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["model"] != "gpt-test" || body["store"] != false {
			t.Errorf("request model/store: %#v", body)
		}
		format := body["text"].(map[string]any)["format"].(map[string]any)
		if format["name"] != SchemaName || format["strict"] != true {
			t.Errorf("request schema mode: %#v", format)
		}
		response := `{"id":"resp_test","object":"response","model":"gpt-test","status":"completed","output":[{"type":"message","id":"msg_test","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"guides\":[{\"title\":\"Change\",\"description\":\"Why\",\"sections\":[{\"title\":\"Step\",\"description\":\"How\",\"unit_ids\":[\"u1\"]}]}]}"}]}]}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	a, err := NewFantasyOpenAI(FantasyOptions{APIKey: "fake-key", Model: "gpt-test", BaseURL: "https://openai.example/v1", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := a.Analyze(context.Background(), Input{})
	if err != nil || bundle.Status != Generated || bundle.Provider != "openai" || bundle.Model != "gpt-test" || len(bundle.Items) != 1 || calls != 1 {
		t.Fatalf("bundle=%#v calls=%d err=%v", bundle, calls, err)
	}
}

func TestFantasyOpenAIUploadOmitsWithheldMaterial(t *testing.T) {
	b := &builder{}
	allowed := b.unit("main.go", "@@ -0,0 +1 @@\n+func main() {}\n", inventory.TextHunk)
	b.unit("config/.env", "@@ -0,0 +1 @@\n+SECRET=blocked-value\n", inventory.TextHunk)
	b.unit("config.json", "@@ -0,0 +1 @@\n+{\"api_key\":\"synthetic-secret\"}\n", inventory.TextHunk)
	inv := b.build()
	input := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Defaults)
	calls := 0
	client := &http.Client{Transport: openAIFixtureTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"config/.env", "blocked-value", "config.json", "synthetic-secret"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("withheld material %q left the machine", forbidden)
			}
		}
		if !strings.Contains(string(body), "func main()") {
			t.Fatal("eligible patch missing from request")
		}
		text, _ := json.Marshal(map[string]any{"guides": []any{map[string]any{"title": "Change", "description": "Why", "sections": []any{map[string]any{"title": "Step", "description": "How", "unit_ids": []string{allowed}}}}}})
		response, _ := json.Marshal(map[string]any{"id": "resp_test", "object": "response", "status": "completed", "output": []any{map[string]any{"type": "message", "id": "msg_test", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": string(text)}}}}})
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(response)))}, nil
	})}
	a, err := NewFantasyOpenAI(FantasyOptions{APIKey: "fake-key", Model: "gpt-test", BaseURL: "https://openai.example/v1", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	bundle := Analyze(context.Background(), a, inv, input)
	if bundle.Status != Generated || calls != 1 || len(bundle.WithheldPaths) != 2 {
		t.Fatalf("bundle=%#v calls=%d", bundle, calls)
	}
}
