package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guideconfig"
	"prui/internal/source"
)

func modalClick(m *Model, x, y int) tea.Cmd {
	cmd, _ := m.mouseModal(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	return cmd
}

func TestGuideConsentMousePreservesConsentAndOwnsBackground(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 90, 28
	options := []guideconfig.Selection{{Provider: "openai", Model: "first", Destination: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"}, {Provider: "anthropic", Model: "second", Destination: "https://api.anthropic.com", APIKeyEnv: "ANTHROPIC_API_KEY"}}
	m.SetGuideOptions(options, options[0], nil)
	m.push(pageGuideConsent)
	view := strings.Split(ansi.Strip(m.View().Content), "\n")
	for row, line := range view {
		if x := strings.Index(line, "Provider:"); x >= 0 {
			modalClick(m, x, row)
			break
		}
	}
	if m.currentGuideSelection().Provider != "anthropic" || m.guideFocus != 1 {
		t.Fatal("provider click did not cycle")
	}
	for row, line := range view {
		if x := strings.Index(line, "Model:"); x >= 0 {
			modalClick(m, x, row)
			break
		}
	}
	if m.guideFocus != 2 {
		t.Fatal("model field did not focus")
	}
	if _, owned := m.mouseModal(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0}); !owned || m.top() != pageGuideConsent {
		t.Fatal("background click escaped modal")
	}
	m.Width, m.Height = 30, 9
	m.guideFocus = 0
	if cmd := modalClick(m, 5, 8); cmd != nil || m.Busy {
		t.Fatal("clipped consent allowed upload")
	}
}

func TestMouseReviewFormDecisionPendingAndSeparateConfirmation(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 120, 28
	m.openReviewForm()
	m.ReviewForm.Body = "Summary"
	m.ReviewForm.Cursor = 7
	m.Pending = []source.ReviewComment{{Body: "enter: submit review and pending comments", Target: source.ReviewCommentTarget{Path: "file.go", Side: "RIGHT", Line: 1}}, {Body: "other", Target: source.ReviewCommentTarget{Path: "file.go", Side: "RIGHT", Line: 2}}}
	form := *m
	form.Width = min(96, m.Width-8)
	form.Height = min(22, m.Height-2)
	body := form.reviewFormView()
	b := actionBodyBounds(m.Width, m.Height, body)
	modalClick(m, b.Min.X, b.Min.Y+5)
	if m.ReviewForm.Event != 1 || m.ReviewForm.Focus != 0 {
		t.Fatal("approval click did not select")
	}
	clickReviewText(t, m, "other")
	if m.ReviewForm.Selected != 1 || m.ReviewForm.Focus != 2 {
		t.Fatal("pending click did not select")
	}
	if m.Busy || m.Composer != nil {
		t.Fatal("pending click activated write or editor")
	}
	m.ReviewForm.Focus = 1
	form = *m
	form.Width = 96
	form.Height = 22
	body = form.reviewFormView()
	b = actionBodyBounds(m.Width, m.Height, body)
	modalClick(m, b.Min.X, b.Min.Y+len(strings.Split(body, "\n"))-1)
	if !m.ReviewForm.Confirm || m.Busy {
		t.Fatal("confirmation click bypassed final confirmation")
	}
}

func TestLoadingMouseCancelUsesOnlyVisibleControl(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {15, 4}, {30, 6}} {
		m := New(context.Background(), nil)
		m.Width, m.Height = size[0], size[1]
		m.Busy = true
		m.notice = "esc: cancel"
		cancelled := false
		m.cancelAction = func() { cancelled = true }
		b := m.loadingCancelBounds(m.loadingModal())
		modalClick(m, 0, 0)
		if cancelled {
			t.Fatal("background cancelled loading")
		}
		modalClick(m, b.Min.X, b.Min.Y)
		if !cancelled {
			t.Fatalf("cancel control inactive at %v", size)
		}
		m.Close()
	}
}

func TestMouseEditorControlsAndUntrustedBody(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 120, 28
	m.push(pageDiscussions)
	m.discussions.editor = &generalCommentEditor{draft: "Enter: post · esc: discard", cursor: 26}
	modalClick(m, 0, 2)
	if m.discussions.editor == nil {
		t.Fatal("draft text interpreted as a control")
	}
	lines := strings.Split(m.generalCommentView(), "\n")
	x := visibleWidth("Enter: post · shift+enter: newline · ctrl+r: refresh · ")
	modalClick(m, x, len(lines)-1)
	if m.discussions.editor != nil {
		t.Fatal("discard control inactive")
	}
}

func TestMouseRecoveryScrollAndControls(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 120, 28
	m.ReviewForm = &reviewForm{Body: strings.Repeat("private\n", 35)}
	m.push(pageDraftRecovery)
	body := m.draftRecoveryView()
	b := actionBodyBounds(m.Width, m.Height, body)
	m.mouseModal(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: b.Min.X, Y: b.Min.Y + 1})
	if m.draft.recoveryScroll != mouseWheelStep {
		t.Fatal("recovery wheel ignored")
	}
	body = m.draftRecoveryView()
	b = actionBodyBounds(m.Width, m.Height, body)
	x := visibleWidth("j/k: scroll · ")
	modalClick(m, b.Min.X+x, b.Min.Y+len(strings.Split(body, "\n"))-1)
	if m.top() == pageDraftRecovery || m.ReviewForm == nil {
		t.Fatal("continue lost drafts or stayed in recovery")
	}
}

