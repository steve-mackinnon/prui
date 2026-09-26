package tui

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/inventory"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/theme"
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

// reviewView is deliberately UI-local: frozen sessions record source review
// content, while an open workspace tab owns which surface the reviewer sees.
type reviewView uint8

const (
	viewChanges reviewView = iota
	viewDescription
	viewCommits
)

// diffLayout is a tab-local display preference. Its zero value deliberately
// keeps newly opened reviews in the existing unified representation.
type diffLayout uint8

const (
	diffLayoutUnified diffLayout = iota
	diffLayoutSideBySide
)

func (l diffLayout) toggled() diffLayout {
	if l == diffLayoutSideBySide {
		return diffLayoutUnified
	}
	return diffLayoutSideBySide
}

// The switcher keeps a bounded set of concurrently open reviews.
const maxTabs = 9

type workspaceTab struct {
	identity source.Identity
	title    string
	review   *reviewTabState
}

// descriptionRenderCache is tab-owned because each frozen review can have a
// different body and an independent Description scroll position. It retains
// ANSI lines exactly as produced by the Markdown adapter; navigation only
// slices them and must not parse the body again.
type descriptionRenderCache struct {
	body      string
	width     int
	themeName string
	lines     []string
	valid     bool
}

// reviewTabState is the reviewer-visible state that must travel with an open
// review. Window dimensions and services remain shared by the workspace.
type reviewTabState struct {
	savedCursorTarget                                    *source.ReviewCommentTarget
	savedCursorCommentID                                 int64
	CursorTarget, GuideCursorTarget                      map[int]source.ReviewCommentTarget
	Session                                              *review.Session
	ContextView                                          reviewView
	DescriptionScroll                                    int
	descriptionCache                                     descriptionRenderCache
	Err                                                  error
	Selected                                             int
	Row                                                  int
	Files                                                bool
	collapsed                                            expansion
	Scroll                                               map[int]int
	GuideScroll                                          map[int]int
	Cursor                                               map[int]int
	GuideCursor                                          map[int]int
	cursorActive                                         bool
	Horizontal                                           int
	layout                                               diffLayout
	guidePathOffset, guidePathPause, guidePathGeneration int
	Inventory                                            bool
	Focus                                                pane
	Stack                                                []page
	Loading                                              bool
	Busy                                                 bool
	ActionError                                          error
	Composer                                             *commentComposer
	Pending                                              []source.ReviewComment
	ReviewForm                                           *reviewForm
	ReviewSubmitted                                      bool
	CommentMenu                                          *commentActionMenu
	Comments                                             []source.ReviewComment
	CommentReactions                                     map[int64][]source.ReviewCommentReaction
	Viewer                                               string
	commentGeneration                                    uint64
	editorCursorVisible                                  bool
	editorCursorGeneration                               uint64
	notice                                               string
	loadingFrame                                         int
}

type page int

const (
	pageReview page = iota
	pageEvidence
	pageHelp
	pageURL
	pagePicker
	pageRepositoryPicker
	pagePullRequestPicker
	pageGuideConsent
	pageReviewSubmit
	pageQuitPending
	pageThemePicker
)

// commentComposer is deliberately tab-owned. Its target is copied from the
// immutable diff provenance when the reviewer opens the composer, so later
// navigation cannot silently retarget a draft.
type commentComposer struct {
	Target       source.ReviewCommentTarget
	Draft        string
	Cursor       int // rune offset, never a byte offset
	PendingIndex int // -1 for a new comment; otherwise edits a local pending draft
	generation   uint64
}

type commentActionMenu struct {
	CommentID  int64
	ReplyToID  int64 // GitHub permits replies only to the thread's root comment.
	Target     source.ReviewCommentTarget
	Author     string
	mode       commentActionMode
	Draft      string
	Reaction   string
	Cursor     int
	generation uint64
}

type commentActionMode uint8

const (
	commentActionPick commentActionMode = iota
	commentActionReply
	commentActionReact
	commentActionDeleteConfirm
)

type Model struct {
	CursorTarget, GuideCursorTarget map[int]source.ReviewCommentTarget
	railWidth                       int
	drag                            dividerDrag

	Session                                              *review.Session
	ContextView                                          reviewView
	DescriptionScroll                                    int
	descriptionCache                                     descriptionRenderCache
	Err                                                  error
	Selected                                             int
	Row                                                  int  // selected guide hierarchy row
	Files                                                bool // F: navigate the deterministic file plan instead of guides
	collapsed                                            expansion
	Scroll                                               map[int]int
	GuideScroll                                          map[int]int
	Cursor                                               map[int]int
	GuideCursor                                          map[int]int
	cursorActive                                         bool
	pendingCenter                                        bool
	helpScroll                                           int
	Width, Height, Horizontal                            int
	layout                                               diffLayout
	guidePathOffset, guidePathPause, guidePathGeneration int
	Inventory                                            bool
	Focus                                                pane
	Stack                                                []page
	Loading                                              bool
	Busy                                                 bool
	SessionPicker                                        pickerState
	RepositoryPicker                                     pickerState
	PullRequestPicker                                    pickerState
	ThemePicker                                          pickerState
	Entries                                              []session.Entry
	Repositories                                         []session.Repository
	PullRequests                                         []source.PullRequest
	SwitcherQuery                                        string
	ActionError                                          error
	Composer                                             *commentComposer
	Pending                                              []source.ReviewComment
	ReviewForm                                           *reviewForm
	ReviewSubmitted                                      bool
	CommentMenu                                          *commentActionMenu
	Comments                                             []source.ReviewComment
	CommentReactions                                     map[int64][]source.ReviewCommentReaction
	Viewer                                               string
	commentGeneration                                    uint64
	reactionEmoji                                        bool
	editorCursorVisible                                  bool
	editorCursorGeneration                               uint64
	store                                                *session.Store
	reader                                               review.MetadataReader
	fresh                                                FreshLoader
	worker                                               <-chan struct{}
	notice                                               string
	loadingFrame                                         int
	ctx                                                  context.Context
	cancel                                               context.CancelFunc
	load                                                 Loader
	notify                                               func(string)
	listPullRequests                                     PullRequestLoader
	openPullRequest                                      PullRequestOpener
	refreshPullRequest                                   PullRequestRefresher
	generateGuide                                        GuideLoader
	submitComment                                        CommentSubmitter
	submitReview                                         ReviewSubmitter
	readComments                                         CommentReader
	readViewer                                           ViewerReader
	submitCommentAction                                  CommentActionSubmitter
	cancelAction                                         context.CancelFunc
	actionCtx                                            context.Context
	listSessions                                         func() ([]session.Entry, error)
	listRepositories                                     func() ([]session.Repository, error)
	currentRepository                                    string
	currentCheckout                                      string
	tabs                                                 []workspaceTab
	activeTab                                            int
	theme                                                theme.Theme
	themeOverrides                                       map[theme.Token]string
	styles                                               map[lineClass]lipgloss.Style
	saveTheme                                            func(string) (theme.PersistResult, error)
	themeSelectionLocked                                 bool
}

type PullRequestLoader func(context.Context, string) ([]source.PullRequest, error)
type PullRequestOpener func(context.Context, string, source.Identity, func(string)) (*review.Session, error)
type PullRequestRefresher func(context.Context, *review.Session, func(string)) (*review.Session, error)

func newModel(parent context.Context) *Model {
	ctx, cancel := context.WithCancel(parent)
	terminal, err := theme.Resolve(theme.Terminal, nil)
	if err != nil {
		panic(err)
	}
	m := &Model{ctx: ctx, cancel: cancel, Scroll: map[int]int{}, GuideScroll: map[int]int{}, Cursor: map[int]int{}, GuideCursor: map[int]int{}, Files: true, collapsed: newExpansion(), Stack: []page{pageReview}, Width: 100, Height: 24, activeTab: -1, reactionEmoji: defaultEmojiSupport()}
	m.SetTheme(terminal)
	return m
}

