package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
)

type Loader func(context.Context, func(string)) (*review.Session, error)
type Loaded struct {
	Session *review.Session
	Err     error
}
type Notice string

type pane int

const (
	paneList pane = iota
	paneDiff
)

const diffStep = 5

// tabKind distinguishes the permanent PR browser from reviews opened in this
// process.
type tabKind int

const (
	tabPullRequests tabKind = iota
	tabReview
)

type workspaceTab struct {
	kind     tabKind
	identity source.Identity
	review   *reviewTabState
}

// reviewTabState is the reviewer-visible state that must travel with an open
// review. Window dimensions and services remain shared by the workspace.
type reviewTabState struct {
	Session     *review.Session
	Err         error
	Selected    int
	Row         int
	Files       bool
	collapsed   expansion
	Scroll      map[int]int
	GuideScroll map[int]int
	Horizontal  int
	Inventory   bool
	Focus       pane
	Stack       []page
	Loading     bool
	Busy        bool
	EditIndex   int
	ActionError error
	notice      string
}

type page int

const (
	pageReview page = iota
	pageEvidence
	pageAnalysis
	pageHelp
	pageURL
	pagePicker
	pageRepositoryPicker
	pagePullRequestPicker
	pageEdit
	pageReorder
	pageGuideConsent
)

type Model struct {
	Session                   *review.Session
	Err                       error
	Selected                  int
	Row                       int  // selected guide hierarchy row
	Files                     bool // G: navigate the deterministic file plan instead of guides
	collapsed                 expansion
	Scroll                    map[int]int
	GuideScroll               map[int]int
	Width, Height, Horizontal int
	Inventory                 bool
	Focus                     pane
	Stack                     []page
	Loading                   bool
	Busy                      bool
	EditIndex                 int
	SessionPicker             pickerState
	RepositoryPicker          pickerState
	PullRequestPicker         pickerState
	Entries                   []session.Entry
	Repositories              []session.Repository
	PullRequests              []source.PullRequest
	ActionError               error
	store                     *session.Store
	reader                    review.MetadataReader
	fresh                     FreshLoader
	worker                    <-chan struct{}
	notice                    string
	ctx                       context.Context
	cancel                    context.CancelFunc
	load                      Loader
	notify                    func(string)
	listPullRequests          PullRequestLoader
	openPullRequest           PullRequestOpener
	generateGuide             GuideLoader
	cancelAction              context.CancelFunc
	actionCtx                 context.Context
	listSessions              func() ([]session.Entry, error)
	listRepositories          func() ([]session.Repository, error)
	currentRepository         string
	currentCheckout           string
	tabs                      []workspaceTab
	activeTab                 int
}

type PullRequestLoader func(context.Context, string) ([]source.PullRequest, error)
type PullRequestOpener func(context.Context, string, source.Identity, func(string)) (*review.Session, error)

func newModel(parent context.Context) *Model {
	ctx, cancel := context.WithCancel(parent)
	return &Model{ctx: ctx, cancel: cancel, Scroll: map[int]int{}, GuideScroll: map[int]int{}, collapsed: newExpansion(), Stack: []page{pageReview}, Width: 100, Height: 24, tabs: []workspaceTab{{kind: tabPullRequests}}}
}

func New(parent context.Context, load Loader) *Model {
	m := newModel(parent)
	m.load, m.Loading = load, load != nil
	m.notice = "Loading GitHub metadata and pinned committed objects..."
	return m
}

// NewPullRequestBrowser starts with local repository selection and makes no
// network request until the user explicitly selects a repository. Init loads
// remembered repositories in a tracked worker, just like the other entry path.
func NewPullRequestBrowser(parent context.Context, store *session.Store, list PullRequestLoader, open PullRequestOpener) *Model {
	m := newModel(parent)
	m.Stack = []page{pageRepositoryPicker}
	m.SetLifecycle(store, nil, nil)
	m.SetPullRequestLifecycle(list, open)
	return m
}

