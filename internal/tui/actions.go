package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

// Actions are workspace UI state only. Merely inspecting this menu never runs
// its commands or changes the originating tab's reading state.
type actionMenuState struct {
	query string
	index int
}

type namedReviewAction struct {
	label, shortcut, keywords, unavailable string
	run                                    func() tea.Cmd
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
		{label: "Submit review", shortcut: "R", keywords: "approve request changes comment pending", run: local(m.openReviewForm)},
		{label: "Discussions", shortcut: "D", keywords: "conversation comments threads", run: local(m.openOverviewDiscussions)},
		{label: "Add PR comment", keywords: "conversation general feedback", unavailable: unavailable(m.submitGeneralComment != nil, "PR comments unavailable"), run: func() tea.Cmd { m.openOverviewDiscussions(); cmd, _ := m.overviewKey("n"); return cmd }},
		{label: "View Overview", shortcut: "1", keywords: "description conversation", run: local(func() { m.selectReviewView(viewDescription) })},
		{label: "View Files", shortcut: "2", keywords: "changes diff", run: local(func() { m.selectReviewView(viewFiles) })},
		{label: "View Guide", shortcut: "3", keywords: "analysis", run: local(func() { m.selectReviewView(viewGuide) })},
		{label: "View Commits", shortcut: "4", run: local(func() { m.selectReviewView(viewCommits) })},
		{label: "PR readiness and checks", shortcut: "Alt+R", keywords: "reviews blockers ci", run: func() tea.Cmd { m.push(pageReadiness); return m.refreshReadiness() }},
		{label: "PR lifecycle controls", keywords: "merge close reopen draft ready auto queue", run: func() tea.Cmd { m.push(pageLifecycle); return m.refreshLifecycle() }},
		{label: "Linked issues and reviewers", shortcut: "I", keywords: "context linear", run: m.openIssueContext},
		{label: "Saved reviews", shortcut: "s", keywords: "sessions resume", unavailable: unavailable(m.listSessions != nil, "session storage unavailable"), run: m.loadSessions},
		{label: "Switch pull request", shortcut: "P", keywords: "open tabs repository", run: m.openSwitcher},
		{label: "Generate guide", shortcut: "g", keywords: "provider model upload analysis", unavailable: unavailable(m.generateGuide != nil, "guide generation unavailable"), run: local(func() { m.resetGuideDraft(); m.guideFocus = 0; m.guideModelEditing = false; m.push(pageGuideConsent) })},
		{label: "New comparison", shortcut: "N", keywords: "refresh source latest reconcile", unavailable: unavailable(m.fresh != nil && m.store != nil, "new comparison unavailable"), run: lifecycle("N")},
		{label: "Changes since previous review", shortcut: "5", keywords: "incremental draft outcomes", run: local(m.openIncremental)},
		{label: "Filter files by name", shortcut: "F", keywords: "path filename", run: local(func() { m.selectReviewView(viewFiles); m.openFileFilter() })},
		{label: "Find code", shortcut: "/", keywords: "search text", run: func() tea.Cmd {
			if !m.diffReviewView() {
				m.selectReviewView(viewFiles)
			}
			return m.openSearch()
		}},
		{label: "Filter by commits", shortcut: "C", keywords: "captured net diff", run: local(func() {
			if !m.diffReviewView() {
				m.selectReviewView(viewFiles)
			}
			m.openCommitFilter()
		})},
		{label: "Choose theme", shortcut: "t", keywords: "display appearance colors", run: local(m.openThemePicker)},
		{label: "Toggle side-by-side diff", shortcut: "S", keywords: "display unified split", run: local(func() {
			if !m.diffReviewView() {
				m.selectReviewView(viewFiles)
			}
			m.toggleDiffLayout()
		})},
		{label: "Toggle line numbers", shortcut: "L", keywords: "display", run: local(func() {
			if m.selectedReviewView() == viewDescription {
				m.selectReviewView(viewFiles)
			}
			m.toggleLineNumbers()
		})},
		{label: "Toggle file grouping", shortcut: "B", keywords: "display category", run: local(func() { m.selectReviewView(viewFiles); m.groupFiles = !m.groupFiles; m.saveLayout() })},
		{label: "Collapse or reveal generated files", shortcut: "Alt+C", keywords: "display", run: local(func() { m.selectReviewView(viewFiles); m.toggleGeneratedFiles() })},
		{label: "Evidence scope", shortcut: "e", keywords: "source limits provenance", run: local(func() { m.push(pageEvidence); m.Inventory, m.Focus = false, paneList })},
		{label: "Help and health", shortcut: "?", keywords: "keyboard shortcuts diagnostics getting started", run: local(func() { m.push(pageHelp) })},
		{label: "GitHub URL", shortcut: "U", keywords: "browser link", run: local(func() { m.push(pageURL) })},
		{label: "Refresh GitHub metadata", shortcut: "r", keywords: "freshness revision", unavailable: unavailable(m.store != nil && m.reader != nil, "metadata refresh unavailable"), run: lifecycle("r")},
		{label: "Refresh discussions and comments", shortcut: "c", keywords: "threads conversation", unavailable: unavailable(m.readDiscussions != nil || m.readComments != nil, "discussion refresh unavailable"), run: func() tea.Cmd { return tea.Batch(m.refreshDiscussions(), m.refreshComments()) }},
	}
	var reviewControls []namedReviewAction
	if m.diffReviewView() && !m.commitFilter.subset {
		reviewControls = append(reviewControls, namedReviewAction{label: "Mark selected file read or unread", shortcut: "m", keywords: "progress reviewed", unavailable: unavailable(m.store != nil && m.Selected >= 0 && m.Selected < len(m.Session.UnitFiles) && len(m.Session.Slices) > 0, "select a file with local progress storage"), run: lifecycle("m")})
		reviewControls = append(reviewControls, namedReviewAction{label: "Add file comment", shortcut: "Ctrl+F", keywords: "feedback", unavailable: unavailable(m.Selected >= 0 && m.Selected < len(m.Session.UnitFiles), "select a file first"), run: m.openFileComposer})
	}
	if (m.diffReviewView() && !m.commitFilter.subset) || m.selectedReviewView() == viewCommits {
		target := m.commentSelectionTarget()
		valid := target != nil && source.ValidateReviewCommentTarget(*target) == nil
		compose := m.openCommentComposer
		if m.selectedReviewView() == viewCommits {
			compose = m.openCommitComposer
		}
		reviewControls = append(reviewControls, namedReviewAction{label: "Add line comment", shortcut: "Enter", keywords: "feedback", unavailable: unavailable(valid, "select a commentable source line in the diff"), run: compose})
		reviewControls = append(reviewControls, namedReviewAction{label: "Start or cancel comment range", shortcut: "Ctrl+V", keywords: "multiline selection", unavailable: unavailable(valid || m.rangeStart != nil, "select a commentable source line in the diff"), run: local(m.toggleCommentRange)})
		if m.diffReviewView() {
			reviewControls = append(reviewControls, namedReviewAction{label: "Compose suggestion", shortcut: "Ctrl+S", keywords: "replacement code", unavailable: unavailable(valid && target.Side == "RIGHT", "select right-side source lines in the diff"), run: m.openSuggestionComposer})
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
			namedReviewAction{label: "Expand or collapse file context", shortcut: "E", keywords: "source display", run: navigate("E")},
			namedReviewAction{label: "Restore compact diff", shortcut: "Ctrl+D", keywords: "source display context", run: navigate("ctrl+d")},
		)
		if m.selectedReviewView() == viewFiles {
			actions = append(actions,
				namedReviewAction{label: "Show full OLD file", shortcut: "Alt+O", keywords: "source display", run: navigate("alt+o")},
				namedReviewAction{label: "Show full NEW file", shortcut: "Alt+N", keywords: "source display", run: navigate("alt+n")},
				namedReviewAction{label: "Toggle whitespace presentation", shortcut: "Ctrl+W", keywords: "source display", run: navigate("ctrl+w")},
			)
		}
	}
	if m.diffReviewView() {
		actions = append(actions, namedReviewAction{label: "Toggle full inventory", shortcut: "i", keywords: "units raw source", run: local(func() {
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
			actions = append(actions, namedReviewAction{label: "Expand or collapse selected guide section", keywords: "guide navigation", run: local(func() { m.toggle(m.navigable()) })})
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

func (m *Model) matchingActions() []namedReviewAction {
	query := strings.Fields(strings.ToLower(m.actions.query))
	actions := m.reviewActions()
	result := make([]namedReviewAction, 0, len(actions))
	for _, action := range actions {
		text := strings.ToLower(action.label + " " + action.keywords)
		matches := true
		for _, word := range query {
			if !strings.Contains(text, word) {
				matches = false
				break
			}
		}
		if matches {
			result = append(result, action)
		}
	}
	return result
}

func (m *Model) actionsInput(text string) tea.Cmd {
	remaining := 256 - utf8.RuneCountInString(m.actions.query)
	if remaining <= 0 {
		return nil
	}
	var b strings.Builder
	for _, r := range text {
		if remaining == 0 {
			break
		}
		if r == utf8.RuneError || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
		remaining--
	}
	if b.Len() != 0 {
		m.actions.query += b.String()
		m.actions.index = 0
	}
	return nil
}

func (m *Model) actionsKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.pop()
	case "up":
		m.actions.index = max(0, m.actions.index-1)
	case "down":
		m.actions.index = min(max(0, len(m.matchingActions())-1), m.actions.index+1)
	case "home":
		m.actions.index = 0
	case "end":
		m.actions.index = max(0, len(m.matchingActions())-1)
	case "enter":
		return m.activateAction()
	case "backspace":
		if m.actions.query != "" {
			_, size := utf8.DecodeLastRuneInString(m.actions.query)
			m.actions.query = m.actions.query[:len(m.actions.query)-size]
			m.actions.index = 0
		}
	default:
		if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
			return m.actionsInput(key.Text)
		}
	}
	return nil
}

func (m *Model) activateAction() tea.Cmd {
	actions := m.matchingActions()
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
	count := min(total, max(1, height-4))
	index := min(max(0, m.actions.index), max(0, total-1))
	start := max(0, min(index-count+1, total-count))
	return start, count
}
func (m *Model) actionsBody() string {
	width, height := m.actionsDimensions()
	actions := m.matchingActions()
	start, count := m.actionsWindow(len(actions), height)
	lines := []string{"Actions", "Find: " + m.actions.query}
	if len(actions) == 0 {
		lines = append(lines, "No matching actions")
	} else {
		for i := start; i < start+count; i++ {
			action := actions[i]
			text := action.label
			if action.shortcut != "" {
				text += "  [" + action.shortcut + "]"
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
	detail := "Type to filter"
	if len(actions) > 0 {
		index := min(max(0, m.actions.index), len(actions)-1)
		if reason := actions[index].unavailable; reason != "" {
			detail = "Unavailable: " + reason
		} else {
			detail = fmt.Sprintf("%d of %d actions", index+1, len(actions))
		}
	}
	lines = append(lines, detail, "↑/↓ select · Enter open · Esc close")
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
	actions := m.matchingActions()
	start, count := m.actionsWindow(len(actions), height)
	left, top := 0, 0
	if m.Width >= 20 && m.Height >= 8 {
		left = max(0, (m.Width-width-4)/2) + 2
		top = max(0, (m.Height-(min(height, count+4)+2))/2) + 1
	}
	if x < left || x >= left+width {
		return nil
	}
	row := y - top - 2
	if row < 0 || row >= count {
		return nil
	}
	m.actions.index = start + row
	return m.activateAction()
}
