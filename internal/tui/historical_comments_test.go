package tui

import (
	"context"
	"strings"
	"testing"

	"prui/internal/commits"
	"prui/internal/review"
	"prui/internal/source"
)

func historicalCommentFixture() (*review.Session, DiscussionSnapshot) {
	s := screenSession()
	head := s.Inventory.Comparison.Metadata.HeadSHA
	old := strings.Repeat("f", 40)
	target := source.ReviewCommentTarget{Identity: s.Inventory.Comparison.Metadata.Identity, CommitID: old, Path: "main.go", Side: "RIGHT", Line: 1}
	s.Commits = &commits.Bundle{HeadSHA: head, Status: commits.Captured, Composition: &commits.Composition{Status: commits.Captured, Trees: map[string][]commits.TreeEntry{
		old:  {{Path: []byte("main.go"), Mode: "100644", OID: "old"}},
		head: {{Path: []byte("main.go"), Mode: "100644", OID: "head"}},
	}, Blobs: map[string][]byte{"old": []byte("hello world\n"), "head": []byte("hello world\n")}}}
	yes := true
	thread := source.Discussion{ID: "old-thread", OriginalCommitID: old, OriginalAnchor: &target, Outdated: &yes, Comments: []source.ReviewComment{{ID: 7, Body: "still relevant"}, {ID: 8, ParentID: 7, Body: "reply"}}}
	snapshot := DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{thread}}}
	return s, snapshot
}

func TestOutdatedDiscussionUnchangedCodeStaysInline(t *testing.T) {
	s, snapshot := historicalCommentFixture()
	head := s.Inventory.Comparison.Metadata.HeadSHA
	thread := snapshot.Snapshot.Threads[0]
	target := *thread.OriginalAnchor
	comments := discussionCurrentComments(snapshot, s)
	if len(comments) != 2 || comments[0].Target.CommitID != head || comments[0].Target.Line != 1 || comments[1].ParentID != 7 {
		t.Fatalf("older thread missing from Files: %#v", comments)
	}
	if *thread.OriginalAnchor != target || !*thread.Outdated || thread.CurrentAnchor != nil {
		t.Fatal("display mapping changed authoritative history")
	}
	for _, text := range []string{"changed code\n", "hello world\nhello world\n"} {
		s.Commits.Composition.Blobs["head"] = []byte(text)
		if got := discussionCurrentComments(snapshot, s); len(got) != 0 {
			t.Fatalf("changed or ambiguous code anchored: %#v", got)
		}
	}
}

func TestHistoricalCommentMappingShiftedRange(t *testing.T) {
	s, snapshot := historicalCommentFixture()
	target := snapshot.Snapshot.Threads[0].OriginalAnchor
	target.StartLine, target.StartSide, target.Line = 1, "RIGHT", 2
	s.Commits.Composition.Blobs["old"] = []byte("hello world\nsecond line\n")
	s.Commits.Composition.Blobs["head"] = []byte("inserted\nhello world\nsecond line\n")
	s.Inventory.Patches["patch"] = []byte("@@ -1 +1,3 @@\n-old greeting\n+inserted\n+hello world\n+second line\n")
	comments := discussionCurrentComments(snapshot, s)
	if len(comments) != 2 || comments[0].Target.StartLine != 2 || comments[0].Target.Line != 3 || !comments[0].HistoricalProjection || comments[0].CurrentAnchor != nil || comments[0].OriginalAnchor != target {
		t.Fatalf("range not preserved when shifted: %#v", comments)
	}
	s.Commits.Composition.Blobs["head"] = []byte("inserted\nhello world\nchanged line\n")
	if got := discussionCurrentComments(snapshot, s); len(got) != 0 {
		t.Fatal("partially changed range projected")
	}
}

func TestHistoricalCommentMappingRequiresCapturedEvidence(t *testing.T) {
	for _, name := range []string{"stale", "retained", "missing blobs", "missing commits", "wrong head", "left", "outside diff", "old ambiguous", "binary"} {
		t.Run(name, func(t *testing.T) {
			s, snapshot := historicalCommentFixture()
			thread := &snapshot.Snapshot.Threads[0]
			switch name {
			case "stale":
				snapshot.CurrentVerified = false
			case "retained":
				thread.Retained = true
			case "missing blobs":
				delete(s.Commits.Composition.Blobs, "old")
			case "missing commits":
				s.Commits = nil
			case "wrong head":
				s.Commits.HeadSHA = strings.Repeat("b", 40)
			case "left":
				thread.OriginalAnchor.Side = "LEFT"
			case "outside diff":
				s.Inventory.Patches["patch"] = []byte("@@ -5 +5 @@\n-old greeting\n+hello world\n")
			case "old ambiguous":
				s.Commits.Composition.Blobs["old"] = []byte("hello world\nhello world\n")
			case "binary":
				s.Commits.Composition.Blobs["old"] = []byte("hello world\x00\n")
			}
			if got := discussionCurrentComments(snapshot, s); len(got) != 0 {
				t.Fatalf("unsupported mapping rendered: %#v", got)
			}
		})
	}
}

func TestHistoricalInlineCommentOpensOriginalDiscussion(t *testing.T) {
	s, snapshot := historicalCommentFixture()
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(s)
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: s, Snapshot: snapshot})
	m.Focus = paneDiff
	for i, row := range m.displayDetail() {
		if row.commentID == 7 {
			m.setCursor(i)
			if !m.openCommentActionMenu() || m.top() != pageDiscussions || !m.discussions.detail || m.CommentMenu != nil {
				t.Fatal("projected comment used current-coordinate actions")
			}
			if target := m.currentDiscussionTarget(snapshot.Snapshot.Threads[0]); target == nil || target.CommitID != s.Inventory.Comparison.Metadata.HeadSHA {
				t.Fatal("overview cannot navigate to projected Files comment")
			}
			return
		}
	}
	t.Fatal("projected thread not rendered inline")
}
