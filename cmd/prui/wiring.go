package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"prui/internal/guide"
	"prui/internal/guideconfig"
	"prui/internal/review"
	"prui/internal/source"
	"prui/internal/theme"
	"prui/internal/tui"
)

func (a *application) online(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.offline {
		return errors.New("offline mode: network actions are unavailable")
	}
	return nil
}

// Every entry screen receives the same operations. Only its initial destination
// differs, so arriving through the browser cannot silently lose capabilities.
func (a *application) model(ctx context.Context, o options) *tui.Model {
	a.resolveGuideSelection()
	var m *tui.Model
	switch o.Command {
	case "prs":
		m = tui.NewPullRequestBrowser(ctx, a.store, a.listPullRequests, a.openFromPullRequestList)
	case "current":
		m = tui.NewCurrentRepositoryBrowser(ctx, a.store, o.Repository, o.Checkout, a.listPullRequests, a.openFromPullRequestList)
	default:
		m = tui.New(ctx, func(c context.Context, notify func(string)) (*review.Session, error) {
			return a.load(c, o, notify)
		})
	}
	var reader review.MetadataReader = a
	if a.offline {
		reader = nil
	}
	m.SetLifecycle(a.store, reader, func(c context.Context, old *review.Session, notify func(string)) (*review.Session, error) {
		override := ""
		if old.ID == o.SessionID {
			override = o.Checkout
		}
		return a.fresh(c, old, override, notify)
	})
	m.SetPullRequestLifecycle(a.listPullRequests, a.openFromPullRequestList)
	if o.Command == "prs" || o.Command == "current" || o.Command == "open" {
		m.SetPullRequestRefresh(a.refreshOpenedPullRequest)
	}
	guideApp := *a
	if a.guideSelectionError != nil {
		m.SetGuideSelection("invalid configuration", "unavailable", "invalid", false, false)
	} else {
		choices, selected, rememberedErr := a.interactiveGuideChoices()
		if rememberedErr != nil {
			m.ActionError = rememberedErr
		}
		if selected.Provider != "" {
			a.guideSelection = selected
		}
		s := a.guideSelection
		m.SetGuideSelection(s.Provider, s.Model, s.Destination, s.Provider == "openai", s.APIKeyEnv != "")
		m.SetGuideOptions(choices, selected, func(choice guideconfig.Selection) error {
			a.guideSelection = choice
			return guideconfig.PersistRemembered(guideconfig.RememberedPath(a.guideConfigPath), guideconfig.RememberedSelection{Provider: choice.Provider, Model: choice.Model})
		})
	}
	m.SetGuideLifecycle(guideApp.requestGuide)
	if _, supported := a.gh.(source.ReadinessReader); !a.offline && supported {
		m.SetReadinessReader(a.readReadiness)
	}
	if _, supported := a.gh.(source.DiscussionReader); !a.offline && supported {
		m.SetDiscussionReader(a.listDiscussions)
	}
	if _, supported := a.gh.(source.GeneralCommentWriter); !a.offline && supported {
		m.SetGeneralCommentSubmitter(a.submitGeneralComment)
	}
	m.SetCommentSubmitter(a.submitReviewComment)
	m.SetReviewSubmitter(a.submitPullRequestReview)
	if !a.offline {
		m.SetDraftReconciler(a.reconcileDraft)
	}
	m.SetCommentActionSubmitter(a.submitReviewCommentAction)
	if !a.offline {
		m.SetPublishedSubmitter(a.submitPublished)
	}
	_, hasDiscussions := a.gh.(source.DiscussionReader)
	if _, supported := a.gh.(source.ReviewCommentReader); !a.offline && supported && !hasDiscussions {
		m.SetCommentReader(a.listReviewComments)
	}
	if _, supported := a.gh.(source.ReviewCommentViewer); !a.offline && supported {
		m.SetViewerReader(a.viewer)
	}
	if o.Theme.Name != "" {
		m.SetTheme(o.Theme)
	}
	if o.ThemeConfigPath != "" {
		configPath := o.ThemeConfigPath
		m.SetThemeSelectionSaver(func(name string) (theme.PersistResult, error) {
			return theme.PersistSelection(configPath, name)
		})
	}
	m.SetThemeSelectionLocked(o.ThemeName != "")
	return m
}

