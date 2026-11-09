package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"prui/internal/agentchat"
	"prui/internal/guideconfig"
)

func TestAgentAnswerUsesPinnedGuideSelectionOnce(t *testing.T) {
	answerBytes, err := json.Marshal(agentchat.Answer{Text: "See the captured hunk.", References: []agentchat.Reference{{EvidenceID: "hunk-1", StartLine: 4, EndLine: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	manifest := agentchat.Manifest{Question: "What changed?", Hunk: agentchat.Excerpt{ID: "hunk-1", Start: 4, End: 4, Lines: []agentchat.SourceLine{{Line: 4, Text: "changed", Selected: true}}}}
	for _, tc := range []struct {
		name, provider, model, base, destination, keyEnv, path string
	}{
		{"OpenAI", "openai", "gpt-test", "https://api.openai.com/v1", "https://api.openai.com", "OPENAI_API_KEY", "/v1/responses"},
		{"Anthropic", "anthropic", "claude-test", "https://api.anthropic.com", "https://api.anthropic.com", "ANTHROPIC_API_KEY", "/v1/messages"},
		{"Google", "google", "gemini-test", "https://generativelanguage.googleapis.com", "https://generativelanguage.googleapis.com", "GEMINI_API_KEY", "/models/gemini-test:generateContent"},
		{"compatible", "openai-compatible", "local-test", "http://127.0.0.1:1234/v1", "http://127.0.0.1:1234", "", "/v1/chat/completions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.keyEnv != "" {
				t.Setenv(tc.keyEnv, "fake-key")
			}
			choice := guideconfig.Selection{Provider: tc.provider, Model: tc.model, BaseURL: tc.base, Destination: tc.destination, APIKeyEnv: tc.keyEnv}
			calls := 0
			app := &application{guideClient: &http.Client{Transport: guideProviderRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, tc.path) || r.URL.Scheme+"://"+r.URL.Host != tc.destination {
					t.Errorf("unexpected recipient: %s %s", r.Method, r.URL)
				}
				body, readErr := io.ReadAll(r.Body)
				if readErr != nil || !strings.Contains(string(body), "What changed?") {
					t.Errorf("request omitted question or was unreadable: %v", readErr)
				}
				return guideProviderResponse(t, tc.provider, tc.model, answerBytes), nil
			})}}
			answer, err := app.answerAgent(context.Background(), choice, manifest)
			if err != nil || calls != 1 || answer.Text != "See the captured hunk." || len(answer.References) != 1 || !answer.References[0].Available {
				t.Fatalf("answer=%#v calls=%d err=%v", answer, calls, err)
			}
		})
	}
}

func TestAgentAnswerOfflineAndMissingCredentialNeverSend(t *testing.T) {
	choice := guideconfig.Selection{Provider: "openai", Model: "gpt-test", BaseURL: "https://api.openai.com/v1", APIKeyEnv: "OPENAI_API_KEY"}
	manifest := agentchat.Manifest{Question: "question"}
	calls := 0
	client := &http.Client{Transport: guideProviderRoundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, nil
	})}
	app := &application{offline: true, guideClient: client}
	if _, err := app.answerAgent(context.Background(), choice, manifest); err == nil || !strings.Contains(err.Error(), "offline") || calls != 0 {
		t.Fatalf("offline err=%v calls=%d", err, calls)
	}
	app.offline = false
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := app.answerAgent(context.Background(), choice, manifest); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") || calls != 0 {
		t.Fatalf("missing credential err=%v calls=%d", err, calls)
	}
}

func TestAgentAnswerAnthropicHTTPFailureKeepsOnlyStatus(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: guideProviderRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected provider path: %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Status:     "400 Bad Request",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"private-source fake-key"}}`)),
			Request:    r,
		}, nil
	})}
	t.Setenv("ANTHROPIC_API_KEY", "fake-key")
	app := &application{guideClient: client}
	choice := guideconfig.Selection{Provider: "anthropic", Model: "claude-test", BaseURL: "https://api.anthropic.com", Destination: "https://api.anthropic.com", APIKeyEnv: "ANTHROPIC_API_KEY"}
	_, err := app.answerAgent(context.Background(), choice, agentchat.Manifest{Question: "question"})
	if calls != 1 || err == nil || err.Error() != "agent provider request failed: HTTP 400" {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
