package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

// The footer owns the last row. At tiny heights, selection takes precedence.
type prPickerGeometry struct {
	header, listTop, listHeight, previewTop, previewHeight, leftWidth int
	split                                                             bool
}

func (m *Model) prPickerGeometry(headers int) prPickerGeometry {
	h := max(1, m.Height)
	g := prPickerGeometry{header: min(headers, max(0, h-2)), split: m.Width >= 100, leftWidth: max(1, (m.Width-3)*2/5)}
	available := max(1, h-g.header-1)
	g.listTop = g.header
	g.listHeight = available
	if g.split {
		g.previewTop, g.previewHeight = g.listTop, available
	} else if available >= 8 {
		g.previewHeight = min(7, available/2)
		g.listHeight = available - g.previewHeight - 1
		g.previewTop = g.listTop + g.listHeight + 1
	}
	return g
}

func (m *Model) prPickerRow(pr source.PullRequest) string {
	if m.Width < 100 {
		return m.pullRequestRow(pr)
	}
	// Keep the rail scannable; full metadata belongs to the preview.
	width := m.prPickerGeometry(1).leftWidth - 2
	suffix := m.prPickerSuffix(pr, true)
	titleWidth := max(1, width-visibleWidth(suffix))
	return m.paddedPRPickerTitle(fmt.Sprintf("#%d  %s", pr.Identity.Number, Escape(pr.Title)), titleWidth) + suffix
}

func (m *Model) selectedPickerPR() (source.PullRequest, bool) {
	index := m.PullRequestPicker.Index
	if m.Session != nil {
		results := m.switcherResults()
		if index < 0 || index >= len(results) {
			return source.PullRequest{}, false
		}
		for _, pr := range m.PullRequests {
			if pr.Identity == results[index].identity {
				return pr, true
			}
		}
		return source.PullRequest{}, false
	}
	if index < 0 || index >= len(m.PullRequests) {
		return source.PullRequest{}, false
	}
	return m.PullRequests[index], true
}

func (m *Model) prPickerDetail(pr source.PullRequest) []string {
	p := &m.PullRequestPicker
	if p.previewIdentity != pr.Identity {
		p.previewIdentity, p.previewScroll = pr.Identity, 0
	}
	width := m.Width
	if m.Width >= 100 {
		width = m.Width - m.prPickerGeometry(1).leftWidth - 3
	}
	width = max(1, width)
	lines := []string{fmt.Sprintf("#%d  %s", pr.Identity.Number, Escape(pr.Title))}
	lines = append(lines, m.pullRequestDetail(pr)...)
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Wrap(line, width, ""), "\n")...)
	}
	lines = append(wrapped, "DESCRIPTION")
	body := []string{"No description provided."}
	if pr.Description != "" {
		cache := &p.previewCache
		if !cache.valid || cache.body != pr.Description || cache.width != width || cache.themeName != m.theme.Name {
			rendered, err := renderDescriptionMarkdownWithTheme(pr.Description, width, m.theme)
			if err != nil {
				rendered = []string{"Description could not be rendered."}
			}
			for len(rendered) > 0 && strings.TrimSpace(rendered[0]) == "" {
				rendered = rendered[1:]
			}
			*cache = descriptionRenderCache{body: pr.Description, width: width, themeName: m.theme.Name, valid: true, lines: rendered}
		}
		body = p.previewCache.lines
	}
	return append(lines, body...)
}

