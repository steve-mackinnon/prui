package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
	"testing"
)

func rangeTestModel() *Model {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	meta := &m.Session.Inventory.Comparison.Metadata
	meta.BaseSHA = strings.Repeat("a", 40)
	meta.HeadSHA = strings.Repeat("b", 40)
	m.Session.Inventory.Patches["p"] = []byte("@@ -1,3 +1,3 @@\n-one\n-two\n-three\n+first\n+second\n+third\n")
	m.Selected = 1
	m.selectReviewView(viewFiles)
	m.Focus = paneDiff
	m.Width = 180
	m.Height = 24
	return m
}
func selectRawTarget(t *testing.T, m *Model, side string, line int) {
	t.Helper()
	for i, row := range m.displayDetail() {
		targets := []source.ReviewCommentTarget{}
		if row.target != nil {
			targets = append(targets, *row.target)
		}
		if row.sideBySide != nil {
			targets = append(targets, rowTargets(*row.sideBySide)...)
		}
		for _, target := range targets {
			if target.Side == side && target.Line == line {
				m.setCursor(i)
				m.setSelectedDiffTarget(&target)
				return
			}
		}
	}
	t.Fatal("target not visible", side, line)
}
func TestKeyboardRangesKeepTargetsThroughLayoutQueueEditAndRefresh(t *testing.T) {
	for _, side := range []string{"LEFT", "RIGHT"} {
		for _, split := range []bool{false, true} {
			t.Run(side+map[bool]string{false: " unified", true: " split"}[split], func(t *testing.T) {
				m := rangeTestModel()
				if split {
					m.layout = diffLayoutSideBySide
				}
				if split {
					selectRawTarget(t, m, "RIGHT", 1)
					if side == "LEFT" {
						ctrlKey(m, 'o')
					}
				} else {
					selectRawTarget(t, m, side, 1)
				}
				ctrlKey(m, 'v')
				key(m, 'j')
				if !strings.Contains(m.reviewHints(), "start "+side+":1") || !strings.Contains(m.reviewHints(), "end "+side+":2") {
					t.Fatal(m.reviewHints())
				}
				namedKey(m, tea.KeyEnter)
				if m.Composer == nil || m.Composer.Target.StartLine != 1 || m.Composer.Target.Line != 2 || m.Composer.Target.StartSide != side {
					t.Fatalf("lost range: %#v", m.Composer)
				}
				want := m.Composer.Target
				if !strings.Contains(m.View().Content, "start 1 → end 2") {
					t.Fatal("anchors absent from editor")
				}
				key(m, 'x')
				ctrlKey(m, 'p')
				if len(m.Pending) != 1 || m.Pending[0].Target != want {
					t.Fatal("queue lost target")
				}
				m.applyReviewResult(ReviewResult{Target: m.activeTab, Err: nil}) // no form: unrelated refresh must retain queue
				key(m, 'R')
				m.ReviewForm.Focus = 2
				namedKey(m, tea.KeyEnter)
				if m.Composer == nil || m.Composer.Target != want {
					t.Fatal("pending edit lost range", m.ActionError)
				}
				// Layout changes leave the raw target immutable.
				m.layout = diffLayoutUnified
				m.Comments = nil
				if !strings.Contains(m.View().Content, "start 1 → end 2") || m.Composer.Target != want {
					t.Fatal("layout/refresh lost editor")
				}
			})
		}
	}
}
func TestRangeRejectsCrossSideWithoutDiscardingSelection(t *testing.T) {
	m := rangeTestModel()
	selectRawTarget(t, m, "LEFT", 1)
	ctrlKey(m, 'v')
	selectRawTarget(t, m, "RIGHT", 2)
	namedKey(m, tea.KeyEnter)
	if m.Composer != nil || m.rangeStart == nil || m.ActionError == nil || !strings.Contains(m.reviewHints(), "endpoints") {
		t.Fatal("invalid range not visibly rejected")
	}
	namedKey(m, tea.KeyEscape)
	if m.rangeStart != nil {
		t.Fatal("escape retained selection")
	}
}
func TestFileCommentEligibleNonTextAndUnsupportedQueue(t *testing.T) {
	for _, unit := range []int{1, 2, 3, 4} {
		m := rangeTestModel()
		m.Selected = unit
		m.Files = false
		m.Inventory = true
		ctrlKey(m, 'f')
		if unit == 4 {
			if m.Composer != nil || m.ActionError == nil {
				t.Fatal("unavailable file accepted")
			}
			continue
		}
		if m.Composer == nil || m.Composer.Target.SubjectType != "file" || m.Composer.Target.Line != 0 || m.Composer.Target.Side != "" {
			t.Fatal("file target invented coordinates")
		}
		if !strings.Contains(m.View().Content, " · file") {
			t.Fatal("file anchor invisible")
		}
		key(m, 'x')
		ctrlKey(m, 'p')
		if m.Composer == nil || len(m.Pending) != 0 || m.ActionError == nil {
			t.Fatal("file silently queued")
		}
		var got CommentSubmission
		m.SetCommentSubmitter(func(_ context.Context, s CommentSubmission) (source.ReviewComment, error) {
			got = s
			return s.Comment, nil
		})
		want := m.Composer.Target
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		completeAction(t, m, cmd)
		if got.Comment.Target != want || got.CommitInventory == nil || m.Composer != nil {
			t.Fatal("file immediate submission lost target", m.ActionError)
		}
	}
}
func TestExtendedDraftOfflineRecoveryAndImmutableAttempt(t *testing.T) {
	for _, file := range []bool{false, true} {
		t.Run(map[bool]string{false: "range", true: "file"}[file], func(t *testing.T) {
			store, err := session.Open(t.TempDir() + "/private")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			m := rangeTestModel()
			m.Session.ID = strings.Repeat("d", 32)
			m.SetLifecycle(store, nil, nil)
			m.openReviewTab(m.Session)
			m.selectReviewView(viewFiles)
			m.Selected = 1
			m.Focus = paneDiff
			if file {
				m.Selected = 2
				ctrlKey(m, 'f')
			} else {
				selectRawTarget(t, m, "RIGHT", 1)
				ctrlKey(m, 'v')
				key(m, 'j')
				namedKey(m, tea.KeyEnter)
			}
			if m.Composer == nil {
				t.Fatal("composer absent")
			}
			key(m, 'x')
			want := m.Composer.Target
			if !m.prepareDraftAttempt("comment") {
				t.Fatal(m.ActionError)
			}
			restored := New(context.Background(), nil)
			restored.SetLifecycle(store, nil, nil)
			restored.openReviewTab(m.Session)
			if restored.Composer == nil || restored.Composer.Target != want || restored.draft.attempted.Comment.Target != want {
				t.Fatal("restart lost target")
			}
			if !strings.Contains(restored.draftRecoveryView(), commentTargetLabel(want)) {
				t.Fatal("recovery omitted target")
			}
			namedKey(restored, tea.KeyEnter)
			if restored.top() == pageDraftRecovery {
				t.Fatal("offline target could not be inspected", restored.ActionError)
			}
			if !strings.Contains(restored.View().Content, commentTargetLabel(want)) {
				t.Fatal("recovered editor invisible")
			}
		})
	}
}