// NewCurrentRepositoryBrowser starts directly at the open-PR list for one
// validated checkout. It does not consult remembered repositories.
func NewCurrentRepositoryBrowser(parent context.Context, store *session.Store, repository, checkout string, list PullRequestLoader, open PullRequestOpener) *Model {
	m := newModel(parent)
	m.Stack = []page{pagePullRequestPicker}
	m.currentRepository, m.currentCheckout = repository, checkout
	m.SetLifecycle(store, nil, nil)
	m.SetPullRequestLifecycle(list, open)
	return m
}
func (m *Model) SetNotifier(f func(string)) { m.notify = f }
func (m *Model) Init() tea.Cmd {
	if m.currentRepository != "" {
		return m.loadPullRequests(m.currentRepository)
	}
	if m.top() == pageRepositoryPicker {
		return m.loadRepositories()
	}
	if m.load == nil {
		return nil
	}
	return m.start(func() tea.Msg {
		n := m.notify
		if n == nil {
			n = func(string) {}
		}
		s, e := m.load(m.ctx, n)
		return Loaded{s, e}
	})
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case Loaded:
		m.Err = v.Err
		m.Loading = false
		m.Busy = false
		if v.Err == nil && v.Session != nil {
			m.openReviewTab(v.Session)
		} else {
			m.Session = v.Session
		}
		m.begin()
	case ActionResult:
		v.Err = m.finishAction(v.Err)
		if v.Err == nil && v.Session != nil {
			m.Session = v.Session
			if v.Reset {
				m.Selected, m.Horizontal, m.Row = 0, 0, 0
				m.collapsed = newExpansion()
				m.Scroll = map[int]int{}
				m.GuideScroll = map[int]int{}
				m.Stack = []page{pageReview}
				m.Focus = paneList
				m.begin()
			}
		}
	case PullRequestOpenResult:
		// Opening always starts in the fixed browser. If the reviewer changes
		// tabs while it runs, finish the worker without touching that review's
		// visible state; the completed PR is merely registered as another tab.
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.Err = m.finishAction(v.Err)
		if v.Err == nil && v.Session != nil {
			m.registerReviewTab(v.Session, active == v.Target)
		}
		if active != v.Target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
	case PullRequestListResult:
		v.Err = m.finishAction(v.Err)
		if v.Err == nil {
			m.PullRequests = v.PullRequests
			m.PullRequestPicker = pickerState{}
			m.push(pagePullRequestPicker)
		}
	case SessionListResult:
		if m.finishAction(v.Err) == nil {
			m.Entries = v.Entries
			m.SessionPicker.clamp(len(m.Entries))
		}
	case RepositoryListResult:
		if m.finishAction(v.Err) == nil {
			m.Repositories = v.Repositories
			m.RepositoryPicker.clamp(len(m.Repositories))
		}
	case Notice:
		m.notice = string(v)
	case tea.WindowSizeMsg:
		m.Width = max(1, v.Width)
		m.Height = max(1, v.Height)
	case tea.KeyPressMsg:
		if m.Busy && v.String() == "esc" && m.cancelAction != nil {
			m.cancelAction()
			return m, nil
		}
		previousTab := m.activeTab
		if m.workspaceKey(v.String()) {
			if m.activeTab == 0 && previousTab != 0 && m.currentRepository == "" {
				return m, m.loadRepositories()
			}
			return m, nil
		}
		if v.String() != "q" && v.String() != "ctrl+c" {
			if m.Busy {
				return m, nil
			}
			if p := m.top(); p != pageReview {
				return m, m.pageKey(p, v.String())
			}
			if cmd, handled := m.lifecycleKey(v.String()); handled {
				return m, cmd
			}
		}
		switch v.String() {
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "?":
			m.push(pageHelp)
		case "u":
			m.push(pageURL)
		case "i":
			m.Inventory = !m.Inventory
			m.Focus = paneList
			if !m.Inventory {
				m.syncRow(m.rows())
			}
		case "G":
			m.Files = !m.Files
			m.syncRow(m.rows())
		case "e":
			m.push(pageEvidence)
			m.Inventory, m.Focus = false, paneList
		case "a":
			m.push(pageAnalysis)
			m.Inventory, m.Focus = false, paneList
		case "v":
			m.beginMove()
		case "o":
			m.beginReorder()
		case "esc":
			m.back()
		case "ctrl+h":
			m.Focus = paneList
		case "ctrl+l", "enter":
			m.focusDetail(m.navigable())
		case "tab":
			// enter keeps main's meaning (focus the diff), so expansion gets
			// its own key rather than overloading one the reviewer already uses.
			if rows := m.navigable(); rows != nil {
				m.toggle(rows)
			}
		case "n":
			m.move(1)
		case "p":
			m.move(-1)
		case "]":
			m.file(1)
		case "[":
			m.file(-1)
		case "down":
			if m.Focus == paneDiff {
				m.scroll(1)
			} else {
				m.move(1)
			}
		case "up":
			if m.Focus == paneDiff {
				m.scroll(-1)
			} else {
				m.move(-1)
			}
		case "j":
			if m.Focus == paneDiff {
				m.scroll(1)
			} else {
				m.move(1)
			}
		case "k":
			if m.Focus == paneDiff {
				m.scroll(-1)
			} else {
				m.move(-1)
			}
		case "J":
			m.scroll(diffStep)
		case "K":
			m.scroll(-diffStep)
		case "pgdown":
			m.scroll(m.pageStep())
		case "pgup":
			m.scroll(-m.pageStep())
		case "right", "l":
			m.Horizontal += 8
		case "left", "h":
			m.Horizontal = max(0, m.Horizontal-8)
		case "home":
			m.setOffset(0)
			m.Horizontal = 0
		}
	}
	return m, nil
}

