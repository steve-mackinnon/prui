package tui

import (
	"fmt"
	"sort"
	"strings"

	"prui/internal/guide"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
)

type healthItem struct {
	text     string
	severity int
}

func appHeader(title, action string) string {
	if action == "" {
		return title
	}
	return title + " · " + action
}

func sectionHeader(section, detail string) string {
	if detail == "" {
		return section
	}
	return section + " · " + detail
}

func reviewHealth(s *review.Session) (string, []healthItem) {
	progress := fmt.Sprintf("%d/%d read", len(s.ReviewedSliceIDs), len(s.Slices))
	items := []healthItem{{text: "✓ Inventory complete"}}
	if !s.Inventory.Complete {
		items[0] = healthItem{text: "! Inventory incomplete", severity: 3}
	}
	switch s.RevisionStatus {
	case session.Current:
		items = append(items, healthItem{text: "✓ Freshness current"})
	case session.Stale:
		items = append(items, healthItem{text: "! Freshness stale", severity: 3})
	case session.CheckFailed:
		items = append(items, healthItem{text: "! Freshness unknown", severity: 3})
	default:
		items = append(items, healthItem{text: "? Freshness unknown", severity: 1})
	}
	switch {
	case s.Guides == nil || s.Guides.Status == guide.Unavailable && s.Guides.Reason == "analysis not requested":
		items = append(items, healthItem{text: "Guide available with g", severity: 0})
	case s.Guides.Status == guide.Unavailable:
		items = append(items, healthItem{text: "? Guides unavailable", severity: 1})
	default:
		items = append(items, healthItem{text: fmt.Sprintf("✓ Guides %d", len(s.Guides.Items))})
	}
	unavailable := 0
	for _, u := range s.Inventory.Units {
		if u.Kind == "unavailable" {
			unavailable++
		}
	}
	if unavailable > 0 {
		items = append(items, healthItem{text: fmt.Sprintf("! %d unavailable", unavailable), severity: 3})
	}
	return progress, items
}

func (m *Model) healthStatus(width int, s *review.Session) string {
	progress, items := reviewHealth(s)
	sort.SliceStable(items, func(i, j int) bool { return items[i].severity > items[j].severity })
	action := fmt.Sprintf("R Submit review (%d)", len(m.Pending))
	if width < 110 {
		action = fmt.Sprintf("R Review (%d)", len(m.Pending))
	}
	severity := 0
	var states []string
	for _, item := range items {
		if item.severity == 0 && item.text != "✓ Freshness current" {
			continue
		}
		// Once severe warnings exist, reserve status space for those warnings.
		if severity >= 2 && item.severity < 2 {
			continue
		}
		severity = max(severity, item.severity)
		states = append(states, item.text)
	}
	state := strings.Join(states, " · ")
	parts := []string{progress, action, state}
	if visibleWidth(strings.Join(parts, " · ")) > width {
		state = strings.NewReplacer("Inventory incomplete", "Inventory gap", "Freshness ", "").Replace(state)
		parts[2] = state
	}
	if visibleWidth(strings.Join(parts, " · ")) > width {
		parts = []string{strings.TrimSuffix(progress, " read"), fmt.Sprintf("R (%d)", len(m.Pending)), state}
	}
	if visibleWidth(strings.Join(parts, " · ")) > width && len(states) > 1 {
		prefix := "?"
		if severity >= 2 {
			prefix = "!"
		}
		parts[2] = fmt.Sprintf("%s %d issues", prefix, len(states))
	}

	help := "?: Help"
	if width >= 110 {
		help = "?: Health & help"
	}
	if visibleWidth(strings.Join(append(append([]string(nil), parts...), help), " · ")) <= width {
		parts = append(parts, help)
	}
	class := classMetadata
	if severity >= 2 {
		class = classWarning
	}
	return m.styleLine(class, clip(strings.Join(parts, " · "), width))
}

// healthHelpView keeps routine diagnostics available without filling review chrome.
func (m *Model) healthHelpView() string {
	text := "Health & help"
	if m.Session != nil {
		progress, items := reviewHealth(m.Session)
		text += "\n" + progress + fmt.Sprintf(" · %d pending comments", len(m.Pending))
		for _, item := range items {
			text += "\n" + item.text
		}
		if m.ActionError != nil {
			text += "\n! Action failed: " + Escape(m.ActionError.Error())
		}
	}
	text += "\n\n" + renderHealth() + "\n\nControls and invalid bytes escaped. Mouse selects; Enter activates.\nReading progress is local, not GitHub approval.\nGuides interpret the diff; the raw inventory remains the complete source view.\nMarking any portion of a file marks its whole slice, under every guide.\nEvidence is pinned, bounded, and omissions are reported. Analysis is optional and consent-bound."
	if m.store != nil && m.Session != nil {
		text += "\nStorage: " + Escape(m.store.Path()) + "\nSession: " + Escape(m.Session.ID)
	}
	return text
}

func (m *Model) reviewStatus() string {
	if m.Session == nil {
		return ""
	}
	status := m.healthStatus(m.Width, m.Session)
	hints := m.reviewHints()
	if m.Height < 10 {
		if m.selectedReviewView() == viewCommits && m.Session.Commits != nil && !m.Session.Commits.Complete && m.Session.Commits.Status == "captured" {
			return hints
		}
		if m.Composer != nil || m.CommentMenu != nil || m.ActionError != nil {
			return hints
		}
		return status
	}
	return status + "\n" + hints
}

