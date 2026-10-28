package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

// Locate controls independently in the final rendered screen, including its frame.
func clickReviewText(t *testing.T, m *Model, text string) tea.Cmd {
	t.Helper()
	for row, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if at := strings.Index(line, text); at >= 0 {
			return modalClick(m, visibleWidth(line[:at]), row)
		}
	}
	t.Fatalf("control %q not visible:\n%s", text, m.View().Content)
	return nil
}

func TestReviewFormVisibleCommentControl(t *testing.T) {
	for _, size := range [][2]int{{120, 28}, {80, 10}, {80, 8}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := New(context.Background(), nil)
			defer m.Close()
			m.Session = largeSession(2, 2)
			m.Width, m.Height = size[0], size[1]
			m.openReviewForm()
			text := "tab/enter: comment"
			if size[1] <= 10 {
				text = "tab: comment"
			}
			clickReviewText(t, m, text)
			if m.ReviewForm.Focus != 1 || m.ReviewForm.Confirm || m.Busy {
				t.Fatal("visible comment control did not focus summary safely")
			}
		})
	}
}

func TestReviewFormRenderedGeometryAcrossHeights(t *testing.T) {
	for height := 7; height <= 25; height++ {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("height%d/error%t", height, failed), func(t *testing.T) {
				m := New(context.Background(), nil)
				defer m.Close()
				m.Session = largeSession(2, 2)
				m.Session.Inventory.Complete = false
				m.Width, m.Height = 100, height
				m.openReviewForm()
				if failed {
					m.ActionError = errors.New("synthetic failure")
				}
				clickReviewText(t, m, "○ Approve")
				if m.ReviewForm.Event != 1 || m.ReviewForm.Focus != 0 {
					t.Fatal("rendered decision did not select")
				}
				text := "tab/enter: comment"
				if height <= 10 {
					text = "tab: comment"
				}
				clickReviewText(t, m, text)
				if m.ReviewForm.Focus != 1 {
					t.Fatal("comment focus missing")
				}
				m.ReviewForm.Body = "private summary"
				clickReviewText(t, m, "enter: confirm")
				if !m.ReviewForm.Confirm || m.Busy {
					t.Fatal("first action bypassed confirmation")
				}
				clickReviewText(t, m, "esc: edit")
				if m.ReviewForm.Confirm || m.ReviewForm.Body != "private summary" {
					t.Fatal("cancel lost draft")
				}
			})
		}
	}
}

func TestReviewFormSummaryViewportAndInertRows(t *testing.T) {
	for _, height := range []int{14, 15, 16, 28} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			m := New(context.Background(), nil)
			defer m.Close()
			m.Session = largeSession(2, 2)
			m.Width, m.Height = 120, height
			m.openReviewForm()
			m.ReviewForm.Body = "line-00\nline-01\nline-02\nline-03\nline-04\nline-05\nline-06\nline-07\nline-08\nline-09"
			m.ReviewForm.Cursor = len(m.ReviewForm.Body)
			m.ReviewForm.Focus = 1
			clickReviewText(t, m, "line-09")
			if m.ReviewForm.Cursor != strings.Index(m.ReviewForm.Body, "line-09") {
				t.Fatalf("scrolled summary cursor %d", m.ReviewForm.Cursor)
			}
			m.ReviewForm.Focus = 0
			// This heading and whitespace are display-only; neither is the editor.
			for row, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
				if at := strings.Index(line, "Pending comments ("); at >= 0 {
					modalClick(m, visibleWidth(line[:at]), row)
					if m.ReviewForm.Focus != 0 {
						t.Fatal("heading focused summary")
					}
				}
				if at := strings.Index(line, "○ Approve"); at >= 0 {
					modalClick(m, visibleWidth(line[:at])+55, row)
					if m.ReviewForm.Focus != 0 || m.ReviewForm.Event != 0 {
						t.Fatal("blank decision tail activated")
					}
				}
			}
		})
	}
}

