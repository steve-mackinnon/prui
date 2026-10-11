package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPasteIntoEveryTextField(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Model) *string
		text  string
		want  string
	}{
		{"comment", func(m *Model) *string {
			m.Composer = &commentComposer{Draft: "ab", Cursor: 1}
			return &m.Composer.Draft
		}, "é\nq", "aé\nqb"},
		{"suggestion", func(m *Model) *string {
			m.Composer = &commentComposer{Suggestion: true, Draft: "ab", Cursor: 1}
			return &m.Composer.Draft
		}, "é\nq", "aé\nqb"},
		{"reply", func(m *Model) *string {
			m.CommentMenu = &commentActionMenu{mode: commentActionReply, Draft: "ab", Cursor: 1}
			return &m.CommentMenu.Draft
		}, "é\nq", "aé\nqb"},
		{"general comment", func(m *Model) *string {
			m.push(pageDiscussions)
			m.discussions.editor = &generalCommentEditor{draft: "ab", cursor: 1}
			return &m.discussions.editor.draft
		}, "é\nq", "aé\nqb"},
		{"published comment", func(m *Model) *string {
			m.discussions.published = &publishedEditor{draft: "ab", cursor: 1}
			return &m.discussions.published.draft
		}, "é\nq", "aé\nqb"},
		{"review summary", func(m *Model) *string {
			m.openReviewForm()
			m.ReviewForm.Focus = 1
			m.ReviewForm.Body = "ab"
			m.ReviewForm.Cursor = 1
			return &m.ReviewForm.Body
		}, "é\nq", "aé\nqb"},
		{"file filter", func(m *Model) *string { m.selectReviewView(viewFiles); m.openFileFilter(); return &m.fileFilter }, "sample.go", "sample.go"},
		{"search", func(m *Model) *string { m.selectReviewView(viewFiles); m.openSearch(); return &m.searchState().query }, "needle", "needle"},
		{"model", func(m *Model) *string {
			m.push(pageGuideConsent)
			m.guideFocus = 2
			m.guideChoice.Provider = "openai"
			m.guideChoice.Model = ""
			return &m.guideChoice.Model
		}, "model-name", "model-name"},
		{"switcher", func(m *Model) *string { m.push(pagePullRequestPicker); return &m.SwitcherQuery }, "fix bug", "fix bug"},
		{"inbox repository", func(m *Model) *string {
			m.push(pageInboxFilters)
			m.inbox.field = 1
			return &m.inbox.draftOptions.Repository
		}, "owner/repo", "owner/repo"},
		{"inbox author", func(m *Model) *string {
			m.push(pageInboxFilters)
			m.inbox.field = 2
			return &m.inbox.draftOptions.Author
		}, "author", "author"},
	}
	for _, tc := range cases {
		for _, clipboard := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/clipboard=%t", tc.name, clipboard), func(t *testing.T) {
				m := commitModel(t)
				field := tc.setup(m)
				var msg tea.Msg = tea.PasteMsg{Content: tc.text}
				if clipboard {
					_, cmd := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
					if cmd == nil || fmt.Sprintf("%T", cmd()) != fmt.Sprintf("%T", tea.ReadClipboard()) {
						t.Fatal("no clipboard read command")
					}
					msg = tea.ClipboardMsg{Content: tc.text, Selection: 'c'}
				}
				m.Update(msg)
				if *field != tc.want {
					t.Fatalf("text=%q, want %q", *field, tc.want)
				}
			})
		}
	}
}

func TestPasteDoesNotTriggerCommands(t *testing.T) {
	m := commitModel(t)
	m.Composer = &commentComposer{Draft: "", PendingIndex: -1}
	for _, s := range []string{"enter", "esc", "ctrl+p", "q"} {
		_, cmd := m.Update(tea.PasteMsg{Content: s})
		if cmd != nil {
			t.Fatal("paste executed a command")
		}
	}
	if m.Composer == nil || m.Composer.Draft != "enterescctrl+pq" {
		t.Fatal("paste interpreted as keys")
	}
	m.push(pageQuitPending)
	m.Update(tea.PasteMsg{Content: "hidden"})
	if strings.Contains(m.Composer.Draft, "hidden") {
		t.Fatal("paste reached covered editor")
	}
}

