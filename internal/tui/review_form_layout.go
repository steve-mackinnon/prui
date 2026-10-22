package tui

import (
	"fmt"
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type reviewFormHitKind uint8

const (
	reviewDecision reviewFormHitKind = iota
	reviewSummary
	reviewPending
	reviewKey
)

// Rows and their trusted controls are allocated together. Private draft text is
// only display content; it can never supply a key or a pending-comment identity.
type reviewFormHit struct {
	bounds        image.Rectangle
	kind          reviewFormHitKind
	index, offset int
	singleLine    bool
	key           string
}

type reviewFormLayout struct {
	lines []string
	hits  []reviewFormHit
	width int
}

func (l *reviewFormLayout) add(row, left, right int, hit reviewFormHit) {
	if left >= l.width || right <= left {
		return
	}
	if hit.kind == reviewDecision && right > l.width {
		return
	}
	hit.bounds = image.Rect(left, row, min(right, l.width), row+1)
	l.hits = append(l.hits, hit)
}

func (l *reviewFormLayout) truncate(rows int) {
	kept := l.hits[:0]
	for _, hit := range l.hits {
		if hit.bounds.Min.Y < rows {
			kept = append(kept, hit)
		}
	}
	l.hits = kept
}

func (l reviewFormLayout) text() string {
	lines := make([]string, len(l.lines))
	for i, line := range l.lines {
		lines[i] = clip(line, l.width)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) reviewFormView() string { return m.reviewFormLayout().text() }

type reviewFormControl struct{ text, key string }

// Each trusted label and its action are declared together, without parsing the
// resulting footer. Combined shortcuts deliberately have one click action.
func reviewFormControls(f *reviewForm, compact bool) []reviewFormControl {
	if compact {
		switch {
		case f.Confirm:
			return []reviewFormControl{{"enter: submit review", "enter"}, {"esc: edit", "esc"}}
		case f.Focus == 1:
			return []reviewFormControl{{"enter: confirm", "enter"}, {"esc: back", "esc"}}
		case f.Focus == 2:
			return []reviewFormControl{{"enter: edit", "enter"}, {"d: remove", "d"}, {"tab: choices", "tab"}}
		default:
			return []reviewFormControl{{"j/k: choose", "down"}, {"tab: comment", "tab"}, {"esc: back", "esc"}}
		}
	}
	switch {
	case f.Confirm:
		return []reviewFormControl{{"enter: submit review and pending comments", "enter"}, {"esc: edit", "esc"}}
	case f.Focus == 1:
		return []reviewFormControl{{"enter: confirm", "enter"}, {"shift+enter: newline", "shift+enter"}, {"esc: back", "esc"}}
	case f.Focus == 2:
		return []reviewFormControl{{"j/k: select", "down"}, {"enter: edit", "enter"}, {"d: remove", "d"}, {"esc: back", "esc"}}
	default:
		return []reviewFormControl{{"j/k or ↑/↓: choose", "down"}, {"tab/enter: comment", "tab"}, {"esc: back", "esc"}}
	}
}

func (l *reviewFormLayout) footer(row int, controls []reviewFormControl) string {
	text := ""
	for i, control := range controls {
		if i > 0 {
			text += " · "
		}
		left := visibleWidth(text)
		text += control.text
		right := visibleWidth(text)
		// A truncated label cannot invoke even a partially visible write control.
		if right <= l.width {
			l.add(row, left, right, reviewFormHit{kind: reviewKey, key: control.key})
		}
	}
	return text
}

func (m *Model) clickReviewForm(layout reviewFormLayout, x, row int) tea.Cmd {
	for _, hit := range layout.hits {
		if !image.Pt(x, row).In(hit.bounds) {
			continue
		}
		f := m.ReviewForm
		switch hit.kind {
		case reviewKey:
			return m.reviewFormKey(mouseKey(hit.key))
		case reviewDecision:
			f.Event, f.Focus = hit.index, 0
		case reviewPending:
			f.Selected, f.Focus = hit.index, 2
		case reviewSummary:
			f.Focus = 1
			if hit.index >= 0 {
				offset := hit.offset
				if at, ok := editorCursorAt(f.Body, f.Cursor, editorCursorLayout{width: 1 << 20, hardwrap: true, multiline: !hit.singleLine}, x-offset, hit.index); ok {
					f.Cursor = at
				}
			}
		}
		return nil
	}
	return nil
}

func (m *Model) reviewFormLayout() reviewFormLayout {
	layout := reviewFormLayout{width: m.Width}
	f := m.ReviewForm
	if f == nil || m.Session == nil {
		layout.lines = []string{"Review unavailable · esc: back"}
		return layout
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
		if !f.Confirm {
			layout.add(len(lines), 0, visibleWidth(marker+radio+" "+option.label), reviewFormHit{kind: reviewDecision, index: i})
		}
		lines = append(lines, row)
	}
	if m.Height <= 8 {
		return m.compactReviewFormLayout(lines, f)
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
		if f.Event == 2 || f.Event == 0 && len(m.Pending) == 0 {
			label += " (required)"
		}
		layout.add(len(lines), 0, visibleWidth(label), reviewFormHit{kind: reviewSummary, index: -1})
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
		bodyStart := 0
		if len(bodyLines) > bodyLimit {
			cursorLine := strings.Count(before, "\n")
			start := max(0, min(cursorLine-bodyLimit+1, len(bodyLines)-bodyLimit))
			bodyStart = start
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
			for i, line := range bodyLines {
				content := clip(Escape(line), inner)
				layout.add(len(lines), 4, 4+visibleWidth(content), reviewFormHit{kind: reviewSummary, index: bodyStart + i, offset: 4})
				lines = append(lines, "  │ "+content+strings.Repeat(" ", inner-visibleWidth(content))+" │")
			}
			lines = append(lines, "  └"+strings.Repeat("─", inner+2)+"┘")
		} else {
			edge := "│"
			if f.Focus == 1 {
				edge = "┃"
			}
			for i, line := range bodyLines {
				layout.add(len(lines), 4, 4+visibleWidth(Escape(line)), reviewFormHit{kind: reviewSummary, index: bodyStart + i, offset: 4})
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
			row := fmt.Sprintf("%s%s · %s", marker, commentTargetLabel(comment.Target), Escape(first))
			layout.add(len(lines), 0, min(m.Width, visibleWidth(row)), reviewFormHit{kind: reviewPending, index: i})
			lines = append(lines, row)
		}
	}
	if !m.Session.Inventory.Complete {
		lines = append(lines, "! Inventory incomplete")
	}
	if m.ActionError != nil {
		lines = append(lines, "! "+Escape(m.ActionError.Error()))
		lines = append(lines, "Check GitHub before retrying; delivery may have succeeded.")
	}
	footer := reviewFormControls(f, false)
	if len(lines) >= m.Height {
		// Keep the latest actionable error or warning beside the submit hint.
		if m.ActionError != nil {
			layout.truncate(max(0, m.Height-3))
			lines = append(lines[:max(0, m.Height-3)], "! "+Escape(m.ActionError.Error()), "Check GitHub before retrying; delivery may have succeeded.")
		} else {
			layout.truncate(max(0, m.Height-1))
			lines = lines[:max(0, m.Height-1)]
		}
	}
	lines = append(lines, layout.footer(len(lines), footer))
	layout.lines = lines
	return layout
}

// On short terminals the decision stays visible, including while an error is
// shown. Supplementary text uses only the rows left after the three choices.
func (m *Model) compactReviewFormLayout(full []string, f *reviewForm) reviewFormLayout {
	layout := reviewFormLayout{width: m.Width}
	choices := full[len(full)-len(reviewEvents):]
	lines := make([]string, 0, m.Height)
	if m.Height >= 7 {
		lines = append(lines, full[0])
	}
	if m.Height >= 6 || m.Height == 5 && m.ActionError == nil {
		lines = append(lines, full[len(full)-len(reviewEvents)-1])
	}
	if !f.Confirm {
		for i := range choices {
			layout.add(len(lines)+i, 0, 4+visibleWidth(reviewEvents[i].label), reviewFormHit{kind: reviewDecision, index: i})
		}
	}
	lines = append(lines, choices...)
	if m.Height <= len(lines) {
		lines = lines[:max(0, m.Height)]
	} else {
		footer := reviewFormControls(f, true)
		if m.Height-len(lines) > 1 {
			switch {
			case m.ActionError != nil:
				lines = append(lines, "! "+Escape(m.ActionError.Error()))
			case f.Confirm:
				lines = append(lines, fmt.Sprintf("Confirm submission · %d pending comments", len(m.Pending)))
			case f.Focus == 1:
				body := []rune(f.Body)
				cursor := max(0, min(len(body), f.Cursor))
				text := "Comment: " + Escape(string(body[:cursor])) + "▏" + Escape(string(body[cursor:]))
				layout.add(len(lines), 9, visibleWidth(text), reviewFormHit{kind: reviewSummary, index: 0, offset: 9, singleLine: true})
				lines = append(lines, text)
			default:
				lines = append(lines, fmt.Sprintf("%d pending comments", len(m.Pending)))
			}
		}
		if m.ActionError != nil && m.Height-len(lines) > 1 {
			lines = append(lines, "Check GitHub before retrying.")
		}
		lines = append(lines, layout.footer(len(lines), footer))
	}
	layout.lines = lines
	return layout
}

// Modal rendering and input use the same responsive inner viewport.
func (m *Model) reviewFormModalLayout() reviewFormLayout {
	form := *m
	if m.Width >= 20 && m.Height >= 8 {
		form.Width = min(96, m.Width-8)
		form.Height = min(22, m.Height-2)
	}
	return form.reviewFormLayout()
}
