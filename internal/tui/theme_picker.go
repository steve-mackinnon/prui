package tui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/theme"
)

var errThemeSelectionNotSaved = errors.New("theme selection was not saved; active theme unchanged")

var themeDescriptions = map[string]string{
	theme.Terminal:     "Use terminal palette",
	theme.Light:        "Light backgrounds",
	theme.Dark:         "Dark backgrounds",
	theme.HighContrast: "Stronger contrast",
}

// themePickerAvailable limits theme controls to review surfaces and the three
// existing navigation pickers. Editors, comment actions, loading, help, and
// confirmations all retain exclusive ownership of their keys before this is
// reached by Update.
func (m *Model) themePickerAvailable() bool {
	if m.Busy || m.Loading || m.Composer != nil || m.CommentMenu != nil {
		return false
	}
	switch m.top() {
	case pageReview, pagePicker, pageRepositoryPicker, pagePullRequestPicker:
		return true
	default:
		return false
	}
}

func (m *Model) openThemePicker() {
	names := theme.BuiltInNames()
	m.ThemePicker.Index = 0
	for i, name := range names {
		if name == m.theme.Name {
			m.ThemePicker.Index = i
			break
		}
	}
	m.push(pageThemePicker)
}

func (m *Model) themePickerKey(key string) tea.Cmd {
	switch key {
	case "t", "esc":
		m.pop()
	case "j", "down":
		m.ThemePicker.clamp(len(theme.BuiltInNames()))
		m.ThemePicker.Index = min(m.ThemePicker.Index+1, len(theme.BuiltInNames())-1)
	case "k", "up":
		m.ThemePicker.clamp(len(theme.BuiltInNames()))
		m.ThemePicker.Index = max(0, m.ThemePicker.Index-1)
	case "enter":
		m.applySelectedTheme()
	}
	return nil
}

func (m *Model) applySelectedTheme() {
	names := theme.BuiltInNames()
	m.ThemePicker.clamp(len(names))
	name := names[m.ThemePicker.Index]
	if m.saveTheme == nil {
		m.ActionError = errThemeSelectionNotSaved
		m.pop()
		return
	}
	result, err := m.saveTheme(name)
	if err != nil {
		m.ActionError = errThemeSelectionNotSaved
		m.pop()
		return
	}
	if result.DurabilityWarning != nil {
		m.notice = "Theme selection saved; its durability could not be confirmed"
	}
	if m.themeSelectionLocked {
		m.ActionError = nil
		m.pop()
		return
	}

	resolved, err := theme.Resolve(name, m.themeOverrides)
	if err != nil {
		// This should be unreachable because the old palette and built-ins were
		// validated before rendering. Keep the old palette if an invariant ever
		// regresses rather than applying a partial presentation state.
		m.ActionError = errThemeSelectionNotSaved
		m.pop()
		return
	}
	m.SetTheme(resolved)
	m.ActionError = nil
	m.pop()
}

func (m *Model) themePickerView() string {
	names := theme.BuiltInNames()
	m.ThemePicker.clamp(len(names))
	_, _, modalWidth, available := themePickerBounds(m.Width, m.Height)
	if !available {
		return m.compactThemePickerView(names)
	}

	inside := max(1, modalWidth-4)
	content := []string{
		"╭" + strings.Repeat("─", modalWidth-2) + "╮",
		modalLine("  Theme", inside),
	}
	for i, name := range names {
		marker := "  "
		if name == m.theme.Name {
			// The current resolved base has a persistent textual marker; the
			// selected candidate also keeps a distinct glyph when it differs.
			marker = "› "
		} else if i == m.ThemePicker.Index {
			marker = "> "
		}
		line := marker + name + " — " + themeDescriptions[name]
		if name == m.theme.Name {
			line += " (active)"
		}
		line = modalLine(line, inside)
		if i == m.ThemePicker.Index {
			line = m.styleLine(selectedClass(true), line)
		}
		content = append(content, line)
	}
	content = append(content,
		modalLine("", inside),
		modalLine("  j/k or ↑/↓: choose", inside),
		modalLine("  "+m.themePickerEnterHint(), inside),
		modalLine("  esc/t: cancel", inside),
		"╰"+strings.Repeat("─", modalWidth-2)+"╯",
	)

	return placeThemeModal(m.Width, m.Height, modalWidth, content)
}

func (m *Model) themePickerEnterHint() string {
	if m.themeSelectionLocked {
		return "enter: save for next launch"
	}
	return "enter: apply & save"
}

func (m *Model) compactThemePickerView(names []string) string {
	selected := names[m.ThemePicker.Index]
	lines := []string{
		clip("Theme: "+selected, m.Width),
		clip("↑↓/jk choose · "+m.themePickerEnterHint()+" · esc/t cancel", m.Width),
	}
	return strings.Join(lines[:min(len(lines), m.Height)], "\n")
}

func placeThemeModal(width, height, modalWidth int, content []string) string {
	lines := make([]string, height)
	left, startRow := themeModalOrigin(width, height, modalWidth, len(content))
	for i, line := range content {
		if startRow+i >= len(lines) {
			break
		}
		lines[startRow+i] = strings.Repeat(" ", left) + line
	}
	return strings.Join(lines, "\n")
}
