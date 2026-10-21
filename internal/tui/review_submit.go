package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"prui/internal/commits"
	"prui/internal/source"
)

var reviewEvents = []struct{ label, description, event string }{
	{"Comment", "General feedback without approval", "COMMENT"},
	{"Approve", "Approve merging these changes", "APPROVE"},
	{"Request changes", "Suggest changes before merging", "REQUEST_CHANGES"},
}

// The form and queued comments belong to one pinned comparison; the separate
// mutable draft store never includes them in immutable snapshots. The confirmation state keeps Enter from writing accidentally.
type reviewForm struct {
	Event, Focus, Selected int // focus: event, summary, pending comments
	Body                   string
	Cursor                 int // rune offset
	Confirm                bool
	generation             uint64
}

func (m *Model) unsentReviewDrafts() bool {
	if m.discussions.published != nil || m.discussions.editor != nil || m.SuggestionApply != nil || m.Composer != nil && (m.Composer.Suggestion || strings.TrimSpace(m.Composer.Draft) != "") || len(m.Pending) > 0 || m.ReviewForm != nil && strings.TrimSpace(m.ReviewForm.Body) != "" {
		return true
	}
	for i, tab := range m.tabs {
		if i != m.activeTab && tab.review != nil && (tab.review.discussions.published != nil || tab.review.discussions.editor != nil || tab.review.SuggestionApply != nil || tab.review.Composer != nil && (tab.review.Composer.Suggestion || strings.TrimSpace(tab.review.Composer.Draft) != "") || len(tab.review.Pending) > 0 || tab.review.ReviewForm != nil && strings.TrimSpace(tab.review.ReviewForm.Body) != "") {
			return true
		}
	}
	return false
}

func (m *Model) openReviewForm() {
	if m.ReviewForm == nil {
		m.ReviewForm = &reviewForm{}
	}
	m.push(pageReviewSubmit)
	m.ReviewSubmitted = false
}

func (m *Model) reviewFormKey(key tea.KeyPressMsg) tea.Cmd {
	f := m.ReviewForm
	if f == nil {
		m.pop()
		return nil
	}
	k := key.String()
	if k == "ctrl+r" && m.draft.attempt != "" {
		return m.reconcileDraftCommand()
	}
	if k == "ctrl+d" {
		m.Pending = nil
		m.ReviewForm = nil
		m.Composer = nil
		m.CommentMenu = nil
		m.draft.attempt = ""
		m.draft.attempted = nil
		m.pop()
		return nil
	}
	if m.draft.attempt != "" && k != "esc" {
		m.ActionError = errors.New("delivery uncertain: ctrl+r to check GitHub or ctrl+d to discard")
		return nil
	}
	if f.Confirm {
		switch k {
		case "esc":
			f.Confirm = false
		case "enter":
			if m.submitReview == nil || m.Session == nil {
				m.ActionError = errors.New("review submission unavailable")
				return nil
			}
			if !m.prepareDraftAttempt("review") {
				return nil
			}
			f.generation++
			generation, target, submit := f.generation, m.activeTab, m.submitReview
			metadata := m.Session.Inventory.Comparison.Metadata
			comments := append([]source.ReviewComment(nil), m.Pending...)
			submission := ReviewSubmission{Inventory: &m.Session.Inventory, Metadata: metadata, Review: source.PullRequestReview{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Event: reviewEvents[f.Event].event, Body: f.Body, Comments: comments}}
			m.notice = "Submitting pull request review..."
			ctx := m.beginAction()
			return m.start(func() tea.Msg {
				return ReviewResult{Target: target, Generation: generation, Err: submit(ctx, submission)}
			})
		}
		return nil
	}
	if f.Focus == 1 && (draftEditor{&f.Body, &f.Cursor, summaryDraft}).key(key) != draftUnhandled {
		return nil
	}

	switch k {
	case "esc":
		m.pop()
	case "tab":
		f.Focus = (f.Focus + 1) % 3
	case "up", "k":
		switch {
		case f.Focus == 0:
			f.Event = (f.Event + len(reviewEvents) - 1) % len(reviewEvents)
		case f.Focus == 2 && len(m.Pending) > 0:
			f.Selected = (f.Selected + len(m.Pending) - 1) % len(m.Pending)
		}
	case "down", "j":
		switch {
		case f.Focus == 0:
			f.Event = (f.Event + 1) % len(reviewEvents)
		case f.Focus == 2 && len(m.Pending) > 0:
			f.Selected = (f.Selected + 1) % len(m.Pending)
		}
	case "enter":
		switch f.Focus {
		case 0:
			f.Focus = 1
		case 1:
			if (f.Event == 2 || f.Event == 0 && len(m.Pending) == 0) && strings.TrimSpace(f.Body) == "" {
				m.ActionError = errors.New("review comment is required")
				return nil
			}
			m.ActionError = nil
			f.Confirm = true
		case 2:
			if f.Selected < len(m.Pending) {
				pending := m.Pending[f.Selected]
				if !m.focusPendingTarget(pending.Target) {
					m.ActionError = errors.New("pending comment target is unavailable in this comparison")
					return nil
				}
				m.Composer = &commentComposer{Target: pending.Target, Draft: pending.Body, Cursor: len([]rune(pending.Body)), PendingIndex: f.Selected}
				if replacement, err := source.ParseSuggestion(pending.Body); err == nil {
					if before, ok := m.targetSource(pending.Target); ok {
						m.Composer.Suggestion = true
						m.Composer.Before = before
						m.Composer.Draft = replacement
						m.Composer.Cursor = len([]rune(replacement))
					}
				}
				m.pop()
			}
		}
	case "d":
		if f.Focus == 2 && f.Selected < len(m.Pending) {
			m.Pending = append(m.Pending[:f.Selected], m.Pending[f.Selected+1:]...)
			f.Selected = max(0, min(f.Selected, len(m.Pending)-1))
		}
	}
	return nil
}

