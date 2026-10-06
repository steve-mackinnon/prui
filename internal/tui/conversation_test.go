package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
	"strings"
	"testing"
	"time"
)

func TestConversationChronologyAndStableSelection(t *testing.T) {
	m := commitModel(t)
	early := time.Date(2026, 10, 4, 1, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)
	snap := DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:2", Kind: "PR comment", Author: "bob", Body: "general", CreatedAt: late}}, Threads: []source.Discussion{{ID: "thread", Comments: []source.ReviewComment{{ID: 1, Author: "alice", Body: "inline", CreatedAt: early}}}}}}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: snap})
	entries := m.discussionEntries()
	if len(entries) != 2 || entries[0].ID != "inline:1" {
		t.Fatal(entries)
	}
	m.openDiscussions()
	m.discussionKey("j")
	m.discussionKey("enter")
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: snap})
	if m.discussions.selectedID != "PR comment:2" || !m.discussions.detail || len(m.discussionEntries()) != 2 {
		t.Fatal(m.discussions)
	}
}

func TestGeneralCommentComposerAndUncertainRetry(t *testing.T) {
	m := commitModel(t)
	calls := 0
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		calls++
		return source.ConversationEvent{}, source.ErrCommentDeliveryUnknown
	})
	m.openDiscussions()
	m.discussionKey("n")
	m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.discussions.editor == nil || m.discussions.editor.draft != "tq" || m.top() != pageDiscussions {
		t.Fatal("text intercepted")
	}
	cmd := m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(cmd())
	if calls != 1 || m.discussions.editor == nil || !m.discussions.editor.uncertain {
		t.Fatal("uncertain draft lost")
	}
	if m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil {
		t.Fatal("retried without refresh")
	}
	result := DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: false, Timeline: true}}}
	m.applyDiscussionResult(result)
	if !m.discussions.editor.uncertain {
		t.Fatal("partial refresh allowed retry")
	}
	result.Snapshot.Snapshot.Complete = true
	m.applyDiscussionResult(result)
	if m.discussions.editor.uncertain {
		t.Fatal("complete reconciliation did not allow intentional retry")
	}
	m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.discussions.editor != nil {
		t.Fatal("escape retained editor")
	}
}
func TestConfirmedGeneralCommentRetainedAndDeduplicated(t *testing.T) {
	m := commitModel(t)
	e := &generalCommentEditor{draft: "hello", posting: true}
	m.discussions.editor = e
	event := source.ConversationEvent{ID: "PR comment:1", Kind: "PR comment", Author: "alice", Body: "hello", CreatedAt: time.Now()}
	result := GeneralCommentResult{Target: m.activeTab, Session: m.Session, Editor: e, Event: event}
	wrong := result
	wrong.Session = nil
	m.applyGeneralCommentResult(wrong)
	if len(m.discussions.snapshot.Snapshot.Events) != 0 {
		t.Fatal("wrong session applied")
	}
	m.applyGeneralCommentResult(result)
	refresh := DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true}}}
	m.applyDiscussionResult(refresh)
	if len(m.discussions.snapshot.Snapshot.Events) != 1 || m.discussions.snapshot.Snapshot.Complete {
		t.Fatal("confirmed creation lost")
	}
	refresh.Snapshot.Snapshot.Events = []source.ConversationEvent{event}
	m.applyDiscussionResult(refresh)
	if len(m.discussions.snapshot.Snapshot.Events) != 1 || !m.discussions.snapshot.Snapshot.Complete || len(m.discussions.confirmedEvents) != 0 {
		t.Fatal("reconciliation duplicated event")
	}
}
func TestGeneralReplyDistinctFromInline(t *testing.T) {
	m := commitModel(t)
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		t.Fatal("write before enter")
		return source.ConversationEvent{}, nil
	})
	m.discussions.snapshot.Snapshot = source.DiscussionSnapshot{Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Author: "alice"}}}
	m.discussions.loaded = true
	m.openDiscussions()
	m.discussionKey("enter")
	m.discussionKey("r")
	if m.discussions.editor == nil || m.discussions.editor.draft != "@alice " || !strings.Contains(m.generalCommentView(), "new PR comment mentioning the author") {
		t.Fatal("general reply not distinct")
	}
	m.discussions.editor = nil
	m.discussions.snapshot.Snapshot.Events = nil
	m.discussions.snapshot.Snapshot.Threads = []source.Discussion{{ID: "inline", Comments: []source.ReviewComment{{ID: 1}}}}
	m.discussionKey("r")
	if m.discussions.editor != nil {
		t.Fatal("inline reply silently became general")
	}
}

