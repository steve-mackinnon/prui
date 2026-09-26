package review

import (
	"context"
	"errors"
	reviewcontext "pr-review/internal/context"
	"pr-review/internal/guide"
	"pr-review/internal/privacy"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"regexp"
	"time"
)

// PrepareGuide reopens saved object identities locally, never GitHub or moving
// refs. A missing checkout/object view leaves saved evidence as an explicit option.
func PrepareGuide(parent context.Context, original *Session, runner source.Runner, limits source.Limits) (*guide.Preparation, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if original == nil {
		return nil, errors.New("guide preparation requires a saved review")
	}
	fallback := func() (*guide.Preparation, error) {
		p := guide.NewPreparation(original.Inventory, original.Context, nil, privacy.Policy{})
		p.UnavailableReason = "Pinned objects unavailable locally; only saved patches and evidence can be sent."
		return p, nil
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	view, err := source.NewView(ctx, string(original.Checkout), runner, limits)
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	if err != nil {
		return fallback()
	}
	defer func() { _ = view.Close() }()
	comparison := original.Inventory.Comparison
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, pin := range []string{comparison.MergeBaseSHA, comparison.Metadata.HeadSHA} {
		if !sha.MatchString(pin) {
			return fallback()
		}
		if _, err := view.Git(ctx, 100, "cat-file", "-e", pin+"^{tree}"); err != nil {
			if parent.Err() != nil {
				return nil, parent.Err()
			}
			return fallback()
		}
	}
	corpus, err := reviewcontext.PrepareSearch(ctx, view, comparison, privacy.Policy{})
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	if err != nil {
		return fallback()
	}
	return guide.NewPreparation(original.Inventory, original.Context, corpus, privacy.Policy{}), nil
}

// DerivePreparedGuide stores only the uploaded evidence, never the searchable
// corpus or provider transcript. The caller owns approval and cancellation.
func DerivePreparedGuide(ctx context.Context, original *Session, analyzer guide.Analyzer, input guide.Input) session.Snapshot {
	derived := original.Snapshot
	bundle := guide.Analyze(ctx, analyzer, original.Inventory, input)
	bundle.UploadedEvidence = nil
	derived.Guides = &bundle
	derived.DerivedFrom = original.ID
	return derived
}
