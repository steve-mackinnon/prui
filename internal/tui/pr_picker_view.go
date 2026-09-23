package tui

import (
	"fmt"
	"strings"

	"pr-review/internal/source"
)

type cardPart struct {
	text  string
	class lineClass
}

func (m *Model) pullRequestPickerView() string {
	if m.Session != nil {
		return m.switcherView()
	}
	m.PullRequestPicker.clamp(len(m.PullRequests))
	header := []string{appHeader("Open pull requests", fmt.Sprintf("%d open", len(m.PullRequests)))}
	if m.Width < 20 || m.Height < 8 {
		rows := make([]string, len(m.PullRequests))
		for i, pr := range m.PullRequests {
			rows[i] = fmt.Sprintf("#%d %s", pr.Identity.Number, Escape(pr.Title))
		}
		return m.pickerScreen(&m.PullRequestPicker, header, rows, "No open pull requests.", "enter: open · esc: back")
	}
	rows := make([][]string, len(m.PullRequests))
	for i, pr := range m.PullRequests {
		rows[i] = m.pullRequestCard(pr, i == m.PullRequestPicker.Index)
	}
	return m.pickerScreenRowsStyled(&m.PullRequestPicker, header, rows, "No open pull requests.", "enter: open · esc: back", true)
}

func (m *Model) pullRequestCard(pr source.PullRequest, selected bool) []string {
	width := min(100, m.Width-2) // two columns for the picker selection gutter
	borderClass := paneBorderClass(selected)
	bottom := "╰" + strings.Repeat("─", width-2) + "╯"
	name := func(s string) string {
		if s == "" {
			return "unknown"
		}
		escaped := Escape(s)
		limit := min(20, max(8, (width-20)/2))
		if visibleWidth(escaped) > limit {
			return clip(escaped, limit-1) + "…"
		}
		return escaped
	}
	opened := "unknown"
	if !pr.OpenedAt.IsZero() {
		opened = pr.OpenedAt.Local().Format("Jan 2, 2006")
	}
	check, checkClass := "? Checks unknown", classMetadata
	switch pr.Checks {
	case source.ChecksPassed:
		check, checkClass = "✓ Checks pass", classAdded
	case source.ChecksFailed:
		check, checkClass = "✗ Checks fail", classRemoved
	case source.ChecksPending:
		check, checkClass = "… Checks pending", classWarning
	}
	if width < 45 {
		switch pr.Checks {
		case source.ChecksPassed:
			check = "✓ Pass"
		case source.ChecksFailed:
			check = "✗ Fail"
		case source.ChecksPending:
			check = "… Pending"
		default:
			check = "? Unknown"
		}
	}
	check = clip(check, width-5)
	top := m.styleLine(borderClass, "╭─ ") + m.styleLine(checkClass, check) +
		m.styleLine(borderClass, " "+strings.Repeat("─", width-5-visibleWidth(check))+"╮")
	rows := []string{
		top,
		m.pullRequestCardLine(width, borderClass, cardPart{fmt.Sprintf("#%d  ", pr.Identity.Number), classMetadata}, cardPart{Escape(pr.Title), classTitle}),
	}
	if m.Width <= 80 {
		rows = append(rows,
			m.pullRequestCardLine(width, borderClass, cardPart{"Author " + name(pr.Author) + "  ·  Opened " + opened, classMetadata}),
			m.pullRequestCardLine(width, borderClass, cardPart{"Last commit " + name(pr.LastModifier), classMetadata}),
		)
	} else {
		rows = append(rows, m.pullRequestCardLine(width, borderClass,
			cardPart{"Author " + name(pr.Author) + "  ·  Opened " + opened + "  ·  Last commit " + name(pr.LastModifier), classMetadata},
		))
	}
	return append(rows, m.styleLine(borderClass, bottom))
}

func (m *Model) pullRequestCardLine(width int, borderClass lineClass, parts ...cardPart) string {
	available := width - 4 // │, space, space, │
	content := ""
	used := 0
	for _, part := range parts {
		if used == available {
			break
		}
		piece := clip(part.text, available-used)
		content += m.styleLine(part.class, piece)
		used += visibleWidth(piece)
	}
	return m.styleLine(borderClass, "│") + " " + content + strings.Repeat(" ", available-used) + " " + m.styleLine(borderClass, "│")
}
