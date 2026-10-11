package guide

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"
)

// NewFantasyCompatible sends one forced schema-tool call to the selected
// OpenAI-compatible endpoint. A keyless endpoint must be explicit loopback.
func NewFantasyCompatible(o FantasyOptions) (*Fantasy, error) {
	if strings.TrimSpace(o.Model) == "" || strings.TrimSpace(o.BaseURL) == "" {
		return nil, errors.New("compatible guide model and endpoint are required")
	}
	u, err := url.Parse(o.BaseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && (u.Scheme != "http" || !guideLoopback(u.Hostname()))) {
		return nil, errors.New("invalid compatible guide endpoint")
	}
	if strings.TrimSpace(o.APIKey) == "" && (u.Scheme != "http" || !guideLoopback(u.Hostname())) {
		return nil, errors.New("compatible guide endpoint requires an API key")
	}
	provider, err := openaicompat.New(
		openaicompat.WithBaseURL(o.BaseURL),
		openaicompat.WithAPIKey(o.APIKey),
		openaicompat.WithHTTPClient(newOpenAIHTTPClient(o.Client, o.APIKey)),
		openaicompat.WithObjectMode(fantasy.ObjectModeTool),
	)
	if err != nil {
		return nil, errors.New("compatible guide provider could not be configured")
	}
	model, err := provider.LanguageModel(context.Background(), o.Model)
	if err != nil {
		return nil, errors.New("compatible guide model could not be configured")
	}
	return newFantasy(model, "openai-compatible", o.Model, o.APIKey), nil
}