func TestProgramGeneralPRCommentJourney(t *testing.T) {
	m := commitModel(t)
	m.Loading = false
	m.selectReviewView(viewDescription)
	m.SetGeneralCommentSubmitter(func(_ context.Context, _ source.Metadata, body string) (source.ConversationEvent, error) {
		return source.ConversationEvent{ID: "PR comment:5", Kind: "PR comment", Author: "alice", Body: body, CreatedAt: time.Date(2026, 10, 4, 1, 0, 0, 0, time.UTC)}, nil
	})
	h := runProgram(t, m)
	h.expect("review", func(f programFrame) bool { return f.page == pageReview && !f.loading })
	h.key('D')
	h.key('n')
	h.expect("general composer", func(f programFrame) bool { return strings.Contains(f.text, "New PR comment") })
	h.key('t')
	h.key('q')
	h.key(tea.KeyEnter)
	h.expect("confirmed comment", func(f programFrame) bool {
		return strings.Contains(f.text, "PR comment posted") && strings.Contains(f.text, "tq") && strings.Contains(f.text, "2026-10-04T01:00:00Z")
	})
	h.key('r')
	h.expect("general reply", func(f programFrame) bool {
		return strings.Contains(f.text, "new PR comment mentioning the author") && strings.Contains(f.text, "@alice")
	})
	h.key(tea.KeyEscape)
	h.expect("discard", func(f programFrame) bool {
		return !strings.Contains(f.text, "Enter: post") && strings.Contains(f.text, "tq")
	})
	h.key(tea.KeyEscape)
	h.key(tea.KeyEscape)
	h.expect("review restored", func(f programFrame) bool { return f.page == pageReview })
	h.quit()
}

func TestUncertainAttemptReconciliationUsesOriginalBodyAfterEdit(t *testing.T) {
	m := commitModel(t)
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		return source.ConversationEvent{}, source.ErrCommentDeliveryUnknown
	})
	m.openDiscussions()
	m.discussionKey("n")
	m.discussions.editor.draft = "original attempt"
	m.discussions.editor.cursor = len([]rune("original attempt"))
	cmd := m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(cmd())
	m.generalCommentKey(tea.KeyPressMsg{Code: 'x', Text: " edited"})
	refresh := DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{CurrentVerified: false, Snapshot: source.DiscussionSnapshot{Timeline: true, Complete: true, Events: []source.ConversationEvent{{ID: "PR comment:7", Kind: "PR comment", Body: "original attempt"}}}}}
	m.applyDiscussionResult(refresh)
	if !m.discussions.editor.uncertain {
		t.Fatal("stale refresh cleared uncertainty")
	}
	refresh.Snapshot.CurrentVerified = true
	m.applyDiscussionResult(refresh)
	e := m.discussions.editor
	if !e.matched || e.attemptedBody != "original attempt" || e.draft != "original attempt edited" {
		t.Fatal("reconciled edited body instead of immutable attempt", e)
	}
	if m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil {
		t.Fatal("retried an observed attempt")
	}
}

func TestPartialConversationPreservesSelectedPriorActivityAsStale(t *testing.T) {
	for _, inline := range []bool{false, true} {
		m := commitModel(t)
		old := DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:2", Kind: "PR comment", Author: "alice", Body: "general"}}}}
		if inline {
			old.Snapshot.Events = nil
			old.Snapshot.Threads = []source.Discussion{{ID: "thread", CurrentAnchor: &source.ReviewCommentTarget{Line: 1}, Comments: []source.ReviewComment{{ID: 1, Body: "inline"}}}}
		}
		m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: old})
		m.openDiscussions()
		m.discussionKey("enter")
		selected := m.discussions.selectedID
		m.discussions.scroll = 1
		partial := DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Timeline: true, Complete: false, Reason: "general comments unavailable"}}}
		m.applyDiscussionResult(partial)
		if m.discussions.selectedID != selected || !m.discussions.detail || len(m.discussionEntries()) != 1 || !strings.Contains(m.discussionsView(), "stale retained") {
			t.Fatal("partial refresh lost navigation", m.discussions)
		}
		m.applyDiscussionResult(partial)
		if len(m.discussionEntries()) != 1 {
			t.Fatal("partial refresh duplicated retained event")
		}
		if inline && m.discussions.snapshot.Snapshot.Threads[0].CurrentAnchor != nil {
			t.Fatal("retained current anchor trusted")
		}
	}
}
func TestGeneralComposerOwnsMouseAndFitsViewport(t *testing.T) {
	m := commitModel(t)
	m.Width = 45
	m.Height = 10
	m.openDiscussions()
	m.discussions.editor = &generalCommentEditor{draft: strings.Repeat("long draft\n", 20), cursor: 220}
	if m.mouseAvailable() {
		t.Fatal("mouse routed under editor")
	}
	text := m.generalCommentView()
	if len(strings.Split(text, "\n")) > m.Height {
		t.Fatal("editor escaped viewport")
	}
	for _, line := range strings.Split(text, "\n") {
		if ansi.StringWidth(line) > m.Width {
			t.Fatal("editor line too wide")
		}
	}
}
