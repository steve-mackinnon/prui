package main

import (
	"context"
	"errors"
	"net/url"
	"pr-review/internal/guide"
	"pr-review/internal/review"
	"strings"
)

func (a *application) guideRecipient() string {
	endpoint := strings.TrimSpace(a.guideEndpoint)
	if endpoint == "" {
		endpoint = guide.DefaultEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "invalid configured endpoint"
	}
	return u.Scheme + "://" + u.Host
}

func (a *application) prepareGuide(ctx context.Context, original *review.Session) (*guide.Preparation, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	if original == nil || original.ID == "" {
		return nil, errors.New("guide preparation requires a saved review session")
	}
	p, err := review.PrepareGuide(ctx, original, a.runner, a.limits)
	if err != nil {
		return nil, err
	}
	p.Recipient = a.guideRecipient()
	p.Model = guide.DefaultModel
	return p, nil
}

func (a *application) requestPreparedGuide(ctx context.Context, original *review.Session, p *guide.Preparation) (*review.Session, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	if err := p.ValidateApproval(); err != nil {
		return nil, err
	}
	if original == nil || original.ID == "" || p.Preview().ComparisonID != original.Inventory.Comparison.InventoryID || p.Recipient != a.guideRecipient() || p.Model != guide.DefaultModel {
		return nil, errors.New("guide comparison or recipient changed; inspect and confirm again")
	}
	analyzer, err := a.createGuideAnalyzer()
	if err != nil {
		return nil, err
	}
	derived := review.DerivePreparedGuide(ctx, original, analyzer, p.Preview())
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The guide bundle carries its own pinned search evidence, so reuse does not
	// depend on retaining or reconstructing a searchable corpus.
	if derived.Guides != nil && derived.Guides.Status == guide.Generated {
		if err := a.store.SaveGeneratedGuide(guideCacheKey(original), *derived.Guides, original.Inventory); err != nil {
			return nil, err
		}
	}
	return a.store.Create(derived)
}
