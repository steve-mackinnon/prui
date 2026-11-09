package guide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"prui/internal/agentchat"
)

type agentObjectModel struct {
	fantasy.LanguageModel
	calls    int
	call     fantasy.ObjectCall
	response *fantasy.ObjectResponse
	err      error
}

type cancelledAgentModel struct{ fantasy.LanguageModel }

func (cancelledAgentModel) GenerateObject(ctx context.Context, _ fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, ctx.Err()
}

func (m *agentObjectModel) GenerateObject(_ context.Context, call fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	m.calls++
	m.call = call
	return m.response, m.err
}

func TestAgentAnswerUsesOneStructuredRequestAndExactManifestCitations(t *testing.T) {
	manifest := agentchat.Manifest{Question: "What changed?", Hunk: agentchat.Excerpt{ID: "hunk-1", Revision: "head", Start: 4, End: 4, Lines: []agentchat.SourceLine{{Line: 4, Text: "new code", Selected: true}}}}
	model := &agentObjectModel{response: &fantasy.ObjectResponse{FinishReason: fantasy.FinishReasonStop, Object: map[string]any{
		"text":       "It adds new code.",
		"references": []any{map[string]any{"evidence_id": "hunk-1", "start_line": 4, "end_line": 4}},
	}}}
	answer, err := newFantasy(model, "openai", "test-model", "fake-key").Answer(context.Background(), manifest)
	if err != nil || model.calls != 1 || answer.Text != "It adds new code." || len(answer.References) != 1 || !answer.References[0].Available || answer.References[0].Excerpt.Lines[0].Text != "new code" {
		t.Fatalf("answer=%#v calls=%d err=%v", answer, model.calls, err)
	}
	prompt, _ := json.Marshal(model.call.Prompt)
	if model.call.SchemaName != AgentAnswerSchema || len(model.call.Prompt) != 1 || !strings.Contains(string(prompt), "What changed?") {
		t.Fatalf("structured request did not contain manifest: %#v", model.call)
	}
}

func TestAgentAnswerPromptNamesSelectedHunkAsPrimaryCitableEvidence(t *testing.T) {
	manifest := agentchat.Manifest{Question: "Does this old-side line change behavior?", Hunk: agentchat.Excerpt{
		ID: "hunk-old", Path: "src/mod.rs", Side: "LEFT", Revision: "base", Start: 8, End: 8,
		Lines: []agentchat.SourceLine{{Line: 8, Text: "old behavior", Selected: true}},
	}}
	model := &agentObjectModel{response: &fantasy.ObjectResponse{FinishReason: fantasy.FinishReasonStop, Object: map[string]any{
		"text": "The selected old-side line is supplied; the caller is not.", "references": []any{},
	}}}
	if _, err := newFantasy(model, "anthropic", "test-model", "fake-key").Answer(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	prompt, err := json.Marshal(model.call.Prompt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"hunk is mandatory primary evidence", "lines, especially those marked selected", "before claiming the selected source was not supplied",
		"hunk.id is a citable evidence ID", "evidence array contains optional captured context", "absent surrounding source",
		"hunk-old", "src/mod.rs", "LEFT", "old behavior", "selected",
	} {
		if !strings.Contains(string(prompt), want) {
			t.Errorf("prompt omitted %q", want)
		}
	}
}

func TestAgentAnswerRejectsUnsafeProviderResults(t *testing.T) {
	manifest := agentchat.Manifest{Question: "question"}
	for _, tc := range []struct {
		name     string
		response *fantasy.ObjectResponse
		err      error
	}{
		{"source-echo error", nil, errors.New("fake-key source excerpt")},
		{"refusal", &fantasy.ObjectResponse{FinishReason: fantasy.FinishReasonContentFilter}, nil},
		{"unknown field", &fantasy.ObjectResponse{FinishReason: fantasy.FinishReasonStop, Object: map[string]any{"text": "answer", "references": []any{}, "leak": "source excerpt"}}, nil},
		{"oversized answer", &fantasy.ObjectResponse{FinishReason: fantasy.FinishReasonStop, Object: map[string]any{"text": strings.Repeat("x", agentchat.MaxAnswerBytes+1), "references": []any{}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &agentObjectModel{response: tc.response, err: tc.err}
			_, err := newFantasy(model, "fixture", "m", "fake-key").Answer(context.Background(), manifest)
			if err == nil || model.calls != 1 || strings.Contains(err.Error(), "fake-key") || strings.Contains(err.Error(), "source excerpt") {
				t.Fatalf("calls=%d unsafe error=%v", model.calls, err)
			}
		})
	}
}

func TestAgentAnswerPropagatesCancellationWithoutRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newFantasy(cancelledAgentModel{}, "fixture", "m", "fake-key").Answer(ctx, agentchat.Manifest{Question: "question"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestAgentAnswerReportsOnlyProviderHTTPStatus(t *testing.T) {
	providerError := fmt.Errorf("provider call failed: %w", &fantasy.ProviderError{
		StatusCode:   400,
		URL:          "https://example.invalid/v1/messages?source=private-source",
		RequestBody:  []byte("private-source fake-key"),
		ResponseBody: []byte(`{"error":{"message":"private-source fake-key"}}`),
	})
	model := &agentObjectModel{err: providerError}
	_, err := newFantasy(model, "anthropic", "test-model", "fake-key").Answer(context.Background(), agentchat.Manifest{Question: "question"})
	if model.calls != 1 || err == nil || err.Error() != "agent provider request failed: HTTP 400" {
		t.Fatalf("calls=%d error=%v", model.calls, err)
	}
}

func TestAgentAnswerFailureCategories(t *testing.T) {
	for _, tc := range []struct {
		name          string
		providerError error
		want          error
		message       string
	}{
		{"transport", errors.New("private-source fake-key"), nil, "agent provider request failed"},
		{"timeout", fmt.Errorf("deadline: %w", context.DeadlineExceeded), context.DeadlineExceeded, "context deadline exceeded"},
		{"cancel", fmt.Errorf("canceled: %w", context.Canceled), context.Canceled, "context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &agentObjectModel{err: tc.providerError}
			_, err := newFantasy(model, "fixture", "m", "fake-key").Answer(context.Background(), agentchat.Manifest{Question: "question"})
			if model.calls != 1 || err == nil || err.Error() != tc.message || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("calls=%d error=%v", model.calls, err)
			}
		})
	}
}
