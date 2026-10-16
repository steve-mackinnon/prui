package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

// Actions are workspace UI state only. Merely inspecting this menu never runs
// its commands or changes the originating tab's reading state.
type actionMenuState struct {
	index int
}

type namedReviewAction struct {
	label, shortcut, unavailable string
	run                          func() tea.Cmd
}

func (m *Model) openActions() {
	if !m.actionsAvailable() {
		return
	}
	m.actions = actionMenuState{}
	m.push(pageActions)
}

func (m *Model) reviewActions() []namedReviewAction {
	local := func(f func()) func() tea.Cmd { return func() tea.Cmd { f(); return nil } }
	unavailable := func(ok bool, reason string) string {
		if ok {
			return ""
		}
		return reason
	}
	lifecycle := func(key string) func() tea.Cmd { return func() tea.Cmd { cmd, _ := m.lifecycleKey(key); return cmd } }
	actions := []namedReviewAction{
		{label: "Submit review", shortcut: "R", run: local(m.openReviewForm)},
		{label: "Discussions", shortcut: "D", run: local(m.openOverviewDiscussions)},
		{label: "Add PR comment", shortcut: "n", unavailable: unavailable(m.submitGeneralComment != nil, "PR comments unavailable"), run: func() tea.Cmd { m.openOverviewDiscussions(); cmd, _ := m.overviewKey("n"); return cmd }},
		{label: "View Overview", shortcut: "1", run: local(func() { m.selectReviewView(viewDescription) })},
		{label: "View Files", shortcut: "2", run: local(func() { m.selectReviewView(viewFiles) })},
		{label: "View Guide", shortcut: "3", run: local(func() { m.selectReviewView(viewGuide) })},
		{label: "View Commits", shortcut: "4", run: local(func() { m.selectReviewView(viewCommits) })},
		{label: "PR readiness and checks", shortcut: "Alt+R", run: func() tea.Cmd { m.push(pageReadiness); return m.refreshReadiness() }},
		{label: "PR lifecycle controls", run: func() tea.Cmd { m.push(pageLifecycle); return m.refreshLifecycle() }},
		{label: "Linked issues and reviewers", shortcut: "I", run: m.openIssueContext},
		{label: "Saved reviews", shortcut: "s", unavailable: unavailable(m.listSessions != nil, "session storage unavailable"), run: m.loadSessions},
		{label: "Switch pull request", shortcut: "P", run: m.openSwitcher},
		{label: "Generate guide", shortcut: "g", unavailable: unavailable(m.generateGuide != nil, "guide generation unavailable"), run: local(func() { m.resetGuideDraft(); m.guideFocus = 0; m.guideModelEditing = false; m.push(pageGuideConsent) })},
		{label: "New comparison", shortcut: "N", unavailable: unavailable(m.fresh != nil && m.store != nil, "new comparison unavailable"), run: lifecycle("N")},
		{label: "Changes since previous review", shortcut: "5", run: local(m.openIncremental)},
		{label: "Filter files by name", shortcut: "F", run: local(func() { m.selectReviewView(viewFiles); m.openFileFilter() })},
		{label: "Find code", shortcut: "/", run: func() tea.Cmd {
			if !m.diffReviewView() {
				m.selectReviewView(viewFiles)
			}
			return m.openSearch()
		}},
		{label: "Filter by commits", shortcut: "C", run: local(func() {
			if !m.diffReviewView() {
				m.selectReviewView(viewFiles)
			}
			m.openCommitFilter()
		})},
		{label: "Choose theme", shortcut: "t", run: local(m.openThemePicker)},
		{label: "Toggle side-by-side diff", shortcut: "S", run: local(func() {
			if !m.diffReviewView() {
				m.selectReviewView(viewFiles)
			}
			m.toggleDiffLayout()
		})},
		{label: "Toggle line numbers", shortcut: "L", run: local(func() {
			if m.selectedReviewView() == viewDescription {
				m.selectReviewView(viewFiles)
			}
			m.toggleLineNumbers()
		})},
		{label: "Toggle file grouping", shortcut: "B", run: local(func() { m.selectReviewView(viewFiles); m.groupFiles = !m.groupFiles; m.saveLayout() })},
		{label: "Collapse or reveal generated files", shortcut: "Alt+C", run: local(func() { m.selectReviewView(viewFiles); m.toggleGeneratedFiles() })},
		{label: "Evidence scope", shortcut: "e", run: local(func() { m.push(pageEvidence); m.Inventory, m.Focus = false, paneList })},
		{label: "Help and health", shortcut: "?", run: local(func() { m.push(pageHelp) })},
		{label: "GitHub URL", shortcut: "U", run: local(func() { m.push(pageURL) })},
		{label: "Refresh GitHub metadata", shortcut: "r", unavailable: unavailable(m.store != nil && m.reader != nil, "metadata refresh unavailable"), run: lifecycle("r")},
		{label: "Refresh discussions and comments", shortcut: "c", unavailable: unavailable(m.readDiscussions != nil || m.readComments != nil, "discussion refresh unavailable"), run: func() tea.Cmd { return tea.Batch(m.refreshDiscussions(), m.refreshComments()) }},
	}
	var reviewControls []namedReviewAction
	if m.diffReviewView() && !m.commitFilter.subset {
		reviewControls = append(reviewControls, namedReviewAction{label: "Mark selected file read or unread", shortcut: "m", unavailable: unavailable(m.store != nil && m.Selected >= 0 && m.Selected < len(m.Session.UnitFiles) && len(m.Session.Slices) > 0, "select a file with local progress storage"), run: lifecycle("m")})
		reviewControls = append(reviewControls, namedReviewAction{label: "Add file comment", shortcut: "Ctrl+F", unavailable: unavailable(m.Selected >= 0 && m.Selected < len(m.Session.UnitFiles), "select a file first"), run: m.openFileComposer})
	}
	if (m.diffReviewView() && !m.commitFilter.subset) || m.selectedReviewView() == viewCommits {
		target := m.commentSelectionTarget()
		valid := target != nil && source.ValidateReviewCommentTarget(*target) == nil
		compose := m.openCommentComposer
		if m.selectedReviewView() == viewCommits {
			compose = m.openCommitComposer
		}
		reviewControls = append(reviewControls, namedReviewAction{label: "Add line comment", unavailable: unavailable(valid, "select a commentable source line in the diff"), run: compose})
		reviewControls = append(reviewControls, namedReviewAction{label: "Start or cancel comment range", shortcut: "Ctrl+V", unavailable: unavailable(valid || m.rangeStart != nil, "select a commentable source line in the diff"), run: local(m.toggleCommentRange)})
		if m.diffReviewView() {
			reviewControls = append(reviewControls, namedReviewAction{label: "Compose suggestion", shortcut: "Ctrl+S", unavailable: unavailable(valid && target.Side == "RIGHT", "select right-side source lines in the diff"), run: m.openSuggestionComposer})
		}
	}
	actions = slices.Concat(actions[:3], reviewControls, actions[3:])
	if m.diffReviewView() && !m.commitFilter.subset && !m.Inventory && len(m.Session.Inventory.Files) > 0 {
		navigate := func(key string) func() tea.Cmd {
			return func() tea.Cmd {
				if m.codeNavigationKey(key) {
					return m.loadNavigationSource()
				}
				return nil
			}
		}
		actions = append(actions,
			namedReviewAction{label: "Expand or collapse file context", shortcut: "E", run: navigate("E")},
			namedReviewAction{label: "Restore compact diff", shortcut: "Ctrl+D", run: navigate("ctrl+d")},
		)
		if m.selectedReviewView() == viewFiles {
			actions = append(actions,
				namedReviewAction{label: "Show full OLD file", shortcut: "Alt+O", run: navigate("alt+o")},
				namedReviewAction{label: "Show full NEW file", shortcut: "Alt+N", run: navigate("alt+n")},
				namedReviewAction{label: "Toggle whitespace presentation", shortcut: "Ctrl+W", run: navigate("ctrl+w")},
			)
		}
	}
	if m.diffReviewView() {
		actions = append(actions, namedReviewAction{label: "Toggle full inventory", shortcut: "i", run: local(func() {
			m.Inventory = !m.Inventory
			m.Focus = paneList
			if !m.Inventory {
				m.syncRow(m.rows())
				if m.fileView() {
					m.file(0)
				}
			}
		})})
	}
	if m.selectedReviewView() == viewGuide && !m.Inventory && !m.commitFilter.subset {
		rows := m.navigable()
		if m.Row >= 0 && m.Row < len(rows) && (rows[m.Row].kind == guideRow || rows[m.Row].kind == sectionRow) {
			actions = append(actions, namedReviewAction{label: "Expand or collapse selected guide section", run: local(func() { m.toggle(m.navigable()) })})
		}
	}
	if m.commitFilter.subset {
		// Searches and filename filters cannot reinterpret a captured commit net diff.
		for i := range actions {
			switch actions[i].label {
			case "Filter files by name", "Find code", "Toggle full inventory", "Toggle file grouping", "Collapse or reveal generated files":
				actions[i].unavailable = "select All commits first"
			}
		}
	}
	return actions
}