func (m *Model) prPickerPreviewKey(key string) bool {
	p := &m.PullRequestPicker
	headers := 1
	if m.Session != nil {
		headers = 2
	}
	g := m.prPickerGeometry(headers)
	if g.previewHeight == 0 {
		p.previewFocus = false
		return false
	}
	if key == "tab" || key == "shift+tab" {
		p.previewFocus = !p.previewFocus
		return true
	}
	shiftScroll := key == "J" || key == "K" || key == "shift+j" || key == "shift+k"
	if !p.previewFocus && !shiftScroll {
		return false
	}
	if key == "esc" {
		p.previewFocus = false
		return true
	}
	delta := 0
	switch key {
	case "J", "shift+j":
		delta = diffStep
	case "K", "shift+k":
		delta = -diffStep
	case "down", "j":
		delta = 1
	case "up", "k":
		delta = -1
	case "pgdown":
		delta = max(1, g.previewHeight-1)
	case "pgup":
		delta = -max(1, g.previewHeight-1)
	case "home":
		p.previewScroll = 0
		return true
	case "end":
		// Jump to the exact bottom after resolving the current preview below.
	default:
		return false
	}
	if pr, ok := m.selectedPickerPR(); ok {
		lines := m.prPickerDetail(pr)
		limit := max(0, len(lines)-g.previewHeight)
		if key == "end" {
			p.previewScroll = limit
		} else {
			p.previewScroll = max(0, min(p.previewScroll+delta, limit))
		}
	}
	return true
}

func (m *Model) prPickerScreen(header, rows, detail []string, empty, footer string) string {
	g := m.prPickerGeometry(len(header))
	p := &m.PullRequestPicker
	if m.Busy {
		footer = Escape(m.notice) + " · esc: cancel"
	} else if m.ActionError != nil {
		footer = "! Action failed: " + Escape(m.ActionError.Error())
	} else {
		hints := "j/k: select · tab: preview · "
		if m.Width < 80 && m.Session == nil {
			hints = "tab: preview · "
		}
		if p.previewFocus {
			hints = "j/k: scroll · tab: list · "
		}
		footer = hints + footer
	}
	if m.Width >= 110 {
		footer += " · ?: Health & help"
	} else {
		footer += " · ?: help"
	}
	lines := append([]string(nil), header[:g.header]...)
	selected := max(0, min(p.Index, len(rows)-1))
	start := max(0, selected-g.listHeight+1)
	left := make([]string, g.listHeight)
	if len(rows) == 0 {
		if m.Busy {
			empty = Escape(m.notice)
		} else if m.ActionError != nil {
			empty = "Action failed: " + Escape(m.ActionError.Error())
		}
		left[0] = empty
	} else {
		for i := start; i < min(len(rows), start+g.listHeight); i++ {
			line := selectionMarker(i == selected) + rows[i]
			width := m.Width
			if g.split {
				width = g.leftWidth
			}
			line = clip(line, width)
			if i == selected {
				line = m.styleLine(selectedClass(false), line)
			}
			left[i-start] = line
		}
	}
	if len(detail) == 0 {
		detail = []string{"Select a pull request to preview."}
	}
	p.previewScroll = min(p.previewScroll, max(0, len(detail)-g.previewHeight))
	right := make([]string, g.previewHeight)
	for i := range right {
		n := p.previewScroll + i
		if n < len(detail) {
			right[i] = detail[n]
		}
	}
	if g.split {
		divider := " │ "
		if p.previewFocus {
			divider = m.styleLine(classTitle, divider)
		} else {
			divider = m.styleLine(classMetadata, divider)
		}
		for i, line := range left {
			line = clip(line, g.leftWidth)
			lines = append(lines, line+strings.Repeat(" ", max(0, g.leftWidth-visibleWidth(line)))+divider+clip(right[i], max(1, m.Width-g.leftWidth-3)))
		}
	} else {
		lines = append(lines, left...)
		if g.previewHeight > 0 {
			borderClass := classPaneBorder
			if p.previewFocus {
				borderClass = classPaneBorderFocused
			}
			lines = append(lines, m.styleLine(borderClass, strings.Repeat("─", max(0, m.Width))))
			for _, line := range right {
				lines = append(lines, clip(line, m.Width))
			}
		}
	}
	if m.Height > 1 {
		lines = append(lines, clip(footer, m.Width))
	}
	for i := range lines {
		lines[i] = clip(lines[i], m.Width)
	}
	return strings.Join(lines, "\n")
}
