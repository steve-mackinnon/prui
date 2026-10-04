package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/review"
	"prui/internal/source"
)

// PublishedAction selects one identity-based mutation. Resolve is nil for edits.
type PublishedAction struct {
	Metadata  source.Metadata
	ThreadID  string
	CommentID int64
	General   bool
	Body      string
	Resolve   *bool
}
type PublishedValue struct {
	Comment source.PublishedComment
	Thread  source.Discussion
}
type PublishedSubmitter func(context.Context, PublishedAction) (PublishedValue, error)
type publishedEditor struct {
	action                      PublishedAction
	draft                       string
	cursor                      int
	posting, uncertain, matched bool
	attempted                   PublishedAction // immutable while an outcome is unresolved
	notice                      string
}
type PublishedResult struct {
	Target  int
	Session *review.Session
	Editor  *publishedEditor
	Value   PublishedValue
	Err     error
}

func (m *Model) SetPublishedSubmitter(f PublishedSubmitter) { m.submitPublished = f }
func (m *Model) openPublished(commentID int64, eventID string, resolve bool) bool {
	if m.submitPublished == nil || m.Session == nil {
		return false
	}
	a := PublishedAction{Metadata: m.Session.Inventory.Comparison.Metadata, CommentID: commentID}
	author, body := "", ""
	var thread *source.Discussion
	for i := range m.discussions.snapshot.Snapshot.Threads {
		t := &m.discussions.snapshot.Snapshot.Threads[i]
		for _, c := range t.Comments {
			if c.ID == commentID {
				thread = t
				author, body = c.Author, c.Body
				break
			}
		}
	}
	if eventID != "" {
		for _, e := range m.discussions.snapshot.Snapshot.Events {
			if e.ID == eventID && e.Kind == "PR comment" && !e.Retained {
				cid, err := strconv.ParseInt(strings.TrimPrefix(e.ID, "PR comment:"), 10, 64)
				if err != nil || cid <= 0 {
					return false
				}
				a.CommentID, a.General = cid, true
				author, body = e.Author, e.Body
			}
		}
	}
	if resolve {
		if thread == nil || thread.Retained || thread.Resolved == nil {
			return false
		}
		desired := !*thread.Resolved
		allowed := thread.CanResolve
		if !desired {
			allowed = thread.CanUnresolve
		}
		if allowed == nil || !*allowed {
			return false
		}
		a.ThreadID, a.Resolve, a.CommentID = thread.ID, &desired, 0
	} else if author == "" || m.Viewer == "" || author != m.Viewer {
		return false
	}
	m.discussions.published = &publishedEditor{action: a, draft: body, cursor: len([]rune(body))}
	return true
}
func (m *Model) publishedKey(k tea.KeyPressMsg) tea.Cmd {
	e := m.discussions.published
	if e == nil || e.posting {
		return nil
	}
	switch k.String() {
	case "esc":
		m.discussions.published = nil
		return nil
	case "ctrl+r":
		return m.refreshDiscussions()
	case "enter":
		if e.matched {
			e.notice = "Attempt observed; close and inspect the canonical result"
			return nil
		}
		if e.uncertain {
			e.notice = "Outcome unknown; ctrl+r refresh before intentional retry"
			return nil
		}
		if e.action.Resolve == nil {
			if err := source.ValidateGeneralComment(e.draft); err != nil {
				e.notice = "Edit requires 1–65536 bytes"
				return nil
			}
			e.action.Body = e.draft
		}
		e.attempted = e.action
		m.discussions.generation++ // invalidate reads begun before this attempt
		if m.discussions.cancel != nil {
			m.discussions.cancel()
		}
		e.posting = true
		e.notice = "Submitting published action..."
		target, s, submit, a, ctx := m.activeTab, m.Session, m.submitPublished, e.attempted, m.ctx
		return func() tea.Msg { value, err := submit(ctx, a); return PublishedResult{target, s, e, value, err} }
	}
	if e.action.Resolve != nil {
		return nil
	}
	switch k.String() {
	case "left":
		e.cursor = max(0, e.cursor-1)
	case "right":
		e.cursor = min(len([]rune(e.draft)), e.cursor+1)
	case "home":
		e.cursor = 0
	case "end":
		e.cursor = len([]rune(e.draft))
	case "shift+enter":
		e.draft, e.cursor = insertEditorText(e.draft, e.cursor, "\n")
	case "backspace":
		r := []rune(e.draft)
		if e.cursor > 0 {
			e.draft = string(append(r[:e.cursor-1], r[e.cursor:]...))
			e.cursor--
		}
	case "delete":
		r := []rune(e.draft)
		if e.cursor < len(r) {
			e.draft = string(append(r[:e.cursor], r[e.cursor+1:]...))
		}
	default:
		if k.Text != "" && !k.Mod.Contains(tea.ModCtrl) && !k.Mod.Contains(tea.ModAlt) {
			e.draft, e.cursor = insertEditorText(e.draft, e.cursor, k.Text)
		}
	}
	return nil
}
func (m *Model) applyPublishedResult(v PublishedResult) {
	state := m.reviewStateForTarget(v.Target)
	if state == nil || state.Session != v.Session || state.discussions.published != v.Editor {
		return
	}
	e := v.Editor
	e.posting = false
	if v.Err != nil {
		e.notice = Escape(v.Err.Error())
		e.uncertain = errors.Is(v.Err, source.ErrCommentDeliveryUnknown)
		return
	}
	applyPublishedValue(state, e.attempted, v.Value)
	state.discussions.published = nil
	state.discussions.notice = "Published action confirmed"
}
func applyPublishedValue(state *reviewTabState, a PublishedAction, v PublishedValue) {
	d := &state.discussions
	if d.confirmedPublished == nil {
		d.confirmedPublished = map[string]confirmedPublished{}
	}
	d.confirmedPublished[publishedKeyID(a)] = confirmedPublished{a, v}
	switch {
	case a.Resolve != nil:
		for i := range d.snapshot.Snapshot.Threads {
			t := &d.snapshot.Snapshot.Threads[i]
			if t.ID == a.ThreadID {
				t.Resolved = v.Thread.Resolved
				t.CanResolve = v.Thread.CanResolve
				t.CanUnresolve = v.Thread.CanUnresolve
			}
		}
	case a.General:
		id := fmt.Sprintf("PR comment:%d", a.CommentID)
		for i := range d.snapshot.Snapshot.Events {
			e := &d.snapshot.Snapshot.Events[i]
			if e.ID == id {
				e.Body = v.Comment.Body
				e.Author = v.Comment.Author
			}
		}
	default:
		for i := range d.snapshot.Snapshot.Threads {
			for j := range d.snapshot.Snapshot.Threads[i].Comments {
				c := &d.snapshot.Snapshot.Threads[i].Comments[j]
				if c.ID == a.CommentID {
					c.Body = v.Comment.Body
					c.Author = v.Comment.Author
				}
			}
		}
		for i := range state.Comments {
			if state.Comments[i].ID == a.CommentID {
				state.Comments[i].Body = v.Comment.Body
				state.Comments[i].Author = v.Comment.Author
			}
		}
	}
	d.generation++
	d.anchorIndex = nil
	state.commit.cache = commitRenderCache{}
}
func (m *Model) publishedView() string {
	e := m.discussions.published
	title := "Edit published comment · preserves identity"
	if e.action.Resolve != nil {
		title = "Resolve review thread?"
		if !*e.action.Resolve {
			title = "Reopen review thread?"
		}
	}
	lines := []string{title, e.notice}
	if e.action.Resolve == nil {
		r := []rune(e.draft)
		c := min(e.cursor, len(r))
		text := Escape(string(r[:c])) + "█" + Escape(string(r[c:]))
		body := strings.Split(ansi.Wrap(text, max(1, m.Width), ""), "\n")
		prefix := strings.Split(ansi.Wrap(Escape(string(r[:c]))+"█", max(1, m.Width), ""), "\n")
		h := max(1, m.Height-4)
		start := max(0, len(prefix)-h)
		lines = append(lines, body[start:min(len(body), start+h)]...)
	}
	lines = append(lines, "Enter: confirm · shift+enter: newline · ctrl+r: refresh · esc: discard")
	return strings.Join(lines, "\n")
}