func (m *Model) actionsKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.pop()
	case "up":
		m.actions.index = max(0, m.actions.index-1)
	case "down":
		m.actions.index = min(max(0, len(m.reviewActions())-1), m.actions.index+1)
	case "home":
		m.actions.index = 0
	case "end":
		m.actions.index = max(0, len(m.reviewActions())-1)
	case "enter":
		return m.activateAction()
	default:
		for i, action := range m.reviewActions() {
			shortcut := action.shortcut
			if strings.Contains(shortcut, "+") {
				shortcut = strings.ToLower(shortcut)
			}
			if shortcut != "" && shortcut == key.String() {
				m.actions.index = i
				return m.activateAction()
			}
		}
	}
	return nil
}

func (m *Model) activateAction() tea.Cmd {
	actions := m.reviewActions()
	if len(actions) == 0 {
		return nil
	}
	m.actions.index = min(max(0, m.actions.index), len(actions)-1)
	action := actions[m.actions.index]
	if action.unavailable != "" {
		return nil
	}
	m.pop()
	return action.run()
}

// These display toggles use the same cursor preservation as their shortcuts.
func (m *Model) toggleDiffLayout() {
	keepMatch := m.searchMatchAtCursor()
	restore := m.sourceCursorRestorer()
	target, id := m.cursorAnchor()
	m.layout = m.layout.toggled()
	m.saveLayout()
	m.restoreCursorAnchor(target, id)
	if restore != nil {
		restore()
	}
	if keepMatch {
		m.revealSearchMatch()
	}
}
func (m *Model) toggleGeneratedFiles() {
	keepMatch := m.searchMatchAtCursor()
	restore := m.sourceCursorRestorer()
	target, id := m.cursorAnchor()
	m.collapseGenerated = !m.collapseGenerated
	m.restoreCursorAnchor(target, id)
	if restore != nil {
		restore()
	}
	if keepMatch {
		m.revealSearchMatch()
	}
	m.saveLayout()
}

