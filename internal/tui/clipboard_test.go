package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"prui/internal/source"
	"testing"
)

func TestCopyDiscussionURL(t *testing.T) {
	for _, overview := range []bool{false, true} {
		t.Run(fmt.Sprint(overview), func(t *testing.T) {
			m := commitModel(t)
			target := "https://github.com/a/b/pull/1#issuecomment-42"
			m.discussions.snapshot.Snapshot.Timeline = true
			m.discussions.snapshot.Snapshot.Events = []source.ConversationEvent{{ID: "PR comment:42", Kind: "PR comment", URL: target}}
			m.discussions.overviewFocus = true
			var cmd tea.Cmd
			if overview {
				cmd, _ = m.overviewKey("y")
			} else {
				cmd = m.discussionKey("y")
			}
			if cmd == nil {
				t.Fatal("no clipboard command")
			}
			if got := clipboardText(cmd); got != target {
				t.Fatalf("clipboard = %q", got)
			}
		})
	}
}

func TestCopyCommentURLUsesExactReply(t *testing.T) {
	m := commitModel(t)
	m.Comments = []source.ReviewComment{{ID: 1, URL: "https://github.com/a/b/pull/1#discussion_r1"}, {ID: 2, ParentID: 1, URL: "https://github.com/a/b/pull/1#discussion_r2"}}
	m.CommentMenu = &commentActionMenu{CommentID: 2}
	cmd := m.commentActionKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("no clipboard command")
	}
	if got := clipboardText(cmd); got != m.Comments[1].URL {
		t.Fatalf("clipboard = %q", got)
	}
}

func TestCopyURLUnavailable(t *testing.T) {
	m := commitModel(t)
	for _, target := range []string{"", "https://example.com/\x1b]52;evil", "file:///tmp/private"} {
		if cmd := m.copyItemURL(target); cmd != nil {
			t.Fatal("invalid URL copied")
		}
	}
}

func TestCopyOverviewInlineReplyDispatch(t *testing.T) {
	m := commitModel(t)
	target := "https://github.com/a/b/pull/1#discussion_r2"
	m.discussions.snapshot.Snapshot = source.DiscussionSnapshot{Timeline: true, Threads: []source.Discussion{{ID: "thread", URL: "https://github.com/a/b/pull/1#discussion_r1", Comments: []source.ReviewComment{{ID: 2, ParentID: 1, URL: target}}}}}
	key(m, '1')
	key(m, 'D')
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("no clipboard command")
	}
	if got := clipboardText(cmd); got != target {
		t.Fatalf("clipboard = %q", got)
	}
	m.discussions.editor = &generalCommentEditor{}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd != nil || m.discussions.editor.draft != "y" {
		t.Fatal("copy shortcut intercepted text editor")
	}
}

func TestCopyDiscussionMissingSelectionAndFallback(t *testing.T) {
	m := commitModel(t)
	if m.copyDiscussionURL() != nil {
		t.Fatal("empty selection copied")
	}
	target := "https://github.com/a/b/pull/1#discussion_r1"
	m.discussions.snapshot.Snapshot.Threads = []source.Discussion{{URL: target}}
	cmd := m.copyDiscussionURL()
	if cmd == nil || clipboardText(cmd) != target {
		t.Fatal("thread URL lost")
	}
	m.discussions.selected = 4
	if m.copyDiscussionURL() != nil {
		t.Fatal("invalid selection copied")
	}
}

func TestCopyInlineCommentDispatch(t *testing.T) {
	m := rangeTestModel()
	t.Cleanup(m.Close)
	selectRawTarget(t, m, "RIGHT", 1)
	target := *m.selectedDiffTarget()
	url := "https://github.com/a/b/pull/1#discussion_r42"
	m.Comments = []source.ReviewComment{{ID: 42, Target: target, URL: url, Body: "Selected comment"}}
	found := false
	for i, row := range m.displayDetail() {
		if row.commentID == 42 {
			m.setCursor(i)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("comment not rendered")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil || clipboardText(cmd) != url {
		t.Fatal("selected diff comment URL not copied")
	}
}
