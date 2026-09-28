package tui

import (
	"net/url"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guide"
	"prui/internal/guideconfig"
)

// SetGuideOptions installs the currently usable, secret-free request identities.
// The save callback records a confirmed choice outside the reviewed checkout.
func (m *Model) SetGuideOptions(options []guideconfig.Selection, initial guideconfig.Selection, save func(guideconfig.Selection) error) {
	m.guideOptionsSet = true
	m.guideOptions = append([]guideconfig.Selection(nil), options...)
	m.guideSave = save
	m.guideModelDrafts = make(map[string]string, len(options))
	for _, option := range options {
		m.guideModelDrafts[option.Provider] = option.Model
	}
	m.guideChoice = guideconfig.Selection{}
	for _, option := range m.guideOptions {
		if option.Provider == initial.Provider {
			m.guideChoice = option
			if guideconfig.ValidModel(initial.Model) {
				m.guideChoice.Model = initial.Model
			}
			break
		}
	}
	if m.guideChoice.Provider == "" && len(m.guideOptions) > 0 {
		m.guideChoice = m.guideOptions[0]
	}
	if m.guideChoice.Provider != "" {
		m.guideModelDrafts[m.guideChoice.Provider] = m.guideChoice.Model
	}
	m.guideConfirmed = m.guideChoice
}

func (m *Model) resetGuideDraft() {
	if !m.guideOptionsSet {
		return
	}
	m.guideChoice = m.guideConfirmed
	m.guideModelDrafts = make(map[string]string, len(m.guideOptions))
	for _, option := range m.guideOptions {
		m.guideModelDrafts[option.Provider] = option.Model
	}
	if m.guideChoice.Provider != "" {
		m.guideModelDrafts[m.guideChoice.Provider] = m.guideChoice.Model
	}
}

func (m *Model) currentGuideSelection() guideconfig.Selection {
	if m.guideOptionsSet {
		return m.guideChoice
	}
	provider, model := m.guideProvider, m.guideModel
	if provider == "" {
		provider = "openai"
	}
	if model == "" {
		model = guide.DefaultModel
	}
	return guideconfig.Selection{Provider: provider, Model: model, Destination: m.guideRecipient(), APIKeyEnv: guideconfig.OpenAIEnvVariable}
}

func (m *Model) cycleGuideProvider(delta int) {
	if len(m.guideOptions) == 0 {
		return
	}
	if m.guideChoice.Provider != "" {
		m.guideModelDrafts[m.guideChoice.Provider] = m.guideChoice.Model
	}
	index := 0
	for i, option := range m.guideOptions {
		if option.Provider == m.guideChoice.Provider {
			index = i
			break
		}
	}
	index = (index + delta + len(m.guideOptions)) % len(m.guideOptions)
	m.guideChoice = m.guideOptions[index]
	if model, ok := m.guideModelDrafts[m.guideChoice.Provider]; ok {
		m.guideChoice.Model = model
	}
}

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
	choice := m.currentGuideSelection()
	provider, model := choice.Provider, choice.Model
	if provider == "" {
		provider = "(none configured)"
	}
	if model == "" {
		model = "(enter model ID)"
	}
	uploadSuffix := ""
	hasCredential := choice.APIKeyEnv != ""
	storeFalse := choice.Provider == "openai"
	if !m.guideOptionsSet {
		hasCredential, storeFalse = m.guideHasCredential || m.guideProvider == "", m.guideStoreFalse || m.guideProvider == ""
	}
	if hasCredential {
		uploadSuffix = " and your API key"
	}
	privacy := "Credential filtering can miss secrets. Send only source you are authorized to share. Your API key is not persisted."
	if storeFalse {
		privacy = "Credential filtering can miss secrets. Send only source you are authorized to share. Requests use store:false; your API key is not persisted."
	}
	if !hasCredential {
		privacy = "Credential filtering can miss secrets. Send only source you are authorized to share."
	}
	recipient := choice.Destination
	if recipient == "" {
		recipient = "No usable provider"
	}
	if !m.guideOptionsSet {
		recipient = m.guideRecipient()
	}
	providerPrefix, modelPrefix, actionPrefix := "", "", ""
	if m.guideFocus == 1 {
		providerPrefix = "> "
	}
	if m.guideFocus == 2 {
		modelPrefix = "> "
	}
	if m.guideFocus == 0 {
		actionPrefix = "> "
	}
	action := actionPrefix + "Enter: send source and generate"
	if choice.Provider == "" {
		action = "Set a provider key or endpoint to enable generation"
	}
	if !guideconfig.ValidModel(choice.Model) && choice.Provider != "" {
		action = "Enter a valid model ID to enable generation"
	}
	hint := "tab: switch | ←/→: provider | type: model | ctrl+a: clear model | esc: cancel | q: quit"
	if m.guideFocus == 2 {
		hint = "tab: switch | type: model | ctrl+a: clear model | esc: cancel"
	}
	paragraphs := []string{
		"Generate guide?",
		providerPrefix + "Provider: " + Escape(provider),
		modelPrefix + "Model: " + Escape(model),
		"Recipient: " + Escape(recipient),
		"This sends bounded pinned patches, repository evidence" + uploadSuffix + " to this recipient.",
		privacy,
		action,
		hint,
	}
	return strings.Join(paragraphs, "\n")
}

func renderGuideConsentModal(width, height int, background, body string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if !guideConsentFits(width, height, body) {
		parts := strings.Split(body, "\n")
		parts[0] = "Resize to review consent"
		body = strings.Join(parts, "\n")
	}
	inside := max(1, min(68, width-6))
	modalWidth := inside + 4
	rows := []string{"╭" + strings.Repeat("─", modalWidth-2) + "╮"}
	for _, paragraph := range strings.Split(body, "\n") {
		for _, line := range strings.Split(ansi.Wrap(paragraph, inside, ""), "\n") {
			rows = append(rows, modalLine(line, inside))
		}
	}
	rows = append(rows, "╰"+strings.Repeat("─", modalWidth-2)+"╯")
	if width < 20 || height < 8 {
		compact := strings.Split(body, "\n")
		out := make([]string, 0, min(len(compact), height))
		for _, line := range compact {
			if len(out) == height {
				break
			}
			out = append(out, clip(line, width))
		}
		return strings.Join(out, "\n")
	}
	if len(rows) > height {
		rows = append(rows[:height-1], rows[len(rows)-1])
	}
	left, top := max(0, (width-modalWidth)/2), max(0, (height-len(rows))/2)
	canvas := lipgloss.NewCanvas(width, height)
	return canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(strings.Join(viewportLines(background, width, height), "\n")),
		lipgloss.NewLayer(strings.Join(rows, "\n")).X(left).Y(top),
	)).Render()
}

func guideConsentFits(width, height int, body string) bool {
	if width < 20 || height < 8 {
		return false
	}
	inside := max(1, min(68, width-6))
	rows := 2
	for _, paragraph := range strings.Split(body, "\n") {
		rows += len(strings.Split(ansi.Wrap(paragraph, inside, ""), "\n"))
	}
	return rows <= height
}
