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

func (m *Model) reviewHealth(s *review.Session) (string, []healthItem) {
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
	items = append(items, m.syntaxDiagnosticsFor(s).health...)
	return progress, items
}

func (m *Model) healthStatus(width int, s *review.Session) string {
	progress, items := m.reviewHealth(s)
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
		progress, items := m.reviewHealth(m.Session)
		text += "\n" + progress + fmt.Sprintf(" · %d pending comments", len(m.Pending))
		for _, item := range items {
			text += "\n" + item.text
		}
		text += m.syntaxHealthDetails(m.Session)
		if m.ActionError != nil {
			text += "\n! Action failed: " + Escape(m.ActionError.Error())
		}
	}
	if m.helpExpanded {
		text += "\n\n" + renderHealth()
		text += "\n\nCommits (4)\nArrows: select in the list or scroll the focused diff. Tab/Shift+Tab: switch panes.\nn/p: next/previous commit. Enter/l: focus diff; Escape/h: return to list.\n[/]: resize the commit rail at 100+ columns. Below 100 columns, focus selects the visible pane."
		text += "\n\nFiles\nF: filter by filename or path. Enter: keep filter and navigate. Escape: clear filter.\n/: find code text in Files or the selected Guide section.\nL: toggle line numbers in Files, Guide, and Commits."
		text += "\n\nMouse\nClick: select an item or diff cell; Enter: activate.\nClick view tabs to switch. Drag the file/diff divider to resize.\nScroll over a pane to move it. Drag readable text to copy; terminal selection modifiers also work."
	}
	text += "\n\nControls and invalid bytes escaped. Mouse selects; Enter activates.\nReading progress is local, not GitHub approval.\nGuides interpret the diff; the raw inventory remains the complete source view.\nMarking any portion of a file marks its whole slice, under every guide.\nEvidence is pinned, bounded, and omissions are reported. Guide generation is optional: choose Generate guide in Actions, then confirm the provider, model, and source upload."
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
		text := "Comment actions: r reply · a react · y copy URL · z resolve/reopen · esc cancel"
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
	hints := "↑/↓: files · Tab: panes · enter: diff · esc: back"
	switch {
	case m.selectedReviewView() == viewCommits:
		if m.Session.Commits != nil && !m.Session.Commits.Complete && m.Session.Commits.Status == "captured" {
			return m.styleLine(classWarning, clip(fmt.Sprintf("Space: Actions · Showing first %d commits; more may exist.", len(m.Session.Commits.Entries)), m.Width))
		}
		hints = "↑/↓: commits · Tab: panes · enter: diff · esc: back"
		if m.commit.focus == paneDiff {
			hints = "↑/↓: scroll · Tab: panes · enter: comment/menu · esc: list"
		}
	case m.selectedReviewView() == viewDescription:
		hints = "↑/↓: scroll · enter: expand · esc: back"
		if m.discussions.overviewFocus && m.discussions.overviewExpanded[m.discussions.selectedID] {
			hints = "↑/↓: scroll · enter: collapse · esc: collapse"
		}
	case m.Focus == paneDiff:
		hints = "↑/↓: scroll · Tab: panes · enter: comment/menu · esc: list"
	case !m.Files && !m.Inventory:
		hints = "↑/↓: guides · Tab: panes · enter: diff · esc: back"
	case m.Inventory:
		hints = "↑/↓: units · Tab: panes · enter: diff · esc: back"
	}
	// The header keeps Actions visible; reserve narrower footers for navigation.
	if m.Width < 48 {
		hints = "↑↓ · Tab · enter · esc: back"
	} else if m.Width < 90 {
		hints = strings.ReplaceAll(hints, "comment/menu", "comment")
	}
	if m.Width >= 90 {
		hints = "Space: Actions · " + hints
	}
	return m.styleLine(classMetadata, clip(hints, m.Width))
}