// Only a complete freshly verified read can establish a retry decision. Matching
// uses the immutable attempted payload, never the editor's subsequently changed text.
func reconcilePublished(d *discussionState, in DiscussionSnapshot) {
	e := d.published
	if e == nil || !e.uncertain || !in.CurrentVerified || !in.Snapshot.Complete {
		return
	}
	a := e.attempted
	found, matched := false, false
	switch {
	case a.Resolve != nil:
		for _, t := range in.Snapshot.Threads {
			if t.ID == a.ThreadID && t.Resolved != nil {
				found = true
				matched = *t.Resolved == *a.Resolve
			}
		}
	case a.General:
		for _, v := range in.Snapshot.Events {
			if v.ID == fmt.Sprintf("PR comment:%d", a.CommentID) && !v.Retained {
				found = true
				matched = v.Body == a.Body
			}
		}
	default:
		for _, t := range in.Snapshot.Threads {
			for _, c := range t.Comments {
				if c.ID == a.CommentID && !t.Retained {
					found = true
					matched = c.Body == a.Body
				}
			}
		}
	}
	if !found {
		e.notice = "Attempted identity unavailable; keep edit and refresh again"
		return
	}
	e.uncertain = false
	e.matched = matched
	if matched {
		e.notice = "Attempt observed; close and inspect canonical result"
	} else {
		e.notice = "Attempt not observed; edit retained for intentional retry"
	}
}

