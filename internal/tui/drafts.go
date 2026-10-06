package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"prui/internal/session"
	"prui/internal/source"
)

type draftState struct {
	key            session.DraftKey
	loaded         bool
	generation     uint64
	saved          []byte
	attempt        string
	attempted      *session.DraftAttempt
	err            error
	recoveryScroll int
}

func draftContent(state *reviewTabState) session.Draft {
	d := session.Draft{Version: 2, SuggestionApply: state.SuggestionApply, Pending: state.Pending, Attempt: state.draft.attempt, Attempted: state.draft.attempted}
	if f := state.ReviewForm; f != nil {
		d.Summary, d.Event = f.Body, f.Event
	}
	if c := state.Composer; c != nil {
		d.Composer = &session.DraftEditor{Target: c.Target, Body: c.Draft, PendingIndex: c.PendingIndex, CommitSHA: c.CommitSHA, Suggestion: c.Suggestion, Before: c.Before, Modes: c.Modes}
	}
	if r := state.CommentMenu; r != nil && r.mode == commentActionReply {
		d.Reply = &session.DraftEditor{Target: r.Target, Body: r.Draft, CommentID: r.CommentID, ReplyToID: r.ReplyToID, RootAnchor: r.RootAnchor}
	}
	if d.Attempt == "suggestion" && d.Attempted != nil && d.Attempted.Application != nil {
		d.SuggestionApply = nil
	}
	if d.SuggestionApply != nil || d.Composer != nil && d.Composer.Suggestion || d.Attempt == "suggestion" {
		d.Version = 3
	}
	if e := state.discussions.editor; e != nil {
		ids := make([]string, 0, len(e.attemptedIDs))
		for id := range e.attemptedIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		d.General = &session.GeneralDraft{Body: e.draft, ReplyTo: e.replyTo, Cursor: e.cursor, AttemptedBody: e.attemptedBody, ObservedIDs: ids, Uncertain: e.uncertain || e.posting, Matched: e.matched}
		d.Version = 4
	}
	return d
}
func (m *Model) loadDraft(state *reviewTabState) {
	if m.store == nil || state == nil || state.Session == nil || state.Session.ID == "" {
		return
	}
	key := session.DraftKeyFor(state.Session.Inventory.Comparison.Metadata)
	if state.draft.loaded && state.draft.key == key {
		return
	}
	state.draft = draftState{key: key, loaded: true}
	d, err := m.store.LoadDraft(context.Background(), key)
	if err != nil {
		state.draft.err = err
		state.ActionError = errors.New("private draft recovery failed; stored work retained")
		return
	}
	if d.Generation == 0 {
		empty, _ := json.Marshal(session.Draft{Version: 2})
		state.draft.saved = empty
		return
	}
	state.draft.generation = d.Generation
	state.draft.attempt = d.Attempt
	state.draft.attempted = d.Attempted
	state.Pending = d.Pending
	state.SuggestionApply = d.SuggestionApply
	if state.SuggestionApply == nil && d.Attempted != nil && d.Attempted.Application != nil {
		copy := *d.Attempted.Application
		state.SuggestionApply = &copy
	}
	state.ReviewForm = nil
	state.Composer = nil
	state.CommentMenu = nil
	if d.Summary != "" || d.Event != 0 {
		state.ReviewForm = &reviewForm{Body: d.Summary, Cursor: len([]rune(d.Summary)), Event: d.Event}
	}
	if c := d.Composer; c != nil {
		state.Composer = &commentComposer{Target: c.Target, Draft: c.Body, Cursor: len([]rune(c.Body)), PendingIndex: c.PendingIndex, CommitSHA: c.CommitSHA, Suggestion: c.Suggestion, Before: c.Before, Modes: c.Modes}
	}
	if c := state.Composer; c != nil && c.CommitSHA != "" {
		c.CommitBundle = state.Session.Commits
		c.CommitInventory = &state.Session.Inventory
		state.ContextView = viewCommits
		state.commit.selectedSHA = c.CommitSHA
		state.commit.focus = paneDiff
	}
	if r := d.Reply; r != nil {
		state.CommentMenu = &commentActionMenu{Target: r.Target, Draft: r.Body, Cursor: len([]rune(r.Body)), CommentID: r.CommentID, ReplyToID: r.ReplyToID, mode: commentActionReply, RootAnchor: r.RootAnchor}
	}
	if g := d.General; g != nil {
		ids := map[string]bool{}
		for _, id := range g.ObservedIDs {
			ids[id] = true
		}
		state.discussions.editor = &generalCommentEditor{draft: g.Body, replyTo: g.ReplyTo, cursor: g.Cursor, attemptedBody: g.AttemptedBody, attemptedIDs: ids, uncertain: g.Uncertain, matched: g.Matched}
	}
	content, _ := json.Marshal(draftContent(state))
	state.draft.saved = content
	if d.General != nil || d.SuggestionApply != nil || d.Composer != nil || d.Reply != nil || len(d.Pending) > 0 || d.Summary != "" || d.Event != 0 || d.Attempt != "" {
		state.Stack = append(state.Stack, pageDraftRecovery)
	}
	if d.Attempt != "" {
		state.ActionError = errors.New("delivery uncertain: ctrl+r to check GitHub before retrying")
	}
}
func (m *Model) persistDraft(state *reviewTabState) bool {
	if m.store == nil || state == nil || state.Session == nil || state.Session.ID == "" {
		return true
	}
	m.loadDraft(state)
	if state.draft.err != nil {
		state.ActionError = errors.New("private draft storage failed; reload before editing or submitting")
		return false
	}
	d := draftContent(state)
	content, _ := json.Marshal(d)
	if bytes.Equal(content, state.draft.saved) {
		return true
	}
	saved, err := m.store.SaveDraft(context.Background(), state.draft.key, state.draft.generation, d)
	if err != nil {
		state.draft.err = err
		state.ActionError = errors.New("private draft save failed; work remains in memory; submission blocked")
		return false
	}
	state.draft.generation = saved.Generation
	state.draft.saved = content
	return true
}
func (m *Model) persistDrafts() {
	m.persistDraft(m.reviewTabState)
	for i, tab := range m.tabs {
		if i != m.activeTab {
			m.persistDraft(tab.review)
		}
	}
}
func (m *Model) prepareDraftAttempt(kind string) bool {
	if m.store == nil || m.Session == nil || m.Session.ID == "" {
		return true
	}
	m.loadDraft(m.reviewTabState)
	if m.draft.err != nil {
		return m.persistDraft(m.reviewTabState)
	}
	if m.draft.attempt != "" {
		m.ActionError = errors.New("delivery uncertain: ctrl+r to check GitHub before retrying")
		return false
	}
	m.draft.attempt = kind
	m.draft.attempted = &session.DraftAttempt{Kind: kind}
	switch kind {
	case "suggestion":
		copy := *m.SuggestionApply
		m.draft.attempted.Application = &copy
	case "comment":
		c := m.Composer
		m.draft.attempted.Comment = &source.ReviewComment{Target: c.Target, Body: composerBody(c)}
	case "reply":
		r := m.CommentMenu
		if !m.resolveReplyRoot(r) {
			m.draft.attempt = ""
			m.draft.attempted = nil
			m.ActionError = errors.New("refresh discussions to resolve the reply root before submitting")
			return false
		}
		action := m.commentActionRequest(r)
		raw := action.Comment.Target
		if action.Comment.CurrentAnchor != nil {
			raw = *action.Comment.CurrentAnchor
		}
		m.draft.attempted.Comment = &source.ReviewComment{Target: raw, Body: action.Body}
		m.draft.attempted.ParentID = action.Comment.ID
	case "review":
		meta := m.Session.Inventory.Comparison.Metadata
		f := m.ReviewForm
		m.draft.attempted.Review = &source.PullRequestReview{Identity: meta.Identity, CommitID: meta.HeadSHA, Event: reviewEvents[f.Event].event, Body: f.Body, Comments: append([]source.ReviewComment(nil), m.Pending...)}
	}
	return m.persistDraft(m.reviewTabState)
}

