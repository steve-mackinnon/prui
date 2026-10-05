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
		rows[i] = m.prPickerRow(pr)
		if i == m.PullRequestPicker.Index {
			detail = m.prPickerDetail(pr)
		}
	}
	footer := "enter: open · esc: back"
	if m.inbox.load != nil {
		footer = "enter: open · ctrl+o: inbox · esc: back"
	}
	return m.prPickerScreen(header, rows, detail, "No open pull requests.", footer)
}

func pullRequestChecks(pr source.PullRequest) string {
	switch pr.Checks {
	case source.ChecksPassed:
		return "✅ Checks pass"
	case source.ChecksFailed:
		return "❌ Checks fail"
	case source.ChecksPending:
		return "⏳ Checks pending"
	default:
		return "? Checks unknown"
	}
}

func (m *Model) pullRequestRow(pr source.PullRequest) string {
	return m.compactPullRequestRow(fmt.Sprintf("#%d  %s", pr.Identity.Number, Escape(pr.Title)), pr.Author, pr.ViewerReview, pullRequestChecks(pr))
}

func (m *Model) compactPullRequestRow(title, author, review, status string) string {
	// Keep columns together on wide terminals instead of stretching metadata
	// to the right edge. Narrow terminals still use all available row space.
	width := min(120, max(0, m.Width-2))
	// At narrow widths keep the PR identity readable; the selected detail also
	// carries the full check state when there is room beneath the list.
	if width < 32 {
		return m.prPickerTitle(title, width)
	}
	if author == "" {
		author = "unknown"
	}
	// Reserve a stable author column while leaving space for the PR identity.
	statusWidth := visibleWidth("⏳ Checks pending")
	authorWidth := min(20, max(1, width-statusWidth-15))
	author = clip("@"+Escape(author), authorWidth)
	marker := "○"
	switch review {
	case "APPROVED":
		marker = "✅"
	case "COMMENTED":
		marker = "💬"
	case "CHANGES_REQUESTED":
		marker = "🔄"
	case "PENDING":
		marker = "✎"
	}
	author = m.styleLine(classHunk, author)
	status = m.styleLine(prCheckAccent(status), status)
	metadata := marker + strings.Repeat(" ", 3-visibleWidth(marker)) + author + strings.Repeat(" ", authorWidth-visibleWidth(author)) + strings.Repeat(" ", 2+statusWidth-visibleWidth(status)) + status
	title = m.prPickerTitle(title, width-visibleWidth(metadata)-2)
	return title + strings.Repeat(" ", width-visibleWidth(title)-visibleWidth(metadata)) + metadata
}

func (m *Model) pullRequestDetail(pr source.PullRequest) []string {
	name := func(value string) string {
		if value == "" {
			return "unknown"
		}
		return Escape(value)
	}
	opened := "unknown"
	if !pr.OpenedAt.IsZero() {
		opened = pr.OpenedAt.Local().Format("Jan 2, 2006")
	}
	return []string{
		"Author " + name(pr.Author) + "  ·  Opened " + opened,
		"Last commit " + name(pr.LastModifier) + "  ·  " + pullRequestChecks(pr),
		"Your review: " + pullRequestReviewLabel(pr.ViewerReview),
	}
}

func pullRequestReviewLabel(state string) string {
	switch state {
	case "APPROVED":
		return "Approved"
	case "CHANGES_REQUESTED":
		return "Requested Changes"
	case "COMMENTED":
		return "Commented"
	default:
		return "None"
	}
}