type confirmedPublished struct {
	Action PublishedAction
	Value  PublishedValue
}

func publishedKeyID(a PublishedAction) string {
	if a.Resolve != nil {
		return "thread:" + a.ThreadID
	}
	return fmt.Sprintf("comment:%t:%d", a.General, a.CommentID)
}

// Mutation generations reject reads begun before success. Later verified reads
// are authoritative, including a subsequent external edit/reopen or deletion.
// A partial read cannot establish absence of a confirmed identity.
func retainPublished(d *discussionState, in *DiscussionSnapshot) {
	for key, p := range d.confirmedPublished {
		a, v := p.Action, p.Value
		found := false
		switch {
		case a.Resolve != nil:
			for i := range in.Snapshot.Threads {
				t := &in.Snapshot.Threads[i]
				if t.ID == a.ThreadID {
					found = true
					if !in.CurrentVerified {
						t.Resolved = v.Thread.Resolved
						t.CanResolve = v.Thread.CanResolve
						t.CanUnresolve = v.Thread.CanUnresolve
					}
				}
			}
			if !found && (!in.CurrentVerified || !in.Snapshot.Complete) {
				for _, t := range d.snapshot.Snapshot.Threads {
					if t.ID == a.ThreadID {
						t.CurrentAnchor = nil
						t.Retained = true
						in.Snapshot.Threads = append(in.Snapshot.Threads, t)
					}
				}
			}
		case a.General:
			id := fmt.Sprintf("PR comment:%d", a.CommentID)
			for i := range in.Snapshot.Events {
				e := &in.Snapshot.Events[i]
				if e.ID == id && !e.Retained {
					found = true
					if !in.CurrentVerified {
						e.Body = v.Comment.Body
						e.Author = v.Comment.Author
					}
				}
			}
			if !found && (!in.CurrentVerified || !in.Snapshot.Complete) {
				for _, e := range d.snapshot.Snapshot.Events {
					if e.ID == id {
						e.Retained = true
						in.Snapshot.Events = append(in.Snapshot.Events, e)
					}
				}
			}
		default:
			for i := range in.Snapshot.Threads {
				for j := range in.Snapshot.Threads[i].Comments {
					c := &in.Snapshot.Threads[i].Comments[j]
					if c.ID == a.CommentID && !in.Snapshot.Threads[i].Retained {
						found = true
						if !in.CurrentVerified {
							c.Body = v.Comment.Body
							c.Author = v.Comment.Author
						}
					}
				}
			}
			if !found && (!in.CurrentVerified || !in.Snapshot.Complete) {
				for _, t := range d.snapshot.Snapshot.Threads {
					for _, c := range t.Comments {
						if c.ID == a.CommentID {
							t.CurrentAnchor = nil
							t.Retained = true
							in.Snapshot.Threads = append(in.Snapshot.Threads, t)
							break
						}
					}
				}
			}
		}
		if in.CurrentVerified && (found || in.Snapshot.Complete) {
			delete(d.confirmedPublished, key)
		} else {
			in.Snapshot.Complete = false
			in.Snapshot.Reason = "Confirmed published action identity unavailable or freshness unknown"
		}
	}
}

func (m *Model) publishedStatus(cid int64) string {
	for _, t := range m.discussions.snapshot.Snapshot.Threads {
		for _, c := range t.Comments {
			if c.ID == cid && t.Resolved != nil {
				if *t.Resolved {
					return "Resolved"
				}
				return "Unresolved"
			}
		}
	}
	return ""
}
