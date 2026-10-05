package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/review"
	"prui/internal/source"
	"strings"
)

type GeneralCommentSubmitter func(context.Context, source.Metadata, string) (source.ConversationEvent, error)
type generalCommentEditor struct {
	attemptedBody  string
	attemptedIDs   map[string]bool
	matched        bool
	draft, replyTo string
	cursor         int
	posting        bool
	uncertain      bool
}
type GeneralCommentResult struct {
	Target  int
	Session *review.Session
	Editor  *generalCommentEditor
	Event   source.ConversationEvent
	Err     error
}

func (m *Model) SetGeneralCommentSubmitter(submit GeneralCommentSubmitter) {
	m.submitGeneralComment = submit
}
func (m *Model) generalCommentKey(key tea.KeyPressMsg) tea.Cmd {
	e := m.discussions.editor
	if e == nil {
		return nil
	}
	if e.posting {
		return nil
	}
	switch key.String() {
	case "esc":
		m.discussions.editor = nil
		return nil
	case "left":
		e.cursor = max(0, e.cursor-1)
		return nil
	case "right":
		e.cursor = min(len([]rune(e.draft)), e.cursor+1)
		return nil
	case "home":
		e.cursor = 0
		return nil
	case "end":
		e.cursor = len([]rune(e.draft))
		return nil
	case "backspace":
		r := []rune(e.draft)
		if e.cursor > 0 {
			e.draft = string(append(r[:e.cursor-1], r[e.cursor:]...))
			e.cursor--
		}
		return nil
	case "delete":
		r := []rune(e.draft)
		if e.cursor < len(r) {
			e.draft = string(append(r[:e.cursor], r[e.cursor+1:]...))
		}
		return nil
	case "shift+enter":
		m.insertGeneralCommentText("\n")
		return nil
	case "ctrl+r":
		return m.refreshDiscussions()
	case "enter":
		if e.matched {
			m.discussions.notice = "Matching attempted PR comment found; inspect it before starting another comment"
			return nil
		}
		if e.uncertain {
			m.discussions.notice = "Posting outcome unknown; refresh (ctrl+r), then intentionally retry"
			return nil
		}
		if strings.TrimSpace(e.draft) == "" || len(e.draft) > 65536 {
			m.discussions.notice = "PR comment requires 1–65536 bytes"
			return nil
		}
		if m.submitGeneralComment == nil || m.Session == nil {
			m.discussions.notice = "PR comment submission unavailable"
			return nil
		}
		observed := map[string]bool{}
		for _, event := range m.discussions.snapshot.Snapshot.Events {
			if event.Kind == "PR comment" {
				observed[event.ID] = true
			}
		}
		if len(observed) > 500 {
			m.discussions.notice = "Observed activity exceeds private draft limit; refresh before posting"
			return nil
		}
		e.attemptedBody, e.attemptedIDs = e.draft, observed
		e.posting = true
		if !m.persistDraft(m.reviewTabState) {
			e.posting = false
			e.uncertain = true
			m.discussions.notice = "Private draft save failed; submission blocked"
			return nil
		}
		// A read begun before this durable attempt cannot prove its absence.
		if m.discussions.cancel != nil {
			m.discussions.cancel()
		}
		m.discussions.generation++
		target, s, submit, body, ctx := m.activeTab, m.Session, m.submitGeneralComment, e.draft, m.ctx
		m.discussions.notice = "Posting general PR comment..."
		return func() tea.Msg {
			event, err := submit(ctx, s.Inventory.Comparison.Metadata, body)
			return GeneralCommentResult{Target: target, Session: s, Editor: e, Event: event, Err: err}
		}
	}
	if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
		m.insertGeneralCommentText(key.Text)
	}
	return nil
}

// Reject over-limit edits before mutating a payload that must remain savable.
func (m *Model) insertGeneralCommentText(text string) {
	e := m.discussions.editor
	if len(e.draft)+len(text) > 65536 {
		m.discussions.notice = "PR comment requires 1–65536 bytes"
		return
	}
	e.draft, e.cursor = insertEditorText(e.draft, e.cursor, text)
}

func (m *Model) applyGeneralCommentResult(v GeneralCommentResult) {
	state := m.reviewStateForTarget(v.Target)
	if state == nil || state.Session != v.Session || state.discussions.editor != v.Editor {
		return
	}
	d := &state.discussions
	v.Editor.posting = false
	if v.Err != nil {
		d.notice = Escape(v.Err.Error())
		v.Editor.uncertain = errors.Is(v.Err, source.ErrCommentDeliveryUnknown)
		return
	}
	found := false
	for i, event := range d.snapshot.Snapshot.Events {
		if event.ID == v.Event.ID {
			d.snapshot.Snapshot.Events[i] = v.Event
			found = true
			break
		}
	}
	if !found {
		d.snapshot.Snapshot.Events = append(d.snapshot.Snapshot.Events, v.Event)
	}
	if d.confirmedEvents == nil {
		d.confirmedEvents = map[string]source.ConversationEvent{}
	}
	d.confirmedEvents[v.Event.ID] = v.Event
	d.snapshot.Snapshot.Timeline = true
	d.loaded = true
	d.selectedID = v.Event.ID
	for i, entry := range discussionEntries(d.snapshot.Snapshot) {
		if entry.ID == v.Event.ID {
			d.selected = i
		}
	}
	d.detail = true
	d.scroll = 0
	d.editor = nil
	d.notice = "General PR comment posted"
	// Invalidate any concurrent read that began before this confirmed creation.
	d.generation++
}
func (m *Model) generalCommentView() string {
	e := m.discussions.editor
	title := "New general PR comment · posts immediately"
	if e.replyTo != "" {
		title = "General PR reply · new @mention PR comment"
	}
	draft := []rune(e.draft)
	cursor := max(0, min(e.cursor, len(draft)))
	text := Escape(string(draft[:cursor])) + "█" + Escape(string(draft[cursor:]))
	body := strings.Split(ansi.Wrap(text, max(1, m.Width), ""), "\n")
	prefix := strings.Split(ansi.Wrap(Escape(string(draft[:cursor]))+"█", max(1, m.Width), ""), "\n")
	height := max(1, m.Height-4)
	start := max(0, len(prefix)-height)
	lines := []string{clip(title, m.Width), clip(m.discussions.notice, m.Width)}
	lines = append(lines, body[start:min(len(body), start+height)]...)
	lines = append(lines, clip("Enter: post · shift+enter: newline · ctrl+r: refresh · esc: discard", m.Width))
	return strings.Join(lines, "\n")
}
