package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/tui"
)

type application struct {
	store      *session.Store
	gh         source.GitHub
	setupError error
	runner     source.Runner
	limits     source.Limits
}

func (a *application) Metadata(ctx context.Context, id source.Identity) (source.Metadata, error) {
	if a.setupError != nil {
		return source.Metadata{}, a.setupError
	}
	if a.gh == nil {
		return source.Metadata{}, errors.New("GitHub CLI unavailable")
	}
	return a.gh.Metadata(ctx, id)
}

func (a *application) open(ctx context.Context, checkout string, id source.Identity, notify func(string)) (*review.Session, error) {
	if err := outsideCheckout(a.store.Path(), checkout); err != nil {
		return nil, err
	}
	if a.setupError != nil {
		return nil, a.setupError
	}
	if a.gh == nil {
		return nil, errors.New("gh executable required; install GitHub CLI and authenticate")
	}
	checkout, err := filepath.Abs(checkout)
	if err != nil {
		return nil, err
	}
	raw, err := review.Open(ctx, checkout, id, a.gh, a.runner, a.limits, notify)
	if err != nil {
		return nil, err
	}
	saved, err := a.store.Create(raw.Snapshot)
	if err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Saved session " + saved.ID + "; checking metadata freshness...")
	}
	if err := review.Refresh(ctx, a.store, saved, a); err != nil {
		return nil, err
	}
	return saved, nil
}

func (a *application) fresh(ctx context.Context, old *review.Session, checkout string, notify func(string)) (*review.Session, error) {
	if checkout == "" {
		checkout = string(old.Checkout)
	}
	if checkout == "" {
		return nil, errors.New("new comparison requires --repo checkout")
	}
	return a.open(ctx, checkout, old.Inventory.Comparison.Metadata.Identity, notify)
}

func (a *application) load(ctx context.Context, o options, notify func(string)) (*review.Session, error) {
	if o.Command == "open" {
		return a.open(ctx, o.Checkout, o.Identity, notify)
	}
	var reader review.MetadataReader = a
	if o.Offline {
		reader = nil
	}
	saved, err := review.Resume(ctx, a.store, o.SessionID, reader)
	if err != nil {
		return nil, err
	}
	if o.New {
		return a.fresh(ctx, saved, o.Checkout, notify)
	}
	return saved, nil
}

func listSessions(store *session.Store, out io.Writer) int {
	entries, err := store.List()
	if err != nil {
		fmt.Fprintln(out, tui.Escape(err.Error()))
		return 1
	}
	fmt.Fprintln(out, "Stored freshness checks are historical; resume checks again. Reading is not GitHub approval.")
	code := 0
	for _, entry := range entries {
		if entry.Err != nil {
			fmt.Fprintln(out, entry.ID, "UNREADABLE (retained):", tui.Escape(entry.Err.Error()))
			code = 1
			continue
		}
		s := entry.Record
		fmt.Fprintf(out, "%s %s #%d head %.12s %d/%d read | last check: %s | updated %s\n", entry.ID, tui.Escape(s.Inventory.Comparison.Metadata.Identity.Repository), s.Inventory.Comparison.Metadata.Identity.Number, s.Inventory.Comparison.Metadata.HeadSHA, len(s.ReviewedSliceIDs), len(s.Slices), s.RevisionStatus, s.UpdatedAt.Format("2006-01-02T15:04:05Z"))
	}
	if len(entries) == 0 {
		fmt.Fprintln(out, "No saved sessions.")
	}
	return code
}
