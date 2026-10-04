package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"prui/internal/review"
	"prui/internal/source"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProgramPublishedEditUnknownRefreshAndCanonicalResult(t *testing.T) {
	m := publishedModel(t)
	m.Loading = false
	m.selectReviewView(viewDescription)
	var writes atomic.Int32
	release := make(chan struct{})
	var delivered atomic.Bool
	m.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
		body := "old"
		if delivered.Load() {
			body = "oldq"
		}
		no, yes := false, true
		return DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "thread", Resolved: &no, Outdated: &yes, CanResolve: &yes, CanUnresolve: &no, Comments: []source.ReviewComment{{ID: 1, Author: "alice", Body: body}}}}}}, nil
	})
	m.SetPublishedSubmitter(func(ctx context.Context, a PublishedAction) (PublishedValue, error) {
		writes.Add(1)
		if a.CommentID != 1 || a.Body != "oldq" {
			return PublishedValue{}, context.Canceled
		}
		select {
		case <-release:
		case <-ctx.Done():
			return PublishedValue{}, ctx.Err()
		}
		delivered.Store(true)
		return PublishedValue{}, source.ErrCommentDeliveryUnknown
	})
	h := runProgram(t, m)
	h.expect("review", func(f programFrame) bool { return !f.loading && f.page == pageReview })
	h.key('c')
	h.key('D')
	h.expect("discussion list", func(f programFrame) bool { return f.page == pageDiscussions && strings.Contains(f.text, "old") })
	h.key(tea.KeyEnter)
	h.key('e')
	h.expect("published editor", func(f programFrame) bool { return strings.Contains(f.text, "Edit published comment") })
	h.key('q')
	h.key(tea.KeyEnter)
	h.expect("pending write", func(f programFrame) bool {
		return strings.Contains(f.text, "Submitting published action")
	})
	close(release)
	h.expect("unknown retains draft", func(f programFrame) bool {
		return strings.Contains(f.text, "unknown") && strings.Contains(f.text, "oldq")
	})
	h.key(tea.KeyEnter)
	h.expect("blocked retry", func(f programFrame) bool { return strings.Contains(f.text, "refresh before intentional retry") })
	h.p.Send(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	h.expect("matched attempt", func(f programFrame) bool { return strings.Contains(f.text, "Attempt observed") })
	h.key(tea.KeyEnter)
	h.expect("still blocked", func(f programFrame) bool { return strings.Contains(f.text, "Attempt observed") })
	if writes.Load() != 1 {
		t.Fatal("refresh retried mutation")
	}
	h.key(tea.KeyEscape)
	h.expect("canonical discussion", func(f programFrame) bool {
		return strings.Contains(f.text, "oldq") && !strings.Contains(f.text, "Edit published comment")
	})
	h.key(tea.KeyEscape)
	h.key(tea.KeyEscape)
	h.expect("review restored", func(f programFrame) bool { return f.page == pageReview })
	h.quit()
}
