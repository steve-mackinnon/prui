package main

import (
	"context"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/tui"
	"testing"
)

func TestExtendedCommentPreflightRejectsForgedAndStaleTargets(t *testing.T) {
	app, s := wiringFixture(t)
	meta := s.Inventory.Comparison.Metadata
	gh := app.gh.(*fixtureGH)
	for _, u := range s.Inventory.Units {
		if u.Kind == inventory.TextHunk {
			s.Inventory.Patches[u.PatchReference] = []byte("@@ -1,2 +1,2 @@\n-old\n-old2\n+new\n+new2\n")
		}
	}
	target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "a", Side: "LEFT", Line: 2, StartLine: 1, StartSide: "LEFT"}
	for _, file := range []bool{false, true} {
		candidate := target
		if file {
			candidate = source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "a", SubjectType: "file"}
		}
		submission := tui.CommentSubmission{Metadata: meta, CommitInventory: &s.Inventory, Comment: source.ReviewComment{Target: candidate, Body: "body"}}
		if _, err := app.submitReviewComment(context.Background(), submission); err != nil {
			t.Fatal("captured target rejected", err)
		}
		before := len(gh.comments)
		forged := submission
		forged.Comment.Target.Path = "absent"
		if _, err := app.submitReviewComment(context.Background(), forged); err == nil || len(gh.comments) != before {
			t.Fatal("forged target wrote")
		}
		gh.value.HeadSHA = meta.BaseSHA
		if _, err := app.submitReviewComment(context.Background(), submission); err == nil || len(gh.comments) != before {
			t.Fatal("stale target wrote")
		}
		gh.value = meta
		submission.CommitInventory = nil
		if _, err := app.submitReviewComment(context.Background(), submission); err == nil || len(gh.comments) != before {
			t.Fatal("uncaptured extended target wrote")
		}
	}
	queued := tui.ReviewSubmission{Inventory: &s.Inventory, Metadata: meta, Review: source.PullRequestReview{Identity: meta.Identity, CommitID: meta.HeadSHA, Event: "COMMENT", Body: "summary", Comments: []source.ReviewComment{{Target: target, Body: "body"}}}}
	if err := app.submitPullRequestReview(context.Background(), queued); err != nil {
		t.Fatal(err)
	}
	queued.Review.Comments[0].Target.Line = 3
	if err := app.submitPullRequestReview(context.Background(), queued); err == nil || len(gh.reviews) != 1 {
		t.Fatal("uncaptured queued range wrote")
	}
}
