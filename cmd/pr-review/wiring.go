package main

import (
	"context"
	"errors"
	"os"

	"pr-review/internal/guide"
	"pr-review/internal/review"
	"pr-review/internal/source"
	"pr-review/internal/theme"
	"pr-review/internal/tui"
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
	m.SetGuideLifecycle(a.requestGuide)
	m.SetCommentSubmitter(a.submitReviewComment)
	m.SetCommentActionSubmitter(a.submitReviewCommentAction)
	if _, supported := a.gh.(source.ReviewCommentReader); !a.offline && supported {
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

func (a *application) createGuideAnalyzer() (guide.Analyzer, error) {
	create := a.newAnalyzer
	if create == nil {
		create = func() (guide.Analyzer, error) {
			return guide.NewOpenAI(guide.OpenAIOptions{APIKey: os.Getenv("OPENAI_API_KEY"), Endpoint: os.Getenv("OPENAI_BASE_URL")})
		}
	}
	return create()
}

func (a *application) requestGuide(ctx context.Context, original *review.Session, notify func(string)) (*review.Session, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Creating OpenAI analyzer for this confirmed guide request...")
	}
	analyzer, err := a.createGuideAnalyzer()
	if err != nil {
		return nil, err
	}
	return a.generateGuide(ctx, original, analyzer)
}