func (m *Model) focusPendingTarget(target source.ReviewCommentTarget) bool {
	if m.Session == nil {
		return false
	}
	if target.SubjectType == "file" {
		inv := m.Session.Inventory
		if !commits.InventoryContainsTarget(inv.Files, inv.Units, inv.Patches, target) {
			return false
		}
		for unit, u := range inv.Units {
			for _, f := range inv.Files {
				path := f.NewPath
				if f.Status == "D" {
					path = f.OldPath
				}
				if u.FileChangeID == f.ID && string(path) == target.Path {
					m.resetCommitFilter()
					m.ContextView, m.Files, m.Inventory, m.Selected, m.Focus = viewFiles, true, false, unit, paneDiff
					m.setOffset(0)
					return true
				}
			}
		}
		return false
	}
	for unit := range m.Session.Inventory.Units {
		for _, line := range unitLines(m.Session, unit) {
			if lineHasEndpoint(line, target) {
				m.resetCommitFilter()
				m.ContextView, m.Files, m.Inventory, m.Selected, m.Focus = viewFiles, true, false, unit, paneDiff
				m.cursorActive = true
				for row, detail := range m.displayDetail() {
					matches := lineHasEndpoint(detail, target)
					if detail.sideBySide != nil {
						for _, candidate := range rowTargets(*detail.sideBySide) {
							matches = matches || targetEndsAt(target, candidate)
						}
					}
					if matches {
						m.setCursor(row)
						endpoint := target
						endpoint.StartLine, endpoint.StartSide = 0, ""
						m.setSelectedDiffTarget(&endpoint)
						m.ensureCursorVisible()
						return true
					}
				}
				return false
			}
		}
	}
	return false
}

func (m *Model) pendingLines(target source.ReviewCommentTarget) []diffLine {
	lines := []diffLine{}
	for i, comment := range m.Pending {
		if targetEndsAt(comment.Target, target) {
			first := strings.SplitN(comment.Body, "\n", 2)[0]
			lines = append(lines, diffLine{styledLine: styledLine{Class: classWarning, Text: fmt.Sprintf("  [Pending %d] %s · R to review", i+1, Escape(first))}})
			if replacement, err := source.ParseSuggestion(comment.Body); err == nil {
				if before, ok := m.targetSource(comment.Target); ok {
					lines = append(lines, suggestionPreview(before, replacement)...)
				}
			}
		}
	}
	return lines
}

func (m *Model) applyReviewResult(result ReviewResult) {
	state := m.reviewStateForTarget(result.Target)
	if state == nil || state.ReviewForm == nil || state.ReviewForm.generation != result.Generation {
		return
	}
	state.Busy = false
	state.ActionError = result.Err
	if result.Err == nil {
		state.draft.attempt = ""
		state.draft.attempted = nil
		state.Pending = nil
		state.ReviewForm = nil
		state.ReviewSubmitted = true
		if len(state.Stack) > 1 && state.Stack[len(state.Stack)-1] == pageReviewSubmit {
			state.Stack = state.Stack[:len(state.Stack)-1]
		}
	} else {
		state.ReviewForm.Confirm = false
	}
}
