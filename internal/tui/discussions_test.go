package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"prui/internal/review"
	"prui/internal/source"
	"strings"
	"testing"
)

func testDiscussion(id, sha string) source.Discussion {
	yes, no := true, false
	return source.Discussion{ID: id, OriginalCommitID: sha, Outdated: &yes, Resolved: &no, Comments: []source.ReviewComment{{ID: 1, Author: "alice", Body: "Historical concern"}}, DiffHunk: "+old", URL: "https://github.com/a/b/pull/1#discussion_r1"}
}
func TestDiscussionsUnplaceableStatusAndDetail(t *testing.T) {
	m := commitModel(t)
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{testDiscussion("thread", strings.Repeat("f", 40))}}, CurrentVerified: true}
	m.openDiscussions()
	m.discussionKey("enter")
	text := m.discussionsView()
	for _, want := range []string{"Historical concern", "Outdated on current PR", "Original commit not captured", "Original context unavailable", "+old", "https://github.com"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	m.discussionKey("o")
	if m.top() != pageDiscussions {
		t.Fatal("uncaptured commit changed page")
	}
	m.discussionKey("esc")
	if m.discussions.detail {
		t.Fatal("escape did not return to list")
	}
}
func TestDiscussionResultsBoundToReviewAndGeneration(t *testing.T) {
	m := commitModel(t)
	s := m.Session
	m.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) { return DiscussionSnapshot{}, nil })
	_ = m.refreshDiscussions()
	v := DiscussionResult{Target: m.activeTab, Session: s, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{testDiscussion("one", "a")}}, CurrentVerified: true}}
	old := v
	old.Generation--
	m.applyDiscussionResult(old)
	if m.discussions.loaded {
		t.Fatal("late generation accepted")
	}
	m.applyDiscussionResult(v)
	if !m.discussions.loaded {
		t.Fatal("valid result lost")
	}
	v.Err = errors.New("offline")
	m.applyDiscussionResult(v)
	if len(m.discussions.snapshot.Snapshot.Threads) != 1 || !strings.Contains(m.discussions.notice, "stale") {
		t.Fatal("failure lost prior result")
	}
	v.Err = nil
	v.Session = screenSession()
	m.applyDiscussionResult(v)
	if m.discussions.snapshot.Snapshot.Threads[0].ID != "one" {
		t.Fatal("replacement accepted")
	}
}
func TestCommitDiscussionCountsAndExactAnchor(t *testing.T) {
	m := commitModel(t)
	sha := m.commitEntries()[0].SHA
	target := source.ReviewCommentTarget{CommitID: sha, Path: "changed.go", Line: 1, Side: "RIGHT"}
	d := testDiscussion("thread", sha)
	d.OriginalAnchor = &target
	d.Comments = append(d.Comments, source.ReviewComment{ID: 2, ParentID: 1, Body: "reply"})
	m.discussions.loaded = true
	m.discussions.snapshot.Snapshot.Threads = []source.Discussion{d}
	if got := m.commitDiscussionCount(sha); got != "1 loaded threads · 1 outdated" {
		t.Fatal(got)
	}
	if len(m.commitDiscussionLines(target)) == 0 {
		t.Fatal("exact target absent")
	}
	target.CommitID = m.commitEntries()[1].SHA
	if len(m.commitDiscussionLines(target)) != 0 {
		t.Fatal("guessed cross commit anchor")
	}
}

func TestDiscussionOriginalJumpRestoresContextWithoutRead(t *testing.T) {
	m := commitModel(t)
	m.selectReviewView(viewDescription)
	m.DescriptionScroll = 3
	sha := m.commitEntries()[1].SHA
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{testDiscussion("thread", sha)}}, CurrentVerified: true}
	calls := 0
	m.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
		calls++
		return DiscussionSnapshot{}, nil
	})
	m.commit.offsets = map[string]int{sha: 8}
	m.openDiscussions()
	m.discussionKey("enter")
	m.discussionKey("o")
	if m.selectedReviewView() != viewCommits || m.commit.selectedSHA != sha {
		t.Fatal("did not jump")
	}
	m.commit.offsets[sha] = 1
	if !m.restoreDiscussionContext() || m.ContextView != viewDescription || m.DescriptionScroll != 3 || m.commit.offsets[sha] != 8 {
		t.Fatal("context not restored")
	}
	if calls != 0 {
		t.Fatal("navigation fetched discussions")
	}
}
func TestDiscussionMouseAndWidths(t *testing.T) {
	m := commitModel(t)
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{testDiscussion("one", "a"), testDiscussion("two", "b")}}, CurrentVerified: true}
	m.openDiscussions()
	m.discussionMouseClick(2)
	if m.discussions.selected != 1 {
		t.Fatal("mouse selection")
	}
	for _, width := range []int{60, 99, 100, 120} {
		m.Width = width
		m.Height = 8
		if !strings.Contains(m.View().Content, "Discussions") {
			t.Fatal("missing narrow overlay")
		}
	}
}

