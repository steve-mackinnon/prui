package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	"prui/internal/source"
)

func TestSelectedDiscussionFooterReplyAndReaction(t *testing.T) {
	m := commitModel(t)
	m.Width, m.Height = 100, 35
	m.reactionEmoji = true
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:41", Kind: "PR comment", Author: "alice", Body: "Review this"}}}}
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		return source.ConversationEvent{}, nil
	})
	m.SetCommentActionSubmitter(func(context.Context, CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, nil
	})
	m.openOverviewDiscussions()
	rows := m.overviewRows()
	footer := -1
	for i, row := range rows {
		if row.action == "footer" {
			footer = i
			if !strings.Contains(row.text, "👍 0") || !strings.Contains(row.text, "👎 0") || !strings.Contains(row.text, "❤️ 0") || !strings.Contains(row.text, "↩ Reply") {
				t.Fatal(row.text)
			}
			break
		}
	}
	if footer < 0 {
		t.Fatal("selected activity has no reaction/reply footer")
	}
	m.View()
	y := footer - m.DescriptionScroll + 3
	m.mouseReviewClick(3, y)
	if m.CommentMenu == nil || m.CommentMenu.Reaction != "+1" {
		t.Fatal("reaction click did not dispatch selected comment")
	}
	m.CommentMenu = nil
	rows = m.overviewRows()
	for _, row := range rows {
		if row.action == "footer" {
			x := visibleWidth(strings.Split(row.text, "[↩ Reply]")[0]) + 1
			m.mouseReviewClick(x, y)
			break
		}
	}
	if m.discussions.editor == nil || m.discussions.editor.replyTo != "PR comment:41" {
		t.Fatal("reply click did not focus composer")
	}
}

func TestSelectedDiffFooterUsesSelectedCommentAndReplyRoot(t *testing.T) {
	for _, split := range []bool{false, true} {
		m := rangeTestModel()
		m.Width, m.Height = 160, 45
		if split {
			m.layout = diffLayoutSideBySide
		}
		m.selectReviewView(viewFiles)
		var target source.ReviewCommentTarget
		for _, row := range m.cachedFileDetail(false) {
			if row.target != nil {
				target = *row.target
				break
			}
		}
		m.Comments = []source.ReviewComment{{ID: 7, Target: target, Author: "alice", Body: "Root"}, {ID: 8, ParentID: 7, Target: target, Author: "bob", Body: "Reply"}}
		m.SetCommentActionSubmitter(func(context.Context, CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
			return source.ReviewComment{}, source.ReviewCommentReaction{}, nil
		})
		m.Focus, m.cursorActive = paneDiff, true
		footer := -1
		for i, row := range m.displayDetail() {
			if row.commentID == 8 && row.commentFooter {
				footer = i
				break
			}
		}
		if footer < 0 {
			t.Fatal("no footer")
		}
		m.setCursor(footer)
		m.ensureCursorVisible()
		text := ansi.Strip(m.View().Content)
		clicked := false
		for y, line := range strings.Split(text, "\n") {
			at := strings.Index(line, "[↩ Reply]")
			if at < 0 {
				continue
			}
			m.mouseReviewClick(visibleWidth(line[:at])+1, y)
			clicked = true
			break
		}
		handled := clicked
		if !handled || m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply || m.CommentMenu.CommentID != 8 || m.CommentMenu.ReplyToID != 7 {
			t.Fatalf("wrong reply split=%v: %+v", split, m.CommentMenu)
		}
		m.Close()
	}
}

func TestDiscussionFooterCountsFallbackAndAvailability(t *testing.T) {
	m := commitModel(t)
	m.reactionEmoji = false
	m.SetCommentActionSubmitter(func(context.Context, CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, nil
	})
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Events: []source.ConversationEvent{{ID: "PR comment:7", ReactionCounts: source.ReactionCounts{4, 2, 0, 0, 3}}}}}
	m.CommentReactions = map[int64][]source.ReviewCommentReaction{7: {{Content: "heart"}}, -7: {{Content: "+1"}}}
	entry := source.Discussion{ID: "PR comment:7", Kind: "PR comment", Comments: []source.ReviewComment{{Author: "alice"}}}
	text := pageMouseFooter(m.discussionFooterControls(entry), " ")
	if text != "[+1 5] [-1 2] [heart 3]" {
		t.Fatal(text)
	}
	for _, width := range []int{1, 5, 15, 27} {
		controls := m.discussionFooterControls(entry)
		for x := 0; x < width; x++ {
			key := pageMouseFooterKey(controls, " ", x, width)
			if width < visibleWidth(controls[0].label) && key != "" {
				t.Fatal("clipped control activated")
			}
		}
	}
	entry.Retained = true
	if len(m.discussionFooterControls(entry)) != 0 {
		t.Fatal("retained activity has controls")
	}
	entry.Retained = false
	entry.Kind = "Review"
	if len(m.discussionFooterControls(entry)) != 0 {
		t.Fatal("review event has unsupported controls")
	}
	entry.Kind = "PR comment"
	m.discussions.snapshot.CurrentVerified = false
	if len(m.discussionFooterControls(entry)) != 0 {
		t.Fatal("unverified comparison has controls")
	}
}