func (m *Model) top() page {
	if len(m.Stack) == 0 {
		return pageReview
	}
	return m.Stack[len(m.Stack)-1]
}

// openReviewTab registers an initial review as a workspace tab and makes it
// active. Lifecycle will own capacity and asynchronous opening in later
// slices; keeping duplicate activation here makes this primitive safe for
// callers that discover an already-open review.
func (m *Model) openReviewTab(s *review.Session) {
	m.registerReviewTab(s, true)
}

// registerReviewTab adds a completed review, optionally activating it. The
// latter is false for an opener result that completed after the user moved to
// another tab.
func (m *Model) registerReviewTab(s *review.Session, activate bool) {
	if s == nil {
		return
	}
	identity := s.Inventory.Comparison.Metadata.Identity
	for i, tab := range m.tabs {
		if tab.kind == tabReview && tab.identity == identity {
			if activate {
				m.activateTab(i)
			}
			return
		}
	}
	if len(m.tabs) >= maxTabs {
		return
	}
	if !activate {
		m.tabs = append(m.tabs, workspaceTab{kind: tabReview, identity: identity, review: newReviewTabState(s)})
		return
	}
	m.saveActiveReview()
	m.tabs = append(m.tabs, workspaceTab{kind: tabReview, identity: identity, review: newReviewTabState(s)})
	m.activeTab = len(m.tabs) - 1
	m.restoreReviewTab(m.tabs[m.activeTab].review)
}

func (m *Model) activateTab(index int) bool {
	if index < 0 || index >= len(m.tabs) {
		return false
	}
	if index == m.activeTab {
		return true
	}
	m.saveActiveReview()
	m.activeTab = index
	if tab := m.tabs[index]; tab.kind == tabReview {
		m.restoreReviewTab(tab.review)
		return true
	}
	// The permanent tab is the existing PR browser. Its picker data is already
	// workspace-wide; hiding the active review makes that browser visible.
	m.Session, m.Err = nil, nil
	m.Stack = []page{pagePullRequestPicker}
	return true
}

func newReviewTabState(s *review.Session) *reviewTabState {
	return &reviewTabState{
		Session: s, Scroll: map[int]int{}, GuideScroll: map[int]int{},
		collapsed: newExpansion(), Stack: []page{pageReview}, Focus: paneList,
	}
}

func (m *Model) saveActiveReview() {
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) || m.tabs[m.activeTab].kind != tabReview {
		return
	}
	m.tabs[m.activeTab].review = &reviewTabState{
		Session: m.Session, Err: m.Err, Selected: m.Selected, Row: m.Row, Files: m.Files,
		collapsed: m.collapsed, Scroll: m.Scroll, GuideScroll: m.GuideScroll,
		Horizontal: m.Horizontal, Inventory: m.Inventory, Focus: m.Focus,
		Stack: m.Stack, Loading: m.Loading, Busy: m.Busy, EditIndex: m.EditIndex,
		ActionError: m.ActionError, notice: m.notice,
	}
}

func (m *Model) restoreReviewTab(state *reviewTabState) {
	if state == nil {
		return
	}
	m.Session, m.Err = state.Session, state.Err
	m.Selected, m.Row, m.Files = state.Selected, state.Row, state.Files
	m.collapsed, m.Scroll, m.GuideScroll = state.collapsed, state.Scroll, state.GuideScroll
	m.Horizontal, m.Inventory, m.Focus = state.Horizontal, state.Inventory, state.Focus
	m.Stack, m.Loading, m.Busy, m.EditIndex = state.Stack, state.Loading, state.Busy, state.EditIndex
	m.ActionError, m.notice = state.ActionError, state.notice
}

