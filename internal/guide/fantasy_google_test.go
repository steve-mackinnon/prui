package guide

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFantasyGoogleNativeSchemaRequest(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: googleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/models/gemini-test:generateContent") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Goog-Api-Key"); got != "fake-key" {
			t.Errorf("api key = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		config, _ := body["generationConfig"].(map[string]any)
		if config["responseMimeType"] != "application/json" {
			t.Errorf("response MIME type = %#v", config["responseMimeType"])
		}
		schema, _ := config["responseJsonSchema"].(map[string]any)
		if schema["type"] != "object" || schema["properties"] == nil {
			t.Errorf("native JSON schema missing: %#v", schema)
		}
		return googleResponse(http.StatusOK, `{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"guides\":[{\"title\":\"Guide\",\"description\":\"Summary\",\"sections\":[{\"title\":\"Section\",\"description\":\"Details\",\"unit_ids\":[\"u1\"]}]}]}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`), nil
	})}

	a, err := NewFantasyGoogle(FantasyOptions{APIKey: "fake-key", Model: "gemini-test", BaseURL: "https://google.example", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	b, err := a.Analyze(context.Background(), Input{})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Errorf("requests = %d, want 1", calls.Load())
	}
	if b.Status != Generated || b.Provider != "google" || b.Model != "gemini-test" || len(b.Items) != 1 {
		t.Errorf("bundle = %#v", b)
	}
}

func TestFantasyGoogleFailureAndCancellation(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: googleRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return googleResponse(http.StatusBadRequest, `{"error":{"message":"fake-key secret source","status":"INVALID_ARGUMENT"}}`), nil
	})}
	a, err := NewFantasyGoogle(FantasyOptions{APIKey: "fake-key", Model: "gemini-test", BaseURL: "https://google.example", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	b, err := a.Analyze(context.Background(), Input{})
	if err != nil || b.Status != Unavailable || strings.Contains(b.Reason, "fake-key") || strings.Contains(b.Reason, "secret source") {
		t.Errorf("failure bundle = %#v, err = %v", b, err)
	}
	if calls.Load() != 1 {
		t.Errorf("failure requests = %d, want 1", calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = a.Analyze(ctx, Input{})
	if err != context.Canceled {
		t.Errorf("cancellation = %v", err)
	}
}

func TestFantasyGoogleIgnoresUnrelatedSDKEnvironment(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "wrong-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "https://wrong.example")
	var host, key string
	client := &http.Client{Transport: googleRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		host, key = r.URL.Host, r.Header.Get("X-Goog-Api-Key")
		return googleResponse(http.StatusBadRequest, `{"error":{"message":"no fixture"}}`), nil
	})}
	a, err := NewFantasyGoogle(FantasyOptions{APIKey: "selected-key", Model: "gemini-test", Client: client})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = a.Analyze(context.Background(), Input{})
	if host != "generativelanguage.googleapis.com" || key != "selected-key" {
		t.Errorf("SDK used endpoint %q and credential %q", host, key)
	}
}

type googleRoundTripFunc func(*http.Request) (*http.Response, error)

func (f googleRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func googleResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFantasyGoogleRequiresKeyAndModel(t *testing.T) {
	if _, err := NewFantasyGoogle(FantasyOptions{Model: "gemini-test"}); err == nil {
		t.Fatal("missing key was accepted")
	}
	if _, err := NewFantasyGoogle(FantasyOptions{APIKey: "fake-key"}); err == nil {
		t.Fatal("missing model was accepted")
	}
}
