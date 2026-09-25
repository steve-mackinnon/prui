package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/source"
)

func TestPendingCommentAndReviewSubmission(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	metadata := m.Session.Inventory.Comparison.Metadata
	m.Composer = &commentComposer{Target: source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "a.go", Side: "RIGHT", Line: 1}, Draft: "Fix this edge case", PendingIndex: -1}
	called := 0
	m.SetCommentSubmitter(func(context.Context, CommentSubmission) (source.ReviewComment, error) {
		called++
		return source.ReviewComment{}, nil
	})
	_, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if called != 0 || len(m.Pending) != 1 || m.Composer != nil {
		t.Fatalf("pending queue wrote or lost draft: calls=%d pending=%#v", called, m.Pending)
	}
	if !strings.Contains(m.healthStatus(120, m.Session), "R Submit review (1)") {
		t.Fatal("pending count is not discoverable")
	}
	var submitted ReviewSubmission
	m.SetReviewSubmitter(func(_ context.Context, request ReviewSubmission) error {
		submitted = request
		return errors.New("synthetic failure")
	})
	key(m, 'R')
	if m.top() != pageReviewSubmit {
		t.Fatal("review screen did not open")
	}
	namedKey(m, tea.KeyDown)  // Approve
	namedKey(m, tea.KeyEnter) // Summary
	namedKey(m, tea.KeyEnter) // Confirm
	if !m.ReviewForm.Confirm || submitted.Review.Event != "" {
		t.Fatal("review submitted before confirmation")
	}
	_, command := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, command)
	if submitted.Review.Event != "APPROVE" || len(submitted.Review.Comments) != 1 || len(m.Pending) != 1 || m.ReviewForm == nil {
		t.Fatalf("failure lost drafts: submission=%#v pending=%#v form=%#v", submitted, m.Pending, m.ReviewForm)
	}
	if m.ReviewForm.Confirm {
		t.Fatal("failed request remained one key away from a duplicate submission")
	}
	m.SetReviewSubmitter(func(_ context.Context, request ReviewSubmission) error {
		submitted = request
		return nil
	})
	namedKey(m, tea.KeyEnter) // Confirm again after checking GitHub.
	_, command = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, command)
	if m.top() != pageReview || len(m.Pending) != 0 || m.ReviewForm != nil {
		t.Fatalf("successful submission kept drafts: page=%v pending=%#v form=%#v", m.top(), m.Pending, m.ReviewForm)
	}
}

func TestReviewDecisionRadioAndCommentFocus(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Width, m.Height = screenSession(), 80, 12
	m.openReviewForm()
	assertChoices := func(selected string) {
		t.Helper()
		view := m.reviewFormView()
		for _, label := range []string{"Comment", "Approve", "Request changes"} {
			if !strings.Contains(view, label) {
				t.Fatalf("missing radio choice %q:\n%s", label, view)
			}
		}
		if !strings.Contains(view, "› ◉ "+selected) || strings.Count(view, "◉") != 1 {
			t.Fatalf("wrong selected decision:\n%s", view)
		}
	}
	assertChoices("Comment")
	key(m, 'j')
	assertChoices("Approve")
	namedKey(m, tea.KeyUp)
	assertChoices("Comment")
	key(m, 'k')
	assertChoices("Request changes")
	namedKey(m, tea.KeyTab)
	view := m.reviewFormView()
	if !strings.Contains(view, "  ◉ Request changes") || !strings.Contains(view, "› Comment (required)\n  ┃ ▏") {
		t.Fatalf("comment editor did not follow the radio choices:\n%s", view)
	}
	key(m, 'j')
	if m.ReviewForm.Body != "j" || m.ReviewForm.Event != 2 {
		t.Fatalf("typing in comment changed review decision: %#v", m.ReviewForm)
	}
}