func TestKeyboardOldRangeCanStartEndAndSpanContext(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(map[bool]string{false: "unified", true: "split"}[split], func(t *testing.T) {
			m := rangeTestModel()
			m.Session.Inventory.Patches["p"] = []byte("@@ -1,5 +1,5 @@\n before\n-old\n+new\n middle\n-old2\n+new2\n after\n")
			if split {
				m.layout = diffLayoutSideBySide
			}
			selectRawTarget(t, m, "RIGHT", 1)
			ctrlKey(m, 'o')
			ctrlKey(m, 'v')
			if m.rangeStart == nil || m.rangeStart.Side != "LEFT" || m.rangeStart.Line != 1 {
				t.Fatal("old context start inaccessible by keyboard")
			}
			for n := 0; n < 10; n++ {
				end := m.commentSelectionTarget()
				if end != nil && end.Side == "LEFT" && end.Line == 5 {
					break
				}
				key(m, 'j')
			}
			namedKey(m, tea.KeyEnter)
			if m.Composer == nil || m.Composer.Target.Side != "LEFT" || m.Composer.Target.StartLine != 1 || m.Composer.Target.Line != 5 {
				t.Fatal("old contextual range inaccessible", m.ActionError)
			}
			want := m.Composer.Target
			key(m, 'x')
			ctrlKey(m, 'p')
			key(m, 'R')
			m.ReviewForm.Focus = 2
			namedKey(m, tea.KeyEnter)
			if m.Composer == nil || m.Composer.Target != want || !strings.Contains(m.View().Content, "start 1 → end 5") {
				t.Fatal("old-context endpoint lost on edit", m.ActionError)
			}
		})
	}
}