func TestReviewFormPendingWindowAndTrustedControls(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = kindsSession()
	m.Session.Inventory.Complete = false
	m.Width, m.Height = 120, 28
	m.openReviewForm()
	metadata := m.Session.Inventory.Comparison.Metadata
	target := source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}
	for i := 0; i < 10; i++ {
		m.Pending = append(m.Pending, source.ReviewComment{Target: target, Body: fmt.Sprintf("pending-%02d enter: submit review", i)})
	}
	m.ActionError = errors.New("synthetic failure")
	m.ReviewForm.Focus, m.ReviewForm.Selected = 2, 8
	clickReviewText(t, m, "pending-07 enter: submit review")
	if m.ReviewForm.Selected != 7 || m.ReviewForm.Focus != 2 || m.Busy || m.ReviewForm.Confirm {
		t.Fatal("pending text became an action or selected wrong item")
	}
	clickReviewText(t, m, "d: remove")
	if len(m.Pending) != 9 || strings.HasPrefix(m.Pending[7].Body, "pending-07") {
		t.Fatal("visible remove missed selected item")
	}
	clickReviewText(t, m, "enter: edit")
	if m.Composer == nil || !strings.HasPrefix(m.Composer.Draft, "pending-08") || m.Composer.Target != target {
		t.Fatal("edit lost pending identity")
	}
}

func TestReviewFormClippedControlsAndConfirmationGuard(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		m := New(context.Background(), nil)
		m.Session = largeSession(2, 2)
		m.Width, m.Height = 18, 24
		m.openReviewForm()
		m.ReviewForm.Focus = 1
		m.ReviewForm.Body = "enter: submit review and pending comments"
		m.ReviewForm.Confirm = confirm
		clickReviewText(t, m, "enter: sub") // private body or clipped footer; neither can write
		if m.Busy || m.ActionError != nil || m.ReviewForm.Confirm != confirm {
			t.Fatal("private text or clipped submit activated")
		}
		m.Close()
	}
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = kindsSession()
	m.Width, m.Height = 120, 28
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	m.ReviewForm.Body = "summary"
	calls := 0
	m.SetReviewSubmitter(func(_ context.Context, submission ReviewSubmission) error {
		calls++
		if submission.Review.Body != "summary" {
			t.Error("summary changed")
		}
		return errors.New("synthetic failure")
	})
	clickReviewText(t, m, "enter: confirm")
	if calls != 0 || !m.ReviewForm.Confirm {
		t.Fatal("initial click submitted")
	}
	cmd := clickReviewText(t, m, "enter: submit review and pending comments")
	completeAction(t, m, cmd)
	if calls != 1 || m.ReviewForm.Confirm || m.ReviewForm.Body != "summary" {
		t.Fatal("confirmed submit did not retain failed draft safely")
	}
}

func TestReviewFormCompactEscapedSummaryCaret(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 80, 10
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	m.ReviewForm.Body = "a\nb界c"
	m.ReviewForm.Cursor = len([]rune(m.ReviewForm.Body))
	clickReviewText(t, m, "b界c")
	if m.ReviewForm.Cursor != 2 {
		t.Fatalf("escaped compact summary cursor=%d", m.ReviewForm.Cursor)
	}
}

func TestReviewFormTruncatedFooterAndBack(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 18, 24
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	m.ReviewForm.Body = "private summary"
	m.ReviewForm.Confirm = true
	clickReviewText(t, m, "enter: submit")
	if m.Busy || m.ActionError != nil || !m.ReviewForm.Confirm {
		t.Fatal("partially visible submit label activated")
	}
	m.Width, m.Height = 120, 28
	clickReviewText(t, m, "esc: edit")
	clickReviewText(t, m, "esc: back")
	if m.top() == pageReviewSubmit || m.ReviewForm.Body != "private summary" {
		t.Fatal("back lost review draft")
	}
}
