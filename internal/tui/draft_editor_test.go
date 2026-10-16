package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"
	"testing"
)

func TestMultilineInputPolicyRouting(t *testing.T) {
	for _, surface := range []string{"comment", "suggestion", "reply", "published", "general", "summary"} {
		for _, input := range []string{"typed", "newline", "paste", "clipboard"} {
			t.Run(surface+"/"+input, func(t *testing.T) {
				m := commitModel(t)
				body := strings.Repeat("x", 65536)
				var field *string
				switch surface {
				case "comment", "suggestion":
					m.Composer = &commentComposer{Draft: body, Cursor: 65536, Suggestion: surface == "suggestion"}
					field = &m.Composer.Draft
				case "reply":
					m.CommentMenu = &commentActionMenu{mode: commentActionReply, Draft: body, Cursor: 65536}
					field = &m.CommentMenu.Draft
				case "published":
					m.discussions.published = &publishedEditor{draft: body, cursor: 65536}
					field = &m.discussions.published.draft
				case "general":
					m.push(pageDiscussions)
					m.discussions.editor = &generalCommentEditor{draft: body, cursor: 65536}
					field = &m.discussions.editor.draft
				case "summary":
					m.openReviewForm()
					m.ReviewForm.Focus = 1
					m.ReviewForm.Body = body
					m.ReviewForm.Cursor = 65536
					field = &m.ReviewForm.Body
				}
				text := "é"
				var msg tea.Msg = tea.KeyPressMsg{Code: 'é', Text: text}
				switch input {
				case "newline":
					text = "\n"
					msg = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
				case "paste":
					msg = tea.PasteMsg{Content: text}
				case "clipboard":
					m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
					msg = tea.ClipboardMsg{Content: text, Selection: 'c'}
				}
				m.Update(msg)
				want := body
				if surface == "summary" {
					want += text
				}
				if *field != want {
					t.Fatalf("length=%d, want %d", len(*field), len(want))
				}
			})
		}
	}
}

func TestDraftEditorRuneOperations(t *testing.T) {
	cases := []struct {
		name, body, key, want string
		cursor, wantCursor    int
	}{
		{"left clamps past end", "猫🙂\nz", "left", "猫🙂\nz", 99, 3},
		{"right clamps negative", "猫🙂\nz", "right", "猫🙂\nz", -9, 1},
		{"home", "猫🙂\nz", "home", "猫🙂\nz", 3, 0},
		{"end", "猫🙂\nz", "end", "猫🙂\nz", 0, 4},
		{"backspace rune", "猫🙂\nz", "backspace", "猫\nz", 2, 1},
		{"delete rune", "猫🙂\nz", "delete", "猫\nz", 1, 1},
		{"backspace beginning", "ab", "backspace", "ab", -1, 0},
		{"delete end", "ab", "delete", "ab", 99, 2},
		{"backspace empty", "", "backspace", "", 99, 0},
		{"delete empty", "", "delete", "", -1, 0},
	}
	codes := map[string]rune{"left": tea.KeyLeft, "right": tea.KeyRight, "home": tea.KeyHome, "end": tea.KeyEnd, "backspace": tea.KeyBackspace, "delete": tea.KeyDelete}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, cursor := tc.body, tc.cursor
			if (draftEditor{&body, &cursor, commentDraft}).key(tea.KeyPressMsg{Code: codes[tc.key]}) != draftEdited {
				t.Fatal("edit unhandled")
			}
			if body != tc.want || cursor != tc.wantCursor {
				t.Fatalf("got %q at %d, want %q at %d", body, cursor, tc.want, tc.wantCursor)
			}
		})
	}
}

func TestDraftEditorInsertionPolicy(t *testing.T) {
	for _, policy := range []draftPolicy{commentDraft, suggestionDraft, replyDraft, publishedDraft, generalDraft, summaryDraft} {
		t.Run(fmt.Sprint(policy), func(t *testing.T) {
			body, cursor := "猫犬", 1
			editor := draftEditor{&body, &cursor, policy}
			if !editor.insert("🙂\n```suggestion\nq\r\n") || body != "猫🙂\n```suggestion\nq\r\n犬" || cursor != len([]rune(body))-1 {
				t.Fatalf("literal insertion: %q at %d", body, cursor)
			}
			before := body
			if editor.insert(string([]byte{0xff})) || body != before {
				t.Fatal("invalid UTF-8 changed draft")
			}
			body, cursor = strings.Repeat("x", 65534), 65534
			if !editor.insert("é") || len(body) != 65536 || cursor != 65535 {
				t.Fatal("exact byte budget rejected")
			}
			accepted := editor.insert("界")
			if accepted != (policy == summaryDraft) {
				t.Fatal("incorrect per-surface budget")
			}
			// Recovered oversized text must stay editable down to a valid size.
			body, cursor = strings.Repeat("x", 65537), 65537
			editor.key(tea.KeyPressMsg{Code: tea.KeyBackspace})
			if len(body) != 65536 || cursor != 65536 {
				t.Fatal("oversized draft cannot be reduced")
			}
		})
	}
	for _, tc := range []struct {
		cursor     int
		want       string
		wantCursor int
	}{
		{-3, "Xab", 1}, {1, "aXb", 2}, {99, "abX", 3},
	} {
		body, cursor := "ab", tc.cursor
		e := draftEditor{&body, &cursor, commentDraft}
		if !e.insert("X") || body != tc.want || cursor != tc.wantCursor {
			t.Fatalf("insertion at %d: %q at %d, want %q at %d", tc.cursor, body, cursor, tc.want, tc.wantCursor)
		}
	}
}

func TestDraftEditorLeavesWorkflowKeysWithOwner(t *testing.T) {
	body, cursor := "original", 3
	editor := draftEditor{&body, &cursor, commentDraft}
	for _, key := range []tea.KeyPressMsg{
		{Code: tea.KeyEnter}, {Code: tea.KeyEscape}, {Code: tea.KeyTab},
		{Code: 'p', Mod: tea.ModCtrl}, {Code: 'x', Text: "x", Mod: tea.ModAlt},
	} {
		if editor.key(key) != draftUnhandled || body != "original" || cursor != 3 {
			t.Fatal("editor consumed workflow key")
		}
	}
}

func TestReviewSummaryConfirmationRejectsKeyboardEdits(t *testing.T) {
	m := commitModel(t)
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	m.ReviewForm.Body = "original"
	m.ReviewForm.Cursor = 3
	m.ReviewForm.Confirm = true
	for _, msg := range []tea.Msg{tea.KeyPressMsg{Code: 'x', Text: "x"}, tea.KeyPressMsg{Code: tea.KeyBackspace}, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}, tea.PasteMsg{Content: "x"}} {
		m.Update(msg)
		if m.ReviewForm.Body != "original" || m.ReviewForm.Cursor != 3 || !m.ReviewForm.Confirm {
			t.Fatal("confirmation allowed draft mutation")
		}
	}
}
