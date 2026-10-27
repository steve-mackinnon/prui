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
					m.openOverviewDiscussions()
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
		{Code: 'p', Mod: tea.ModCtrl}, {Code: 'x', Text: "x", Mod: tea.ModSuper}, {Code: 'x', Text: "x", Mod: tea.ModMeta}, {Code: 'x', Text: "x", Mod: tea.ModAlt},
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

func TestDraftEditorMacShortcuts(t *testing.T) {
	cases := []struct {
		name, body, want   string
		cursor, wantCursor int
		code               rune
		mod                tea.KeyMod
	}{
		{"previous word", "one two three", "one two three", 7, 4, tea.KeyLeft, tea.ModAlt},
		{"next word", "one two three", "one two three", 4, 7, tea.KeyRight, tea.ModAlt},
		{"skip punctuation backward", "one,  two", "one,  two", 6, 0, tea.KeyLeft, tea.ModAlt},
		{"skip punctuation forward", "one,  two", "one,  two", 3, 9, tea.KeyRight, tea.ModAlt},
		{"combining mark word", "one cafe\u0301", "one cafe\u0301", 9, 4, tea.KeyLeft, tea.ModAlt},
		{"unicode word", "猫 café!", "猫 café!", 6, 2, tea.KeyLeft, tea.ModAlt},
		{"cross newline", "one\ntwo", "one\ntwo", 3, 7, tea.KeyRight, tea.ModAlt},
		{"line start", "first\n猫 dog\nlast", "first\n猫 dog\nlast", 9, 6, tea.KeyLeft, tea.ModSuper},
		{"line end", "first\n猫 dog\nlast", "first\n猫 dog\nlast", 8, 11, tea.KeyRight, tea.ModSuper},
		{"delete to line start", "first\n猫 dog\nlast", "first\nog\nlast", 9, 6, tea.KeyBackspace, tea.ModSuper},
		{"delete whole line from end", "first\n猫 dog\nlast", "first\n\nlast", 11, 6, tea.KeyBackspace, tea.ModSuper},
		{"delete at line start", "one\ntwo", "one\ntwo", 4, 4, tea.KeyBackspace, tea.ModSuper},
		{"empty", "", "", 0, 0, tea.KeyLeft, tea.ModAlt},
		{"clamp before start", "one", "one", -5, 0, tea.KeyLeft, tea.ModSuper},
		{"clamp after end", "one", "one", 99, 3, tea.KeyRight, tea.ModAlt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, cursor := tc.body, tc.cursor
			result := (draftEditor{&body, &cursor, commentDraft}).key(tea.KeyPressMsg{Code: tc.code, Mod: tc.mod})
			if result != draftEdited || body != tc.want || cursor != tc.wantCursor {
				t.Fatalf("got %q at %d (result %d), want %q at %d", body, cursor, result, tc.want, tc.wantCursor)
			}
		})
	}
}

func TestDraftEditorWordDeletionAndLineMovement(t *testing.T) {
	cases := []struct {
		name, body, want   string
		cursor, wantCursor int
		code               rune
		mod                tea.KeyMod
	}{
		{"delete previous word", "one two three", "one  three", 7, 4, tea.KeyBackspace, tea.ModAlt},
		{"delete punctuation and word", "one,  two", "two", 6, 0, tea.KeyBackspace, tea.ModAlt},
		{"delete combining word", "one cafe\u0301!", "one !", 9, 4, tea.KeyBackspace, tea.ModAlt},
		{"delete across newline", "one\ntwo", "two", 4, 0, tea.KeyBackspace, tea.ModAlt},
		{"delete at start", "one", "one", 0, 0, tea.KeyBackspace, tea.ModAlt},
		{"delete empty", "", "", 99, 0, tea.KeyBackspace, tea.ModAlt},
		{"up same column", "abcd\n猫🙂ef", "abcd\n猫🙂ef", 7, 2, tea.KeyUp, 0},
		{"down same column", "猫🙂ef\nabcd", "猫🙂ef\nabcd", 2, 7, tea.KeyDown, 0},
		{"up shorter line", "a\nlong", "a\nlong", 6, 1, tea.KeyUp, 0},
		{"down shorter line", "long\na", "long\na", 4, 6, tea.KeyDown, 0},
		{"up empty line", "ab\n\ncd", "ab\n\ncd", 5, 3, tea.KeyUp, 0},
		{"down empty line", "ab\n\ncd", "ab\n\ncd", 1, 3, tea.KeyDown, 0},
		{"up first line", "abcd\nef", "abcd\nef", 2, 2, tea.KeyUp, 0},
		{"down last line", "ab\ncdef", "ab\ncdef", 5, 5, tea.KeyDown, 0},
		{"up from trailing newline", "abc\n", "abc\n", 4, 0, tea.KeyUp, 0},
		{"down to trailing newline", "abc\n", "abc\n", 2, 4, tea.KeyDown, 0},
		{"up empty", "", "", -1, 0, tea.KeyUp, 0},
		{"down empty", "", "", 99, 0, tea.KeyDown, 0},
		{"newline fallback", "abcd", "ab\ncd", 2, 3, 'j', tea.ModCtrl},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, cursor := tc.body, tc.cursor
			result := (draftEditor{&body, &cursor, commentDraft}).key(tea.KeyPressMsg{Code: tc.code, Mod: tc.mod})
			if result != draftEdited || body != tc.want || cursor != tc.wantCursor {
				t.Fatalf("got %q at %d (result %d), want %q at %d", body, cursor, result, tc.want, tc.wantCursor)
			}
		})
	}
}