func TestCommentFooterScreens(t *testing.T) {
	for _, width := range []int{100, 32} {
		m := New(context.Background(), nil)
		m.Session, m.Width, m.Height, m.Loading = screenSession(), width, 24, false
		m.reactionEmoji = true
		m.discussions.loaded = true
		m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:41", Kind: "PR comment", Author: "alice", Body: "Please check the fallback behavior.", ReactionCounts: source.ReactionCounts{2, 1, 0, 0, 5}}}}}
		m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
			return source.ConversationEvent{}, nil
		})
		m.SetCommentActionSubmitter(func(context.Context, CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
			return source.ReviewComment{}, source.ReviewCommentReaction{}, nil
		})
		m.openOverviewDiscussions()
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		for i, line := range lines {
			if visibleWidth(line) > width {
				t.Fatalf("footer screen exceeds width %d: %q", width, line)
			}
			lines[i] = strings.TrimRight(line, " ")
		}
		if len(lines) > m.Height {
			t.Fatal("footer screen exceeds height")
		}
		checkScreen(t, fmt.Sprintf("comment_footer_%d", width), strings.Join(lines, "\n")+"\n")
		m.Close()
	}
}

func TestPartialReactionRefreshReplacesObservedCounts(t *testing.T) {
	m := commitModel(t)
	m.CommentReactions = map[int64][]source.ReviewCommentReaction{-7: {{ID: 1, Content: "heart"}}, -8: {{ID: 2, Content: "heart"}}}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Timeline: true, Complete: false, Events: []source.ConversationEvent{{ID: "PR comment:7", Kind: "PR comment", ReactionCounts: source.ReactionCounts{0, 0, 0, 0, 3}}}}}})
	if got := m.commentReactionCounts(-7)["heart"]; got != 3 {
		t.Fatalf("canonical count double counted: %d", got)
	}
	if len(m.CommentReactions[-8]) != 1 {
		t.Fatal("omitted confirmation lost")
	}
}

func TestHistoricalDiffFooterReplyAndReaction(t *testing.T) {
	for _, split := range []bool{false, true} {
		for _, id := range []int64{7, 8} {
			for _, label := range []string{"[+1 0]", "[↩ Reply]"} {
				t.Run(fmt.Sprintf("split=%v/comment=%d/%s", split, id, label), func(t *testing.T) {
					s, snapshot := historicalCommentFixture()
					// The displayed line moved; outbound actions must retain the old anchor.
					s.Commits.Composition.Blobs["head"] = []byte("inserted\nhello world\n")
					s.Inventory.Patches["patch"] = []byte("@@ -1 +1,2 @@\n-old greeting\n+inserted\n+hello world\n")
					m := largeModel(s, 180, 45)
					t.Cleanup(m.Close)
					m.openReviewTab(s)
					m.selectReviewView(viewFiles)
					if split {
						m.layout = diffLayoutSideBySide
					}
					m.reactionEmoji = false
					m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: s, Snapshot: snapshot})
					var sent CommentAction
					m.SetCommentActionSubmitter(func(_ context.Context, a CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
						sent = a
						if a.Body != "" {
							return source.ReviewComment{ID: 9, ParentID: a.Comment.ID, Target: a.Comment.Target, Body: a.Body}, source.ReviewCommentReaction{}, nil
						}
						return source.ReviewComment{}, source.ReviewCommentReaction{ID: 10, Content: a.Reaction}, nil
					})
					m.Focus, m.cursorActive = paneDiff, true
					footer := -1
					for i, row := range m.displayDetail() {
						if row.commentID == id && row.commentFooter {
							footer = i
							break
						}
					}
					if footer < 0 {
						t.Fatal("historical footer missing")
					}
					m.setCursor(footer)
					m.ensureCursorVisible()
					clicked := false
					for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
						if at := strings.Index(line, label); at >= 0 {
							cmd := m.mouseReviewClick(visibleWidth(line[:at])+1, y)
							if label == "[↩ Reply]" {
								if m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply || m.CommentMenu.ReplyToID != 7 || m.top() != pageReview {
									t.Fatal("historical reply did not open inline on original thread")
								}
								m.CommentMenu.Draft = "follow up"
								cmd = m.commentActionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
							}
							completeAction(t, m, cmd)
							clicked = true
							break
						}
					}
					if !clicked {
						t.Fatal("historical reaction/reply control not rendered")
					}
					if label == "[↩ Reply]" {
						raw := snapshot.Snapshot.Threads[0].OriginalAnchor
						if sent.Comment.ID != 7 || sent.Comment.CurrentAnchor == nil || *sent.Comment.CurrentAnchor != *raw || sent.Comment.Target.Line != raw.Line {
							t.Fatalf("reply used projected anchor: %#v", sent)
						}
						if len(m.Comments) != 3 || m.Comments[2].Target.Line != 2 || !m.Comments[2].HistoricalProjection {
							t.Fatal("confirmed reply lost historical display location")
						}
					} else if sent.Comment.ID != id || sent.Reaction != "+1" || sent.Comment.Target.Line != 1 {
						t.Fatalf("reaction targeted wrong comment: %#v", sent)
					}
				})
			}
		}
	}
}