// SetTheme replaces this model's presentation palette. It does not mutate
// any process-global style state, so concurrent models can use different
// resolved themes safely.
func (m *Model) SetTheme(t theme.Theme) {
	m.theme = t
	m.themeOverrides = t.Overrides()
	m.styles = stylesFor(t)
	m.descriptionCache = descriptionRenderCache{}
	for i := range m.tabs {
		m.tabs[i].review.descriptionCache = descriptionRenderCache{}
	}
}

// SetThemeSelectionSaver supplies the global preference writer used by the
// interactive picker. The caller owns configuration-path discovery; a model
// never reads a reviewed checkout or user configuration on its own.
func (m *Model) SetThemeSelectionSaver(save func(string) (theme.PersistResult, error)) {
	m.saveTheme = save
}

// SetThemeSelectionLocked preserves an explicit --theme choice for this
// process. The picker may still update the global preference for a later run.
func (m *Model) SetThemeSelectionLocked(locked bool) { m.themeSelectionLocked = locked }

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
	ctx := m.beginAction()
	return m.start(func() tea.Msg {
		n := m.notify
		if n == nil {
			n = func(string) {}
		}
		s, e := m.load(ctx, n)
		return Loaded{s, e}
	})
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() {
		if !m.mouseAvailable() {
			m.cancelMouseDrag()
		}
	}()
	switch v := msg.(type) {
	case guidePathTick:
		if v.generation != m.guidePathGeneration || !m.guidePathScrollEligible() {
			return m, nil
		}
		if m.guidePathPause > 0 {
			m.guidePathPause--
			return m, nextGuidePathTick(v.generation)
		}
		_, _, pathWidth, path := m.guidePathScrollTarget()
		last := max(0, visibleWidth(path)-pathWidth)
		if m.guidePathOffset >= last {
			m.guidePathOffset = 0
			m.guidePathPause = 2
			return m, nextGuidePathTick(v.generation)
		}
		m.guidePathOffset++
		if m.guidePathOffset >= last {
			m.guidePathPause = 2
		}
		return m, nextGuidePathTick(v.generation)
	case loadingTick:
		if !m.loadingModal().active {
			return m, nil
		}
		m.loadingFrame = (m.loadingFrame + 1) % loadingBarWidth
		if v.result != nil {
			return m, waitForLoading(v.result)
		}
		return m, nextLoadingTick()
	case editorCursorTick:
		if (m.Composer == nil && (m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply)) || v.generation != m.editorCursorGeneration {
			return m, nil
		}
		m.editorCursorVisible = !m.editorCursorVisible
		return m, nextEditorCursorTick(v.generation)
	case Loaded:
		m.Err = m.finishAction(v.Err)
		m.Loading = false
		if v.Err == nil && v.Session != nil {
			m.openReviewTab(v.Session)
		} else {
			m.Session = v.Session
		}
		m.begin()
		if v.Err == nil && v.Session != nil {
			return m, tea.Batch(m.refreshCommentsInBackground(), m.refreshOpenedPullRequest(m.activeTab, v.Session))
		}
	case ActionResult:
		v.Err = m.finishAction(v.Err)
		if v.Err == nil && v.Session != nil {
			m.Session = v.Session
			if v.Reset {
				m.Composer, m.ReviewForm, m.CommentMenu = nil, nil, nil
				m.Pending, m.Comments = nil, nil
				m.ReviewSubmitted = false
				m.Selected, m.Horizontal, m.Row = 0, 0, 0
				m.collapsed = newExpansion()
				m.Scroll = map[int]int{}
				m.GuideScroll = map[int]int{}
				m.Cursor = map[int]int{}
				m.GuideCursor = map[int]int{}
				m.CursorTarget, m.GuideCursorTarget = nil, nil
				m.cursorActive = false
				m.Stack = []page{pageReview}
				m.Focus = paneList
				m.begin()
			}
		}
	case CommentResult:
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.Err = m.finishAction(v.Err)
		m.applyCommentResult(v)
		if active != v.Target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
	case ReviewResult:
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.Err = m.finishAction(v.Err)
		m.applyReviewResult(v)
		if active != v.Target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
		if active == v.Target && v.Err == nil {
			return m, m.refreshComments()
		}
	case CommentActionResult:
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.Err = m.finishAction(v.Err)
		m.applyCommentActionResult(v)
		if active != v.Target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
	case CommentListResult:
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.Err = m.finishAction(v.Err)
		m.applyCommentListResult(v.Target, v.Generation, v.Comments, v.Err)
		if active != v.Target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
	case BackgroundCommentListResult:
		m.applyCommentListResult(v.Target, v.Generation, v.Comments, v.Err)
	case ViewerResult:
		v.Err = m.finishAction(v.Err)
		if v.Target == m.activeTab {
			if v.Err == nil {
				m.Viewer = v.Viewer.Login
			}
		} else if v.Target >= 0 && v.Target < len(m.tabs) && v.Err == nil {
			m.tabs[v.Target].review.Viewer = v.Viewer.Login
		}
	case PullRequestOpenResult:
		// Opening always starts in the fixed browser. If the reviewer changes
		// tabs while it runs, finish the worker without touching that review's
		// visible state; the completed PR is merely registered as another tab.
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.Err = m.finishAction(v.Err)
		if v.Err == nil && v.Session != nil {
			if active == v.Target && m.top() == pagePullRequestPicker {
				m.pop()
			}
			m.registerReviewTab(v.Session, active == v.Target)
			if active == v.Target {
				return m, tea.Batch(m.refreshCommentsInBackground(), m.refreshOpenedPullRequest(m.activeTab, v.Session))
			}
		}
		if active != v.Target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
	case PullRequestRefreshResult:
		if v.Err != nil || v.Session == nil || v.Target < 0 || v.Target >= len(m.tabs) {
			return m, nil
		}
		state := m.tabs[v.Target].review
		if state == nil || state.Session == nil || state.Session.ID != v.SessionID {
			return m, nil
		}
		if state.Session.Inventory.Comparison.InventoryID == v.Session.Inventory.Comparison.InventoryID {
			state.Session = v.Session
			if v.Target == m.activeTab {
				m.Session = v.Session
			}
			return m, nil
		}
		// A changed immutable comparison has a fresh session and must not inherit
		// reading progress or detail offsets from the old source.
		m.tabs[v.Target].review = newReviewTabState(v.Session)
		if v.Target == m.activeTab {
			m.restoreReviewTab(m.tabs[v.Target].review)
		}
		return m, m.refreshCommentsInBackground()
	case PullRequestListResult:
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.Err = m.finishAction(v.Err)
		if v.Err == nil && (m.Session == nil || m.Session.Inventory.Comparison.Metadata.Identity.Repository == v.Repository) {
			m.PullRequests = v.PullRequests
			m.PullRequestPicker = pickerState{}
			if m.Session == nil {
				m.push(pagePullRequestPicker)
			}
		}
		if active != v.Target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
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
	case tea.MouseMsg:
		return m, m.mouseUpdate(v)
	case tea.WindowSizeMsg:
		m.cancelMouseDrag()
		cursorTarget, cursorCommentID := m.cursorAnchor()
		m.Width = max(1, v.Width)
		m.Height = max(1, v.Height)
		m.restoreCursorAnchor(cursorTarget, cursorCommentID)
		// A split-preference resize can change the number of display rows above a
		// target. Keep its semantic cursor and saved reading offset intact across
		// the unified fallback instead of replacing it with a nearby visible row.
		if m.diffLayout() != diffLayoutSideBySide {
			m.cursorInViewport(1)
		}
		m.ensureReplyEditorVisible()
		return m, m.restartGuidePathScroll()
	case tea.KeyPressMsg:
		m.cancelMouseDrag()
		pendingCenter := m.pendingCenter
		m.pendingCenter = false
		if m.Busy && v.String() == "esc" && m.cancelAction != nil {
			m.cancelCurrentAction()
			return m, nil
		}
		editingReviewText := m.top() == pageReviewSubmit
		if (v.String() != "q" || editingReviewText) && v.String() != "ctrl+c" {
			if m.Busy && (m.top() != pagePullRequestPicker || m.Session == nil) {
				return m, nil
			}
			if m.Composer != nil {
				return m, m.commentComposerKey(v)
			}
			if m.CommentMenu != nil {
				return m, m.commentActionKey(v)
			}
			if m.top() == pageThemePicker {
				return m, m.themePickerKey(v.String())
			}
			if v.String() == "t" && m.themePickerAvailable() {
				m.openThemePicker()
				return m, nil
			}
			if p := m.top(); p != pageReview {
				return m, m.pageKey(p, v)
			}
			if v.String() == "z" && m.selectedReviewView() == viewChanges && m.Focus == paneDiff {
				if pendingCenter {
					m.centerCursor()
				} else {
					m.pendingCenter = true
				}
				return m, nil
			}
			if v.String() == "ctrl+p" {
				return m, m.openSwitcher()
			}
			if v.String() == "R" && m.Session != nil {
				m.openReviewForm()
				return m, nil
			}
			switch v.String() {
			case "1":
				m.selectReviewView(viewChanges)
				return m, nil
			case "2":
				m.selectReviewView(viewDescription)
				return m, nil
			case "3":
				m.selectReviewView(viewCommits)
				return m, nil
			}
			if v.String() == "v" {
				m.cycleReviewView(1)
				return m, nil
			}
			if v.String() == "V" {
				m.cycleReviewView(-1)
				return m, nil
			}
			if m.selectedReviewView() == viewDescription && m.descriptionKey(v.String()) {
				return m, nil
			}
			if m.selectedReviewView() != viewChanges && m.selectedReviewView() != viewDescription && v.String() != "?" && v.String() != "esc" {
				return m, nil
			}
			if cmd, handled := m.lifecycleKey(v.String()); handled {
				return m, cmd
			}
		}
		switch v.String() {
		case "q", "ctrl+c":
			if v.String() == "q" && m.unsentReviewDrafts() {
				m.push(pageQuitPending)
				return m, nil
			}
			m.cancel()
			return m, tea.Quit
		case "?":
			m.push(pageHelp)
		case "U":
			m.push(pageURL)
		case "i":
			m.Inventory = !m.Inventory
			m.Focus = paneList
			if !m.Inventory {
				m.syncRow(m.rows())
				if m.fileView() {
					m.file(0)
				}
			}
		case "F":
			m.Files, m.Inventory = true, false
			m.file(0)
		case "G":
			m.Files = false
			m.Inventory = false
			m.syncRow(m.rows())
		case "S":
			if m.selectedReviewView() == viewChanges {
				target, commentID := m.cursorAnchor()
				m.layout = m.layout.toggled()
				m.restoreCursorAnchor(target, commentID)
			}
		case "e":
			m.push(pageEvidence)
			m.Inventory, m.Focus = false, paneList
		case "esc":
			m.back()
		case "h", "ctrl+h":
			m.Focus = paneList
		case "l", "ctrl+l":
			m.focusSavedDetail()
		case "enter":
			if m.selectedReviewView() == viewDescription {
				return m, nil
			}
			if m.Focus == paneDiff {
				if m.openCommentActionMenu() {
					if m.Viewer == "" {
						return m, m.refreshViewer()
					}
					return m, nil
				}
				return m, tea.Batch(m.openCommentComposer(), m.restartGuidePathScroll())
			} else {
				m.focusDetail(m.navigable())
			}
		case "tab":
			// enter keeps main's meaning (focus the diff), so expansion gets
			// its own key rather than overloading one the reviewer already uses.
			if rows := m.navigable(); rows != nil {
				m.toggle(rows)
			}
		case "n":
			if m.fileView() && m.Focus == paneDiff {
				m.cursorActive = true
				m.moveCursor(1)
			} else {
				m.move(1)
			}
		case "p":
			if m.fileView() && m.Focus == paneDiff {
				m.cursorActive = true
				m.moveCursor(-1)
			} else {
				m.move(-1)
			}
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
			if m.fileView() {
				m.file(1)
			} else if m.Focus == paneDiff {
				m.cursorActive = true
				m.moveCursor(1)
			} else {
				m.move(1)
			}
		case "k":
			if m.fileView() {
				m.file(-1)
			} else if m.Focus == paneDiff {
				m.cursorActive = true
				m.moveCursor(-1)
			} else {
				m.move(-1)
			}
		case "J":
			m.scroll(diffStep)
		case "K":
			m.scroll(-diffStep)
		case "d":
			m.scroll(m.pageStep())
		case "u":
			m.scroll(-m.pageStep())
		case "pgdown":
			m.scroll(m.pageStep())
		case "pgup":
			m.scroll(-m.pageStep())
		case "right":
			m.Horizontal += 8
		case "left":
			m.Horizontal = max(0, m.Horizontal-8)
		case "home":
			m.setOffset(0)
			m.Horizontal = 0
			m.cursorInViewport(-1)
		}
		return m, m.restartGuidePathScroll()
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
		if tab.identity == identity {
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
		m.tabs = append(m.tabs, workspaceTab{identity: identity, title: m.knownPRTitle(identity), review: newReviewTabState(s)})
		return
	}
	m.saveActiveReview()
	m.tabs = append(m.tabs, workspaceTab{identity: identity, title: m.knownPRTitle(identity), review: newReviewTabState(s)})
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
	m.restoreReviewTab(m.tabs[index].review)
	return true
}

