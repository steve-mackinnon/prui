package main

import (
	"context"
	"errors"
	"fmt"
	"prui/internal/source"
	"prui/internal/tui"
)

// Re-read identity membership and permissions rather than trusting the displayed
// author/eligibility. No code coordinate is needed for an identity-based action.
func (a *application) submitPublished(ctx context.Context, action tui.PublishedAction) (tui.PublishedValue, error) {
	fail := func(s string) (tui.PublishedValue, error) { return tui.PublishedValue{}, errors.New(s) }
	if err := a.online(ctx); err != nil {
		return tui.PublishedValue{}, err
	}
	frozen := action.Metadata
	if !validIdentity(frozen.Identity) || !validSHA(frozen.BaseSHA) || !validSHA(frozen.HeadSHA) || !validRepository(frozen.BaseRepository) || !validRepository(frozen.HeadRepository) {
		return fail("invalid published action comparison")
	}
	if a.setupError != nil {
		return tui.PublishedValue{}, a.setupError
	}
	if action.Resolve == nil {
		if action.CommentID <= 0 || action.ThreadID != "" {
			return fail("invalid edit identity")
		}
		if err := source.ValidateGeneralComment(action.Body); err != nil {
			return tui.PublishedValue{}, err
		}
	} else if action.ThreadID == "" || action.CommentID != 0 || action.General || action.Body != "" {
		return fail("invalid thread action")
	}
	before, err := a.Metadata(ctx, frozen.Identity)
	if err != nil {
		return tui.PublishedValue{}, err
	}
	if !source.SamePinnedRevision(before, frozen) {
		return fail("pull request changed; open a new comparison")
	}
	author := ""
	if action.General {
		reader, ok := a.gh.(source.ConversationReader)
		if !ok {
			return fail("PR comment editing unavailable")
		}
		snapshot, err := reader.ListConversation(ctx, frozen.Identity)
		if err != nil {
			return tui.PublishedValue{}, err
		}
		for _, e := range snapshot.Events {
			if e.ID == fmt.Sprintf("PR comment:%d", action.CommentID) && e.Kind == "PR comment" && !e.Retained {
				author = e.Author
			}
		}
	} else {
		reader, ok := a.gh.(source.DiscussionReader)
		if !ok {
			return fail("review thread actions unavailable")
		}
		snapshot, err := reader.ListDiscussions(ctx, frozen.Identity)
		if err != nil {
			return tui.PublishedValue{}, err
		}
		eligible := false
		for _, t := range snapshot.Threads {
			if action.Resolve != nil && t.ID == action.ThreadID && !t.Retained && t.Resolved != nil && *t.Resolved != *action.Resolve {
				permission := t.CanResolve
				if !*action.Resolve {
					permission = t.CanUnresolve
				}
				eligible = permission != nil && *permission
			}
			for _, c := range t.Comments {
				if c.ID == action.CommentID && !t.Retained {
					author = c.Author
				}
			}
		}
		if action.Resolve != nil && !eligible {
			return fail("thread action denied or no longer eligible; refresh discussions")
		}
	}
	if action.Resolve == nil {
		viewer, ok := a.gh.(source.ReviewCommentViewer)
		if !ok {
			return fail("authenticated viewer unavailable")
		}
		v, err := viewer.Viewer(ctx)
		if err != nil {
			return tui.PublishedValue{}, err
		}
		if author == "" || author != v.Login {
			return fail("only the authenticated comment author may edit")
		}
	}
	after, err := a.Metadata(ctx, frozen.Identity)
	if err != nil {
		return tui.PublishedValue{}, err
	}
	if !source.SamePinnedRevision(after, frozen) {
		return fail("pull request changed; open a new comparison")
	}
	if action.Resolve != nil {
		writer, ok := a.gh.(source.ThreadResolver)
		if !ok {
			return fail("thread resolution unavailable")
		}
		t, err := writer.SetThreadResolved(ctx, frozen.Identity, action.ThreadID, *action.Resolve)
		if err != nil {
			return tui.PublishedValue{}, errors.Join(source.ErrCommentDeliveryUnknown, err)
		}
		if t.ID != action.ThreadID || t.Resolved == nil || *t.Resolved != *action.Resolve || t.CanResolve == nil || t.CanUnresolve == nil {
			return failUnknownPublished()
		}
		return tui.PublishedValue{Thread: t}, nil
	}
	writer, ok := a.gh.(source.PublishedCommentEditor)
	if !ok {
		return fail("published comment editing unavailable")
	}
	c, err := writer.EditPublishedComment(ctx, frozen.Identity, action.CommentID, action.General, action.Body)
	if err != nil {
		return tui.PublishedValue{}, errors.Join(source.ErrCommentDeliveryUnknown, err)
	}
	if c.ID != action.CommentID || c.Author != author || c.Body != action.Body {
		return failUnknownPublished()
	}
	return tui.PublishedValue{Comment: c}, nil
}
func failUnknownPublished() (tui.PublishedValue, error) {
	return tui.PublishedValue{}, fmt.Errorf("%w: invalid canonical published result", source.ErrCommentDeliveryUnknown)
}