func TestPendingCommentCanBeEditedFromReviewScreen(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	s := kindsSession()
	m.openReviewTab(s)
	m.Selected = 1
	var target source.ReviewCommentTarget
	for _, line := range m.baseDetail() {
		if line.target != nil {
			target = *line.target
			break
		}
	}
	if target.Path == "" {
		t.Fatal("fixture has no commentable target")
	}
	m.Pending = []source.ReviewComment{{Target: target, Body: "draft"}}
	m.selectReviewView(viewDescription)
	key(m, 'R')
	namedKey(m, tea.KeyTab)
	namedKey(m, tea.KeyTab)
	namedKey(m, tea.KeyEnter)
	if m.top() != pageReview || m.ContextView != viewChanges || m.Composer == nil || m.Composer.Draft != "draft" || m.Composer.PendingIndex != 0 {
		t.Fatalf("pending draft did not open at its diff target: page=%v view=%v composer=%#v", m.top(), m.ContextView, m.Composer)
	}
	key(m, 'x')
	_, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if len(m.Pending) != 1 || m.Pending[0].Body != "draftx" {
		t.Fatalf("pending draft did not update: %#v", m.Pending)
	}
}

func TestReviewSummaryAcceptsQAndPendingDraftsGuardQuit(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	key(m, 'R')
	namedKey(m, tea.KeyTab)
	key(m, 'q')
	if m.ReviewForm.Body != "q" || m.top() != pageReviewSubmit {
		t.Fatalf("q did not type in summary: %#v", m.ReviewForm)
	}
	namedKey(m, tea.KeyEscape)
	key(m, 'q')
	if m.top() != pageQuitPending {
		t.Fatal("quit did not warn about unsent review summary")
	}
	namedKey(m, tea.KeyEscape)
	if m.top() != pageReview || m.ReviewForm.Body != "q" {
		t.Fatal("canceling quit lost review draft")
	}
}

func TestSubmitReviewActionAvailableFromEveryContextView(t *testing.T) {
	for _, view := range []reviewView{viewChanges, viewDescription, viewCommits} {
		m := New(context.Background(), nil)
		m.Loading = false
		m.openReviewTab(screenSession())
		m.selectReviewView(view)
		key(m, 'R')
		if m.top() != pageReviewSubmit {
			t.Fatalf("R unavailable from view %v", view)
		}
	}
}

func TestReviewFormKeepsErrorVisibleWithLongSummary(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Width, m.Height = screenSession(), 60, 10
	m.openReviewForm()
	m.ReviewForm.Body = strings.Repeat("line\n", 12)
	m.ReviewForm.Confirm = true
	m.ActionError = errors.New("synthetic submit failure")
	view := m.View().Content
	if !strings.Contains(view, "synthetic submit failure") || !strings.Contains(view, "Check GitHub before retrying") || !strings.Contains(view, "enter: submit review") {
		t.Fatalf("failure and retry controls must remain visible:\n%s", view)
	}
}

func TestPendingCommentsStayAnchoredAndTabLocal(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	first := kindsSession()
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "o/r", Number: 1}
	second := kindsSession()
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "o/r", Number: 2}
	m.openReviewTab(first)
	m.Selected = 1
	var target source.ReviewCommentTarget
	for _, line := range m.baseDetail() {
		if line.target != nil {
			target = *line.target
			break
		}
	}
	if target.Path == "" {
		t.Fatal("fixture has no line target")
	}
	m.Pending = []source.ReviewComment{{Target: target, Body: "pending note"}}
	text := ""
	for _, line := range m.detail() {
		text += line.Text + "\n"
	}
	if !strings.Contains(text, "[Pending 1] pending note") {
		t.Fatalf("pending marker absent: %s", text)
	}
	m.openReviewTab(second)
	if len(m.Pending) != 0 {
		t.Fatal("second PR inherited pending comment")
	}
	m.activateTab(0)
	if len(m.Pending) != 1 || m.Pending[0].Body != "pending note" {
		t.Fatalf("first PR lost pending comment: %#v", m.Pending)
	}
}

func TestPendingCommentLimitKeepsEditorDraft(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	metadata := m.Session.Inventory.Comparison.Metadata
	for i := 0; i < source.MaxPendingReviewComments; i++ {
		m.Pending = append(m.Pending, source.ReviewComment{Target: source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "a.go", Side: "RIGHT", Line: i + 1}, Body: "draft"})
	}
	m.Composer = &commentComposer{Target: source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "a.go", Side: "RIGHT", Line: 101}, Draft: "keep me", PendingIndex: -1}
	_, _ = m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if len(m.Pending) != source.MaxPendingReviewComments || m.Composer == nil || m.Composer.Draft != "keep me" || m.ActionError == nil {
		t.Fatalf("limit lost draft or changed queue: pending=%d composer=%#v err=%v", len(m.Pending), m.Composer, m.ActionError)
	}
}