// Reconciliation never writes remotely and never automatically retries. A
// positive remote match asks the reviewer to discard; an authoritative absence
// permits a later explicit submit with the usual freshness preflight.
type DraftReconciler func(context.Context, source.Metadata, session.Draft) (bool, error)
type draftReconciled struct {
	target     int
	key        session.DraftKey
	generation uint64
	delivered  bool
	err        error
}

func (m *Model) SetDraftReconciler(read DraftReconciler) { m.reconcileDraft = read }
func (m *Model) reconcileDraftCommand() tea.Cmd {
	if m.draft.attempt == "" {
		return nil
	}
	if m.reconcileDraft == nil || m.Session == nil {
		m.ActionError = errors.New("offline or unavailable: retain draft and check GitHub before retrying")
		return nil
	}
	if !m.persistDraft(m.reviewTabState) {
		return nil
	}
	target, key, generation := m.activeTab, m.draft.key, m.draft.generation
	meta, d, read := m.Session.Inventory.Comparison.Metadata, draftContent(m.reviewTabState), m.reconcileDraft
	ctx := m.beginAction()
	return m.start(func() tea.Msg {
		found, err := read(ctx, meta, d)
		return draftReconciled{target, key, generation, found, err}
	})
}
func (m *Model) applyDraftReconciled(v draftReconciled) {
	state := m.reviewStateForTarget(v.target)
	if state == nil || state.draft.key != v.key || state.draft.generation != v.generation {
		return
	}
	state.Busy = false
	if v.err != nil {
		state.ActionError = errors.New("GitHub outcome check incomplete; draft retained and retry blocked")
		return
	}
	if v.delivered {
		state.ActionError = errors.New("matching submission found on GitHub; discard this draft explicitly, do not retry")
		return
	}
	state.draft.attempt = ""
	state.draft.attempted = nil
	state.ActionError = nil
	state.notice = "No matching submission found; explicit retry available"
}

