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
}

func (m *Model) guideRecipient() string {
	if m.guideDestination == "" {
		return guide.DefaultEndpoint
	}
	return m.guideDestination
}

func (m *Model) guideConsentView() string {
	if m.guidePreparation != nil {
		return m.preparedGuideConsentView()
	}
	paragraphs := []string{
		"Generate guide?",
		"Recipient: " + Escape(m.guideRecipient()),
		"This sends bounded pinned patches, repository evidence, and your API key to this recipient.",
		"Credential filtering can miss secrets. Send only source you are authorized to share. Requests use store:false; your API key is not persisted.",
		"enter: send source and generate | esc: cancel | q: quit",
	}
	for i, text := range paragraphs {
		paragraphs[i] = ansi.Wrap(text, max(1, m.Width), "")
	}
	return strings.Join(paragraphs, "\n\n")
}