func newReviewTabState(s *review.Session) *reviewTabState {
	return &reviewTabState{
		Session: s, Scroll: map[int]int{}, GuideScroll: map[int]int{}, Cursor: map[int]int{}, GuideCursor: map[int]int{},
		ContextView: viewChanges, DescriptionScroll: 0, Files: true, collapsed: newExpansion(), Stack: []page{pageReview}, Focus: paneList,
	}
}

func (m *Model) saveActiveReview() {
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
		return
	}
	target, commentID := m.cursorAnchor()
	m.tabs[m.activeTab].review = &reviewTabState{
		savedCursorTarget: target, savedCursorCommentID: commentID,
		Session: m.Session, ContextView: m.ContextView, DescriptionScroll: m.DescriptionScroll, descriptionCache: m.descriptionCache, Err: m.Err, Selected: m.Selected, Row: m.Row, Files: m.Files,
		CursorTarget: m.CursorTarget, GuideCursorTarget: m.GuideCursorTarget,
		collapsed: m.collapsed, Scroll: m.Scroll, GuideScroll: m.GuideScroll, Cursor: m.Cursor, GuideCursor: m.GuideCursor,
		Horizontal: m.Horizontal, layout: m.layout, Inventory: m.Inventory, Focus: m.Focus, cursorActive: m.cursorActive,
		guidePathOffset: m.guidePathOffset, guidePathPause: m.guidePathPause, guidePathGeneration: m.guidePathGeneration,
		Stack: m.Stack, Loading: m.Loading, Busy: m.Busy,
		ActionError: m.ActionError, notice: m.notice, loadingFrame: m.loadingFrame,
		Composer: m.Composer, Pending: m.Pending, ReviewForm: m.ReviewForm, ReviewSubmitted: m.ReviewSubmitted, CommentMenu: m.CommentMenu, Comments: m.Comments, CommentReactions: m.CommentReactions, Viewer: m.Viewer, commentGeneration: m.commentGeneration,
		editorCursorVisible: m.editorCursorVisible, editorCursorGeneration: m.editorCursorGeneration,
	}
}