func TestDiscussionCurrentAnchorOldSHAUsesVerifiedHead(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	m.openReviewTab(s)
	var target *source.ReviewCommentTarget
	for _, line := range m.displayDetail() {
		if line.target != nil {
			copy := *line.target
			target = &copy
			break
		}
	}
	if target == nil {
		t.Fatal("no fixture anchor")
	}
	old := *target
	old.CommitID = strings.Repeat("f", 40)
	no := false
	thread := source.Discussion{ID: "thread", OriginalCommitID: old.CommitID, CurrentAnchor: &old, Outdated: &no, Comments: []source.ReviewComment{{ID: 321, Target: old, Body: "still relevant"}}}
	snapshot := DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{thread}}}
	comments := discussionCurrentComments(snapshot, s)
	if len(comments) != 1 || comments[0].Target.CommitID != target.CommitID {
		t.Fatal("current thread on old SHA lost")
	}
	yes := true
	snapshot.Snapshot.Threads[0].Outdated = &yes
	if len(discussionCurrentComments(snapshot, s)) != 0 {
		t.Fatal("outdated thread rendered current")
	}
	snapshot.CurrentVerified = false
	snapshot.Snapshot.Threads[0].Outdated = &no
	if len(discussionCurrentComments(snapshot, s)) != 0 {
		t.Fatal("unverified mapping")
	}
}

func TestDiscussionRefreshCancelsPriorRead(t *testing.T) {
	m := commitModel(t)
	var captured []context.Context
	m.SetDiscussionReader(func(ctx context.Context, _ *review.Session) (DiscussionSnapshot, error) {
		captured = append(captured, ctx)
		return DiscussionSnapshot{}, nil
	})
	first := m.refreshDiscussions()
	first()
	second := m.refreshDiscussions()
	second()
	if captured[0].Err() == nil || captured[1].Err() != nil {
		t.Fatal("prior refresh not canceled exclusively")
	}
	m.Close()
	if captured[1].Err() == nil {
		t.Fatal("close did not cancel read")
	}
}

func TestDiscussionConfirmedCreationSurvivesSuccessivePartialReads(t *testing.T) {
	m := commitModel(t)
	target := source.ReviewCommentTarget{CommitID: m.commitEntries()[0].SHA, Path: "changed.go", Side: "RIGHT", Line: 1}
	comment := source.ReviewComment{ID: 567, Target: target, Body: "confirmed"}
	insertCommentDiscussion(m.reviewTabState, comment)
	canonical := m.discussions.snapshot.Snapshot.Threads[0]
	canonical.ID = "canonical-node"
	apply := func(complete bool, threads []source.Discussion) {
		m.discussions.generation++
		m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: complete, Threads: threads}}})
	}
	apply(false, []source.Discussion{canonical})
	apply(false, nil)
	if len(m.discussions.snapshot.Snapshot.Threads) != 1 || m.discussions.snapshot.Snapshot.Threads[0].ID != "canonical-node" {
		t.Fatal("canonicalized creation disappeared on second partial read")
	}
	apply(false, []source.Discussion{canonical})
	if len(m.discussions.snapshot.Snapshot.Threads) != 1 {
		t.Fatal("duplicate creation")
	}
	apply(true, nil)
	if len(m.discussions.snapshot.Snapshot.Threads) != 0 || len(m.discussions.confirmed) != 0 {
		t.Fatal("complete authoritative deletion not honored")
	}
}

func TestDiscussionActionKeepsRawAssociatedAnchor(t *testing.T) {
	m := commitModel(t)
	head := m.Session.Inventory.Comparison.Metadata.HeadSHA
	shown := source.ReviewCommentTarget{Identity: m.Session.Inventory.Comparison.Metadata.Identity, CommitID: head, Path: "changed.go", Side: "RIGHT", Line: 2}
	raw := shown
	raw.CommitID = strings.Repeat("e", 40)
	root := source.ReviewComment{ID: 42, Author: "alice", Target: shown, CurrentAnchor: &raw, Body: "root"}
	m.Comments = []source.ReviewComment{root}
	m.CommentMenu = &commentActionMenu{CommentID: 42, Author: "alice", Target: shown, mode: commentActionReply, Draft: "reply"}
	var received CommentAction
	m.SetCommentActionSubmitter(func(_ context.Context, a CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
		received = a
		return source.ReviewComment{}, source.ReviewCommentReaction{}, nil
	})
	cmd := m.commentActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("reply action missing")
	}
	cmd()
	if received.Comment.CurrentAnchor == nil || *received.Comment.CurrentAnchor != raw {
		t.Fatal("action lost associated target")
	}
}

func TestDiscussionAnchorIndexInvalidatesOnPublishedSnapshot(t *testing.T) {
	m := commitModel(t)
	sha := m.commitEntries()[0].SHA
	target := source.ReviewCommentTarget{Identity: m.Session.Inventory.Comparison.Metadata.Identity, CommitID: sha, Path: "changed.go", Side: "RIGHT", Line: 1}
	thread := testDiscussion("one", sha)
	thread.OriginalAnchor = &target
	m.discussions.loaded = true
	m.discussions.snapshot.Snapshot.Threads = []source.Discussion{thread}
	if len(m.commitDiscussionLines(target)) == 0 {
		t.Fatal("initial anchor absent")
	}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true}}})
	if len(m.commitDiscussionLines(target)) != 0 || m.commitDiscussionCount(sha) != "0 threads" {
		t.Fatal("published snapshot reused stale index")
	}
}
