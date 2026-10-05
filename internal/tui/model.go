package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/commits"
	"prui/internal/guideconfig"
	"prui/internal/inventory"
	"prui/internal/layoutprefs"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/theme"
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
	viewFiles reviewView = iota
	viewDescription
	viewCommits
	viewGuide
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
	freshnessGeneration uint64
	identity            source.Identity
	title               string
	review              *reviewTabState
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
	incremental                                          incrementalViewState
	draft                                                draftState
	issues                                               issueContextState
	navigation                                           codeNavigation
	search                                               [2]*diffSearchState
	readiness                                            readinessState
	lifecycle                                            lifecycleState
	discussions                                          discussionState
	commit                                               commitState
	commitFilter                                         commitFilterState
	rangeStart                                           *source.ReviewCommentTarget
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
	fileFilter                                           string
	fileFilterEditing                                    bool
	groupFiles, collapseGenerated                        bool
	Files                                                bool
	collapsed                                            expansion
	Scroll                                               map[int]int
	GuideScroll                                          map[int]int
	Cursor                                               map[int]int
	GuideCursor                                          map[int]int
	cursorActive                                         bool
	Horizontal                                           int
	listWidthPreference                                  int
	layout                                               diffLayout
	diffWrapCache                                        diffWrapCache
	guidePathOffset, guidePathPause, guidePathGeneration int
	Inventory                                            bool
	Focus                                                pane
	Stack                                                []page
	Loading                                              bool
	Busy                                                 bool
	ActionError                                          error
	SuggestionApply                                      *source.SuggestionApplication
	SuggestionConfirm                                    bool
	SuggestionScroll                                     int
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
	pageInbox
	pageInboxFilters
	pageRepositoryPicker
	pagePullRequestPicker
	pageGuideConsent
	pageReviewSubmit
	pageQuitPending
	pageThemePicker
	pageDiscussions
	pageDraftRecovery
	pageIncremental
	pageReadiness
	pageSuggestionApply
	pageIssueContext
	pageLifecycle
)

// commentComposer is deliberately tab-owned. Its target is copied from the
// immutable diff provenance when the reviewer opens the composer, so later
// navigation cannot silently retarget a draft.
type commentComposer struct {
	Target          source.ReviewCommentTarget
	CommitSHA       string
	CommitBundle    *commits.Bundle
	CommitInventory *inventory.Inventory
	Modes           *session.CommentModes
	Suggestion      bool
	Before          string
	Draft           string
	Cursor          int // rune offset, never a byte offset
	PendingIndex    int // -1 for a new comment; otherwise edits a local pending draft
	generation      uint64
}

type commentActionMenu struct {
	RootAnchor *source.ReviewCommentTarget
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
	reconcileDraft    DraftReconciler
	prepareSuggestion SuggestionPreparer
	applySuggestion   SuggestionCommitter
	guideCache        guideDetailCache
	fileCache         fileDetailCache
	inbox             inboxState
	*reviewTabState
	layoutPreferences     layoutprefs.Preferences
	saveLayoutPreferences func(layoutprefs.Preferences) error

	drag                 dividerDrag
	pendingCenter        bool
	helpScroll           int
	Width, Height        int
	SessionPicker        pickerState
	RepositoryPicker     pickerState
	PullRequestPicker    pickerState
	prCarousel           prCarouselState
	ThemePicker          pickerState
	Entries              []session.Entry
	Repositories         []session.Repository
	PullRequests         []source.PullRequest
	SwitcherQuery        string
	reactionEmoji        bool
	store                *session.Store
	reader               review.MetadataReader
	fresh                FreshLoader
	worker               <-chan struct{}
	ctx                  context.Context
	cancel               context.CancelFunc
	load                 Loader
	notify               func(string)
	listPullRequests     PullRequestLoader
	openPullRequest      PullRequestOpener
	refreshPullRequest   PullRequestRefresher
	generateGuide        GuideLoader
	guideDestination     string
	guideProvider        string
	guideModel           string
	guideStoreFalse      bool
	guideHasCredential   bool
	guideOptions         []guideconfig.Selection
	guideChoice          guideconfig.Selection
	guideConfirmed       guideconfig.Selection
	guideOptionsSet      bool
	guideFocus           int
	guideModelEditing    bool
	guideModelDrafts     map[string]string
	guideSave            func(guideconfig.Selection) error
	submitComment        CommentSubmitter
	submitReview         ReviewSubmitter
	submitGeneralComment GeneralCommentSubmitter
	readinessGeneration  uint64
	lifecycleGeneration  uint64
	readLifecycle        LifecycleRead
	submitLifecycle      LifecycleSubmit
	readReadiness        ReadinessReader
	readIssueContext     IssueContextReader
	readDiscussions      DiscussionReader
	readComments         CommentReader
	submitPublished      PublishedSubmitter
	readViewer           ViewerReader
	submitCommentAction  CommentActionSubmitter
	cancelAction         context.CancelFunc
	actionCtx            context.Context
	listSessions         func() ([]session.Entry, error)
	listRepositories     func() ([]session.Repository, error)
	currentRepository    string
	currentCheckout      string
	tabs                 []workspaceTab
	activeTab            int
	theme                theme.Theme
	themeOverrides       map[theme.Token]string
	styles               map[lineClass]lipgloss.Style
	saveTheme            func(string) (theme.PersistResult, error)
	themeSelectionLocked bool
	colorProfile         colorprofile.Profile
}

type PullRequestLoader func(context.Context, string) ([]source.PullRequest, error)
type PullRequestOpener func(context.Context, string, source.Identity, func(string)) (*review.Session, error)
type PullRequestRefresher func(context.Context, PullRequestRefreshRequest, func(string)) (PullRequestFreshness, error)

func newModel(parent context.Context) *Model {
	ctx, cancel := context.WithCancel(parent)
	terminal, err := theme.Resolve(theme.Terminal, nil)
	if err != nil {
		panic(err)
	}
	m := &Model{ctx: ctx, cancel: cancel, reviewTabState: newReviewTabState(nil), Width: 100, Height: 24, activeTab: -1, colorProfile: colorprofile.Unknown, reactionEmoji: defaultEmojiSupport()}
	m.SetTheme(terminal)
	return m
}

