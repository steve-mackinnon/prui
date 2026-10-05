package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"prui/internal/theme"
)

var errThemeSelectionNotSaved = errors.New("theme selection was not saved; active theme unchanged")

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
		m.moveThemeCandidate(1)
	case "k", "up":
		m.ThemePicker.clamp(len(theme.BuiltInNames()))
		m.moveThemeCandidate(-1)
	case "enter":
		m.applySelectedTheme()
	}
	return nil
}

func (m *Model) moveThemeCandidate(direction int) {
	rows := themePickerRows()
	selected := themePickerSelectedRow(rows, m.ThemePicker.Index)
	for i := selected + direction; i >= 0 && i < len(rows); i += direction {
		if rows[i].index >= 0 {
			m.ThemePicker.Index = rows[i].index
			return
		}
	}
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
	entries := theme.BuiltIns()
	names := theme.BuiltInNames()
	m.ThemePicker.clamp(len(names))
	_, _, modalWidth, available := themePickerBounds(m.Width, m.Height)
	if !available {
		return m.compactThemePickerView(names)
	}

	inside := max(1, modalWidth-4)
	content := []string{
		"╭" + strings.Repeat("─", modalWidth-2) + "╮",
		modalLine(fmt.Sprintf("  Theme · %s · %d/%d", themeGroupLabel(entries[m.ThemePicker.Index].Appearance), themePickerPosition(m.ThemePicker.Index), len(names)), inside),
	}
	window := themePickerLayout(m.Width, m.Height, m.ThemePicker.Index)
	rows := themePickerRows()
	for row := window.start; row < window.end; row++ {
		if rows[row].index < 0 {
			content = append(content, m.styleLine(classMetadata, modalLine("  ── "+rows[row].heading+" ──", inside)))
			continue
		}
		i := rows[row].index
		name := names[i]
		marker := "  "
		if name == m.theme.Name {
			// The current resolved base has a persistent textual marker; the
			// selected candidate also keeps a distinct glyph when it differs.
			marker = "› "
		} else if i == m.ThemePicker.Index {
			marker = "> "
		}
		line := marker + entries[i].DisplayName
		if name == m.theme.Name {
			line += " (active)"
		}
		line = modalLine(line, inside)
		if i == m.ThemePicker.Index {
			line = m.styleLine(selectedClass(true), line)
		}
		content = append(content, line)
	}
	if m.Width >= 60 && m.Height >= 18 {
		for _, line := range m.themeCandidateSample(names[m.ThemePicker.Index], inside) {
			content = append(content, "│ "+line+" │")
		}
	}
	content = append(content,
		modalLine("", inside),
		modalLine("  j/k or ↑/↓: choose", inside),
		modalLine("  "+m.themePickerEnterHint(), inside),
		modalLine("  esc/t: cancel", inside),
		"╰"+strings.Repeat("─", modalWidth-2)+"╯",
	)

	return placeThemeModal(m.Width, m.Height, modalWidth, strings.Split(m.modalSurface(strings.Join(content, "\n")), "\n"))
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
		clip(fmt.Sprintf("%d/%d %s: %s", themePickerPosition(m.ThemePicker.Index), len(names), themeGroupLabel(theme.BuiltIns()[m.ThemePicker.Index].Appearance), selected), m.Width),
		clip("↑↓/jk choose · "+m.themePickerEnterHint()+" · esc/t cancel", m.Width),
	}
	return m.modalSurface(strings.Join(lines[:min(len(lines), max(0, m.Height))], "\n"))
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

// Samples resolve a local palette; browsing never changes the active model.
func (m *Model) themeCandidateSample(name string, width int) []string {
	candidate, err := theme.Resolve(name, m.themeOverrides)
	if err != nil {
		return nil
	}
	styles := stylesFor(candidate)
	render := func(class lineClass, text string) string { return styles[class].Render(text) }
	lines := []string{
		render(classTitle, "Title") + "  " + render(classHunk, "@@ hunk @@") + "  " + render(classWarning, "warning"),
		"Plain text  " + render(classAdded, "+ added") + "  " + render(classRemoved, "- removed"),
		render(classPaneBorder, "│ border") + "  " + render(classPaneBorderFocused, "┃ focused") + "  " + render(classSelection, "› selected") + "  " + render(classSelectionFocused, "› focused"),
	}
	for i, line := range lines {
		lines[i] = clip(line, width)
		lines[i] += strings.Repeat(" ", max(0, width-visibleWidth(lines[i])))
	}
	if m.colorProfile > colorprofile.Ascii {
		return strings.Split(paintThemeCanvas(strings.Join(lines, "\n"), width, len(lines),
			themeBaseColor(candidate, theme.Foreground), themeBaseColor(candidate, theme.Background)), "\n")
	}
	return lines
}
