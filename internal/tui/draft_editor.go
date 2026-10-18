package tui

import (
	tea "charm.land/bubbletea/v2"
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
	case "left", "right", "home", "end", "backspace", "delete":
		runes := e.clamp()
		switch key.String() {
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
		if key.Text == "" || key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModAlt) {
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
