package tui

import (
	"fmt"
	"strings"

	"prui/internal/source"
)

func (m *Model) pullRequestPickerView() string {
	if m.Session != nil {
		return m.switcherView()
	}
	m.PullRequestPicker.clamp(len(m.PullRequests))
	header := []string{appHeader("Open pull requests", fmt.Sprintf("%d open", len(m.PullRequests)))}
	rows := make([]string, len(m.PullRequests))
	var detail []string
	for i, pr := range m.PullRequests {
		rows[i] = m.pullRequestRow(pr)
		if i == m.PullRequestPicker.Index {
			detail = m.pullRequestDetail(pr)
		}
	}
	return m.pickerScreenDetail(&m.PullRequestPicker, header, rows, detail, "No open pull requests.", "enter: open · esc: back")
}

func pullRequestChecks(pr source.PullRequest) string {
	switch pr.Checks {
	case source.ChecksPassed:
		return "✓ Checks pass"
	case source.ChecksFailed:
		return "✗ Checks fail"
	case source.ChecksPending:
		return "… Checks pending"
	default:
		return "? Checks unknown"
	}
}

func (m *Model) pullRequestRow(pr source.PullRequest) string {
	return m.compactPullRequestRow(fmt.Sprintf("#%d  %s", pr.Identity.Number, Escape(pr.Title)), pullRequestChecks(pr))
}

func (m *Model) compactPullRequestRow(title, status string) string {
	width := max(0, m.Width-2)
	// At narrow widths keep the PR identity readable; the selected detail also
	// carries the full check state when there is room beneath the list.
	if width < 32 {
		return clip(title, width)
	}
	title = clip(title, width-visibleWidth(status)-2)
	return title + strings.Repeat(" ", width-visibleWidth(title)-visibleWidth(status)) + status
}

func (m *Model) pullRequestDetail(pr source.PullRequest) []string {
	name := func(value string) string {
		if value == "" {
			return "unknown"
		}
		return clip(Escape(value), min(20, max(8, m.Width/4)))
	}
	opened := "unknown"
	if !pr.OpenedAt.IsZero() {
		opened = pr.OpenedAt.Local().Format("Jan 2, 2006")
	}
	return []string{
		"Author " + name(pr.Author) + "  ·  Opened " + opened,
		"Last commit " + name(pr.LastModifier) + "  ·  " + pullRequestChecks(pr),
	}
}
