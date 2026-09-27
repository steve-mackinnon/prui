package guide

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"charm.land/fantasy/providers/openai"
	"github.com/openai/openai-go/v3/option"
)

// NewFantasyOpenAI retains the existing Responses request shape while using
// Fantasy for protocol handling. OpenAI-compatible endpoints have a separate
// constructor because they use schema-tool object generation.
func NewFantasyOpenAI(o FantasyOptions) (*Fantasy, error) {
	if strings.TrimSpace(o.APIKey) == "" {
		return nil, errors.New("OPENAI_API_KEY is required to send source for analysis")
	}
	if strings.TrimSpace(o.Model) == "" {
		o.Model = DefaultModel
	}
	if strings.TrimSpace(o.BaseURL) == "" {
		o.BaseURL = "https://api.openai.com/v1"
	}
	provider, err := openai.New(
		openai.WithAPIKey(o.APIKey),
		openai.WithBaseURL(o.BaseURL),
		openai.WithHTTPClient(newOpenAIHTTPClient(o.Client, o.APIKey)),
		openai.WithUseResponsesAPI(),
		openai.WithResponsesAPIFunc(func(string) bool { return true }),
		openai.WithSDKOptions(option.WithJSONSet("text.format.strict", true)),
	)
	if err != nil {
		return nil, errors.New("guide provider could not be configured")
	}
	model, err := provider.LanguageModel(context.Background(), o.Model)
	if err != nil {
		return nil, errors.New("guide model could not be configured")
	}
	return newFantasy(model, "openai", o.Model), nil
}

// The OpenAI SDK reads optional headers from process environment. Keep the
// outbound request limited to headers needed for this guide operation so
// unrelated OPENAI_CUSTOM_HEADERS cannot be sent to a selected endpoint.
func newOpenAIHTTPClient(base *http.Client, key string) *http.Client {
	client := newSingleRequestHTTPClient(base)
	client.Transport = openAIHeaderTransport{base: client.Transport, key: key}
	return client
}

type openAIHeaderTransport struct {
	base http.RoundTripper
	key  string
}

func (t openAIHeaderTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header = make(http.Header)
	for _, name := range []string{"Content-Type", "Accept", "User-Agent"} {
		if values, ok := r.Header[http.CanonicalHeaderKey(name)]; ok {
			copy.Header[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
		}
	}
	if t.key != "" {
		copy.Header.Set("Authorization", "Bearer "+t.key)
	}
	return t.base.RoundTrip(copy)
}