func TestPublishedRefreshPreservesRangeAndFileTargets(t *testing.T) {
	m := rangeTestModel()
	meta := m.Session.Inventory.Comparison.Metadata
	rangeTarget := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 2, StartLine: 1, StartSide: "RIGHT"}
	fileTarget := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "image.png", SubjectType: "file"}
	comments := []source.ReviewComment{{ID: 1, Target: rangeTarget, Body: "range"}, {ID: 2, Target: fileTarget, Body: "file"}}
	got := commentOverlay(comments, m.Session)
	if len(got) != 2 || got[0].Target != rangeTarget || got[1].Target != fileTarget {
		t.Fatal("overlay lost extended targets")
	}
	m.Comments = got
	if !strings.Contains(m.View().Content, "range") {
		t.Fatal("range not placed at endpoint")
	}
	m.Composer = &commentComposer{Target: rangeTarget, PendingIndex: -1, Draft: "keep"}
	no := false
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "range", CurrentAnchor: &rangeTarget, Outdated: &no, Comments: []source.ReviewComment{comments[0]}}, {ID: "file", CurrentAnchor: &fileTarget, Outdated: &no, Comments: []source.ReviewComment{comments[1]}}}}}})
	if m.Composer.Target != rangeTarget || len(m.Comments) != 2 || m.Comments[0].Target != rangeTarget || m.Comments[1].Target != fileTarget {
		t.Fatal("published refresh flattened target")
	}
}

func TestRangeReplyAttemptPreservesAssociatedRawSHA(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	meta := m.Session.Inventory.Comparison.Metadata
	display := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 2, StartLine: 1, StartSide: "RIGHT"}
	raw := display
	raw.CommitID = strings.Repeat("c", 40)
	m.Comments = []source.ReviewComment{{ID: 7, Target: display, CurrentAnchor: &raw}}
	m.CommentMenu = &commentActionMenu{CommentID: 7, ReplyToID: 7, Target: display, Draft: "reply", mode: commentActionReply}
	if !m.prepareDraftAttempt("reply") {
		t.Fatal("reply raw range rejected", m.ActionError)
	}
	restarted := draftTestModel(t, store)
	if restarted.draft.attempted == nil || restarted.draft.attempted.Comment.Target != raw || restarted.CommentMenu.Target != display || restarted.CommentMenu.RootAnchor == nil || *restarted.CommentMenu.RootAnchor != raw {
		t.Fatal("reply associated range/SHA lost on restart")
	}
	var got session.Draft
	restarted.SetDraftReconciler(func(_ context.Context, _ source.Metadata, d session.Draft) (bool, error) { got = d; return false, nil })
	_, cmd := restarted.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	completeAction(t, restarted, cmd)
	if got.Attempted.Comment.Target != raw || got.Attempted.ParentID != 7 {
		t.Fatal("reply reconciliation lost raw range")
	}
}
