package tui

import (
	"net/url"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/guide"
)

// SetGuideDestination describes the recipient without displaying endpoint
// credentials, query strings, or paths. It never reads the API key.
func (m *Model) SetGuideDestination(endpoint string) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = guide.DefaultEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		m.guideDestination = "invalid configured endpoint"
		return
	}
	m.guideDestination = u.Scheme + "://" + u.Host
	m.guideProvider = "openai"
	m.guideModel = guide.DefaultModel
	m.guideStoreFalse = true
	m.guideHasCredential = true
}

// SetGuideSelection captures the selected request identity for the upload
// notice. The recipient is reduced to an origin before it reaches the view.
func (m *Model) SetGuideSelection(provider, model, endpoint string, storeFalse, hasCredential bool) {
	m.SetGuideDestination(endpoint)
	m.guideProvider = provider
	m.guideModel = model
	m.guideStoreFalse = storeFalse
	m.guideHasCredential = hasCredential
}

func (m *Model) guideRecipient() string {
	if m.guideDestination == "" {
		return guide.DefaultEndpoint
	}
	return m.guideDestination
}

func (m *Model) guideConsentView() string {
	provider, model := m.guideProvider, m.guideModel
	if provider == "" {
		provider = "openai"
	}
	if model == "" {
		model = guide.DefaultModel
	}
	uploadSuffix := ""
	if m.guideHasCredential || m.guideProvider == "" {
		uploadSuffix = " and your API key"
	}
	privacy := "Credential filtering can miss secrets. Send only source you are authorized to share. Your API key is not persisted."
	if m.guideStoreFalse || m.guideProvider == "" {
		privacy = "Credential filtering can miss secrets. Send only source you are authorized to share. Requests use store:false; your API key is not persisted."
	}
	if !m.guideHasCredential && m.guideProvider != "" {
		privacy = "Credential filtering can miss secrets. Send only source you are authorized to share."
	}
	paragraphs := []string{
		"Generate guide?",
		"Provider: " + Escape(provider),
		"Model: " + Escape(model),
		"Recipient: " + Escape(m.guideRecipient()),
		"This sends bounded pinned patches, repository evidence" + uploadSuffix + " to this recipient.",
		privacy,
		"enter: send source and generate | esc: cancel | q: quit",
	}
	for i, text := range paragraphs {
		paragraphs[i] = ansi.Wrap(text, max(1, m.Width), "")
	}
	return strings.Join(paragraphs, "\n\n")
}
