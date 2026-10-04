package main

import (
	"context"
	"errors"
	"prui/internal/source"
	"prui/internal/tui"
	"strings"
	"testing"
)

type publishedGH struct {
	conversationGH
	viewer          string
	viewerCalls     int
	edits, resolves int
	err             error
	afterRead       func()
}

func (g *publishedGH) Viewer(context.Context) (source.Viewer, error) {
	g.viewerCalls++
	return source.Viewer{Login: g.viewer}, nil
}
func (g *publishedGH) ListDiscussions(ctx context.Context, id source.Identity) (source.DiscussionSnapshot, error) {
	s, e := g.discussionGH.ListDiscussions(ctx, id)
	if g.afterRead != nil {
		g.afterRead()
	}
	return s, e
}
func (g *publishedGH) EditPublishedComment(_ context.Context, _ source.Identity, id int64, _ bool, body string) (source.PublishedComment, error) {
	g.edits++
	return source.PublishedComment{ID: id, Body: body, Author: g.viewer}, g.err
}
func (g *publishedGH) SetThreadResolved(_ context.Context, _ source.Identity, id string, v bool) (source.Discussion, error) {
	g.resolves++
	yes, no := true, false
	return source.Discussion{ID: id, Resolved: &v, CanResolve: &no, CanUnresolve: &yes}, g.err
}
func TestPublishedActionOfflineFreshnessOwnershipAndMembership(t *testing.T) {
	app, s := wiringFixture(t)
	frozen := s.Inventory.Comparison.Metadata
	yes, no := true, false
	g := &publishedGH{viewer: "alice", conversationGH: conversationGH{discussionGH: discussionGH{fixtureGH: fixtureGH{value: frozen}, snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "thread", Outdated: &yes, Resolved: &no, CanResolve: &yes, CanUnresolve: &no, Comments: []source.ReviewComment{{ID: 1, Author: "alice", Body: "old"}}}}}}, events: source.ConversationSnapshot{Complete: true, Events: []source.ConversationEvent{{ID: "PR comment:2", Kind: "PR comment", Author: "alice", Body: "old"}}}}}
	app.gh = g
	action := tui.PublishedAction{Metadata: frozen, CommentID: 1, Body: "edited"}
	app.offline = true
	if _, e := app.submitPublished(context.Background(), action); e == nil || g.metadataCalls != 0 || g.calls != 0 || g.viewerCalls != 0 {
		t.Fatal("offline accessed client")
	}
	app.offline = false
	g.value.HeadSHA = strings.Repeat("f", 40)
	if _, e := app.submitPublished(context.Background(), action); e == nil || g.edits != 0 {
		t.Fatal("stale wrote")
	}
	g.value = frozen
	g.viewer = "bob"
	if _, e := app.submitPublished(context.Background(), action); e == nil || g.edits != 0 {
		t.Fatal("other author wrote")
	}
	g.viewer = "alice"
	action.CommentID = 9
	if _, e := app.submitPublished(context.Background(), action); e == nil || g.edits != 0 {
		t.Fatal("foreign identity wrote")
	}
	action.CommentID = 1
	if got, e := app.submitPublished(context.Background(), action); e != nil || got.Comment.ID != 1 || g.edits != 1 {
		t.Fatal(got, e)
	}
	action.CommentID = 2
	action.General = true
	if got, e := app.submitPublished(context.Background(), action); e != nil || got.Comment.ID != 2 || g.edits != 2 {
		t.Fatal(got, e)
	}
	action = tui.PublishedAction{Metadata: frozen, ThreadID: "thread", Resolve: &yes}
	if _, e := app.submitPublished(context.Background(), action); e != nil || g.resolves != 1 {
		t.Fatal("outdated unplaceable resolution failed", e)
	}
	g.snapshot.Threads[0].CanResolve = &no
	if _, e := app.submitPublished(context.Background(), action); e == nil || g.resolves != 1 {
		t.Fatal("server permission ignored")
	}
	g.snapshot.Threads[0].Resolved = &yes
	g.snapshot.Threads[0].CanUnresolve = &yes
	action.Resolve = &no
	if _, e := app.submitPublished(context.Background(), action); e != nil || g.resolves != 2 {
		t.Fatal("reopen failed", e)
	}
	g.err = errors.New("synthetic failure")
	if _, e := app.submitPublished(context.Background(), action); !errors.Is(e, source.ErrCommentDeliveryUnknown) || g.resolves != 3 {
		t.Fatal(e)
	}
	g.err = nil
	g.afterRead = func() { g.value.HeadSHA = strings.Repeat("f", 40) }
	if _, e := app.submitPublished(context.Background(), action); e == nil || g.resolves != 3 {
		t.Fatal("mid-read revision movement wrote")
	}
}