func (m *Model) actionsDimensions() (int, int) {
	if m.Width >= 20 && m.Height >= 8 {
		return min(96, m.Width-8), m.Height - 2
	}
	return max(1, m.Width), max(1, m.Height)
}
func (m *Model) actionsWindow(total, height int) (int, int) {
	count := min(total, max(1, height-3))
	index := min(max(0, m.actions.index), max(0, total-1))
	start := max(0, min(index-count+1, total-count))
	return start, count
}
func (m *Model) actionsBody() string {
	width, height := m.actionsDimensions()
	actions := m.reviewActions()
	start, count := m.actionsWindow(len(actions), height)
	lines := []string{"Actions"}
	if len(actions) == 0 {
		lines = append(lines, "No actions available")
	} else {
		for i := start; i < start+count; i++ {
			action := actions[i]
			text := action.label
			if action.shortcut != "" {
				text = "[" + action.shortcut + "] " + text
			}
			if action.unavailable != "" {
				text += " (unavailable)"
			}
			marker := "  "
			if i == min(max(0, m.actions.index), len(actions)-1) {
				marker = "› "
			}
			lines = append(lines, marker+text)
		}
	}
	detail := "Use the displayed hotkey"
	if len(actions) > 0 {
		index := min(max(0, m.actions.index), len(actions)-1)
		if reason := actions[index].unavailable; reason != "" {
			detail = "Unavailable: " + reason
		} else {
			detail = fmt.Sprintf("%d of %d actions", index+1, len(actions))
		}
	}
	lines = append(lines, detail, "Hotkey or ↑/↓ · Enter open · Esc close")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
}
func (m *Model) actionsView() string {
	return renderActionModal(m.Width, m.Height, m.actionModalBackground(), m.actionsBody(), m.modalSurface)
}
func (m *Model) actionsMouseClick(x, y int) tea.Cmd {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return nil
	}
	width, height := m.actionsDimensions()
	actions := m.reviewActions()
	start, count := m.actionsWindow(len(actions), height)
	left, top := 0, 0
	if m.Width >= 20 && m.Height >= 8 {
		left = max(0, (m.Width-width-4)/2) + 2
		top = max(0, (m.Height-(min(height, count+3)+2))/2) + 1
	}
	if x < left || x >= left+width {
		return nil
	}
	row := y - top - 1
	if row < 0 || row >= count {
		return nil
	}
	m.actions.index = start + row
	return m.activateAction()
}