func (m *Model) restoreReviewTab(state *reviewTabState) {
	m.pendingCenter = false
	m.cancelMouseDrag()
	if state == nil {
		return
	}
	m.Session, m.ContextView, m.DescriptionScroll, m.descriptionCache, m.Err = state.Session, state.ContextView, state.DescriptionScroll, state.descriptionCache, state.Err
	m.CursorTarget, m.GuideCursorTarget = state.CursorTarget, state.GuideCursorTarget
	m.Selected, m.Row, m.Files = state.Selected, state.Row, state.Files
	m.collapsed, m.Scroll, m.GuideScroll, m.Cursor, m.GuideCursor = state.collapsed, state.Scroll, state.GuideScroll, state.Cursor, state.GuideCursor
	m.Horizontal, m.layout, m.Inventory, m.Focus = state.Horizontal, state.layout, state.Inventory, state.Focus
	m.cursorActive = state.cursorActive
	m.guidePathOffset, m.guidePathPause, m.guidePathGeneration = state.guidePathOffset, state.guidePathPause, state.guidePathGeneration
	m.Stack, m.Loading, m.Busy = state.Stack, state.Loading, state.Busy
	m.ActionError, m.notice, m.loadingFrame = state.ActionError, state.notice, state.loadingFrame
	m.Composer, m.Pending, m.ReviewForm, m.ReviewSubmitted, m.CommentMenu, m.Comments, m.CommentReactions, m.Viewer, m.commentGeneration = state.Composer, state.Pending, state.ReviewForm, state.ReviewSubmitted, state.CommentMenu, state.Comments, state.CommentReactions, state.Viewer, state.commentGeneration
	m.editorCursorVisible, m.editorCursorGeneration = state.editorCursorVisible, state.editorCursorGeneration
	m.restoreCursorAnchor(state.savedCursorTarget, state.savedCursorCommentID)
}

func (m *Model) selectedReviewView() reviewView {
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
		return viewChanges
	}
	return m.ContextView
}

func (m *Model) cycleReviewView(delta int) {
	m.cancelMouseDrag()
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
		return
	}
	m.ContextView = reviewView((int(m.selectedReviewView()) + delta + 3) % 3)
}

func (m *Model) selectReviewView(view reviewView) {
	m.cancelMouseDrag()
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
		return
	}
	m.ContextView = view
}