// SetTheme replaces this model's presentation palette. It does not mutate
// any process-global style state, so concurrent models can use different
// resolved themes safely.
func (m *Model) SetTheme(t theme.Theme) {
	m.PullRequestPicker.previewCache = descriptionRenderCache{}
	m.theme = t
	m.themeOverrides = t.Overrides()
	m.styles = stylesFor(t)
	if m.reviewTabState != nil {
		m.descriptionCache = descriptionRenderCache{}
	}
	for i := range m.tabs {
		if m.tabs[i].review != nil {
			m.tabs[i].review.descriptionCache = descriptionRenderCache{}
		}
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
	if m.inbox.initial {
		return m.loadInbox(m.inbox.refresh)
	}
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
	m.loadDraft(m.reviewTabState)
	defer m.persistDrafts()
	model, cmd := m.update(msg)
	carouselCmd := m.syncPRCarousel()
	if carouselCmd == nil {
		return model, cmd
	}
	return model, tea.Batch(cmd, carouselCmd)
}

func (m *Model) update(msg tea.Msg) (updated tea.Model, command tea.Cmd) {
	defer func() {
		if m.Session != nil && m.diffReviewView() && m.existingSearch() != nil {
			s := m.searchState()
			if s.scope != m.currentSearchScope() {
				command = tea.Batch(command, m.startSearch())
			}
		}
	}()
	defer func() {
		if !m.mouseAvailable() {
			m.cancelMouseDrag()
		}
	}()
	switch v := msg.(type) {
	case diffSearchResult:
		m.applySearchResult(v)
		return m, nil
	case tea.PasteMsg:
		if m.searchOpen() {
			return m, m.insertSearchText(v.Content)
		}
		return m, nil
	case tea.ColorProfileMsg:
		m.colorProfile = v.Profile
		return m, nil
	case GeneralCommentResult:
		m.applyGeneralCommentResult(v)
		return m, nil
	case commitFilterResult:
		m.applyCommitFilterResult(v)
		return m, nil
	case IssueContextResult:
		m.applyIssueContextResult(v)
		return m, nil
	case DiscussionResult:
		m.applyDiscussionResult(v)
		return m, nil
	case prCarouselTick:
		return m, m.advancePRCarousel(v)
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
		m.loadingFrame = (m.loadingFrame + 1) % loadingAnimationFrames
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
	case LifecycleResult:
		m.applyLifecycleResult(v)
	case ReadinessResult:
		m.applyReadinessResult(v)
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
			return m, tea.Batch(m.refreshDiscussions(), m.refreshCommentsInBackground(), m.refreshOpenedPullRequest(m.activeTab, v.Session))
		}
	case ActionResult:
		v.Err = m.finishAction(v.Err)
		if v.SaveErr != nil && v.Err == nil {
			m.ActionError = v.SaveErr
		}
		if v.Err == nil && v.Session != nil {
			m.Session = v.Session
			if v.Reset {
				m.notice = "New comparison: previous drafts remain with the previous saved comparison"
				if m.commitFilter.cancel != nil {
					m.commitFilter.cancel()
				}
				m.commitFilter = commitFilterState{}
				m.incremental = incrementalViewState{}
				for _, search := range m.search {
					if search != nil && search.cancel != nil {
						search.cancel()
					}
				}
				m.search = [2]*diffSearchState{}
				m.navigation = codeNavigation{}
				if m.issues.cancel != nil {
					m.issues.cancel()
				}
				m.issues = issueContextState{}
				if m.discussions.cancel != nil {
					m.discussions.cancel()
				}
				m.discussions = discussionState{}
				if m.readiness.cancel != nil {
					m.readiness.cancel()
				}
				if m.lifecycle.cancel != nil {
					m.lifecycle.cancel()
				}
				m.readiness = readinessState{}
				m.lifecycle = lifecycleState{}
				m.fileCache = fileDetailCache{}
				m.Composer, m.ReviewForm, m.CommentMenu = nil, nil, nil
				m.SuggestionApply = nil
				m.SuggestionConfirm, m.SuggestionScroll = false, 0
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
				m.loadDraft(m.reviewTabState)
				m.begin()
				return m, tea.Batch(m.refreshDiscussions(), m.refreshCommentsInBackground())
			}
		}
	case draftReconciled:
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.err = m.finishAction(v.err)
		m.applyDraftReconciled(v)
		if active != v.target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
	case suggestionPrepared:
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.err = m.finishAction(v.err)
		m.acceptSuggestionPrepared(v)
		if active != v.target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
	case suggestionApplied:
		active := m.activeTab
		busy, actionErr, notice := m.Busy, m.ActionError, m.notice
		v.err = m.finishAction(v.err)
		m.acceptSuggestionApplied(v)
		if active != v.target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
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
			return m, tea.Batch(m.refreshDiscussions(), m.refreshComments())
		}
	case PublishedResult:
		m.applyPublishedResult(v)
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
		active := m.activeTab
		busy, actionErr := m.Busy, m.ActionError
		v.Err = m.finishAction(v.Err)
		if active != v.Target {
			m.Busy, m.ActionError = busy, actionErr
		}
		if state := m.reviewStateForTarget(v.Target); state != nil {
			state.Busy, state.ActionError = false, v.Err
			if v.Err == nil {
				state.Viewer = v.Viewer.Login
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
			if active == v.Target && (m.top() == pagePullRequestPicker || m.top() == pageInbox) {
				m.pop()
			}
			m.registerReviewTab(v.Session, active == v.Target)
			if active == v.Target {
				if v.Frozen {
					return m, nil
				}
				return m, tea.Batch(m.refreshDiscussions(), m.refreshCommentsInBackground(), m.refreshOpenedPullRequest(m.activeTab, v.Session))
			}
		}
		if active != v.Target {
			m.Busy, m.ActionError, m.notice = busy, actionErr, notice
		}
	case PullRequestRefreshResult:
		if v.Foreground {
			v.Err = m.finishAction(v.Err)
		}
		if v.Err != nil || m.ctx.Err() != nil || v.Target < 0 || v.Target >= len(m.tabs) {
			return m, nil
		}
		state := m.tabs[v.Target].review
		if state == nil || state.Session == nil || m.tabs[v.Target].freshnessGeneration != v.Generation {
			return m, nil
		}
		current := state.Session
		if current == nil || current.ID != v.SessionID {
			return m, nil
		}
		keepDrafts := state.SuggestionApply != nil || state.discussions.published != nil || state.discussions.editor != nil || state.Composer != nil || len(state.Pending) > 0 || state.ReviewForm != nil || state.CommentMenu != nil || slices.Contains(state.Stack, pageGuideConsent)
		if v.Freshness.Session != nil && keepDrafts {
			// Keep local editing anchored to its frozen source; write preflights
			// still reject the stale comparison. Opening a new one stays explicit.
			v.Freshness = PullRequestFreshness{Status: session.Stale}
		}
		if v.Freshness.Session == nil {
			// Merge only freshness into the latest progress on its event-loop owner.
			if m.store != nil {
				updated, err := m.store.UpdateState(m.ctx, current.ID, current.Generation, current.SnapshotReference, session.StateUpdate{ReviewedSliceIDs: current.ReviewedSliceIDs, RevisionStatus: v.Freshness.Status})
				if err != nil {
					state.ActionError = err
					return m, nil
				}
				current.State = updated
			} else {
				current.RevisionStatus = v.Freshness.Status
			}
			return m, nil
		}
		// Changed comparisons never inherit progress or offsets from old source.
		if state.lifecycle.cancel != nil {
			state.lifecycle.cancel()
		}
		if state.readiness.cancel != nil {
			state.readiness.cancel()
		}
		if state.issues.cancel != nil {
			state.issues.cancel()
		}
		if state.discussions.cancel != nil {
			state.discussions.cancel()
		}
		m.tabs[v.Target].review = m.newDraftReviewTab(v.Freshness.Session)
		if v.Target == m.activeTab {
			m.restoreReviewTab(m.tabs[v.Target].review)
			return m, tea.Batch(m.refreshDiscussions(), m.refreshCommentsInBackground())
		}
		return m, nil
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
	case inboxResult, inboxReadResult, inboxOpenResult:
		return m, m.applyInboxMessage(v)
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
		if m.commitFilter.subset && m.diffReviewView() {
			m.Width, m.Height = max(1, v.Width), max(1, v.Height)
			return m, nil
		}
		if m.selectedReviewView() == viewCommits {
			m.Width, m.Height = max(1, v.Width), max(1, v.Height)
			m.commitRail()
			m.commitOffset()
			m.ensureCommitEditorVisible()
			return m, nil
		}
		keepSearchMatch := m.searchMatchAtCursor()
		sourceRestore := m.sourceCursorRestorer()
		cursorTarget, cursorCommentID := m.cursorAnchor()
		m.Width = max(1, v.Width)
		m.Height = max(1, v.Height)
		m.restoreCursorAnchor(cursorTarget, cursorCommentID)
		if sourceRestore != nil {
			sourceRestore()
		}
		// A split-preference resize can change the number of display rows above a
		// target. Keep its semantic cursor and saved reading offset intact across
		// the unified fallback instead of replacing it with a nearby visible row.
		if m.diffLayout() != diffLayoutSideBySide && cursorTarget == nil && cursorCommentID == 0 && sourceRestore == nil {
			m.cursorInViewport(1)
		}
		m.ensureReplyEditorVisible()
		if keepSearchMatch {
			m.revealSearchMatch()
		}
		return m, m.restartGuidePathScroll()
	case tea.KeyPressMsg:
		m.cancelMouseDrag()
		pendingCenter := m.pendingCenter
		m.pendingCenter = false
		if m.top() == pageInbox && v.String() != "ctrl+c" && v.String() != "q" {
			return m, m.inboxKey(v.String())
		}
		if m.Busy && v.String() == "esc" && m.cancelAction != nil {
			m.cancelCurrentAction()
			return m, nil
		}
		if m.top() == pageDraftRecovery && v.String() != "q" && v.String() != "ctrl+c" {
			return m, m.draftRecoveryKey(v)
		}
		if m.top() == pageQuitPending && v.String() != "ctrl+c" {
			// The modal owns Enter/Escape; an underlying composer must never
			// interpret discard confirmation as a remote comment submission.
			return m, m.pageKey(pageQuitPending, v)
		}
		if m.searchAvailable() {
			if m.searchOpen() && v.String() != "ctrl+c" {
				return m, m.searchKey(v)
			}
			if v.String() == "/" {
				return m, m.openSearch()
			}
		}
		if m.top() == pageReview && m.fileView() && m.discussions.published == nil && m.discussions.editor == nil && !m.commitFilter.open && !m.commitFilter.subset && m.Composer == nil && m.CommentMenu == nil && !m.Busy {
			if m.fileFilterEditing && v.String() != "ctrl+c" {
				m.fileFilterKey(v)
				return m, m.restartGuidePathScroll()
			}
			if v.String() == "F" {
				m.openFileFilter()
				return m, nil
			}
			if v.String() == "esc" && m.fileFilter != "" && m.Focus == paneList {
				m.fileFilter = ""
				return m, nil
			}
		}
		editingReviewText := m.discussions.published != nil || m.top() == pageReviewSubmit || (m.top() == pageDiscussions && m.discussions.editor != nil)
		if m.top() == pageInboxFilters && v.String() != "ctrl+c" {
			return m, m.inboxFilterKey(v.String())
		}
		editingGuideModel := m.top() == pageGuideConsent && m.guideFocus == 2
		if (v.String() != "q" || editingReviewText || editingGuideModel) && v.String() != "ctrl+c" {
			if m.Busy && ((m.top() != pagePullRequestPicker && m.top() != pageInbox) || m.Session == nil) {
				return m, nil
			}
			if m.discussions.published != nil {
				return m, m.publishedKey(v)
			}
			if m.top() == pageSuggestionApply {
				return m, m.suggestionApplyKey(v)
			}
			if m.Composer != nil {
				return m, m.commentComposerKey(v)
			}
			if m.CommentMenu != nil {
				return m, m.commentActionKey(v)
			}
			if m.top() == pageDiscussions && m.discussions.editor != nil {
				return m, m.generalCommentKey(v)
			}
			if m.top() == pageThemePicker {
				return m, m.themePickerKey(v.String())
			}
			if v.String() == "t" && m.themePickerAvailable() {
				m.openThemePicker()
				return m, nil
			}
			if m.top() == pageLifecycle {
				return m, m.prLifecycleKey(v.String())
			}
			if m.top() == pageReadiness {
				return m, m.readinessKey(v.String())
			}
			if m.top() == pageIssueContext {
				return m, m.issueContextKey(v.String())
			}
			if m.top() == pageDiscussions {
				if m.discussions.editor != nil {
					return m, m.generalCommentKey(v)
				}
				return m, m.discussionKey(v.String())
			}
			if m.top() == pageIncremental {
				return m, m.incrementalKey(v.String())
			}
			if p := m.top(); p != pageReview {
				return m, m.pageKey(p, v)
			}
			if m.commitFilter.open {
				return m, m.commitFilterKey(v.String())
			}
			if v.String() == "C" && m.Session != nil && m.diffReviewView() {
				m.openCommitFilter()
				return m, nil
			}
			if v.String() == "f3" || v.String() == "shift+f3" {
				if !m.searchAvailable() {
					return m, nil
				}
				s := m.existingSearch()
				if s != nil && len(s.matches) > 0 {
					delta := 1
					if v.String() == "shift+f3" {
						delta = -1
					}
					s.selected = (s.selected + delta + len(s.matches)) % len(s.matches)
					m.activateSearchMatch()
				}
				return m, nil
			}
			if m.codeNavigationKey(v.String()) {
				return m, nil
			}
			if v.String() == "alt+up" || v.String() == "alt+down" {
				delta := 1
				if v.String() == "alt+up" {
					delta = -1
				}
				m.nextUnresolved(delta)
				return m, nil
			}
			if v.String() == "alt+r" && m.Session != nil {
				m.push(pageReadiness)
				return m, m.refreshReadiness()
			}
			if v.String() == "I" && m.Session != nil {
				return m, m.openIssueContext()
			}
			if v.String() == "D" && m.Session != nil {
				m.openDiscussions()
				return m, nil
			}
			if v.String() == "5" && m.Session != nil {
				m.openIncremental()
				return m, nil
			}
			if v.String() == "c" && m.readDiscussions != nil {
				return m, tea.Batch(m.refreshDiscussions(), m.refreshComments())
			}
			if v.String() == "esc" && !m.commitFilter.subset && m.restoreDiscussionContext() {
				return m, nil
			}
			if v.String() == "z" && !m.commitFilter.subset && m.diffReviewView() && m.Focus == paneDiff {
				if pendingCenter {
					m.centerCursor()
				} else {
					m.pendingCenter = true
				}
				return m, nil
			}
			if v.String() == "P" {
				return m, m.openSwitcher()
			}
			if v.String() == "R" && m.Session != nil {
				m.openReviewForm()
				return m, nil
			}
			switch v.String() {
			case "1":
				m.selectReviewView(viewDescription)
				return m, nil
			case "2", "F":
				m.selectReviewView(viewFiles)
				return m, m.restartGuidePathScroll()
			case "3", "G":
				m.selectReviewView(viewGuide)
				return m, m.restartGuidePathScroll()
			case "4":
				m.selectReviewView(viewCommits)
				return m, nil
			}
			if v.String() == "v" {
				m.cycleReviewView(1)
				return m, m.restartGuidePathScroll()
			}
			if v.String() == "V" {
				m.cycleReviewView(-1)
				return m, m.restartGuidePathScroll()
			}
			if v.String() == "ctrl+o" {
				m.switchCommentSide()
				return m, nil
			}
			if v.String() == "ctrl+v" {
				m.toggleCommentRange()
				return m, nil
			}
			if v.String() == "ctrl+s" {
				return m, m.openSuggestionComposer()
			}
			if v.String() == "ctrl+f" {
				return m, m.openFileComposer()
			}
			if v.String() == "esc" && m.rangeStart != nil {
				m.rangeStart = nil
				return m, nil
			}
			if m.selectedReviewView() == viewDescription && m.descriptionKey(v.String()) {
				return m, nil
			}
			if m.selectedReviewView() == viewCommits && v.String() != "?" && v.String() != "U" {
				if v.String() == "enter" && m.commit.focus == paneDiff {
					return m, m.openCommitComposer()
				}
				m.commitKey(v.String())
				return m, nil
			}
			if m.commitFilter.subset && m.diffReviewView() && m.filteredReadingKey(v.String()) {
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
			m.selectReviewView(viewFiles)
			m.openFileFilter()
		case "G":
			m.selectReviewView(viewGuide)
		case "B":
			if m.fileView() {
				m.groupFiles = !m.groupFiles
				m.saveLayout()
			}
		case "alt+c":
			if m.fileView() {
				keepSearchMatch := m.searchMatchAtCursor()
				sourceRestore := m.sourceCursorRestorer()
				target, commentID := m.cursorAnchor()
				m.collapseGenerated = !m.collapseGenerated
				m.restoreCursorAnchor(target, commentID)
				if sourceRestore != nil {
					sourceRestore()
				}
				if keepSearchMatch {
					m.revealSearchMatch()
				}
				m.saveLayout()
			}
		case "S":
			if m.diffReviewView() {
				keepSearchMatch := m.searchMatchAtCursor()
				sourceRestore := m.sourceCursorRestorer()
				target, commentID := m.cursorAnchor()
				m.layout = m.layout.toggled()
				m.saveLayout()
				m.restoreCursorAnchor(target, commentID)
				if sourceRestore != nil {
					sourceRestore()
				}
				if keepSearchMatch {
					m.revealSearchMatch()
				}
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
		case "}":
			m.file(1)
		case "{":
			m.file(-1)
		case "]":
			m.resizeList(2)
		case "[":
			m.resizeList(-2)
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
			switch {
			case m.fileView() && m.Focus == paneDiff:
				m.cursorActive = true
				m.moveCursor(1)
			case m.fileView():
				m.file(1)
			case m.Focus == paneDiff:
				m.cursorActive = true
				m.moveCursor(1)
			default:
				m.move(1)
			}
		case "k":
			switch {
			case m.fileView() && m.Focus == paneDiff:
				m.cursorActive = true
				m.moveCursor(-1)
			case m.fileView():
				m.file(-1)
			case m.Focus == paneDiff:
				m.cursorActive = true
				m.moveCursor(-1)
			default:
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
			m.syncGuideToLine(0)
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
		m.tabs = append(m.tabs, workspaceTab{identity: identity, title: m.knownPRTitle(identity), review: m.newDraftReviewTab(s)})
		return
	}
	m.saveActiveCursorAnchor()
	m.tabs = append(m.tabs, workspaceTab{identity: identity, title: m.knownPRTitle(identity), review: m.newDraftReviewTab(s)})
	m.activeTab = len(m.tabs) - 1
	m.restoreReviewTab(m.tabs[m.activeTab].review)
}

func (m *Model) activateTab(index int) bool {
	m.closeSearchPopovers()
	if index < 0 || index >= len(m.tabs) {
		return false
	}
	if index == m.activeTab {
		return true
	}
	m.saveActiveCursorAnchor()
	m.activeTab = index
	m.restoreReviewTab(m.tabs[index].review)
	return true
}

// reviewStateForTarget resolves an asynchronous result to its originating
// review, including the initial state before a review has been registered.
func (m *Model) reviewStateForTarget(target int) *reviewTabState {
	if target == m.activeTab {
		return m.reviewTabState
	}
	if target >= 0 && target < len(m.tabs) {
		return m.tabs[target].review
	}
	return nil
}

func newReviewTabState(s *review.Session) *reviewTabState {
	initialView := viewFiles
	if s != nil && s.Inventory.Comparison.Metadata.Identity.Number > 0 {
		initialView = viewDescription
	}
	return &reviewTabState{
		Session: s, Scroll: map[int]int{}, GuideScroll: map[int]int{}, Cursor: map[int]int{}, GuideCursor: map[int]int{},
		ContextView: initialView, DescriptionScroll: 0, Files: true, collapsed: newExpansion(), Stack: []page{pageReview}, Focus: paneList,
	}
}

// saveActiveCursorAnchor preserves the selected source line while the shared
// workspace width may change before this tab is visited again.
func (m *Model) saveActiveCursorAnchor() {
	if m.activeTab < 0 || m.activeTab >= len(m.tabs) {
		return
	}
	m.savedCursorTarget, m.savedCursorCommentID = m.cursorAnchor()
}

func (m *Model) restoreReviewTab(state *reviewTabState) {
	m.pendingCenter = false
	m.cancelMouseDrag()
	if state == nil {
		return
	}
	m.reviewTabState = state
	if m.fileCache.session != m.Session {
		m.fileCache = fileDetailCache{}
	}
	m.restoreCursorAnchor(state.savedCursorTarget, state.savedCursorCommentID)
}

// reviewViews is the displayed order, independent of the enum's zero-value
// Files default. Rendering, cycling, and mouse selection share this order.
var reviewViews = [...]reviewView{viewDescription, viewFiles, viewGuide, viewCommits}

func (m *Model) selectedReviewView() reviewView {
	// Files remains the internal selector for the existing shared diff workspace.
	// Models constructed directly for rendering also use it to select Guide.
	if m.ContextView == viewFiles && !m.Files && !m.Inventory {
		return viewGuide
	}
	return m.ContextView
}

func (m *Model) diffReviewView() bool {
	view := m.selectedReviewView()
	return view == viewFiles || view == viewGuide
}

func (m *Model) cycleReviewView(delta int) {
	if m.Session == nil {
		return
	}
	for i, view := range reviewViews {
		if view == m.selectedReviewView() {
			m.selectReviewView(reviewViews[(i+delta%len(reviewViews)+len(reviewViews))%len(reviewViews)])
			return
		}
	}
}

func (m *Model) selectReviewView(view reviewView) {
	m.closeSearchPopovers()
	m.cancelMouseDrag()
	if m.Session == nil {
		return
	}
	m.ContextView = view
	switch view {
	case viewFiles:
		m.Files, m.Inventory = true, false
	case viewGuide:
		m.Files, m.Inventory = false, false
		if m.commitFilter.subset {
			return
		}
		m.begin()
		if _, ok := m.activeGuide(); ok {
			m.setOffset(0)
			m.setCursor(0)
		}
	}
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
	case pageInbox:
		return m.inboxKey(k)
	case pageInboxFilters:
		return m.inboxFilterKey(k)
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
		switch k {
		case "esc":
			m.pop()
		case "s":
			if !m.saveDraftsForQuit() {
				return nil
			}
			m.cancel()
			return tea.Quit
		case "enter":
			if !m.discardDraftsForQuit() {
				return nil
			}
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
	selected, err := m.completeCommentRange(*target)
	if err != nil {
		m.ActionError = err
		return nil
	}
	if selected.Side == "LEFT" && selected.StartLine == 0 && targetIsOldContext(m.displayDetail()[cursor], selected) {
		m.ActionError = errors.New("old-side context requires a multiline range; use ctrl+v")
		return nil
	}
	m.rangeStart = nil
	m.Composer = &commentComposer{Target: selected, PendingIndex: -1}
	for i, pending := range m.Pending {
		if pending.Target == selected {
			m.Composer.PendingIndex = i
			m.Composer.Draft = pending.Body
			m.Composer.Cursor = len([]rune(pending.Body))
			break
		}
	}
	m.restoreSuggestionEditor(m.Composer)
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
			m.resolveReplyRoot(m.CommentMenu)
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
		name, width := fileRailName(m.Session.Inventory.Files[file], m.listWidth()-visibleWidth(prefix))
		return row{}, prefix, width - visibleWidth(name) - 2, fileDirectory(m.Session.Inventory.Files[file])
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
	if m.Width < 100 {
		return max(1, m.Width-2)
	}
	width := m.listWidthPreference
	if width == 0 {
		width = min(36, m.Width/3)
	}
	return m.clampListWidth(width)
}

func (m *Model) clampListWidth(width int) int {
	return min(max(18, width), m.Width-3-40, 4096)
}

func (m *Model) resizeList(delta int) {
	if m.Width < 100 || m.Session == nil || !m.diffReviewView() {
		return
	}
	if m.commitFilter.subset {
		m.listWidthPreference = m.clampListWidth(m.listWidth() + delta)
		m.saveLayout()
		return
	}
	keepSearchMatch := m.searchMatchAtCursor()
	sourceRestore := m.sourceCursorRestorer()
	target, commentID := m.cursorAnchor()
	m.listWidthPreference = m.clampListWidth(m.listWidth() + delta)
	m.saveLayout()
	m.restoreCursorAnchor(target, commentID)
	if sourceRestore != nil {
		sourceRestore()
	}
	if keepSearchMatch {
		m.revealSearchMatch()
	}
	if m.diffLayout() != diffLayoutSideBySide && target == nil && commentID == 0 && !keepSearchMatch && sourceRestore == nil {
		m.cursorInViewport(1)
	}
	m.ensureReplyEditorVisible()
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
	if m.fileView() && (m.fileFilter != "" || m.groupFiles || m.collapseGenerated) {
		files := m.filteredFiles()
		if len(files) == 0 {
			return
		}
		index := slices.Index(files, f)
		if index < 0 {
			index = 0
		}
		f = files[max(0, min(len(files)-1, index+delta))]
	} else {
		f = max(0, min(len(m.Session.Slices)-1, f+delta))
	}
	m.selectFile(f)
}

func (m *Model) selectFile(f int) {
	m.Selected = m.Session.Slices[f].Units[0]
	if !m.Inventory {
		start := m.fileOffset(f)
		m.setOffset(m.clampOffset(start))
		end := len(m.displayDetail())
		if f+1 < len(m.Session.Slices) {
			end = m.fileOffset(f + 1)
		}
		for i, line := range m.displayDetail()[start:end] {
			if m.navigableLine(line) {
				m.setCursor(start + i)
				break
			}
		}
	}
	m.Horizontal = 0
}
func (m *Model) scroll(delta int) {
	if m.Session == nil || len(m.Session.Inventory.Units) == 0 {
		return
	}
	if delta != 0 && m.offset() == m.clampOffset(m.offset()+delta) && m.crossGuideBoundary(delta) {
		return
	}
	// The clamp counts the same display lines the diff pane renders, so a diff
	// that already fits cannot be scrolled past its end.
	m.setOffset(m.clampOffset(m.offset() + delta))
	m.syncGuideToLine(m.offset())
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
		if offset, ok := m.wrappedGuideAnchor(m.cachedGuideDetail(r.guide), r); ok {
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
		if !m.collapseGenerated || m.Session.UnitFiles[m.Selected] == file {
			m.Selected = m.Session.Slices[file].Units[0]
			return
		}
		// A generated body can contract above the current cursor as selection moves.
		// Preserve its raw target and viewport position, never the old display index.
		sourceRestore := m.sourceCursorRestorer()
		target, commentID := m.cursorAnchor()
		oldCursor, oldOffset := m.cursor(), m.offset()
		relativeOffset := oldOffset - m.fileOffset(file)
		m.Selected = m.Session.Slices[file].Units[0]
		m.restoreCursorAnchor(target, commentID)
		if sourceRestore != nil {
			sourceRestore()
		}
		after, afterID := m.cursorAnchor()
		if sourceRestore != nil || target != nil && after != nil && *target == *after || commentID > 0 && afterID == commentID {
			m.setOffset(m.clampOffset(oldOffset + m.cursor() - oldCursor))
		} else {
			m.setOffset(m.clampOffset(m.fileOffset(file) + max(0, relativeOffset)))
		}
	}
}

func (m *Model) baseDetail() []diffLine {
	if guide, ok := m.activeGuide(); ok {
		return m.cachedGuideDetail(guide).lines
	}
	if m.fileView() {
		return m.presentedFileDetail(false)
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
	// Source rows are immutable; the renderer copies just the visible viewport.
	// Avoid rebuilding the whole review on every navigation call without overlays.
	if m.Composer != nil && m.Composer.Target.SubjectType == "file" {
		return append(m.inlineEditorLines(), m.wrapSource(base)...)
	}
	if len(m.Comments) == 0 && len(m.Pending) == 0 && m.Composer == nil && (m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply) {
		return m.wrapSource(base)
	}
	lines := make([]diffLine, 0, len(base)+len(m.Comments)+2)
	for _, line := range base {
		lines = append(lines, m.wrapSource([]diffLine{line})...)
		for _, target := range sourceLineTargets(line) {
			for _, comment := range m.Comments {
				if targetEndsAt(comment.Target, target) && comment.ParentID == 0 {
					lines = append(lines, m.reviewCommentThread(comment, 0)...)
				}
			}
			lines = append(lines, m.recoveredReplyLines(target)...)
			lines = append(lines, m.pendingLines(target)...)
			if m.Composer != nil && targetEndsAt(m.Composer.Target, target) {
				lines = append(lines, m.inlineEditorLines()...)
			}
		}
	}
	return lines
}

// displayDetail is the single source of truth for rendered-row navigation.
// Unified mode wraps source rows; split mode projects and wraps source rows
// before attaching full-width comment and editor overlays.
func (m *Model) displayDetail() []diffLine {
	if !m.sideBySideEnabled() {
		return m.detail()
	}
	return m.sideBySideDetail()
}

func (m *Model) sideBySideEnabled() bool {
	return m.diffLayout() == diffLayoutSideBySide && m.diffReviewView() && m.Width >= sideBySideMinimumWidth
}

func (m *Model) diffLayout() diffLayout { return m.layout }

func (m *Model) sideBySideDetail() []diffLine {
	var base []diffLine
	if guide, ok := m.activeGuide(); ok {
		base = m.cachedGuideDetail(guide).splitLines
	} else if m.fileView() {
		base = m.presentedFileDetail(true)
	} else {
		base = projectSideBySideDetail(m.baseDetail())
	}
	if m.Composer != nil && m.Composer.Target.SubjectType == "file" {
		return append(m.inlineEditorLines(), m.wrapSource(base)...)
	}
	if len(m.Comments) == 0 && len(m.Pending) == 0 && m.Composer == nil && (m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply) {
		return m.wrapSource(base)
	}
	lines := make([]diffLine, 0, len(base)+len(m.Comments)+2)
	for _, line := range base {
		lines = append(lines, m.wrapSource([]diffLine{line})...)
		if line.sideBySide == nil {
			continue
		}
		row := *line.sideBySide
		// A paired row can have comments on both sides. Keep their overlay
		// order stable: old/LEFT before new/RIGHT.
		for _, target := range rowTargets(row) {
			for _, comment := range m.Comments {
				if targetEndsAt(comment.Target, target) && comment.ParentID == 0 {
					lines = append(lines, m.reviewCommentThread(comment, 0)...)
				}
			}
			lines = append(lines, m.recoveredReplyLines(target)...)
			lines = append(lines, m.pendingLines(target)...)
			if m.Composer != nil && targetEndsAt(m.Composer.Target, target) {
				lines = append(lines, m.inlineEditorLines()...)
			}
		}
	}
	return lines
}

func rowTargets(row diffRow) []source.ReviewCommentTarget {
	targets := make([]source.ReviewCommentTarget, 0, 2)
	if row.old != nil && row.old.line != nil {
		targets = append(targets, sourceLineTargets(*row.old.line)...)
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
	if label := m.publishedStatus(comment.ID); label != "" {
		lines = append(lines, wrappedLines(classMetadata, label)...)
	}
	if replacement, err := source.ParseSuggestion(comment.Body); err == nil {
		before, ok := m.targetSource(comment.Target)
		if !ok {
			before = "(captured source unavailable; application unsupported)"
		}
		for _, p := range suggestionPreview(before, replacement) {
			lines = append(lines, wrappedLines(p.Class, p.Text)...)
		}
		lines = append(lines, wrappedLines(classMetadata, "ctrl+a in comment actions: apply suggestion")...)
	}
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
	lines := m.inlineEditorLinesFor(c.Draft, c.Cursor, 0)
	if c.Suggestion {
		preview := suggestionPreview(c.Before, c.Draft)
		for i := range preview {
			preview[i].editor = true
		}
		lines = append(preview, lines...)
	}
	if c.Target.StartLine != 0 || c.Target.SubjectType == "file" {
		label := diffLine{styledLine: styledLine{Class: classWarning, Text: "Comment target · " + commentTargetLabel(c.Target)}, editor: true}
		lines = append([]diffLine{label}, lines...)
	}
	selector := m.commentTypeSelector()
	if c.CommitSHA != "" {
		return lines
	}
	lines = append([]diffLine{{styledLine: styledLine{Class: classWarning, Text: clip(selector, m.overlayInnerWidth(0)+4)}, editor: true}}, lines...)
	return lines
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
	width := m.detailWidth()
	if m.selectedReviewView() == viewCommits {
		width = m.commitDetailWidth()
	}
	return max(8, width-(2+indent)-6)
}

func (m *Model) detailWidth() int {
	if m.Width < 100 {
		return max(1, m.Width-2)
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
	return m.cursorInDetail(m.displayDetail())
}

func (m *Model) cursorInDetail(detail []diffLine) int {
	if len(detail) == 0 {
		return -1
	}
	stored, ok := m.cursorValue()
	if ok && stored >= 0 && stored < len(detail) && m.navigableLine(detail[stored]) {
		return stored
	}
	for i, line := range detail {
		if m.navigableLine(line) {
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

// sourceCursorRestorer preserves a read-only captured source row across layout
// projections without assigning it a GitHub comment target.
func (m *Model) sourceCursorRestorer() func() {
	if m.navigation.mode == "" || m.selectedDiffTarget() != nil {
		return nil
	}
	cursor := m.cursor()
	if cursor < 0 {
		return nil
	}
	line := m.displayDetail()[cursor]
	if line.commentID > 0 {
		return nil
	}
	if line.sideBySide != nil {
		if line.sideBySide.new != nil {
			line = *line.sideBySide.new.line
		} else if line.sideBySide.old != nil {
			line = *line.sideBySide.old.line
		}
	}
	if line.searchID.Unit == "" {
		return nil
	}
	id, offset := line.searchID, line.sourceOffset
	return func() {
		fallback := -1
		for i, row := range m.displayDetail() {
			cells := []diffLine{row}
			if row.sideBySide != nil {
				cells = nil
				if row.sideBySide.new != nil {
					cells = append(cells, *row.sideBySide.new.line)
				}
				if row.sideBySide.old != nil {
					cells = append(cells, *row.sideBySide.old.line)
				}
			}
			for _, cell := range cells {
				if cell.searchID == id {
					fallback = i
					if cell.sourceOffset >= offset {
						m.setCursor(i)
						return
					}
				}
			}
		}
		if fallback >= 0 {
			m.setCursor(fallback)
		}
	}
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
	detail := m.displayDetail()
	current := m.cursorInDetail(detail)
	if current < 0 {
		return
	}
	targets := make([]int, 0, len(detail))
	for i, line := range detail {
		if m.navigableLine(line) {
			targets = append(targets, i)
		}
	}
	for i, target := range targets {
		if target == current {
			if (delta > 0 && i == len(targets)-1 || delta < 0 && i == 0) && m.crossGuideBoundary(delta) {
				return
			}
			m.setCursor(targets[max(0, min(len(targets)-1, i+delta))])
			m.ensureCursorVisible()
			m.syncGuideToLine(m.cursor())
			if m.fileView() {
				m.syncFileToLine(m.cursor())
			}
			return
		}
	}
}

// stickyFileHeader returns the file boundary that has scrolled above the viewport.
// At a boundary the original row is already visible, so no overlay is needed.
func stickyFileHeader(detail []diffLine, offset int) int {
	if offset < 0 || offset >= len(detail) {
		return -1
	}
	for i := offset; i >= 0; i-- {
		if detail[i].Class == classFileHeader && strings.HasPrefix(detail[i].Text, "── ") {
			if i == offset {
				return -1
			}
			return i
		}
	}
	return -1
}

// cursorInViewport replaces a cursor only when scrolling would hide it.
// Forward scrolling chooses the first available target in view; backward
// scrolling chooses the last, while stable cursors are left untouched.
func (m *Model) cursorInViewport(delta int) {
	current := m.cursor()
	start, end := m.offset(), m.offset()+m.bodyHeight()
	detail := m.displayDetail()
	if m.bodyHeight() > 1 && stickyFileHeader(detail, start) >= 0 {
		start++
	}
	if current >= start && current < end {
		return
	}
	if delta < 0 {
		for i := min(len(detail)-1, end-1); i >= start; i-- {
			if m.navigableLine(detail[i]) {
				m.setCursor(i)
				return
			}
		}
		return
	}
	for i := max(0, start); i < min(len(detail), end); i++ {
		if m.navigableLine(detail[i]) {
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
	if cursor < offset || (cursor == offset && m.bodyHeight() > 1 && stickyFileHeader(m.displayDetail(), offset) >= 0) {
		m.setOffset(max(0, cursor-1))
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
	return max(1, m.Height-4-m.reviewFooterRows())
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
		case pageSuggestionApply:
			text = renderActionModal(m.Width, m.Height, m.actionModalBackground(), m.suggestionApplyView(), m.modalSurface)
		case pageDraftRecovery:
			text = renderActionModal(m.Width, m.Height, m.actionModalBackground(), m.draftRecoveryView(), m.modalSurface)
		case pageLifecycle:
			text = m.lifecycleView()
		case pageReadiness:
			text = m.readinessView()
		case pageIssueContext:
			text = m.issueContextView()
		case pageDiscussions:
			text = m.discussionsView()
		case pageIncremental:
			text = m.incrementalView()
		case pageInbox:
			text = m.inboxView()
		case pageInboxFilters:
			text = m.inboxFiltersView()
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
			text = renderGuideConsentModal(m.Width, m.Height, m.reviewView(), m.guideConsentView(), m.modalSurface)
		case pageReviewSubmit:
			text = m.reviewFormModalView()
		case pageQuitPending:
			body := "Discard unsent review drafts and quit?\n\ns: save and quit · enter: discard and quit · esc: keep reviewing"
			if m.ActionError != nil {
				body += "\n! " + Escape(m.ActionError.Error())
			}
			text = renderActionModal(m.Width, m.Height, m.actionModalBackground(), body, m.modalSurface)
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
	if m.commitFilter.open && m.top() == pageReview && m.diffReviewView() {
		text = m.commitFilterModalView(text)
	}
	if m.discussions.published != nil {
		text = m.publishedView()
	}
	if m.searchOpen() {
		text = m.searchPopover(text)
	}
	if modal := m.loadingModal(); modal.active {
		text = renderLoadingModal(m.Width, m.Height, text, modal, m.modalSurface)
	}
	lines := strings.Split(text, "\n")
	if len(lines) > m.Height {
		lines = lines[:m.Height]
	}
	for i := range lines {
		lines[i] = clip(lines[i], m.Width)
	}
	v := tea.NewView(m.themeContent(strings.Join(lines, "\n")))
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

func (m *Model) reviewView() string {
	return m.reviewViewForLayout(m.diffLayout() == diffLayoutSideBySide)
}

// reviewViewForLayout is the layout boundary used by the tab preference added
// in the next slice. Keeping the choice an argument here leaves this slice
// stateless while making its responsive fallback directly testable.
func (m *Model) reviewViewForLayout(preferSideBySide bool) string {
	s := m.Session
	title := m.workspaceIdentity() + "\n" + m.contextViewTabs()
	if !m.diffReviewView() {
		if m.selectedReviewView() == viewDescription {
			return title + "\n" + m.descriptionView() + "\n" + m.reviewStatus()
		}
		return title + "\n" + m.commitsView() + "\n" + m.reviewStatus()
	}
	if m.commitFilter.subset {
		return m.filteredReviewView(title)
	}
	if len(s.Inventory.Units) == 0 {
		return title + "\nCommits [C] · " + m.commitFilterLabel() + "\nEmpty comparison: no net tree changes.\n" + m.reviewStatus()
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
	leftLabel, _ := m.fileFilterHeader()
	if m.selectedReviewView() == viewGuide {
		leftLabel = "Guide"
	}
	if m.Inventory {
		leftLabel = "Full inventory (i)"
	}
	rightLabel := "Find (/) · Diff · " + text
	if m.Width < 100 {
		leftLabel += " · Commits [C] · " + m.commitFilterLabel()
	} else {
		rightLabel = "Commits [C] · " + m.commitFilterLabel() + " · " + rightLabel
	}
	header := m.paneFrameHeader(leftLabel, rightLabel)
	bodyHeight := m.bodyHeight()
	list, selectedRow := m.reviewListPresentation()
	var detail []diffLine
	if useSideBySide && m.sideBySideEnabled() {
		detail = m.displayDetail()
	} else {
		detail = m.detail()
		if useSideBySide {
			detail = m.wrapSource(projectSideBySideDetail(m.baseDetail()))
		}
	}
	cursor := m.cursorInDetail(detail)
	var selected *source.ReviewCommentTarget
	if useSideBySide && m.cursorActive && cursor >= 0 {
		selected = m.selectedDiffTargetForLine(detail[cursor])
	}
	offset := min(m.offset(), max(0, len(detail)-1))
	activeHeader := -1
	activeLine := offset
	if m.Focus == paneDiff && m.cursorActive && cursor >= 0 {
		activeLine = cursor
	} else if m.fileView() {
		activeLine = m.fileOffset(s.UnitFiles[m.Selected])
	}
	for i, line := range detail {
		if i > activeLine {
			break
		}
		if line.Class == classFileHeader && strings.HasPrefix(line.Text, "── ") {
			activeHeader = i
		}
	}
	// Copy the viewport so selection styling never changes cached source rows.
	sticky := stickyFileHeader(detail, offset)
	var pinned diffLine
	if sticky >= 0 && bodyHeight > 1 {
		pinned = detail[sticky]
	}
	detail = append([]diffLine(nil), detail[offset:min(len(detail), offset+bodyHeight)]...)
	if i := activeHeader - offset; i >= 0 && i < len(detail) {
		detail[i].Class = classSelection
		if detail[i].sideBySide != nil && detail[i].sideBySide.full != nil {
			row := *detail[i].sideBySide
			full := *row.full
			full.Class = classSelection
			row.full = &full
			detail[i].sideBySide = &row
		}
	}
	if sticky >= 0 && bodyHeight > 1 {
		// Replace only the occluded top row; source indexes and bottom geometry stay stable.
		detail[0] = pinned
		if sticky == activeHeader {
			detail[0].Class = classSelection
		}
	}
	if useSideBySide {
		detail = m.renderSideBySideViewport(detail, m.detailWidth(), m.Horizontal, cursor-offset, selected)
	}
	// Horizontal scrolling stays on unstyled text; styles are applied after clipping.
	// Split rows need the same stable cursor gutter as unified rows. Without it,
	// a focused comment target remains selectable but has no visible location.
	for i, line := range detail {
		if i == 0 && sticky >= 0 && bodyHeight > 1 {
			detail[i].Text = pinned.Text
			detail[i].Class = pinned.Class
			if sticky == activeHeader {
				detail[i].Class = classSelection
			}
			if m.cursorActive {
				detail[i].Text = cursorMarker(false) + pinned.Text
			}
			continue
		}
		marker := ""
		if m.cursorActive {
			marker = cursorMarker(offset+i == cursor)
		}
		if useSideBySide {
			if line.sideBySide != nil && line.sideBySide.full == nil {
				marker = ""
			}
			detail[i].Text = marker + line.Text
			if line.commentID > 0 && offset+i == cursor {
				detail[i].Class = selectedClass(true)
			}
			continue
		}
		detail[i].Text = m.syntaxText(line, m.Horizontal, m.detailWidth(), marker)
		if line.commentID > 0 && offset+i == cursor {
			detail[i].Class = selectedClass(true)
		}
	}
	body := []string{}
	borders := paneBodyBorders{paneBorderClass(m.Focus == paneList), classPaneBorderFocused, paneBorderClass(m.Focus == paneDiff)}
	for row := 0; row < bodyHeight; row++ {
		left, right := "", ""
		class, leftClass := classPlain, classPlain
		mutedFrom := 0
		if row < len(list) {
			left = list[row].text
			mutedFrom = list[row].mutedFrom
			if list[row].row == selectedRow {
				leftClass = selectedClass(m.Focus == paneList)
			}
		}
		if row < len(detail) {
			right, class = detail[row].Text, detail[row].Class
		}
		body = append(body, m.paneBodyRow(styledLine{Class: leftClass, Text: left}, styledLine{Class: class, Text: right}, m.listWidth(), m.detailWidth(), m.Focus, borders, mutedFrom))
	}
	return title + "\n" + header + "\n" + strings.Join(body, "\n") + "\n" + m.paneFrameFooter() + "\n" + m.reviewStatus()
}

func (m *Model) frameBodyLine(content string, class lineClass, width int, edges bool) string {
	content = clip(content, width)
	padding := strings.Repeat(" ", max(0, width-visibleWidth(content)))
	if class == classAdded || class == classRemoved {
		content += padding
		padding = ""
	}
	line := m.styleLine(class, content) + padding
	if edges {
		border := m.styleLine(paneBorderClass(true), "│")
		return border + line + border
	}
	return line
}

func paneHeaderText(label string, width int) string {
	if width <= 0 {
		return ""
	}
	label = clip(" "+label+" ", width)
	return label + strings.Repeat("─", max(0, width-visibleWidth(label)))
}

func (m *Model) paneFrameHeader(listLabel, detailLabel string) string {
	if m.Width < 100 {
		label := listLabel + " · List"
		if m.Focus == paneDiff {
			label = listLabel + " · Find (/) · Diff"
		}
		return m.styleLine(classPaneHeaderFocused, "┌"+paneHeaderText(label, m.Width-2)+"┐")
	}
	left, right := m.listWidth(), m.detailWidth()
	listClass, detailClass := classPaneBorder, classPaneBorder
	if m.Focus == paneList {
		listClass = classPaneHeaderFocused
	} else {
		detailClass = classPaneHeaderFocused
	}
	return m.styleLine(listClass, "┌"+paneHeaderText(listLabel, left)) +
		m.styleLine(classPaneBorderFocused, "┬") +
		m.styleLine(detailClass, paneHeaderText(detailLabel, right)+"┐")
}

func (m *Model) paneFrameFooter() string {
	if m.Width < 100 {
		return m.styleLine(paneBorderClass(true), "└"+strings.Repeat("─", max(0, m.Width-2))+"┘")
	}
	return m.styleLine(paneBorderClass(m.Focus == paneList), "└"+strings.Repeat("─", m.listWidth())) +
		m.styleLine(classPaneBorderFocused, "┴") +
		m.styleLine(paneBorderClass(m.Focus == paneDiff), strings.Repeat("─", m.detailWidth())+"┘")
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

func (m *Model) contextViewTabs() string {
	tabs := m.contextTabLabels()
	for i, tab := range tabs {
		class := classTitle
		if i > 0 && reviewViews[i-1] == m.selectedReviewView() {
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
	if target := m.Session.Inventory.Comparison.Metadata.TargetBranch; target != "" {
		branchLines := strings.Split(ansi.Wrap("Target branch: "+Escape(target), max(1, m.Width), ""), "\n")
		lines = append(branchLines, lines...)
	}
	height := m.descriptionBodyHeight()
	m.DescriptionScroll = max(0, min(m.DescriptionScroll, max(0, len(lines)-height)))
	end := min(len(lines), m.DescriptionScroll+height)
	visible := append([]string(nil), lines[m.DescriptionScroll:end]...)
	for len(visible) < height {
		visible = append(visible, "")
	}
	return m.styleLine(classTitle, "Description") + "\nFrozen from GitHub when this review opened.\n" + strings.Join(visible, "\n")
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
	lines, err := renderDescriptionMarkdownWithTheme(body, width, m.theme)
	if err != nil {
		lines = []string{"Description could not be rendered."}
	}
	m.descriptionCache = descriptionRenderCache{body: body, width: width, themeName: m.theme.Name, lines: lines, valid: true}
	return lines
}

func (m *Model) descriptionBodyHeight() int { return max(1, m.Height-4-m.reviewFooterRows()) }

func (m *Model) descriptionKey(key string) bool {
	var delta int
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
