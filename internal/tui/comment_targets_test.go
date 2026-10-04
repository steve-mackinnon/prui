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
