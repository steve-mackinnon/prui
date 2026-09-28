package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEditorInsertionPreservesSuffixAndMovesRuneCursor(t *testing.T) {
	cases := []struct {
		name, initial, insert, want string
		cursor, wantCursor          int
		newline                     bool
	}{
		{name: "ASCII beginning", initial: "abcd", insert: "X", cursor: 0, want: "Xabcd", wantCursor: 1},
		{name: "ASCII middle", initial: "abcd", insert: "X", cursor: 1, want: "aXbcd", wantCursor: 2},
		{name: "ASCII end", initial: "abcd", insert: "X", cursor: 4, want: "abcdX", wantCursor: 5},
		{name: "Unicode middle", initial: "猫犬", insert: "🙂", cursor: 1, want: "猫🙂犬", wantCursor: 2},
		{name: "multiline text", initial: "ab\ncd", insert: "界\n🌿", cursor: 3, want: "ab\n界\n🌿cd", wantCursor: 6},
		{name: "newline middle", initial: "abcd", cursor: 2, want: "ab\ncd", wantCursor: 3, newline: true},
	}
	editors := []struct {
		name  string
		apply func(*Model, string, int, tea.KeyPressMsg) (string, int)
	}{
		{name: "inline comment", apply: func(m *Model, initial string, cursor int, key tea.KeyPressMsg) (string, int) {
			m.Composer = &commentComposer{Draft: initial, Cursor: cursor}
			m.commentComposerKey(key)
			return m.Composer.Draft, m.Composer.Cursor
		}},
		{name: "reply", apply: func(m *Model, initial string, cursor int, key tea.KeyPressMsg) (string, int) {
			m.CommentMenu = &commentActionMenu{mode: commentActionReply, Draft: initial, Cursor: cursor}
			m.commentActionKey(key)
			return m.CommentMenu.Draft, m.CommentMenu.Cursor
		}},
		{name: "review summary", apply: func(m *Model, initial string, cursor int, key tea.KeyPressMsg) (string, int) {
			m.ReviewForm = &reviewForm{Focus: 1, Body: initial, Cursor: cursor}
			m.reviewFormKey(key)
			return m.ReviewForm.Body, m.ReviewForm.Cursor
		}},
	}
	for _, editor := range editors {
		for _, tc := range cases {
			t.Run(editor.name+"/"+tc.name, func(t *testing.T) {
				m := New(context.Background(), nil)
				m.Loading = false
				m.Session = kindsSession()
				key := tea.KeyPressMsg{Code: 'X', Text: tc.insert}
				if tc.newline {
					key = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
				}
				got, cursor := editor.apply(m, tc.initial, tc.cursor, key)
				if got != tc.want || cursor != tc.wantCursor {
					t.Fatalf("after inserting %q at rune %d: text=%q cursor=%d, want %q and %d", tc.insert, tc.cursor, got, cursor, tc.want, tc.wantCursor)
				}
			})
		}
	}
}
