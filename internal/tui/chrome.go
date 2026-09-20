package tui

import (
	"fmt"
	"strings"

	"pr-review/internal/guide"
	"pr-review/internal/review"
	"pr-review/internal/session"
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
	if s.Guides == nil || s.Guides.Status == guide.Unavailable {
		items = append(items, healthItem{text: "? Guides unavailable", severity: 1})
	} else {
		items = append(items, healthItem{text: fmt.Sprintf("✓ Guides %d", len(s.Guides.Items))})
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

func healthStatus(width int, s *review.Session) string {
	progress, items := reviewHealth(s)
	help := "?: Health & help"
	parts := []string{progress}
	severity := 0
	if width >= 100 {
		for _, item := range items {
			parts = append(parts, item.text)
			severity = max(severity, item.severity)
		}
	} else {
		highest := items[0]
		for _, item := range items[1:] {
			if item.severity > highest.severity {
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
	return styleLine(class, line)
}

func (m *Model) reviewStatus() string {
	if menu := m.CommentMenu; menu != nil {
		text := "Comment actions: r reply · a react · esc cancel"
		switch menu.mode {
		case commentActionReply:
			text = "Reply draft: " + Escape(menu.Draft) + " · enter submit · esc cancel"
		case commentActionReact:
			text = "React: +1, -1, laugh, confused, heart, hooray, rocket, eyes · esc cancel"
		case commentActionDeleteConfirm:
			text = "Delete this comment? enter confirm · esc cancel"
		default:
			if menu.Author == m.Viewer && m.Viewer != "" {
				text += " · d delete"
			}
		}
		return styleLine(classWarning, clip(text, m.Width))
	}
	if m.ActionError != nil {
		progress, _ := reviewHealth(m.Session)
		return styleLine(classWarning, clip(progress+" · ! Action failed: "+Escape(m.ActionError.Error())+" · ?: Health & help", m.Width))
	}
	return healthStatus(m.Width, m.Session)
}
