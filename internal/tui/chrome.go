package tui

import (
	"fmt"
	"sort"
	"strings"

	"pr-review/internal/guide"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
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
	if s.Guides == nil || s.Guides.Status == guide.Unavailable {
		items = append(items, healthItem{text: "? Guides unavailable", severity: 1})
	} else {
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
	if width < 50 {
		highest := items[0]
		for _, item := range items[1:] {
			if item.severity >= highest.severity {
				highest = item
			}
		}
		action := "R Review"
		if len(m.Pending) > 0 {
			action = fmt.Sprintf("R Review:%d", len(m.Pending))
		}
		status := highest.text
		if status == "! Inventory incomplete" && visibleWidth(strings.Join([]string{strings.TrimSuffix(progress, " read"), action, status}, " · ")) > width {
			status = "! Inventory gap"
		}
		class := classTitle
		if highest.severity >= 2 {
			class = classWarning
		}
		return m.styleLine(class, clip(strings.Join([]string{strings.TrimSuffix(progress, " read"), action, status}, " · "), width))
	}
	help := "?: Health & help"
	action := fmt.Sprintf("R Submit review (%d)", len(m.Pending))
	if width < 110 {
		action = fmt.Sprintf("R Review (%d)", len(m.Pending))
		help = "?: Help"
	}
	parts := []string{progress, action}
	severity := 0
	if width >= 100 {
		sort.SliceStable(items, func(i, j int) bool {
			priority := func(item healthItem) int {
				if item.severity >= 2 {
					return item.severity
				}
				return 0
			}
			return priority(items[i]) > priority(items[j])
		})
		for _, item := range items {
			candidate := append(append([]string(nil), parts...), item.text, help)
			if visibleWidth(strings.Join(candidate, " · ")) <= width {
				parts = append(parts, item.text)
				severity = max(severity, item.severity)
			}
		}
	} else {
		highest := items[0]
		for _, item := range items[1:] {
			if item.severity >= highest.severity {
				highest = item
			}
		}
		parts = append(parts, highest.text)
		severity = highest.severity
	}
	parts = append(parts, help)
	line := clip(strings.Join(parts, " · "), width)
	class := classTitle
	if severity >= 2 {
		class = classWarning
	}
	return m.styleLine(class, line)
}

func (m *Model) reviewStatus() string {
	if m.Composer != nil {
		return m.styleLine(classWarning, clip("enter: post now · ctrl+p: add to pending review · shift+enter: newline · esc: discard", m.Width))
	}
	if menu := m.CommentMenu; menu != nil {
		text := "Comment actions: r reply · a react · esc cancel"
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
				text += " · d delete"
			}
		}
		return m.styleLine(classWarning, clip(text, m.Width))
	}
	if m.ActionError != nil {
		progress, _ := reviewHealth(m.Session)
		return m.styleLine(classWarning, clip(progress+" · ! Action failed: "+Escape(m.ActionError.Error())+" · ?: Health & help", m.Width))
	}
	if m.ReviewSubmitted {
		progress, _ := reviewHealth(m.Session)
		return m.styleLine(classTitle, clip("✓ Review submitted on GitHub · "+progress+" · ?: Health & help", m.Width))
	}
	return m.healthStatus(m.Width, m.Session)
}
