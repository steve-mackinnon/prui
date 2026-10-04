package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"prui/internal/inbox"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/tui"
)

func (a *application) inbox(ctx context.Context, o source.InboxOptions, refresh bool) (source.Inbox, error) {
	if err := o.Validate(); err != nil {
		return source.Inbox{}, err
	}
	c, err := inbox.Open(filepath.Join(a.store.Path(), "inbox.sqlite"))
	if err != nil {
		return source.Inbox{}, err
	}
	defer func() { _ = c.Close() }()
	if !refresh {
		return c.Load(ctx, o)
	}
	if err := a.online(ctx); err != nil {
		return source.Inbox{}, err
	}
	reader, ok := a.gh.(source.InboxReader)
	if !ok {
		return source.Inbox{}, errors.New("account inbox unavailable; authenticate GitHub CLI")
	}
	r, err := reader.ReadInbox(ctx, o)
	if err != nil {
		return r, err
	}
	if r.Viewer == "" {
		return r, nil
	} // no account identity: never overwrite a valid cache
	if err := c.Save(ctx, o, r); err != nil {
		return r, err
	}
	loaded, err := c.Load(ctx, o)
	if err != nil {
		return r, err
	}
	loaded.Cached = false
	return loaded, nil
}
func (a *application) markInboxRead(ctx context.Context, viewer string, item source.InboxItem) error {
	c, err := inbox.Open(filepath.Join(a.store.Path(), "inbox.sqlite"))
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	return c.MarkRead(ctx, viewer, item)
}
func (a *application) openInbox(ctx context.Context, _ string, id source.Identity, notify func(string)) (*review.Session, error) {
	// Resolve only the item's own repository. Frozen sessions remain available
	// without a checkout or network; checkout validation applies to new source.
	latest, err := a.store.LatestComparison(id)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		saved, err := a.store.Load(latest.ID)
		if err != nil {
			return nil, err
		}
		saved.RevisionStatus = session.Unchecked
		return saved, nil
	}
	if a.offline {
		return nil, errors.New("offline: no frozen session for this PR")
	}
	checkout, err := a.store.LookupRepository(id.Repository)
	if errors.Is(err, session.ErrRepositoryNotFound) {
		return nil, fmt.Errorf("no checkout for %s; open this PR from its checkout with prui open %s, then return to the inbox", id.Repository, id.URL())
	}
	if err != nil {
		return nil, err
	}
	repository, err := source.RepositoryFromCheckout(checkout)
	if err != nil || !strings.EqualFold(repository, id.Repository) {
		return nil, errors.New("remembered checkout origin does not match selected PR; open from its own valid checkout")
	}
	return a.openFromPullRequestList(ctx, checkout, id, notify)
}
func listInbox(w io.Writer, r source.Inbox) {
	fmt.Fprintln(w, tui.Escape(inbox.Label(r)))
	fmt.Fprintln(w, "Local activity only · alerts off · refresh explicit · unknown = not marked read")
	for _, i := range r.Items {
		request := ""
		if i.PersonalRequest {
			request += " personal request"
		}
		if i.TeamRequest {
			request += " team request"
		}
		if !i.RequestsComplete {
			request += " requests unknown/partial"
		}
		fmt.Fprintf(w, "%s#%d [%s%s] %s · @%s · %s draft=%t review=%s\n", tui.Escape(i.PullRequest.Identity.Repository), i.PullRequest.Identity.Number, i.Activity, request, tui.Escape(i.PullRequest.Title), tui.Escape(i.PullRequest.Author), i.State, i.Draft, i.ReviewDecision)
	}
	for _, p := range r.Problems {
		fmt.Fprintln(w, "!", tui.Escape(p))
	}
}