func (m *Model) push(p page) {
	m.cancelMouseDrag()
	if m.top() != p {
		if p == pageHelp {
			m.helpScroll = 0
		}
		m.Stack = append(m.Stack, p)
	}
}
func (m *Model) pop() {
	m.cancelMouseDrag()
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
func (m *Model) pageKey(p page, key tea.KeyPressMsg) tea.Cmd {
	k := key.String()
	switch p {
	case pagePicker:
		return m.pickerKey(k)
	case pageRepositoryPicker:
		return m.repositoryPickerKey(k)
	case pagePullRequestPicker:
		if k == "?" {
			m.push(pageHelp)
			return nil
		}
		return m.pullRequestPickerKey(k)
	case pageGuideConsent:
		return m.guideConsentKey(k)
	case pageReviewSubmit:
		return m.reviewFormKey(key)
	case pageQuitPending:
		if k == "esc" {
			m.pop()
		} else if k == "enter" {
			m.cancel()
			return tea.Quit
		}
		return nil
	case pageThemePicker:
		return m.themePickerKey(k)
	case pageHelp:
		m.helpKey(k)
	case pageURL, pageEvidence:
		if k == "esc" {
			m.pop()
		}
	}
	return nil
}

func (m *Model) openCommentComposer() tea.Cmd {
	if m.Focus != paneDiff || m.Composer != nil {
		return nil
	}
	cursor := m.cursor()
	if cursor < 0 {
		return nil
	}
	target := m.selectedDiffTarget()
	if target == nil || target.Path == "" || !utf8.ValidString(target.Path) {
		return nil
	}
	m.Composer = &commentComposer{Target: *target, PendingIndex: -1}
	for i, pending := range m.Pending {
		if pending.Target == *target {
			m.Composer.PendingIndex = i
			m.Composer.Draft = pending.Body
			m.Composer.Cursor = len([]rune(pending.Body))
			break
		}
	}
	m.editorCursorVisible = true
	m.editorCursorGeneration++
	m.ensureCursorVisible()
	return nextEditorCursorTick(m.editorCursorGeneration)
}

func (m *Model) openCommentActionMenu() bool {
	if m.Focus != paneDiff {
		return false
	}
	cursor := m.cursor()
	if cursor < 0 {
		return false
	}
	line := m.displayDetail()[cursor]
	if line.commentID <= 0 {
		return false
	}
	for _, comment := range m.Comments {
		if comment.ID == line.commentID {
			m.CommentMenu = &commentActionMenu{CommentID: comment.ID, ReplyToID: m.topLevelCommentID(comment.ID), Target: comment.Target, Author: comment.Author}
			m.ensureCursorVisible()
			return true
		}
	}
	return false
}

func (m *Model) topLevelCommentID(commentID int64) int64 {
	seen := map[int64]bool{}
	for commentID > 0 && !seen[commentID] {
		seen[commentID] = true
		for _, comment := range m.Comments {
			if comment.ID == commentID {
				if comment.ParentID == 0 {
					return comment.ID
				}
				commentID = comment.ParentID
				break
			}
		}
	}
	return 0
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
	if !m.Inventory {
		m.file(delta)
		return
	}
	if m.Session != nil && len(m.Session.Inventory.Units) > 0 {
		m.Selected = max(0, min(len(m.Session.Inventory.Units)-1, m.Selected+delta))
		m.Horizontal = 0
	}
}

func (m *Model) restartGuidePathScroll() tea.Cmd {
	m.guidePathOffset, m.guidePathPause = 0, 2
	m.guidePathGeneration++
	if m.guidePathScrollEligible() {
		return nextGuidePathTick(m.guidePathGeneration)
	}
	return nil
}

func (m *Model) guidePathScrollEligible() bool {
	_, _, pathWidth, path := m.guidePathScrollTarget()
	return pathWidth > 0 && visibleWidth(path) > pathWidth
}

func (m *Model) guidePathScrollTarget() (row, string, int, string) {
	if m.top() != pageReview || m.Session == nil || m.Inventory || m.Focus != paneList || len(m.Session.Inventory.Units) == 0 {
		return row{}, "", 0, ""
	}
	if m.Files {
		file := m.Session.UnitFiles[m.Selected]
		prefix := selectionMarker(true) + readMarker(m.Session, m.Session.Inventory.Files[file].ID)
		return row{}, prefix, m.listWidth() - visibleWidth(prefix), pathLabel(m.Session.Inventory.Files[file])
	}
	rows := m.rows()
	if m.Row < 0 || m.Row >= len(rows) || rows[m.Row].kind != portionRow {
		return row{}, "", 0, ""
	}
	r := rows[m.Row]
	prefix := selectionMarker(true) + strings.Repeat("  ", r.depth) + readMarker(m.Session, m.Session.Inventory.Files[r.file].ID)
	pathWidth := m.listWidth() - visibleWidth(prefix) - visibleWidth(guidePortionSuffix(m.Session, r))
	return r, prefix, pathWidth, pathLabel(m.Session.Inventory.Files[r.file])
}

func (m *Model) listWidth() int {
	if m.Width >= 100 {
		if m.railWidth == 0 {
			return min(36, m.Width/3)
		}
		return max(20, min(m.railWidth, min((m.Width-3)/2, m.Width-43)))
	}
	return m.Width
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
	if !m.Inventory {
		m.setOffset(m.clampOffset(m.fileOffset(f)))
		m.cursorInViewport(1)
	}
	m.Horizontal = 0
}
func (m *Model) scroll(delta int) {
	if m.Session == nil || len(m.Session.Inventory.Units) == 0 {
		return
	}
	// The clamp counts the same display lines the diff pane renders, so a diff
	// that already fits cannot be scrolled past its end.
	m.setOffset(m.clampOffset(m.offset() + delta))
	if m.fileView() {
		m.syncFileToOffset()
	}
	if m.Focus == paneDiff {
		m.cursorInViewport(delta)
	}
}

func (m *Model) clampOffset(offset int) int {
	last := max(0, len(m.displayDetail())-m.bodyHeight())
	return max(0, min(last, offset))
}

// focusDetail preserves a guide row's saved reading position, while section
// and file rows jump to their corresponding guide-detail file occurrence.
func (m *Model) focusDetail(rows []row) {
	if len(rows) > 0 {
		r := rows[max(0, min(len(rows)-1, m.Row))]
		if offset, ok := anchorForLayout(detailFor(m.Session, r.guide), r, m.sideBySideEnabled()); ok {
			m.setOffset(m.clampOffset(offset))
		}
	}
	m.Focus = paneDiff
	m.cursorInViewport(1)
	cursor := m.cursor()
	m.cursorActive = cursor >= m.offset() && cursor < m.offset()+m.bodyHeight()
}

// focusSavedDetail changes panes without changing the current reading position.
// Enter uses focusDetail because section and file rows deliberately jump to
// their corresponding guide-detail occurrence; l only reveals the diff.
func (m *Model) focusSavedDetail() {
	m.Focus = paneDiff
	cursor := m.cursor()
	m.cursorActive = cursor >= m.offset() && cursor < m.offset()+m.bodyHeight()
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

func (m *Model) fileView() bool {
	return m.Session != nil && !m.Inventory && len(m.rows()) == 0
}

// fileOffset finds a file boundary in the rendered stream, including comment
// overlays and side-by-side projection above it.
func (m *Model) fileOffset(file int) int {
	seen := 0
	for i, line := range m.displayDetail() {
		if line.Class != classFileHeader || !strings.HasPrefix(line.Text, "── ") {
			continue
		}
		if seen == file {
			return i
		}
		seen++
	}
	return 0
}

func (m *Model) syncFileToOffset() {
	m.syncFileToLine(m.offset())
}

func (m *Model) syncFileToLine(index int) {
	file := 0
	for i, line := range m.displayDetail() {
		if i > index {
			break
		}
		if line.Class == classFileHeader && strings.HasPrefix(line.Text, "── ") {
			file++
		}
	}
	file = max(0, min(len(m.Session.Slices)-1, file-1))
	if len(m.Session.Slices[file].Units) > 0 {
		m.Selected = m.Session.Slices[file].Units[0]
	}
}

func (m *Model) baseDetail() []diffLine {
	if guide, ok := m.activeGuide(); ok {
		return detailFor(m.Session, guide).lines
	}
	if m.fileView() {
		var lines []diffLine
		for f, slice := range m.Session.Slices {
			lines = append(lines, diffLine{styledLine: styledLine{Class: classFileHeader, Text: fileDivider(m.Session.Inventory.Files[f])}})
			for _, unit := range slice.Units {
				if m.Session.Inventory.Units[unit].Kind != inventory.FileMetadata {
					lines = append(lines, unitLines(m.Session, unit)...)
				}
			}
		}
		return lines
	}
	if m.Session == nil || m.Selected < 0 || m.Selected >= len(m.Session.Inventory.Units) {
		return nil
	}
	lines := unitLines(m.Session, m.Selected)
	if m.Session.Inventory.Units[m.Selected].Kind == inventory.TextHunk {
		file := m.Session.Inventory.Files[m.Session.UnitFiles[m.Selected]]
		return append([]diffLine{{styledLine: styledLine{classFileHeader, fileDivider(file)}}}, lines...)
	}
	return lines
}

// detail expands immutable diff targets into ephemeral overlay/editor rows.
// Only the original target rows retain target metadata, so navigation cannot
// accidentally select remote text or the draft editor.
func (m *Model) detail() []diffLine {
	base := m.baseDetail()
	lines := make([]diffLine, 0, len(base)+len(m.Comments)+2)
	for _, line := range base {
		lines = append(lines, line)
		if line.target == nil {
			continue
		}
		for _, comment := range m.Comments {
			if comment.Target == *line.target && comment.ParentID == 0 {
				lines = append(lines, m.reviewCommentThread(comment, 0)...)
			}
		}
		lines = append(lines, m.pendingLines(*line.target)...)
		if m.Composer != nil && m.Composer.Target == *line.target {
			lines = append(lines, m.inlineEditorLines()...)
		}
	}
	return lines
}

// displayDetail is the single source of truth for rendered-row navigation.
// Unified mode keeps its original line stream; split mode projects source rows
// before attaching full-width comment and editor overlays.
func (m *Model) displayDetail() []diffLine {
	if !m.sideBySideEnabled() {
		return m.detail()
	}
	return m.sideBySideDetail()
}

func (m *Model) sideBySideEnabled() bool {
	return m.diffLayout() == diffLayoutSideBySide && m.selectedReviewView() == viewChanges && m.Width >= sideBySideMinimumWidth
}

func (m *Model) diffLayout() diffLayout { return m.layout }

func (m *Model) sideBySideDetail() []diffLine {
	base := m.baseDetail()
	if guide, ok := m.activeGuide(); ok {
		base = detailFor(m.Session, guide).splitLines
	} else {
		base = projectSideBySideDetail(base)
	}
	lines := make([]diffLine, 0, len(base)+len(m.Comments)+2)
	for _, line := range base {
		lines = append(lines, line)
		if line.sideBySide == nil {
			continue
		}
		row := *line.sideBySide
		// A paired row can have comments on both sides. Keep their overlay
		// order stable: old/LEFT before new/RIGHT.
		for _, target := range rowTargets(row) {
			for _, comment := range m.Comments {
				if comment.Target == target && comment.ParentID == 0 {
					lines = append(lines, m.reviewCommentThread(comment, 0)...)
				}
			}
			lines = append(lines, m.pendingLines(target)...)
			if m.Composer != nil && m.Composer.Target == target {
				lines = append(lines, m.inlineEditorLines()...)
			}
		}
	}
	return lines
}

func rowTargets(row diffRow) []source.ReviewCommentTarget {
	targets := make([]source.ReviewCommentTarget, 0, 2)
	if row.old != nil && row.old.line != nil && row.old.line.target != nil {
		targets = append(targets, *row.old.line.target)
	}
	if row.new != nil && row.new.line != nil && row.new.line.target != nil {
		targets = append(targets, *row.new.line.target)
	}
	if row.full != nil && row.full.target != nil {
		targets = append(targets, *row.full.target)
	}
	return targets
}

// rowTarget is the sole cursor/comment-entry choice for a visible split row:
// GitHub's current/new (RIGHT) target wins, otherwise use deleted (LEFT).
func rowTarget(row diffRow) *source.ReviewCommentTarget {
	if row.new != nil && row.new.line != nil && row.new.line.target != nil {
		return row.new.line.target
	}
	if row.old != nil && row.old.line != nil && row.old.line.target != nil {
		return row.old.line.target
	}
	if row.full != nil {
		return row.full.target
	}
	return nil
}

func (m *Model) reviewCommentThread(comment source.ReviewComment, indent int) []diffLine {
	lines := m.reviewCommentLinesAt(comment, indent)
	for _, reply := range m.Comments {
		if reply.ParentID == comment.ID && reply.Target == comment.Target {
			lines = append(lines, m.reviewCommentThread(reply, indent+4)...)
		}
	}
	if menu := m.CommentMenu; menu != nil && menu.CommentID == comment.ID && menu.mode == commentActionReply {
		lines = append(lines, m.inlineReplyEditorLines(indent+4)...)
	}
	return lines
}

// reviewCommentLines keeps each untrusted remote comment visually attached to
// its anchor while making it clear that it is read-only overlay content.
func (m *Model) reviewCommentLines(comment source.ReviewComment) []diffLine {
	return m.reviewCommentLinesAt(comment, 0)
}

func (m *Model) reviewCommentLinesAt(comment source.ReviewComment, indent int) []diffLine {
	inner := m.overlayInnerWidth(indent)
	prefix := strings.Repeat(" ", 2+indent)
	border := prefix + "+" + strings.Repeat("-", inner+2) + "+"
	line := func(class lineClass, text string) diffLine {
		text += strings.Repeat(" ", max(0, inner-visibleWidth(text)))
		return diffLine{styledLine: styledLine{Class: class, Text: prefix + "| " + text + " |"}, commentID: comment.ID}
	}
	wrappedLines := func(class lineClass, text string) []diffLine {
		wrapped := ansi.Wrap(Escape(text), inner, "")
		lines := make([]diffLine, 0, strings.Count(wrapped, "\n")+1)
		for _, part := range strings.Split(wrapped, "\n") {
			lines = append(lines, line(class, part))
		}
		return lines
	}
	author := comment.Author
	if author == "" {
		author = "unknown"
	}
	lines := []diffLine{{styledLine: styledLine{Class: classMetadata, Text: border}, commentID: comment.ID}}
	lines = append(lines, wrappedLines(classMetadata, "@"+author)...)
	for _, body := range strings.Split(comment.Body, "\n") {
		lines = append(lines, wrappedLines(classPlain, body)...)
	}
	return append(lines, diffLine{styledLine: styledLine{Class: classMetadata, Text: m.commentBottomBorder(prefix, inner, comment.ID)}, commentID: comment.ID})
}

func (m *Model) commentBottomBorder(prefix string, inner int, commentID int64) string {
	counts := map[string]int{}
	for _, reaction := range m.CommentReactions[commentID] {
		counts[reaction.Content]++
	}
	chips := []string{}
	for _, content := range source.ReviewCommentReactions() {
		if counts[content] > 0 {
			chips = append(chips, "["+Escape(m.reactionLabel(content))+" "+fmt.Sprint(counts[content])+"]")
		}
	}
	inside := strings.Repeat("-", inner+2)
	if len(chips) > 0 {
		chipText := strings.Join(chips, " ")
		chipText = clip(chipText, inner+2)
		inside = chipText + strings.Repeat("-", max(0, inner+2-visibleWidth(chipText)))
	}
	return prefix + "+" + inside + "+"
}

func (m *Model) inlineReplyEditorLines(indent int) []diffLine {
	menu := m.CommentMenu
	if menu == nil {
		return nil
	}
	return m.inlineEditorLinesFor(menu.Draft, menu.Cursor, indent)
}

func (m *Model) inlineEditorLines() []diffLine {
	c := m.Composer
	if c == nil {
		return nil
	}
	return m.inlineEditorLinesFor(c.Draft, c.Cursor, 0)
}

// inlineEditorLinesFor is the one editor presentation shared by a new inline
// comment and an indented reply. Its caller owns only the target/action state.
func (m *Model) inlineEditorLinesFor(draft string, editorCursor, indent int) []diffLine {
	// A simple terminal-native box makes the editor a distinct input surface
	// without borrowing a target label or a footer from the surrounding diff.
	inner := m.overlayInnerWidth(indent)
	prefix := strings.Repeat(" ", 2+indent)
	border := prefix + "+" + strings.Repeat("-", inner+2) + "+"
	lines := []diffLine{{styledLine: styledLine{Class: classWarning, Text: border}}}
	runes := []rune(draft)
	cursor := max(0, min(len(runes), editorCursor))
	before, after := string(runes[:cursor]), string(runes[cursor:])
	cursorLine := strings.Count(before, "\n")
	beforeLine := before[strings.LastIndex(before, "\n")+1:]
	afterLine := after
	if i := strings.IndexByte(afterLine, '\n'); i >= 0 {
		afterLine = afterLine[:i]
	}
	for i, text := range strings.Split(draft, "\n") {
		content := Escape(text)
		if i == cursorLine && m.editorCursorVisible {
			content = Escape(beforeLine) + "▏" + Escape(afterLine)
		}
		content = clip(content, inner)
		content += strings.Repeat(" ", max(0, inner-visibleWidth(content)))
		lines = append(lines, diffLine{styledLine: styledLine{Class: classWarning, Text: prefix + "| " + content + " |"}})
	}
	if m.ActionError != nil {
		content := clip("! "+Escape(m.ActionError.Error()), inner)
		content += strings.Repeat(" ", max(0, inner-visibleWidth(content)))
		lines = append(lines, diffLine{styledLine: styledLine{Class: classWarning, Text: prefix + "| " + content + " |"}})
	}
	lines = append(lines, diffLine{styledLine: styledLine{Class: classWarning, Text: border}})
	for i := range lines {
		lines[i].editor = true
	}
	return lines
}

// overlayInnerWidth reserves the detail cursor gutter, indent, and overlay border.
func (m *Model) overlayInnerWidth(indent int) int {
	return max(8, m.detailWidth()-(2+indent)-6)
}

func (m *Model) detailWidth() int {
	if m.Width < 100 {
		return m.Width
	}
	return m.Width - m.listWidth() - 3
}

func (m *Model) offset() int {
	if guide, ok := m.activeGuide(); ok {
		return m.GuideScroll[guide]
	}
	if m.fileView() {
		return m.Scroll[-1]
	}
	return m.Scroll[m.Selected]
}

func (m *Model) setOffset(offset int) {
	if guide, ok := m.activeGuide(); ok {
		m.GuideScroll[guide] = offset
		return
	}
	if m.fileView() {
		m.Scroll[-1] = offset
	} else {
		m.Scroll[m.Selected] = offset
	}
}

// cursor is the selected commentable detail line. It is kept separately for
// every raw unit and guide so changing tabs or detail modes preserves review
// context without changing the list selection.
func (m *Model) cursor() int {
	detail := m.displayDetail()
	if len(detail) == 0 {
		return -1
	}
	stored, ok := m.cursorValue()
	if ok && stored >= 0 && stored < len(detail) && (detail[stored].target != nil || detail[stored].commentID > 0) {
		return stored
	}
	for i, line := range detail {
		if line.target != nil || line.commentID > 0 {
			m.setCursor(i)
			return i
		}
	}
	return -1
}

func (m *Model) cursorValue() (int, bool) {
	if guide, ok := m.activeGuide(); ok {
		if m.GuideCursor == nil {
			return 0, false
		}
		v, found := m.GuideCursor[guide]
		return v, found
	}
	if m.Cursor == nil {
		return 0, false
	}
	key := m.Selected
	if m.fileView() {
		key = -1
	}
	v, found := m.Cursor[key]
	return v, found
}

func (m *Model) setCursor(line int) {
	m.clearSelectedDiffTarget()
	if guide, ok := m.activeGuide(); ok {
		if m.GuideCursor == nil {
			m.GuideCursor = map[int]int{}
		}
		m.GuideCursor[guide] = line
		return
	}
	if m.Cursor == nil {
		m.Cursor = map[int]int{}
	}
	key := m.Selected
	if m.fileView() {
		key = -1
	}
	m.Cursor[key] = line
}

func (m *Model) cursorAnchor() (*source.ReviewCommentTarget, int64) {
	cursor := m.cursor()
	if cursor < 0 {
		return nil, 0
	}
	line := m.displayDetail()[cursor]
	if selected := m.selectedDiffTarget(); selected != nil {
		target := *selected
		return &target, line.commentID
	}
	return nil, line.commentID
}

// restoreCursorAnchor keeps a semantic selection stable while a resize or
// layout toggle changes the number of display rows above it.
func (m *Model) restoreCursorAnchor(target *source.ReviewCommentTarget, commentID int64) {
	if target == nil && commentID == 0 {
		return
	}
	for i, line := range m.displayDetail() {
		if target != nil && diffLineHasTarget(line, *target) {
			m.setCursor(i)
			m.setSelectedDiffTarget(target)
			return
		}
		if target == nil && commentID > 0 && line.commentID == commentID {
			m.setCursor(i)
			return
		}
	}
}

func (m *Model) moveCursor(delta int) {
	if delta == 0 {
		return
	}
	detail, current := m.displayDetail(), m.cursor()
	if current < 0 {
		return
	}
	targets := make([]int, 0, len(detail))
	for i, line := range detail {
		if line.target != nil || line.commentID > 0 {
			targets = append(targets, i)
		}
	}
	for i, target := range targets {
		if target == current {
			m.setCursor(targets[max(0, min(len(targets)-1, i+delta))])
			m.ensureCursorVisible()
			if m.fileView() {
				m.syncFileToLine(m.cursor())
			}
			return
		}
	}
}

// cursorInViewport replaces a cursor only when scrolling would hide it.
// Forward scrolling chooses the first available target in view; backward
// scrolling chooses the last, while stable cursors are left untouched.
func (m *Model) cursorInViewport(delta int) {
	current := m.cursor()
	start, end := m.offset(), m.offset()+m.bodyHeight()
	if current >= start && current < end {
		return
	}
	detail := m.displayDetail()
	if delta < 0 {
		for i := min(len(detail)-1, end-1); i >= start; i-- {
			if detail[i].target != nil || detail[i].commentID > 0 {
				m.setCursor(i)
				return
			}
		}
		return
	}
	for i := max(0, start); i < min(len(detail), end); i++ {
		if detail[i].target != nil || detail[i].commentID > 0 {
			m.setCursor(i)
			return
		}
	}
}

func (m *Model) ensureCursorVisible() {
	cursor := m.cursor()
	if cursor < 0 {
		return
	}
	offset := m.offset()
	if cursor < offset {
		m.setOffset(cursor)
		return
	}
	if cursor >= offset+m.bodyHeight() {
		m.setOffset(m.clampOffset(cursor - m.bodyHeight() + 1))
	}
}

// Keep the reply box in the detail viewport after opening or editing it.
// An editor taller than the viewport cannot fit, so show its final rows.
func (m *Model) ensureReplyEditorVisible() {
	if m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply {
		return
	}
	start, end := -1, -1
	for i, line := range m.displayDetail() {
		if line.editor {
			if start < 0 {
				start = i
			}
			end = i + 1
		}
	}
	if start < 0 {
		return
	}
	height, offset := m.bodyHeight(), m.offset()
	if end-start > height || end > offset+height {
		m.setOffset(m.clampOffset(end - height))
	} else if start < offset {
		m.setOffset(m.clampOffset(start))
	}
}

func (m *Model) centerCursor() {
	if cursor := m.cursor(); cursor >= 0 {
		m.setOffset(m.clampOffset(cursor - m.bodyHeight()/2))
		m.cursorActive = true
	}
}

// reviewFooterRows keeps viewport geometry in sync with status and shortcut rows.
func (m *Model) reviewFooterRows() int {
	if m.Session != nil && m.Height >= 10 {
		return 2
	}
	return 1
}

func (m *Model) bodyHeight() int {
	return max(1, m.Height-3-m.reviewFooterRows())
}

func (m *Model) pageStep() int { return max(1, m.bodyHeight()-1) }
func (m *Model) View() tea.View {
	var text string
	switch {
	case m.Loading:
		text = "Opening review..."
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
		case pageHelp:
			text = m.helpViewport()
		case pageURL:
			text = m.Session.Inventory.Comparison.Metadata.Identity.URL() + "\nOpen this URL in your browser.\nesc: back | q: quit"
		case pageGuideConsent:
			text = m.guideConsentView()
		case pageReviewSubmit:
			text = m.reviewFormView()
		case pageQuitPending:
			text = "Discard unsent review drafts and quit?\n\nenter: discard and quit · esc: keep reviewing"
		case pageThemePicker:
			text = m.themePickerView()
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
	if modal := m.loadingModal(); modal.active {
		text = renderLoadingModal(m.Width, m.Height, text, modal)
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
	if m.mouseAvailable() {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m *Model) loadingModal() loadingModal {
	title := "Working"
	if m.Loading {
		title = "Loading"
	}
	return loadingModal{
		active:     m.Loading || m.Busy,
		cancelable: m.cancelAction != nil,
		frame:      m.loadingFrame,
		notice:     m.notice,
		title:      title,
	}
}

func (m *Model) guideConsentView() string {
	return "Generate OpenAI guide?\n\nThis sends bounded pinned patches and repository evidence to OpenAI. Exclusions and credential-like content are withheld. The request uses store:false; your API key is not persisted.\n\nenter: send source and generate a new guided session | esc: cancel | q: quit"
}
func (m *Model) reviewView() string {
	return m.reviewViewForLayout(m.diffLayout() == diffLayoutSideBySide)
}

// reviewViewForLayout is the layout boundary used by the tab preference added
// in the next slice. Keeping the choice an argument here leaves this slice
// stateless while making its responsive fallback directly testable.
func (m *Model) reviewViewForLayout(preferSideBySide bool) string {
	s := m.Session
	title := m.workspaceIdentity() + "\n" + m.contextViewTabs()
	if m.selectedReviewView() != viewChanges {
		if m.selectedReviewView() == viewDescription {
			return title + "\n" + m.descriptionView() + "\n" + m.reviewStatus()
		}
		return title + "\n" + m.contextViewPlaceholder() + "\n" + m.reviewStatus()
	}
	if len(s.Inventory.Units) == 0 {
		return title + "\nEmpty comparison: no net tree changes.\n" + m.reviewStatus()
	}
	kind := s.Inventory.Units[m.Selected].Kind
	label := "File slices"
	if m.Inventory {
		label = "Full inventory"
	} else if !m.Files {
		label = "Guide"
	}
	rows := m.navigable()
	if rows != nil {
		label = "Guides"
	}
	unavailable := 0
	for _, u := range s.Inventory.Units {
		if u.Kind == "unavailable" {
			unavailable++
		}
	}
	headerClass := classTitle
	if unavailable > 0 {
		headerClass = classWarning
	}
	text := fmt.Sprintf("%s %d · %s [%s]", strings.ToUpper(label), len(s.Inventory.Files), pathLabel(s.Inventory.Files[s.UnitFiles[m.Selected]]), kind)
	if m.fileView() {
		text = fmt.Sprintf("FILES %d · file %d/%d", len(s.Inventory.Files), s.UnitFiles[m.Selected]+1, len(s.Inventory.Files))
	}
	if m.Inventory {
		text = fmt.Sprintf("%s %d · unit %d/%d [%s]", strings.ToUpper(label), len(s.Inventory.Units), m.Selected+1, len(s.Inventory.Units), kind)
	}
	if rows != nil {
		// The hierarchy interprets the change; progress does not follow it, and
		// saying so here keeps a section from looking independently completable.
		guide, _ := m.activeGuide()
		text = fmt.Sprintf("GUIDES %d · guide %d/%d · marking: whole files", len(s.Guides.Items), guide+1, len(s.Guides.Items))
	}
	useSideBySide := preferSideBySide && m.Width >= sideBySideMinimumWidth
	if preferSideBySide && !useSideBySide {
		text += " · side-by-side needs 160 columns"
	}
	focus := "files"
	if !m.Files && !m.Inventory {
		focus = "guides"
	}
	if m.Focus == paneDiff {
		focus = "diff"
	}
	header := m.reviewListTabs() + " · focus: " + focus + " · " + m.styleLine(headerClass, text)
	bodyHeight := m.bodyHeight()
	list, selectedRow := m.reviewListPresentation()
	detail := m.detail()
	if useSideBySide {
		if m.sideBySideEnabled() {
			detail = m.renderProjectedSideBySideDetail(m.displayDetail(), m.detailWidth(), m.Horizontal)
		} else {
			detail = m.renderSideBySideDetail(detail, m.detailWidth(), m.Horizontal)
		}
	}
	offset := min(m.offset(), max(0, len(detail)-1))
	detail = detail[offset:min(len(detail), offset+bodyHeight)]
	// Horizontal scrolling stays on unstyled text; styles are applied after clipping.
	// Split rows need the same stable cursor gutter as unified rows. Without it,
	// a focused comment target remains selectable but has no visible location.
	for i, line := range detail {
		marker := ""
		if m.cursorActive {
			marker = cursorMarker(offset+i == m.cursor())
		}
		if useSideBySide {
			if line.sideBySide != nil && line.sideBySide.full == nil {
				marker = ""
			}
			detail[i].Text = marker + line.Text
			if line.commentID > 0 && offset+i == m.cursor() {
				detail[i].Class = selectedClass(true)
			}
			continue
		}
		runes := []rune(line.Text)
		detail[i].Text = marker + string(runes[min(m.Horizontal, len(runes)):])
		if line.commentID > 0 && offset+i == m.cursor() {
			detail[i].Class = selectedClass(true)
		}
	}
	body := []string{}
	for row := 0; row < bodyHeight; row++ {
		left, right := "", ""
		class, leftClass := classPlain, classPlain
		if row < len(list) {
			left = list[row].text
			if list[row].row == selectedRow {
				leftClass = selectedClass(m.Focus == paneList)
			}
		}
		if row < len(detail) {
			right, class = detail[row].Text, detail[row].Class
		}
		if m.Width < 100 {
			if m.Focus == paneDiff {
				body = append(body, m.styleLine(class, clip(right, m.Width)))
			} else {
				body = append(body, m.styleLine(leftClass, clip(left, m.Width)))
			}
		} else {
			leftWidth := m.listWidth()
			left = clip(left, leftWidth)
			// Padding is measured on the clipped plain row, then the row is styled.
			padding := strings.Repeat(" ", max(0, leftWidth-visibleWidth(left)))
			body = append(body, m.styleLine(leftClass, left)+padding+m.styleLine(paneBorderClass(m.Focus == paneDiff), " │ ")+m.styleLine(class, clip(right, m.Width-leftWidth-3)))
		}
	}
	return title + "\n" + header + "\n" + strings.Join(body, "\n") + "\n" + m.reviewStatus()
}

// workspaceIdentity uses titles already fetched by the PR browser. Direct and
// offline opens still show their frozen repository and PR number without a fetch.
func (m *Model) workspaceIdentity() string {
	identity := m.Session.Inventory.Comparison.Metadata.Identity
	text := fmt.Sprintf("%s #%d", Escape(identity.Repository), identity.Number)
	if title := m.knownPRTitle(identity); title != "" {
		text += " · " + Escape(title)
	} else {
		for _, tab := range m.tabs {
			if tab.identity == identity && tab.title != "" {
				text += " · " + Escape(tab.title)
				break
			}
		}
	}
	hint := "ctrl+p: switch PR"
	if m.Width >= 60 {
		text = clip(text, m.Width-visibleWidth(hint)-3)
		text += strings.Repeat(" ", max(1, m.Width-visibleWidth(text)-visibleWidth(hint))) + hint
	}
	return m.styleLine(classTitle, clip(text, m.Width))
}

func (m *Model) knownPRTitle(identity source.Identity) string {
	for _, pr := range m.PullRequests {
		if pr.Identity == identity {
			return pr.Title
		}
	}
	return ""
}

func (m *Model) reviewListTabs() string {
	labels := m.reviewTabLabels()
	selected := 1
	if m.Files || m.Inventory {
		selected = 0
	}
	labels[selected] = m.styleLine(selectedClass(m.Focus == paneList), labels[selected])
	return strings.Join(labels, "  ")
}

func (m *Model) contextViewTabs() string {
	tabs := m.contextTabLabels()
	for i, tab := range tabs {
		class := classTitle
		if reviewView(i) == m.selectedReviewView() {
			class = selectedClass(true)
		}
		tabs[i] = m.styleLine(class, tab)
	}
	return strings.Join(tabs, "  ")
}

func (m *Model) contextViewPlaceholder() string {
	switch m.selectedReviewView() {
	case viewDescription:
		return "Description is not available in this review yet."
	case viewCommits:
		return "Commits are not available in this review yet."
	default:
		return ""
	}
}

func (m *Model) descriptionView() string {
	lines := m.descriptionLines()
	height := m.descriptionBodyHeight()
	m.DescriptionScroll = max(0, min(m.DescriptionScroll, max(0, len(lines)-height)))
	end := min(len(lines), m.DescriptionScroll+height)
	return m.styleLine(classTitle, "Description") + "\nFrozen from GitHub when this review opened.\n" + strings.Join(lines[m.DescriptionScroll:end], "\n")
}

func (m *Model) descriptionLines() []string {
	if m.Session.PullRequestDescription == nil {
		m.descriptionCache = descriptionRenderCache{}
		return []string{"Description was not captured for this session."}
	}
	if *m.Session.PullRequestDescription == "" {
		m.descriptionCache = descriptionRenderCache{}
		return []string{"No description provided."}
	}
	body := *m.Session.PullRequestDescription
	width := max(1, m.Width)
	if m.descriptionCache.valid && m.descriptionCache.body == body && m.descriptionCache.width == width && m.descriptionCache.themeName == m.theme.Name {
		return m.descriptionCache.lines
	}
	lines, err := renderDescriptionMarkdownForTheme(body, width, m.theme.Name)
	if err != nil {
		lines = []string{"Description could not be rendered."}
	}
	m.descriptionCache = descriptionRenderCache{body: body, width: width, themeName: m.theme.Name, lines: lines, valid: true}
	return lines
}

func (m *Model) descriptionBodyHeight() int { return max(1, m.Height-4-m.reviewFooterRows()) }

func (m *Model) descriptionKey(key string) bool {
	delta := 0
	switch key {
	case "esc":
		return true
	case "j", "down", "n":
		delta = 1
	case "k", "up", "p":
		delta = -1
	case "J", "shift+j":
		delta = diffStep
	case "K", "shift+k":
		delta = -diffStep
	case "d", "pgdown":
		delta = m.descriptionBodyHeight()
	case "u", "pgup":
		delta = -m.descriptionBodyHeight()
	case "home":
		m.DescriptionScroll = 0
		return true
	default:
		return false
	}
	m.DescriptionScroll = max(0, min(m.DescriptionScroll+delta, max(0, len(m.descriptionLines())-m.descriptionBodyHeight())))
	return true
}

// styledFooter paints the footer as chrome, or as a warning when the last
// action failed. Its wording is identical to footer().
func (m *Model) styledFooter() string {
	if m.ActionError != nil {
		return m.styleLine(classWarning, m.footer())
	}
	return m.styleLine(classTitle, m.footer())
}