func TestMousePendingEditAndRemoveControls(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = kindsSession()
	m.Width, m.Height = 120, 28
	m.openReviewForm()
	metadata := m.Session.Inventory.Comparison.Metadata
	target := source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}
	m.Pending = []source.ReviewComment{{Body: "keep", Target: target}, {Body: "remove", Target: target}}
	m.ReviewForm.Focus = 2
	m.ReviewForm.Selected = 1
	form := *m
	form.Width = 96
	form.Height = 22
	body := form.reviewFormView()
	b := actionBodyBounds(m.Width, m.Height, body)
	modalClick(m, b.Min.X+visibleWidth("j/k: select · enter: edit · "), b.Min.Y+len(strings.Split(body, "\n"))-1)
	if len(m.Pending) != 1 || m.Pending[0].Body != "keep" {
		t.Fatal("remove control failed")
	}
	form = *m
	form.Width = 96
	form.Height = 22
	body = form.reviewFormView()
	b = actionBodyBounds(m.Width, m.Height, body)
	modalClick(m, b.Min.X+visibleWidth("j/k: select · "), b.Min.Y+len(strings.Split(body, "\n"))-1)
	if m.Composer == nil || m.Composer.Draft != "keep" || m.top() == pageReviewSubmit {
		t.Fatal("edit control failed")
	}
}

func TestMouseEditorCaretAndClippedControls(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 120, 28
	m.push(pageDiscussions)
	m.discussions.editor = &generalCommentEditor{draft: "hello world", cursor: 11}
	modalClick(m, 2, 2)
	if m.discussions.editor.cursor != 2 {
		t.Fatalf("caret click=%d", m.discussions.editor.cursor)
	}
	m.discussions.editor = nil
	m.Composer = &commentComposer{Draft: "keep", Cursor: 4, PendingIndex: -1}
	m.Stack = []page{pageReview}
	m.Width = 10
	// The visible prefix of 'enter: post now' cannot post.
	row := len(strings.Split(m.reviewView(), "\n")) - 1
	if cmd := modalClick(m, 1, row); cmd != nil || m.ActionError != nil || m.Composer == nil {
		t.Fatal("clipped write label activated")
	}
	m.Composer = nil
	m.Err = context.Canceled
	m.Width = 37
	if cmd := modalClick(m, 35, 2); cmd != nil {
		t.Fatal("clipped quit label activated")
	}
	m.Width = 80
	if cmd := modalClick(m, 35, 2); cmd == nil {
		t.Fatal("visible quit label inactive")
	}
}

func TestEditorCursorAtEscapingWrappingAndLines(t *testing.T) {
	for _, tt := range []struct {
		body                        string
		cursor, width, x, row, want int
		hard, multiline             bool
	}{
		{"ab界c", 4, 80, 3, 0, 2, false, false},
		{"one two three", 13, 7, 2, 1, 10, false, false},
		{"abc\ndef", 7, 80, 1, 1, 5, true, true},
		{"abc\ndef", 7, 80, 4, 0, 3, false, false},
	} {
		got, ok := editorCursorAt(tt.body, tt.cursor, tt.width, tt.hard, tt.multiline, tt.x, tt.row)
		if !ok || got != tt.want {
			t.Fatalf("%q row %d col %d: %d %v want %d", tt.body, tt.row, tt.x, got, ok, tt.want)
		}
	}
}

func TestMouseInlineAndSummaryCaret(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = kindsSession()
	m.Width, m.Height = 120, 28
	m.Files = true
	m.Selected = 1
	m.Focus = paneDiff
	m.cursorActive = true
	metadata := m.Session.Inventory.Comparison.Metadata
	m.Composer = &commentComposer{Target: source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}, Draft: "hello world", Cursor: 11, PendingIndex: -1}
	m.ensureInlineEditorVisible()
	rows := m.displayDetail()
	first := -1
	for i, row := range rows {
		if row.editor {
			first = i
			break
		}
	}
	if first < 0 {
		t.Fatal("editor missing")
	}
	g := m.workspaceGeometry()
	modalClick(m, g.Detail.Min.X+8, g.Detail.Min.Y+first+2-m.offset())
	if m.Composer.Cursor != 2 {
		t.Fatalf("inline caret=%d", m.Composer.Cursor)
	}
	m.Composer = nil
	m.openReviewForm()
	m.ReviewForm.Body = "hello world"
	m.ReviewForm.Cursor = 11
	form := *m
	form.Width = 96
	form.Height = 22
	body := form.reviewFormView()
	b := actionBodyBounds(m.Width, m.Height, body)
	// Decision rows occupy 4–6, blank 7, summary heading 8, border 9.
	modalClick(m, b.Min.X+6, b.Min.Y+10)
	if m.ReviewForm.Focus != 1 || m.ReviewForm.Cursor != 2 {
		t.Fatalf("summary focus=%d caret=%d", m.ReviewForm.Focus, m.ReviewForm.Cursor)
	}
}

