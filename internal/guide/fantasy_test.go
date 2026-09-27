package guide

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/fantasy"
)

type fixtureLanguageModel struct {
	fantasy.LanguageModel
	response *fantasy.ObjectResponse
	err      error
}

func (m fixtureLanguageModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return m.response, m.err
}

func TestFantasyRejectsUnusableObjectsWithoutLeakingProviderText(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *fantasy.ObjectResponse
		err      error
	}{
		{"provider error", nil, errors.New("fake-key and source excerpt")},
		{"refusal", nil, errors.New("refused fake-key")},
		{"incomplete", &fantasy.ObjectResponse{Object: map[string]any{"guides": []any{}}, FinishReason: fantasy.FinishReasonLength}, nil},
		{"malformed schema", &fantasy.ObjectResponse{Object: map[string]any{"unexpected": "fake-key"}, FinishReason: fantasy.FinishReasonStop}, nil},
		{"no guides", &fantasy.ObjectResponse{Object: map[string]any{"guides": []any{}}, FinishReason: fantasy.FinishReasonStop}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newFantasy(fixtureLanguageModel{response: tc.response, err: tc.err}, "fixture", "m")
			b, err := a.Analyze(context.Background(), Input{})
			if err != nil || b.Status != Unavailable || strings.Contains(b.Reason, "fake-key") || strings.Contains(b.Reason, "source excerpt") {
				t.Fatalf("unsafe object accepted: bundle=%#v err=%v", b, err)
			}
		})
	}
}
