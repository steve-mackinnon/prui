package tui

import (
	"fmt"
	"strings"
)

const maxTabs = 9

// tabStrip renders the workspace tabs as escaped, numbered text. Brackets
// identify the active tab without relying on terminal color.
func tabStrip(width int, labels []string, active int) string {
	if width <= 0 || len(labels) == 0 {
		return ""
	}
	if len(labels) > maxTabs {
		labels = labels[:maxTabs]
	}

	parts := make([]string, len(labels))
	for i, label := range labels {
		part := fmt.Sprintf("%d %s", i+1, Escape(label))
		if i == active {
			part = "[" + part + "]"
		}
		parts[i] = part
	}
	return clip(strings.Join(parts, "  "), width)
}