func (a *application) interactiveGuideChoices() ([]guideconfig.Selection, guideconfig.Selection, error) {
	configured := a.guideConfigSelection
	if configured.Provider == "" {
		configured = a.guideSelection
	}
	// Offline mode never inspects a credential. It still presents the configured
	// identity so an attempted request receives the existing explicit refusal.
	if a.offline {
		return []guideconfig.Selection{configured}, configured, nil
	}
	choices := guideconfig.AvailableSelections(configured, os.Getenv)
	remembered, err := guideconfig.LoadRemembered(guideconfig.RememberedPath(a.guideConfigPath))
	if err != nil {
		return choices, chooseGuideSelection(choices, configured, guideconfig.RememberedSelection{}), err
	}
	return choices, chooseGuideSelection(choices, configured, remembered), nil
}

func chooseGuideSelection(choices []guideconfig.Selection, configured guideconfig.Selection, remembered guideconfig.RememberedSelection) guideconfig.Selection {
	for _, choice := range choices {
		if choice.Provider == remembered.Provider && remembered.Model != "" {
			choice.Model = remembered.Model
			return choice
		}
	}
	for _, choice := range choices {
		if choice.Provider == configured.Provider {
			return choice
		}
	}
	if len(choices) != 0 {
		return choices[0]
	}
	return guideconfig.Selection{}
}

func (a *application) createGuideAnalyzer() (guide.Analyzer, error) {
	if a.guideSelectionError != nil {
		return nil, a.guideSelectionError
	}
	create := a.newAnalyzer
	if create == nil {
		create = func() (guide.Analyzer, error) {
			s := a.activeGuideSelection()
			key := ""
			if s.APIKeyEnv != "" {
				key = os.Getenv(s.APIKeyEnv)
				if key == "" {
					return nil, fmt.Errorf("%s is required to send source for analysis", s.APIKeyEnv)
				}
			}
			o := guide.FantasyOptions{APIKey: key, Model: s.Model, BaseURL: s.BaseURL, Client: a.guideClient}
			switch s.Provider {
			case "openai":
				return guide.NewFantasyOpenAI(o)
			case "anthropic":
				return guide.NewFantasyAnthropic(o)
			case "google":
				return guide.NewFantasyGoogle(o)
			case "openai-compatible":
				return guide.NewFantasyCompatible(o)
			default:
				return nil, errors.New("invalid guide provider")
			}
		}
	}
	return create()
}

func (a *application) resolveGuideSelection() {
	if a.guideSelectionResolved {
		return
	}
	a.guideSelectionResolved = true
	home, _ := os.UserHomeDir()
	var err error
	path, err := guideconfig.ConfigPath(guideconfig.PathInputs{Home: home, XDGConfigHome: os.Getenv("XDG_CONFIG_HOME")})
	if err != nil {
		a.guideSelectionError = err
		return
	}
	a.guideConfigPath = path
	a.guideSelection, a.guideSelectionError = guideconfig.Load(path, os.Getenv("OPENAI_BASE_URL"))
	a.guideConfigSelection = a.guideSelection
}

func (a *application) activeGuideSelection() guideconfig.Selection {
	if a.guideSelectionResolved {
		return a.guideSelection
	}
	// Direct application helpers in tests retain the historical default. The
	// interactive model resolves the real user selection before any callbacks.
	return guideconfig.Selection{Provider: "openai", Model: guide.DefaultModel, BaseURL: "https://api.openai.com/v1", APIKeyEnv: guideconfig.OpenAIEnvVariable, Destination: guide.DefaultEndpoint, LegacyDefault: true}
}

func (a *application) guideCacheEligible(b guide.Bundle) bool {
	if a.guideSelectionError != nil {
		return false
	}
	s := a.activeGuideSelection()
	if b.SelectionFingerprint != "" {
		return b.SelectionFingerprint == s.Fingerprint(guide.PromptVersion, guide.SchemaName)
	}
	return s.LegacyDefault && b.Provider == "openai" && b.Model == guide.DefaultModel && b.PromptVersion == guide.PromptVersion && b.SchemaName == guide.SchemaName
}

func (a *application) requestGuide(ctx context.Context, original *review.Session, choice guideconfig.Selection, notify func(string)) (*review.Session, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	if choice.Provider == "" || choice.Model == "" {
		return nil, errors.New("guide provider and model are required")
	}
	selected := *a
	selected.guideSelection = choice
	selected.guideSelectionResolved = true
	if notify != nil {
		notify("Creating analyzer for this confirmed guide request...")
	}
	analyzer, err := selected.createGuideAnalyzer()
	if err != nil {
		return nil, err
	}
	return selected.generateGuide(ctx, original, analyzer)
}
