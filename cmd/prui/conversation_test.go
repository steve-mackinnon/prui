package main

import (
	"context"
	"errors"
	"prui/internal/source"
	"strings"
	"testing"
)

type conversationGH struct {
	discussionGH
	events   source.ConversationSnapshot
	readErr  error
	writes   int
	writeErr error
}

func (g *conversationGH) ListConversation(context.Context, source.Identity) (source.ConversationSnapshot, error) {
	return g.events, g.readErr
}
func (g *conversationGH) CreateGeneralComment(_ context.Context, _ source.Identity, body string) (source.ConversationEvent, error) {
	g.writes++
	return source.ConversationEvent{ID: "PR comment:1", Kind: "PR comment", Body: body}, g.writeErr
}
func TestGeneralCommentOfflineFreshnessAndUnknownOutcome(t *testing.T) {
	app, s := wiringFixture(t)
	frozen := s.Inventory.Comparison.Metadata
	g := &conversationGH{discussionGH: discussionGH{fixtureGH: fixtureGH{value: frozen}}}
	app.gh = g
	app.offline = true
	if _, err := app.submitGeneralComment(context.Background(), frozen, "hello"); err == nil || g.writes != 0 || g.metadataCalls != 0 {
		t.Fatal("offline accessed client")
	}
	app.offline = false
	g.value.HeadSHA = strings.Repeat("f", 40)
	if _, err := app.submitGeneralComment(context.Background(), frozen, "hello"); err == nil || g.writes != 0 {
		t.Fatal("stale comparison wrote")
	}
	g.value = frozen
	if _, err := app.submitGeneralComment(context.Background(), frozen, "hello"); err != nil || g.writes != 1 {
		t.Fatal(err)
	}
	g.writeErr = errors.New("synthetic error")
	if _, err := app.submitGeneralComment(context.Background(), frozen, "hello"); !errors.Is(err, source.ErrCommentDeliveryUnknown) || g.writes != 2 {
		t.Fatal("delivery uncertainty lost", err)
	}
}
func TestConversationPartialStateComposesWithThreads(t *testing.T) {
	app, s := wiringFixture(t)
	g := &conversationGH{discussionGH: discussionGH{fixtureGH: fixtureGH{value: s.Inventory.Comparison.Metadata}, snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "t"}}}}, events: source.ConversationSnapshot{Events: []source.ConversationEvent{{ID: "PR comment:1"}}, Complete: false, Reason: "limit"}}
	app.gh = g
	out, err := app.listDiscussions(context.Background(), s)
	if err != nil || out.Snapshot.Complete || !out.Snapshot.Timeline || len(out.Snapshot.Events) != 1 || len(out.Snapshot.Threads) != 1 || !out.CurrentVerified {
		t.Fatalf("%+v %v", out, err)
	}
}