func TestMacShortcutsRouteToEveryDraft(t *testing.T) {
	for _, surface := range []string{"comment", "suggestion", "reply", "published", "general", "summary"} {
		t.Run(surface, func(t *testing.T) {
			m := commitModel(t)
			body := "first\n猫 two"
			var field *string
			var cursor *int
			switch surface {
			case "comment", "suggestion":
				m.Composer = &commentComposer{Draft: body, Cursor: 11, Suggestion: surface == "suggestion"}
				field, cursor = &m.Composer.Draft, &m.Composer.Cursor
			case "reply":
				m.CommentMenu = &commentActionMenu{mode: commentActionReply, Draft: body, Cursor: 11}
				field, cursor = &m.CommentMenu.Draft, &m.CommentMenu.Cursor
			case "published":
				m.discussions.published = &publishedEditor{draft: body, cursor: 11}
				field, cursor = &m.discussions.published.draft, &m.discussions.published.cursor
			case "general":
				m.openOverviewDiscussions()
				m.discussions.editor = &generalCommentEditor{draft: body, cursor: 11}
				field, cursor = &m.discussions.editor.draft, &m.discussions.editor.cursor
			case "summary":
				m.openReviewForm()
				m.ReviewForm.Focus = 1
				m.ReviewForm.Body, m.ReviewForm.Cursor = body, 11
				field, cursor = &m.ReviewForm.Body, &m.ReviewForm.Cursor
			}
			for _, step := range []struct {
				code rune
				mod  tea.KeyMod
				want string
				pos  int
			}{
				{tea.KeyLeft, tea.ModAlt, body, 8},
				{tea.KeyRight, tea.ModAlt, body, 11},
				{tea.KeyLeft, tea.ModSuper, body, 6},
				{tea.KeyRight, tea.ModSuper, body, 11},
				{tea.KeyBackspace, tea.ModAlt, "first\n猫 ", 8},
				{tea.KeyUp, 0, "first\n猫 ", 2},
				{tea.KeyDown, 0, "first\n猫 ", 8},
				{tea.KeyEnter, tea.ModShift, "first\n猫 \n", 9},
				{'j', tea.ModCtrl, "first\n猫 \n\n", 10},
				{tea.KeyUp, 0, "first\n猫 \n\n", 9},
				{tea.KeyUp, 0, "first\n猫 \n\n", 6},
				{tea.KeyRight, tea.ModSuper, "first\n猫 \n\n", 8},
				{tea.KeyBackspace, tea.ModSuper, "first\n\n\n", 6},
			} {
				m.Update(tea.KeyPressMsg{Code: step.code, Mod: step.mod})
				if *field != step.want || *cursor != step.pos {
					t.Fatalf("%s: got %q at %d, want %q at %d", (tea.KeyPressMsg{Code: step.code, Mod: step.mod}).String(), *field, *cursor, step.want, step.pos)
				}
			}
		})
	}
}

func TestDraftEditorTerminalShortcutAliases(t *testing.T) {
	for _, pair := range [][2]tea.KeyPressMsg{
		{{Code: tea.KeyLeft, Mod: tea.ModAlt}, {Code: 'b', Mod: tea.ModAlt}},
		{{Code: tea.KeyRight, Mod: tea.ModAlt}, {Code: 'f', Mod: tea.ModAlt}},
		{{Code: tea.KeyLeft, Mod: tea.ModSuper}, {Code: 'a', Mod: tea.ModCtrl}},
		{{Code: tea.KeyRight, Mod: tea.ModSuper}, {Code: 'e', Mod: tea.ModCtrl}},
		{{Code: tea.KeyBackspace, Mod: tea.ModSuper}, {Code: 'u', Mod: tea.ModCtrl}},
	} {
		body, cursor := "first\none two", 10
		aliasBody, aliasCursor := body, cursor
		(draftEditor{&body, &cursor, commentDraft}).key(pair[0])
		result := (draftEditor{&aliasBody, &aliasCursor, commentDraft}).key(pair[1])
		if result != draftEdited || aliasBody != body || aliasCursor != cursor {
			t.Fatalf("%s: got %q at %d, want %q at %d", pair[1].String(), aliasBody, aliasCursor, body, cursor)
		}
	}
}