// workspaceKey handles the keys reserved for the process-local workspace.
// It intentionally does not include "tab": that key remains guide expansion.
func (m *Model) workspaceKey(key string) bool {
	switch key {
	case "b":
		// Models constructed by older unit fixtures may have a Session assigned
		// directly, before it has become a review tab. Preserve their legacy
		// browser action until a real review tab exists.
		if len(m.tabs) == 1 {
			return false
		}
		return m.activateTab(0)
	case "t":
		if len(m.tabs) == 0 {
			return false
		}
		return m.activateTab((m.activeTab + 1) % len(m.tabs))
	case "T":
		if len(m.tabs) == 0 {
			return false
		}
		return m.activateTab((m.activeTab - 1 + len(m.tabs)) % len(m.tabs))
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return m.activateTab(int(key[0] - '1'))
	}
	return false
}

func (m *Model) push(p page) {
	if m.top() != p {
		m.Stack = append(m.Stack, p)
	}
}
func (m *Model) pop() {
	if len(m.Stack) > 1 {
		m.Stack = m.Stack[:len(m.Stack)-1]
	}
}
func (m *Model) back() {
	if len(m.Stack) > 1 {
		m.pop()
	} else if m.Focus == paneDiff {
		m.Focus = paneList
	}
}
func (m *Model) pageKey(p page, k string) tea.Cmd {
	switch p {
	case pagePicker:
		return m.pickerKey(k)
	case pageRepositoryPicker:
		return m.repositoryPickerKey(k)
	case pagePullRequestPicker:
		return m.pullRequestPickerKey(k)
	case pageEdit, pageReorder:
		return m.editKey(k)
	case pageGuideConsent:
		return m.guideConsentKey(k)
	case pageHelp, pageURL, pageEvidence, pageAnalysis:
		if k == "esc" {
			m.pop()
		}
	}
	return nil
}

// navigable returns the guide rows when they are the active hierarchy. The
// raw inventory view stays unit-by-unit, so `i` remains the complete source
// view rather than a second guide list.
func (m *Model) navigable() []row {
	if m.Inventory {
		return nil
	}
	rows := m.rows()
	if len(rows) == 0 {
		return nil
	}
	return rows
}
func (m *Model) move(delta int) {
	if rows := m.navigable(); rows != nil {
		m.moveRow(rows, delta)
		return
	}
	if m.Session != nil && len(m.Session.Inventory.Units) > 0 {
		m.Selected = max(0, min(len(m.Session.Inventory.Units)-1, m.Selected+delta))
		m.Horizontal = 0
	}
}
func (m *Model) file(delta int) {
	if rows := m.navigable(); rows != nil {
		m.moveGuide(rows, delta)
		return
	}
	if m.Session == nil || len(m.Session.Inventory.Units) == 0 {
		return
	}
	f := m.Session.UnitFiles[m.Selected]
	f = max(0, min(len(m.Session.Slices)-1, f+delta))
	m.Selected = m.Session.Slices[f].Units[0]
	m.Horizontal = 0
}
func (m *Model) scroll(delta int) {
	if m.Session == nil || len(m.Session.Inventory.Units) == 0 {
		return
	}
	// The clamp counts the same display lines the diff pane renders, so a diff
	// that already fits cannot be scrolled past its end.
	m.setOffset(m.clampOffset(m.offset() + delta))
}

func (m *Model) clampOffset(offset int) int {
	last := max(0, len(m.detail())-m.bodyHeight())
	return max(0, min(last, offset))
}

// focusDetail preserves a guide row's saved reading position, while section
// and file rows jump to their corresponding guide-detail file occurrence.
func (m *Model) focusDetail(rows []row) {
	if len(rows) > 0 {
		r := rows[max(0, min(len(rows)-1, m.Row))]
		if offset, ok := anchorFor(detailFor(m.Session, r.guide), r); ok {
			m.setOffset(m.clampOffset(offset))
		}
	}
	m.Focus = paneDiff
}

func (m *Model) activeGuide() (int, bool) {
	if m.Inventory {
		return 0, false
	}
	rows := m.rows()
	if len(rows) == 0 {
		return 0, false
	}
	return rows[max(0, min(len(rows)-1, m.Row))].guide, true
}