func (m *Model) reviewHints() string {
	if m.discussions.editor != nil && m.selectedReviewView() == viewDescription {
		return m.styleLine(classWarning, clip("Enter: post · shift+enter: newline · ctrl+r: refresh · esc: cancel", m.Width))
	}
	if m.rangeStart != nil {
		end := m.commentSelectionTarget()
		text := fmt.Sprintf("Range start %s:%d", m.rangeStart.Side, m.rangeStart.Line)
		if end != nil {
			text += fmt.Sprintf(" · end %s:%d", end.Side, end.Line)
		}
		if m.ActionError != nil {
			text += " · ! " + Escape(m.ActionError.Error())
		}
		return m.styleLine(classWarning, clip(text+" · enter: compose · esc: cancel", m.Width))
	}
	if m.Composer != nil {
		if m.Composer.CommitSHA != "" {
			return m.styleLine(classWarning, clip("enter: post now · ctrl+r: refresh · esc: discard · shift+enter: newline", m.Width))
		}
		return m.styleLine(classWarning, clip("enter: post now · ctrl+p: save pending · esc: discard · shift+enter: newline", m.Width))
	}
	if menu := m.CommentMenu; menu != nil {
		text := "Comment actions: r reply · a react · z resolve/reopen · esc cancel"
		switch menu.mode {
		case commentActionReply:
			text = "Reply editor open · enter submit · esc cancel"
		case commentActionReact:
			choices := []string{}
			for i, content := range source.ReviewCommentReactions() {
				choices = append(choices, fmt.Sprintf("%d %s", i+1, m.reactionLabel(content)))
			}
			text = "React: " + strings.Join(choices, " · ") + " · esc cancel"
		case commentActionDeleteConfirm:
			text = "Delete this comment? enter confirm · esc cancel"
		default:
			if menu.Author == m.Viewer && m.Viewer != "" {
				text += " · e edit · d delete"
			}
		}
		return m.styleLine(classWarning, clip(text, m.Width))
	}
	if m.ActionError != nil {
		return m.styleLine(classWarning, clip("! Action failed: "+Escape(m.ActionError.Error())+" · ?: Health & help", m.Width))
	}
	if m.ReviewSubmitted {
		return m.styleLine(classTitle, clip("✓ Review submitted on GitHub · ?: Health & help", m.Width))
	}
	hints := "j/k: files · enter: diff · m: mark file · ?: Help"
	switch {
	case m.selectedReviewView() == viewCommits:
		if m.Session.Commits != nil && !m.Session.Commits.Complete && m.Session.Commits.Status == "captured" {
			return m.styleLine(classWarning, clip(fmt.Sprintf("Showing first %d commits; more may exist. · ?: Help", len(m.Session.Commits.Entries)), m.Width))
		}
		hints = "j/k: commits · n/p: commit · enter/l: diff · esc/h: list · ?: Help"
		if m.commit.focus == paneDiff {
			hints = "j/k: scroll · n/p: commit · PgUp/PgDn: page · esc/h: list · ?: Help"
		}
	case m.selectedReviewView() == viewDescription:
		hints = "↑/↓: scroll · n: comment · D: discussions · enter: expand · ?: Help"
		if m.discussions.overviewFocus {
			hints = "↑/↓: scroll · enter: expand · n: comment · c: refresh · esc: back"
			entries := m.discussionEntries()
			if len(entries) > 0 {
				t := entries[m.discussions.selected]
				if t.Kind == "PR comment" {
					hints = "↑/↓: scroll · enter: expand · r: reply · n: comment · c: refresh · esc: back"
				} else if t.Kind != "Review" {
					hints = "↑/↓: scroll · enter: expand"
					if m.submitCommentAction != nil && m.discussionFilesReason(t) == "" {
						hints += " · r: reply"
					}
					if m.discussionFilesReason(t) == "" {
						hints += " · f: Files"
					}
					if m.overviewOriginalAvailable(t) {
						hints += " · o: original"
					}
					hints += " · c: refresh · esc: back"
				}
			}
		}
		if m.discussions.overviewFocus && m.discussions.overviewExpanded[m.discussions.selectedID] {
			hints = "enter: collapse · esc: collapse · ↑/↓: scroll · tab: activity · c: refresh"
			entries := m.discussionEntries()
			if m.discussions.overviewFocus && len(entries) > 0 {
				actions := m.overviewDiscussionActions(entries[max(0, min(m.discussions.selected, len(entries)-1))])
				if len(actions) > 0 {
					hints = strings.Join(actions, " · ") + " · " + hints
				}
			}
		}
	case m.Focus == paneDiff:
		hints = "↑/↓: scroll · enter: comment/menu · m: mark file · esc: list · ?: Help"
	case !m.Files && !m.Inventory:
		hints = "j/k: guides · tab: expand · enter: diff · ?: Help"
	case m.Inventory:
		hints = "j/k: units · enter: diff · m: mark file · ?: Help"
	}
	return m.styleLine(classMetadata, clip(hints, m.Width))
}