func (m *Model) newDraftReviewTab(s *session.Record) *reviewTabState {
	state := newReviewTabState(s)
	m.applyLayoutPreferences(state)
	m.loadDraft(state)
	return state
}
func (m *Model) saveDraftsForQuit() bool {
	if !m.persistDraft(m.reviewTabState) {
		return false
	}
	for _, tab := range m.tabs {
		if !m.persistDraft(tab.review) {
			return false
		}
	}
	return true
}
func (m *Model) discardDraftsForQuit() bool {
	states := []*reviewTabState{m.reviewTabState}
	for i, tab := range m.tabs {
		if i != m.activeTab {
			states = append(states, tab.review)
		}
	}
	for _, state := range states {
		if state == nil {
			continue
		}
		state.Composer = nil
		state.discussions.editor = nil
		state.SuggestionApply = nil
		state.CommentMenu = nil
		state.ReviewForm = nil
		state.Pending = nil
		state.draft.attempt = ""
		state.draft.attempted = nil
		if !m.persistDraft(state) {
			return false
		}
	}
	return true
}

func (m *Model) draftRecoveryView() string {
	meta := m.Session.Inventory.Comparison.Metadata
	heading := fmt.Sprintf("Recovered private drafts · %s#%d · head %.12s", Escape(meta.Identity.Repository), meta.Identity.Number, meta.HeadSHA)
	lines := []string{"Drafts remain anchored to this frozen comparison."}
	if e := m.discussions.editor; e != nil {
		lines = append(lines, "General PR comment: "+Escape(e.draft))
		if e.uncertain || e.matched {
			lines = append(lines, "Delivery uncertain or matched; ctrl+r to inspect before retrying")
		}
	}
	if m.SuggestionApply != nil {
		lines = append(lines, "Retained suggestion application · "+commentTargetLabel(m.SuggestionApply.Target), Escape(m.SuggestionApply.Replacement))
	}
	if m.Composer != nil {
		lines = append(lines, "Inline comment · "+commentTargetLabel(m.Composer.Target)+": "+Escape(m.Composer.Draft))
	}
	if m.CommentMenu != nil {
		lines = append(lines, fmt.Sprintf("Reply to comment %d: %s", m.CommentMenu.ReplyToID, Escape(m.CommentMenu.Draft)))
	}
	if m.ReviewForm != nil {
		lines = append(lines, "Review summary: "+Escape(m.ReviewForm.Body))
	}
	for _, c := range m.Pending {
		lines = append(lines, fmt.Sprintf("Pending %s: %s", commentTargetLabel(c.Target), Escape(c.Body)))
	}
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	available := max(1, m.Height-5)
	if m.ActionError != nil {
		available = max(1, available-1)
	}
	start := max(0, min(m.draft.recoveryScroll, len(lines)-available))
	visible := append([]string{heading}, lines[start:min(len(lines), start+available)]...)
	if m.ActionError != nil {
		visible = append(visible, "! "+Escape(m.ActionError.Error()))
	}
	visible = append(visible, "j/k: scroll · enter/esc: continue · ctrl+d: discard · ctrl+r: check delivery")
	return strings.Join(visible, "\n")
}
func (m *Model) draftRecoveryKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "j", "down":
		m.draft.recoveryScroll++
	case "k", "up":
		m.draft.recoveryScroll = max(0, m.draft.recoveryScroll-1)
	case "home":
		m.draft.recoveryScroll = 0
	case "enter", "esc":
		if m.SuggestionApply != nil {
			m.pop()
			m.push(pageSuggestionApply)
			return nil
		}
		if c := m.Composer; c != nil {
			if c.CommitSHA != "" {
				m.ContextView = viewCommits
				m.ensureCommitEditorVisible()
			} else if !m.focusPendingTarget(c.Target) {
				m.ActionError = errors.New("recovered target unavailable; draft retained here")
				return nil
			}
		}
		if r := m.CommentMenu; r != nil && !m.focusPendingTarget(r.Target) {
			m.ActionError = errors.New("recovered reply target unavailable; draft retained here")
			return nil
		}
		m.pop()
		if m.discussions.editor != nil {
			m.openOverviewDiscussions()
		}
	case "ctrl+r":
		if m.discussions.editor != nil && m.draft.attempt == "" {
			return m.refreshDiscussions()
		}
		return m.reconcileDraftCommand()
	case "ctrl+d":
		m.discussions.editor = nil
		m.SuggestionApply = nil
		m.Composer = nil
		m.CommentMenu = nil
		m.ReviewForm = nil
		m.Pending = nil
		m.draft.attempt = ""
		m.draft.attempted = nil
		m.ActionError = nil
		m.pop()
	}
	return nil
}

// A recovered reply remains editable at its frozen line even offline or before
// its remote thread has loaded. It never invents a remote comment overlay.
func (m *Model) recoveredReplyLines(target source.ReviewCommentTarget) []diffLine {
	r := m.CommentMenu
	if r == nil || r.mode != commentActionReply || r.Target != target {
		return nil
	}
	for _, c := range m.Comments {
		if c.ID == r.CommentID {
			return nil
		}
	}
	lines := []diffLine{{styledLine: styledLine{Class: classWarning, Text: fmt.Sprintf("  Recovered local reply to comment %d", r.ReplyToID)}}}
	return append(lines, m.inlineReplyEditorLines(0)...)
}
