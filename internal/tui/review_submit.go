package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"prui/internal/source"
)

var reviewEvents = []struct{ label, description, event string }{
	{"Comment", "General feedback without approval", "COMMENT"},
	{"Approve", "Approve merging these changes", "APPROVE"},
	{"Request changes", "Suggest changes before merging", "REQUEST_CHANGES"},
}

// The form and queued comments belong to one open review tab and never enter a
// saved session. The confirmation state keeps Enter from writing accidentally.
type reviewForm struct {
	Event, Focus, Selected int // focus: event, summary, pending comments
	Body                   string
	Cursor                 int // rune offset
	Confirm                bool
	generation             uint64
}

func (m *Model) unsentReviewDrafts() bool {
	if m.Composer != nil && strings.TrimSpace(m.Composer.Draft) != "" || len(m.Pending) > 0 || m.ReviewForm != nil && strings.TrimSpace(m.ReviewForm.Body) != "" {
		return true
	}
	for i, tab := range m.tabs {
		if i != m.activeTab && tab.review != nil && (tab.review.Composer != nil && strings.TrimSpace(tab.review.Composer.Draft) != "" || len(tab.review.Pending) > 0 || tab.review.ReviewForm != nil && strings.TrimSpace(tab.review.ReviewForm.Body) != "") {
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

func (m *Model) reviewFormView() string {
	f := m.ReviewForm
	if f == nil || m.Session == nil {
		return "Review unavailable · esc: back"
	}
	metadata := m.Session.Inventory.Comparison.Metadata
	lines := []string{
		fmt.Sprintf("Submit review · %s#%d · head %.12s", Escape(metadata.Identity.Repository), metadata.Identity.Number, metadata.HeadSHA),
	}
	if m.Height >= 9 {
		lines = append(lines, fmt.Sprintf("%d/%d file slices read · %d pending comments", len(m.Session.ReviewedSliceIDs), len(m.Session.Slices), len(m.Pending)))
	}
	if m.Height >= 12 && !f.Confirm {
		lines = append(lines, "")
	}
	decisionHeading := "Review decision (j/k or ↑/↓)"
	if f.Confirm {
		decisionHeading = "Review decision"
	}
	lines = append(lines, decisionHeading)
	for i, option := range reviewEvents {
		marker, radio := "  ", "○"
		if i == f.Event {
			radio = "◉"
			if f.Focus == 0 && !f.Confirm {
				marker = "› "
			}
		}
		row := marker + radio + " " + option.label
		if m.Width >= 75 {
			row += " · " + option.description
		}
		lines = append(lines, row)
	}
	if m.Height <= 8 {
		return m.compactReviewFormView(lines, f)
	}
	if f.Confirm {
		lines = append(lines, fmt.Sprintf("Confirm submission · %d pending comments", len(m.Pending)))
		reserved := 1 // submit footer
		if !m.Session.Inventory.Complete {
			reserved++
		}
		if m.ActionError != nil {
			reserved += 2
		}
		available := max(0, m.Height-len(lines)-reserved)
		if f.Body != "" && available > 0 {
			bodyLines := strings.Split(f.Body, "\n")
			if available == 1 {
				lines = append(lines, "Comment: "+Escape(bodyLines[0]))
			} else {
				lines = append(lines, "Comment:")
				for _, line := range bodyLines[:min(len(bodyLines), available-1)] {
					lines = append(lines, "  "+Escape(line))
				}
			}
		}
	} else {
		if m.Height >= 13 {
			lines = append(lines, "")
		}
		label := "Comment"
		if f.Focus == 1 {
			label = "› Comment"
		}
		if f.Event != 1 {
			label += " (required)"
		}
		lines = append(lines, label)
		body := []rune(f.Body)
		cursor := max(0, min(len(body), f.Cursor))
		before, after := string(body[:cursor]), string(body[cursor:])
		if f.Focus == 1 {
			body = []rune(before + "▏" + after)
		}
		bodyLines := strings.Split(string(body), "\n")
		if f.Body == "" && f.Focus != 1 {
			bodyLines = []string{"Leave a comment"}
		}
		reserved := 2 // pending heading and footer
		if len(m.Pending) > 0 {
			reserved++ // show at least one pending comment when space permits
		}
		if !m.Session.Inventory.Complete {
			reserved++
		}
		if m.ActionError != nil {
			reserved += 2
		}
		bordered := m.Height >= 14 && m.Width >= 20
		if bordered {
			reserved += 2 // top and bottom of the comment box
		}
		bodyLimit := max(1, min(3, m.Height-len(lines)-reserved))
		if len(bodyLines) > bodyLimit {
			cursorLine := strings.Count(before, "\n")
			start := max(0, min(cursorLine-bodyLimit+1, len(bodyLines)-bodyLimit))
			bodyLines = bodyLines[start : start+bodyLimit]
		}
		if m.Height >= 14 {
			for len(bodyLines) < bodyLimit {
				bodyLines = append(bodyLines, "")
			}
		}
		if bordered {
			inner := m.Width - 6
			lines = append(lines, "  ┌"+strings.Repeat("─", inner+2)+"┐")
			for _, line := range bodyLines {
				content := clip(Escape(line), inner)
				lines = append(lines, "  │ "+content+strings.Repeat(" ", inner-visibleWidth(content))+" │")
			}
			lines = append(lines, "  └"+strings.Repeat("─", inner+2)+"┘")
		} else {
			edge := "│"
			if f.Focus == 1 {
				edge = "┃"
			}
			for _, line := range bodyLines {
				lines = append(lines, "  "+edge+" "+Escape(line))
			}
		}
		lines = append(lines, fmt.Sprintf("Pending comments (%d) · tab to select, enter edit, d remove", len(m.Pending)))
		reserved = 1
		if !m.Session.Inventory.Complete {
			reserved++
		}
		if m.ActionError != nil {
			reserved += 2
		}
		available := max(0, m.Height-len(lines)-reserved)
		start := max(0, f.Selected-available+1)
		for i := start; i < len(m.Pending) && i < start+available; i++ {
			comment := m.Pending[i]
			marker := "  "
			if f.Focus == 2 && i == f.Selected {
				marker = "› "
			}
			first := strings.SplitN(comment.Body, "\n", 2)[0]
			lines = append(lines, fmt.Sprintf("%s%s:%d %s · %s", marker, Escape(comment.Target.Path), comment.Target.Line, comment.Target.Side, Escape(first)))
		}
	}
	if !m.Session.Inventory.Complete {
		lines = append(lines, "! Inventory incomplete")
	}
	if m.ActionError != nil {
		lines = append(lines, "! "+Escape(m.ActionError.Error()))
		lines = append(lines, "Check GitHub before retrying; delivery may have succeeded.")
	}
	footer := reviewFormFooter(f)
	if len(lines) >= m.Height {
		// Keep the latest actionable error or warning beside the submit hint.
		if m.ActionError != nil {
			lines = append(lines[:max(0, m.Height-3)], "! "+Escape(m.ActionError.Error()), "Check GitHub before retrying; delivery may have succeeded.")
		} else {
			lines = lines[:max(0, m.Height-1)]
		}
	}
	lines = append(lines, footer)
	for i := range lines {
		lines[i] = clip(lines[i], m.Width)
	}
	return strings.Join(lines, "\n")
}

func reviewFormFooter(f *reviewForm) string {
	if f.Confirm {
		return "enter: submit review and pending comments · esc: edit"
	}
	switch f.Focus {
	case 1:
		return "enter: confirm · shift+enter: newline · esc: back"
	case 2:
		return "j/k: select · enter: edit · d: remove · esc: back"
	default:
		return "j/k or ↑/↓: choose · tab/enter: comment · esc: back"
	}
}

// On short terminals the decision stays visible, including while an error is
// shown. Supplementary text uses only the rows left after the three choices.
func (m *Model) compactReviewFormView(full []string, f *reviewForm) string {
	choices := full[len(full)-len(reviewEvents):]
	lines := make([]string, 0, m.Height)
	if m.Height >= 7 {
		lines = append(lines, full[0])
	}
	if m.Height >= 6 || m.Height == 5 && m.ActionError == nil {
		lines = append(lines, full[len(full)-len(reviewEvents)-1])
	}
	lines = append(lines, choices...)
	if m.Height <= len(lines) {
		lines = lines[:max(0, m.Height)]
	} else {
		footer := "j/k: choose · tab: comment · esc: back"
		switch {
		case f.Confirm:
			footer = "enter: submit review · esc: edit"
		case f.Focus == 1:
			footer = "enter: confirm · esc: back"
		case f.Focus == 2:
			footer = "enter: edit · d: remove · tab: choices"
		}
		if m.Height-len(lines) > 1 {
			switch {
			case m.ActionError != nil:
				lines = append(lines, "! "+Escape(m.ActionError.Error()))
			case f.Confirm:
				lines = append(lines, fmt.Sprintf("Confirm submission · %d pending comments", len(m.Pending)))
			case f.Focus == 1:
				body := []rune(f.Body)
				cursor := max(0, min(len(body), f.Cursor))
				lines = append(lines, "Comment: "+Escape(string(body[:cursor]))+"▏"+Escape(string(body[cursor:])))
			default:
				lines = append(lines, fmt.Sprintf("%d pending comments", len(m.Pending)))
			}
		}
		if m.ActionError != nil && m.Height-len(lines) > 1 {
			lines = append(lines, "Check GitHub before retrying.")
		}
		lines = append(lines, footer)
	}
	for i := range lines {
		lines[i] = clip(lines[i], m.Width)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) reviewFormKey(key tea.KeyPressMsg) tea.Cmd {
	f := m.ReviewForm
	if f == nil {
		m.pop()
		return nil
	}
	k := key.String()
	if f.Confirm {
		switch k {
		case "esc":
			f.Confirm = false
		case "enter":
			if m.submitReview == nil || m.Session == nil {
				m.ActionError = errors.New("review submission unavailable")
				return nil
			}
			f.generation++
			generation, target, submit := f.generation, m.activeTab, m.submitReview
			metadata := m.Session.Inventory.Comparison.Metadata
			comments := append([]source.ReviewComment(nil), m.Pending...)
			submission := ReviewSubmission{Metadata: metadata, Review: source.PullRequestReview{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Event: reviewEvents[f.Event].event, Body: f.Body, Comments: comments}}
			m.notice = "Submitting pull request review..."
			ctx := m.beginAction()
			return m.start(func() tea.Msg {
				return ReviewResult{Target: target, Generation: generation, Err: submit(ctx, submission)}
			})
		}
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
		case f.Focus == 1:
			m.insertReviewSummary(key.Text)
		}
	case "down", "j":
		switch {
		case f.Focus == 0:
			f.Event = (f.Event + 1) % len(reviewEvents)
		case f.Focus == 2 && len(m.Pending) > 0:
			f.Selected = (f.Selected + 1) % len(m.Pending)
		case f.Focus == 1:
			m.insertReviewSummary(key.Text)
		}
	case "enter":
		switch f.Focus {
		case 0:
			f.Focus = 1
		case 1:
			if f.Event != 1 && strings.TrimSpace(f.Body) == "" {
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
				m.pop()
			}
		}
	case "d":
		if f.Focus == 2 && f.Selected < len(m.Pending) {
			m.Pending = append(m.Pending[:f.Selected], m.Pending[f.Selected+1:]...)
			f.Selected = max(0, min(f.Selected, len(m.Pending)-1))
		} else if f.Focus == 1 {
			m.insertReviewSummary(key.Text)
		}
	case "shift+enter":
		if f.Focus == 1 {
			m.insertReviewSummary("\n")
		}
	case "backspace":
		if f.Focus == 1 && f.Cursor > 0 {
			r := []rune(f.Body)
			f.Body = string(append(r[:f.Cursor-1], r[f.Cursor:]...))
			f.Cursor--
		}
	case "delete":
		if f.Focus == 1 {
			r := []rune(f.Body)
			if f.Cursor < len(r) {
				f.Body = string(append(r[:f.Cursor], r[f.Cursor+1:]...))
			}
		}
	case "left":
		if f.Focus == 1 {
			f.Cursor = max(0, f.Cursor-1)
		}
	case "right":
		if f.Focus == 1 {
			f.Cursor = min(len([]rune(f.Body)), f.Cursor+1)
		}
	default:
		if f.Focus == 1 && key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
			m.insertReviewSummary(key.Text)
		}
	}
	return nil
}

func (m *Model) focusPendingTarget(target source.ReviewCommentTarget) bool {
	if m.Session == nil {
		return false
	}
	for unit := range m.Session.Inventory.Units {
		for _, line := range unitLines(m.Session, unit) {
			if line.target != nil && *line.target == target {
				m.ContextView, m.Files, m.Inventory, m.Selected, m.Focus = viewFiles, true, false, unit, paneDiff
				m.cursorActive = true
				for row, detail := range m.displayDetail() {
					matches := detail.target != nil && *detail.target == target
					if detail.sideBySide != nil {
						for _, candidate := range rowTargets(*detail.sideBySide) {
							matches = matches || candidate == target
						}
					}
					if matches {
						m.setCursor(row)
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

func (m *Model) insertReviewSummary(text string) {
	f := m.ReviewForm
	f.Body, f.Cursor = insertEditorText(f.Body, f.Cursor, text)
}

func (m *Model) pendingLines(target source.ReviewCommentTarget) []diffLine {
	lines := []diffLine{}
	for i, comment := range m.Pending {
		if comment.Target == target {
			first := strings.SplitN(comment.Body, "\n", 2)[0]
			lines = append(lines, diffLine{styledLine: styledLine{Class: classWarning, Text: fmt.Sprintf("  [Pending %d] %s · R to review", i+1, Escape(first))}})
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