func TestPasteRespectsEditingGuards(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Model) *string
	}{
		{"busy", func(m *Model) *string { m.Composer = &commentComposer{}; m.Busy = true; return &m.Composer.Draft }},
		{"loading", func(m *Model) *string { m.Composer = &commentComposer{}; m.Loading = true; return &m.Composer.Draft }},
		{"uncertain comment", func(m *Model) *string {
			m.Composer = &commentComposer{}
			m.draft.attempt = "comment"
			return &m.Composer.Draft
		}},
		{"uncertain reply", func(m *Model) *string {
			m.CommentMenu = &commentActionMenu{mode: commentActionReply}
			m.draft.attempt = "reply"
			return &m.CommentMenu.Draft
		}},
		{"posting general comment", func(m *Model) *string {
			m.push(pageDiscussions)
			m.discussions.editor = &generalCommentEditor{posting: true}
			return &m.discussions.editor.draft
		}},
		{"posting published comment", func(m *Model) *string {
			m.discussions.published = &publishedEditor{posting: true}
			return &m.discussions.published.draft
		}},
		{"resolve confirmation", func(m *Model) *string {
			resolve := true
			m.discussions.published = &publishedEditor{action: PublishedAction{Resolve: &resolve}}
			return &m.discussions.published.draft
		}},
		{"review confirmation", func(m *Model) *string {
			m.openReviewForm()
			m.ReviewForm.Focus = 1
			m.ReviewForm.Confirm = true
			return &m.ReviewForm.Body
		}},
		{"review event", func(m *Model) *string { m.openReviewForm(); return &m.ReviewForm.Body }},
		{"model provider", func(m *Model) *string { m.push(pageGuideConsent); m.guideFocus = 1; return &m.guideChoice.Model }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := commitModel(t)
			field := tc.setup(m)
			before := *field
			m.Update(tea.PasteMsg{Content: "changed"})
			if *field != before {
				t.Fatal("paste changed noneditable field")
			}
		})
	}
}

func TestPasteRejectsInvalidAndOversizedText(t *testing.T) {
	m := commitModel(t)
	m.Composer = &commentComposer{Draft: "original"}
	for _, text := range []string{string([]byte{0xff}), strings.Repeat("x", 65536)} {
		m.Update(tea.PasteMsg{Content: text})
		if m.Composer.Draft != "original" {
			t.Fatal("invalid paste changed draft")
		}
	}
	m.Composer = nil
	m.selectReviewView(viewFiles)
	m.openFileFilter()
	for _, text := range []string{"a\nb", "a\rb", strings.Repeat("x", 1025)} {
		m.Update(tea.PasteMsg{Content: text})
		if m.fileFilter != "" {
			t.Fatal("invalid single-line paste accepted")
		}
	}
	m.Update(tea.PasteMsg{Content: "esc"})
	if m.fileFilter != "esc" || !m.fileFilterEditing {
		t.Fatal("filter paste interpreted as escape")
	}
}

func TestClipboardReplyStaysWithRequestedField(t *testing.T) {
	m := commitModel(t)
	m.Composer = &commentComposer{Draft: "original"}
	// Unsolicited replies must not edit a field.
	m.Update(tea.ClipboardMsg{Content: "unsolicited", Selection: 'c'})
	m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	m.Composer = &commentComposer{Draft: "other"}
	m.Update(tea.ClipboardMsg{Content: "late", Selection: 'c'})
	if m.Composer.Draft != "other" {
		t.Fatal("late clipboard reply edited a different field")
	}
	m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.Update(tea.ClipboardMsg{Content: "late", Selection: 'c'})
	if m.Composer.Draft != "other" {
		t.Fatal("intervening key did not invalidate clipboard read")
	}
	m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	m.Update(tea.ClipboardMsg{Content: "primary", Selection: 'p'})
	if m.Composer.Draft != "other" {
		t.Fatal("system read accepted primary selection")
	}
}

func TestTerminalPasteCancelsPendingClipboardRead(t *testing.T) {
	m := commitModel(t)
	m.Composer = &commentComposer{}
	m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	m.Update(tea.PasteMsg{Content: "once"})
	m.Update(tea.ClipboardMsg{Content: "twice", Selection: 'c'})
	if m.Composer.Draft != "once" {
		t.Fatal("clipboard reply duplicated terminal paste")
	}
}