func (m *Model) detail() []styledLine {
	if guide, ok := m.activeGuide(); ok {
		return detailFor(m.Session, guide).lines
	}
	if m.Session == nil || m.Selected < 0 || m.Selected >= len(m.Session.Inventory.Units) {
		return nil
	}
	return unitLines(m.Session, m.Selected)
}

func (m *Model) offset() int {
	if guide, ok := m.activeGuide(); ok {
		return m.GuideScroll[guide]
	}
	return m.Scroll[m.Selected]
}

func (m *Model) setOffset(offset int) {
	if guide, ok := m.activeGuide(); ok {
		m.GuideScroll[guide] = offset
		return
	}
	m.Scroll[m.Selected] = offset
}

func (m *Model) bodyHeight() int {
	// The numbered workspace strip occupies the first row.
	n := m.Height - 5
	if m.Session != nil && m.Session.ID != "" {
		n = m.Height - 6
	}
	return max(1, n)
}

func (m *Model) pageStep() int { return max(1, m.bodyHeight()-1) }
func (m *Model) View() tea.View {
	text := ""
	switch {
	case m.Loading:
		text = "Loading\n" + Escape(m.notice) + "\nq / ctrl+c: cancel"
	case m.Err != nil:
		text = "Unable to open review\n" + Escape(m.Err.Error()) + "\nNo complete comparison available. q: quit"
	default:
		switch m.top() {
		case pagePicker:
			text = m.pickerView()
		case pageRepositoryPicker:
			text = m.repositoryPickerView()
		case pagePullRequestPicker:
			text = m.pullRequestPickerView()
		case pageEdit, pageReorder:
			text = m.editView()
		case pageHelp:
			text = "Keyboard\n" + renderBindings(groupHelp) + "\nControls and invalid bytes escaped. No mouse capture.\nReading progress is local, not GitHub approval.\nGuides interpret the diff; the raw inventory remains the complete source view.\nMarking any portion of a file marks its whole slice, under every guide.\nEvidence is pinned, bounded, and omissions are reported. Analysis is optional and consent-bound."
			if m.store != nil {
				text += "\nStorage: " + Escape(m.store.Path()) + "\nSession: " + m.Session.ID
			}
		case pageURL:
			text = m.Session.Inventory.Comparison.Metadata.Identity.URL() + "\nOpen this URL in your browser for GitHub review actions.\nesc: back | q: quit"
		case pageAnalysis:
			text = m.analysisView()
		case pageGuideConsent:
			text = m.guideConsentView()
		case pageEvidence:
			text = m.evidenceView()
		default:
			if m.Session == nil {
				text = "No review loaded. q: quit"
			} else {
				text = m.reviewView()
			}
		}
	}
	if strip := tabStrip(m.Width, m.tabLabels(), m.activeTab); strip != "" {
		text = strip + "\n" + text
	}
	lines := strings.Split(text, "\n")
	if len(lines) > m.Height {
		lines = lines[:m.Height]
	}
	for i := range lines {
		lines[i] = clip(lines[i], m.Width)
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func (m *Model) tabLabels() []string {
	labels := make([]string, 0, len(m.tabs))
	for _, tab := range m.tabs {
		if tab.kind == tabPullRequests {
			labels = append(labels, "PRs")
			continue
		}
		labels = append(labels, fmt.Sprintf("%s#%d", tab.identity.Repository, tab.identity.Number))
	}
	return labels
}

func (m *Model) guideConsentView() string {
	return "Generate OpenAI guide?\n\nThis sends bounded pinned patches and repository evidence to OpenAI. Exclusions and credential-like content are withheld. The request uses store:false; your API key is not persisted.\n\nenter: send source and generate a new guided session | esc: cancel | q: quit"
}
func (m *Model) reviewView() string {
	s := m.Session
	title := styleLine(statusClass(s), status(s))
	if s.ID != "" {
		title += "\n" + styleLine(progressClass(s), progress(s))
	}
	if len(s.Inventory.Units) == 0 {
		return title + "\nEmpty comparison: no net tree changes.\n" + m.styledFooter()
	}
	kind := s.Inventory.Units[m.Selected].Kind
	focus := "files"
	if m.Focus == paneDiff {
		focus = "diff"
	}
	label := "File slices"
	if m.Inventory {
		label = "Full inventory"
	}
	rows := m.navigable()
	if rows != nil {
		label = "Guides"
	}
	plan := s.CurrentPlan()
	unassigned, unavailable := 0, 0
	if plan != nil {
		unassigned = len(plan.UnassignedUnitIDs)
	}
	for _, u := range s.Inventory.Units {
		if u.Kind == "unavailable" {
			unavailable++
		}
	}
	headerClass := classTitle
	if unavailable > 0 {
		headerClass = classWarning
	}
	text := fmt.Sprintf("%s | focus: %s | unit %d/%d [%s] | unassigned %d | unavailable %d", label, focus, m.Selected+1, len(s.Inventory.Units), kind, unassigned, unavailable)
	if rows != nil {
		// The hierarchy interprets the change; progress does not follow it, and
		// saying so here keeps a section from looking independently completable.
		text += " | m marks the whole file slice"
		guide, _ := m.activeGuide()
		text += fmt.Sprintf(" | guide %d/%d", guide+1, len(s.Guides.Items))
	}
	header := styleLine(headerClass, text)
	bodyHeight := m.bodyHeight()
	leftWidth := m.Width
	if m.Width >= 100 {
		leftWidth = min(36, m.Width/3)
	}
	list := []listLine{}
	selectedRow := 0
	if m.Inventory {
		selectedRow = m.Selected
		for i, u := range s.Inventory.Units {
			marker := "  "
			if i == m.Selected {
				marker = "· "
				if m.Focus == paneList {
					marker = "> "
				}
			}
			list = append(list, listLine{row: i, text: marker + pathLabel(s.Inventory.Files[s.UnitFiles[i]]) + " [" + string(u.Kind) + "]"})
		}
	} else if rows != nil {
		selectedRow = max(0, min(len(rows)-1, m.Row))
		list = guideList(s, rows, selectedRow, leftWidth, m.Focus == paneList)
	} else {
		selectedRow = s.UnitFiles[m.Selected]
		for i, f := range s.Inventory.Files {
			marker := "  "
			if i == selectedRow {
				marker = "· "
				if m.Focus == paneList {
					marker = "> "
				}
			}
			list = append(list, listLine{row: i, text: marker + readMarker(s, f.ID) + pathLabel(f)})
		}
	}
	start := max(0, firstDisplayLine(list, selectedRow)-bodyHeight+1)
	list = list[start:min(len(list), start+bodyHeight)]
	detail := m.detail()
	offset := min(m.offset(), max(0, len(detail)-1))
	detail = detail[offset:min(len(detail), offset+bodyHeight)]
	// Horizontal scrolling stays on unstyled text; styles are applied after clipping.
	for i, line := range detail {
		runes := []rune(line.Text)
		detail[i].Text = string(runes[min(m.Horizontal, len(runes)):])
	}
	body := []string{}
	for row := 0; row < bodyHeight; row++ {
		left, right := "", ""
		class, leftClass := classPlain, classPlain
		if row < len(list) {
			left = list[row].text
			if list[row].row == selectedRow {
				// The focused pane's selection gets the stronger cue; the text
				// markers "> " and "· " already distinguish the two states.
				leftClass = classSelection
				if m.Focus == paneList {
					leftClass = classSelectionFocused
				}
			}
		}
		if row < len(detail) {
			right, class = detail[row].Text, detail[row].Class
		}
		if m.Width < 100 {
			if m.Focus == paneDiff {
				body = append(body, styleLine(class, clip(right, m.Width)))
			} else {
				body = append(body, styleLine(leftClass, clip(left, m.Width)))
			}
		} else {
			leftWidth := min(36, m.Width/3)
			left = clip(left, leftWidth)
			// Padding is measured on the clipped plain row, then the row is styled.
			padding := strings.Repeat(" ", max(0, leftWidth-visibleWidth(left)))
			body = append(body, styleLine(leftClass, left)+padding+" | "+styleLine(class, clip(right, m.Width-leftWidth-3)))
		}
	}
	return title + "\n" + header + "\n" + strings.Join(body, "\n") + "\n" + m.styledFooter()
}

// styledFooter paints the footer as chrome, or as a warning when the last
// action failed. Its wording is identical to footer().
func (m *Model) styledFooter() string {
	if m.ActionError != nil {
		return styleLine(classWarning, m.footer())
	}
	return styleLine(classTitle, m.footer())
}
