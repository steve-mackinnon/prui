package tui

import (
	tea "charm.land/bubbletea/v2"
	"unicode"
	"unicode/utf8"
)

// A policy describes editable text, not submission validity. Empty drafts and
// literal multiline text are allowed; workflow owners validate before posting.
type draftPolicy int

const (
	commentDraft draftPolicy = iota
	suggestionDraft
	replyDraft
	publishedDraft
	generalDraft
	summaryDraft
)

func (p draftPolicy) byteLimit() int {
	switch p {
	case summaryDraft:
		// Review summaries have no individual body cap in source or draft storage.
		// The existing aggregate private-payload limit still applies on persistence.
		return 0
	default:
		// Suggestions limit replacement source, independently of generated fences.
		return 64 << 10
	}
}

// draftEditor owns rune offsets and text mutation without owning focus, target,
// persistence, or delivery state. Both keyboard and clipboard use this seam.
type draftEditor struct {
	body   *string
	cursor *int
	policy draftPolicy
}

func (e draftEditor) clamp() []rune {
	runes := []rune(*e.body)
	*e.cursor = min(max(*e.cursor, 0), len(runes))
	return runes
}

func (e draftEditor) insert(text string) bool {
	*e.cursor = min(max(*e.cursor, 0), utf8.RuneCountInString(*e.body))
	if !utf8.ValidString(text) || e.policy.byteLimit() > 0 && len(*e.body)+len(text) > e.policy.byteLimit() {
		return false
	}
	*e.body, *e.cursor = insertEditorText(*e.body, *e.cursor, text)
	return true
}

type draftEditResult int

const (
	draftUnhandled draftEditResult = iota
	draftEdited
	draftRejected
)

// key translates only editing keys. Enter, Escape, and workflow shortcuts remain
// with the caller; pasted text never passes through key interpretation.
func (e draftEditor) key(key tea.KeyPressMsg) draftEditResult {
	switch key.String() {
	case "left", "right", "home", "end", "backspace", "delete",
		"alt+left", "alt+right", "alt+b", "alt+f",
		"super+left", "super+right", "super+backspace", "ctrl+a", "ctrl+e", "ctrl+u":
		runes := e.clamp()
		switch key.String() {
		case "alt+left", "alt+b":
			for *e.cursor > 0 && !editorWordRune(runes[*e.cursor-1]) {
				*e.cursor--
			}
			for *e.cursor > 0 && editorWordRune(runes[*e.cursor-1]) {
				*e.cursor--
			}
		case "alt+right", "alt+f":
			for *e.cursor < len(runes) && !editorWordRune(runes[*e.cursor]) {
				*e.cursor++
			}
			for *e.cursor < len(runes) && editorWordRune(runes[*e.cursor]) {
				*e.cursor++
			}
		case "super+left", "ctrl+a", "super+backspace", "ctrl+u":
			start := *e.cursor
			for start > 0 && runes[start-1] != '\n' {
				start--
			}
			if key.String() == "super+backspace" || key.String() == "ctrl+u" {
				*e.body = string(append(runes[:start], runes[*e.cursor:]...))
			}
			*e.cursor = start
		case "super+right", "ctrl+e":
			for *e.cursor < len(runes) && runes[*e.cursor] != '\n' {
				*e.cursor++
			}
		case "left":
			*e.cursor = max(0, *e.cursor-1)
		case "right":
			*e.cursor = min(len(runes), *e.cursor+1)
		case "home":
			*e.cursor = 0
		case "end":
			*e.cursor = len(runes)
		case "backspace":
			if *e.cursor > 0 {
				*e.body = string(append(runes[:*e.cursor-1], runes[*e.cursor:]...))
				*e.cursor--
			}
		case "delete":
			if *e.cursor < len(runes) {
				*e.body = string(append(runes[:*e.cursor], runes[*e.cursor+1:]...))
			}
		}
	case "shift+enter":
		if !e.insert("\n") {
			return draftRejected
		}
	default:
		if key.Text == "" || key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModAlt) || key.Mod.Contains(tea.ModSuper) || key.Mod.Contains(tea.ModMeta) {
			return draftUnhandled
		}
		if !e.insert(key.Text) {
			return draftRejected
		}
	}
	return draftEdited
}

func composerDraftPolicy(c *commentComposer) draftPolicy {
	if c.Suggestion {
		return suggestionDraft
	}
	return commentDraft
}

// Include combining marks so accented words keep their rune boundaries intact.
func editorWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_'
}