func TestMouseShiftEnterControlInsertsNewline(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = kindsSession()
	m.Width, m.Height = 120, 28
	m.Files = true
	m.Selected = 1
	m.Focus = paneDiff
	metadata := m.Session.Inventory.Comparison.Metadata
	m.Composer = &commentComposer{Target: source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}, Draft: "hello", Cursor: 5, PendingIndex: -1}
	text := ansi.Strip(m.modalReviewHints())
	x := strings.Index(text, "shift+enter")
	modalClick(m, x, len(strings.Split(m.reviewView(), "\n"))-1)
	if m.Composer == nil || m.Composer.Draft != "hello\n" || m.Composer.Cursor != 6 {
		t.Fatalf("newline control inserted literal shortcut: %#v", m.Composer)
	}
}

func TestMouseOverviewEmbeddedGeneralAndReply(t *testing.T) {
	for _, reply := range []bool{false, true} {
		m := New(context.Background(), nil)
		m.Session = kindsSession()
		m.Width, m.Height = 120, 36
		m.selectReviewView(viewDescription)
		if reply {
			metadata := m.Session.Inventory.Comparison.Metadata
			target := source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}
			root := source.ReviewComment{ID: 501, Target: target, Body: "existing"}
			m.Comments = []source.ReviewComment{root}
			m.discussions.loaded = true
			m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "thread", CurrentAnchor: &target, Comments: []source.ReviewComment{root}}}}}
			m.CommentMenu = &commentActionMenu{CommentID: 501, ReplyToID: 501, mode: commentActionReply, Target: target, Draft: "hello world", Cursor: 11}
		} else {
			m.discussions.editor = &generalCommentEditor{draft: "hello world", cursor: 11}
		}
		m.reviewView()
		rows := m.overviewRows()
		first, last := -1, -1
		for i, row := range rows {
			if row.action == "editor" {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			t.Fatal("overview editor absent")
		}
		column := 4
		if reply {
			column = 8
		}
		modalClick(m, column, 3+first+2-m.DescriptionScroll)
		if reply {
			if m.CommentMenu.Cursor != 2 {
				t.Fatalf("reply overview caret=%d", m.CommentMenu.Cursor)
			}
		} else if m.discussions.editor.cursor != 2 {
			t.Fatalf("general overview caret=%d", m.discussions.editor.cursor)
		}
		// The embedded footer remains clickable within its card.
		column = 2 + visibleWidth("Enter: post · ")
		modalClick(m, column, 3+last-m.DescriptionScroll)
		if reply {
			if m.CommentMenu.Draft != "he\nllo world" {
				t.Fatalf("embedded reply newline=%q", m.CommentMenu.Draft)
			}
		} else if m.discussions.editor.draft != "he\nllo world" {
			t.Fatalf("embedded general newline=%q", m.discussions.editor.draft)
		}
		m.reviewView()
		rows = m.overviewRows()
		last = -1
		for i, row := range rows {
			if row.action == "editor" {
				last = i
			}
		}
		column = 2 + visibleWidth("Enter: post · shift+enter: newline · ctrl+r: refresh · ")
		modalClick(m, column, 3+last-m.DescriptionScroll)
		if m.CommentMenu != nil || m.discussions.editor != nil {
			t.Fatalf("overview discard control inactive reply=%v", reply)
		}
		m.Close()
	}
}

func TestMouseCommentSuggestionPreparationControl(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = kindsSession()
	m.Width, m.Height = 160, 28
	m.Files = true
	m.Selected = 1
	metadata := m.Session.Inventory.Comparison.Metadata
	target := source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "text", Side: "RIGHT", Line: 2}
	m.Comments = []source.ReviewComment{{ID: 501, Target: target, Body: "```suggestion\nreplacement\n```"}}
	m.CommentMenu = &commentActionMenu{CommentID: 501, Target: target}
	prepared := false
	m.SetSuggestionActions(func(context.Context, source.Metadata, source.ReviewComment, string) (source.SuggestionApplication, error) {
		prepared = true
		return source.SuggestionApplication{}, context.Canceled
	}, nil)
	hints := ansi.Strip(m.modalReviewHints())
	x := strings.Index(hints, "ctrl+a apply suggestion")
	if x < 0 {
		t.Fatal("suggestion preparation control hidden")
	}
	cmd := modalClick(m, x, len(strings.Split(m.reviewView(), "\n"))-1)
	if cmd == nil {
		t.Fatalf("suggestion preparation click ignored: %v", m.ActionError)
	}
	completeAction(t, m, cmd)
	if !prepared || m.SuggestionApply != nil || m.ActionError == nil {
		t.Fatal("suggestion preparation did not use guarded handler")
	}
}
