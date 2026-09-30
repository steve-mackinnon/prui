package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/source"
	"prui/internal/tui"
)

type discussionGH struct {
	fixtureGH
	snapshot      source.DiscussionSnapshot
	calls         int
	metadataCalls int
	moveAfterRead bool
	metadataErr   error
}

func (g *discussionGH) ListDiscussions(context.Context, source.Identity) (source.DiscussionSnapshot, error) {
	g.calls++
	return g.snapshot, nil
}
func (g *discussionGH) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	g.metadataCalls++
	m := g.value
	if g.moveAfterRead && g.metadataCalls > 1 {
		m.HeadSHA = strings.Repeat("f", 40)
	}
	return m, g.metadataErr
}
func TestDiscussionsQualifyCurrentPlacementWithFrozenPins(t *testing.T) {
	app, s := wiringFixture(t)
	frozen := s.Inventory.Comparison.Metadata
	g := &discussionGH{fixtureGH: fixtureGH{value: frozen}, snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "thread", OriginalCommitID: frozen.HeadSHA}}}}
	app.gh = g
	got, err := app.listDiscussions(context.Background(), s)
	if err != nil || !got.CurrentVerified || len(got.Snapshot.Threads) != 1 {
		t.Fatalf("matching pins: %#v %v", got, err)
	}
	g.moveAfterRead = true
	g.metadataCalls = 0
	got, err = app.listDiscussions(context.Background(), s)
	if err != nil || got.CurrentVerified || len(got.Snapshot.Threads) != 1 || got.Reason == "" {
		t.Fatalf("moving pins discarded historical context: %#v %v", got, err)
	}
	g.metadataErr = errors.New("metadata unavailable")
	got, err = app.listDiscussions(context.Background(), s)
	if err != nil || got.CurrentVerified || len(got.Snapshot.Threads) != 1 {
		t.Fatalf("unknown freshness: %#v %v", got, err)
	}
}
func TestDiscussionsOfflineRefusesBeforeClient(t *testing.T) {
	app, s := wiringFixture(t)
	g := &discussionGH{}
	app.gh = g
	app.offline = true
	if _, err := app.listDiscussions(context.Background(), s); err == nil || g.calls != 0 || g.metadataCalls != 0 {
		t.Fatal("offline read contacted GitHub")
	}
	app.offline = false
	if _, err := app.listDiscussions(context.Background(), (*review.Session)(nil)); err == nil {
		t.Fatal("nil session accepted")
	}
}

func TestCommitSubmissionRejectsUncapturedOrUnverifiedTargets(t *testing.T) {
	app, s := wiringFixture(t)
	frozen := s.Inventory.Comparison.Metadata
	target := source.ReviewCommentTarget{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "a.go", Side: "RIGHT", Line: 1}
	file := inventory.FileChange{ID: "f", NewPath: []byte("a.go")}
	unit := inventory.ReviewUnit{FileChangeID: "f", Kind: inventory.TextHunk, PatchReference: "p"}
	patches := map[string][]byte{"p": []byte("@@ -0,0 +1 @@\n+line\n")}
	s.Inventory.Files = []inventory.FileChange{file}
	s.Inventory.Units = []inventory.ReviewUnit{unit}
	s.Inventory.Patches = patches
	bundle := &commits.Bundle{BaseSHA: frozen.BaseSHA, HeadSHA: frozen.HeadSHA, Status: commits.Captured, Entries: []commits.Entry{{SHA: frozen.HeadSHA, Status: commits.Captured, Diff: &commits.Diff{Files: s.Inventory.Files, Units: s.Inventory.Units, Patches: patches}}}}
	req := tui.CommentSubmission{Metadata: frozen, Comment: source.ReviewComment{Target: target, Body: "body"}, CommitSHA: target.CommitID, CommitBundle: bundle, CommitInventory: &s.Inventory}
	if _, err := app.submitReviewComment(context.Background(), req); err != nil {
		t.Fatal("verified head target rejected", err)
	}
	gh := app.gh.(*fixtureGH)
	for _, mutate := range []func(*tui.CommentSubmission){
		func(r *tui.CommentSubmission) { r.CommitSHA = strings.Repeat("c", 40) },
		func(r *tui.CommentSubmission) { r.CommitBundle = nil },
		func(r *tui.CommentSubmission) { r.CommitInventory = nil },
		func(r *tui.CommentSubmission) { r.Comment.Target.Line = 9 },
	} {
		invalid := req
		mutate(&invalid)
		before := len(gh.comments)
		if _, err := app.submitReviewComment(context.Background(), invalid); err == nil || len(gh.comments) != before {
			t.Fatalf("forged target wrote: %#v", invalid)
		}
	}
}

func TestReplyOnVerifiedCurrentDiscussionPreservesAssociatedCommit(t *testing.T) {
	app, s := wiringFixture(t)
	frozen := s.Inventory.Comparison.Metadata
	displayed := source.ReviewCommentTarget{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "a.go", Side: "RIGHT", Line: 9}
	raw := displayed
	raw.CommitID = strings.Repeat("e", 40)
	root := source.ReviewComment{ID: 42, Target: displayed, CurrentAnchor: &raw, Body: "root"}
	gh := &actionFixtureGH{fixtureGH: app.gh.(*fixtureGH), comment: source.ReviewComment{ID: 43, ParentID: 42, Target: raw, Body: "reply"}}
	app.gh = gh
	action := tui.CommentAction{Metadata: frozen, Comment: root, Body: "reply"}
	got, _, err := app.submitReviewCommentAction(context.Background(), action)
	if err != nil || got.Target != displayed || got.CurrentAnchor == nil || *got.CurrentAnchor != raw {
		t.Fatalf("valid old-associated reply rejected: %#v %v", got, err)
	}
	for _, mutate := range []func(*source.ReviewComment){
		func(c *source.ReviewComment) { c.Target.CommitID = strings.Repeat("f", 40) },
		func(c *source.ReviewComment) { c.Target.Path = "wrong.go" },
		func(c *source.ReviewComment) { c.Target.Side = "LEFT" },
		func(c *source.ReviewComment) { c.Target.Line++ },
		func(c *source.ReviewComment) { c.ParentID = 99 },
	} {
		gh.comment = source.ReviewComment{ID: 43, ParentID: 42, Target: raw, Body: "reply"}
		mutate(&gh.comment)
		if _, _, err := app.submitReviewCommentAction(context.Background(), action); err == nil {
			t.Fatal("mismatched canonical reply accepted")
		}
	}
}
