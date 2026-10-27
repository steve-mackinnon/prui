package tui

import (
	"fmt"

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

	return m.prPickerScreen(header, rows, detail, "No open pull requests.")
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
	detail := []string{
		"Author " + name(pr.Author) + "  ·  Opened " + opened,
		"Last commit " + name(pr.LastModifier) + "  ·  " + pullRequestChecks(pr),
		"Your review: " + pullRequestReviewLabel(pr.ViewerReview),
	}
	if pr.DiffStatsKnown {
		detail = append(detail, m.styleDiffMetrics(diffStat{added: pr.Additions, removed: pr.Deletions}.String()))
	}
	detail = append(detail, "Target branch: "+name(pr.TargetBranch))
	if pr.StackSize > 1 {
		detail = append(detail, fmt.Sprintf("Stack: %d/%d (base first)", pr.StackPosition, pr.StackSize))
	}
	return detail
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
