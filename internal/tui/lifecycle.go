package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
)

type FreshLoader func(context.Context, *review.Session, func(string)) (*review.Session, error)
type GuideLoader func(context.Context, *review.Session, func(string)) (*review.Session, error)
type ActionResult struct {
	Session *review.Session
	Err     error
	Reset   bool
}

// CommentSubmission carries the immutable comparison that produced the target
// alongside the explicit write request. The model copies Metadata before
// starting asynchronous work, so a tab switch cannot retarget its preflight.
type CommentSubmission struct {
	Comment  source.ReviewComment
	Metadata source.Metadata
}

// CommentSubmitter is the only TUI write seam. The command layer injects an
// implementation after its explicit freshness check; the model never reaches
// directly into a GitHub client.
type CommentSubmitter func(context.Context, CommentSubmission) (source.ReviewComment, error)
type CommentReader func(context.Context, source.Metadata) ([]source.ReviewComment, error)
type ViewerReader func(context.Context) (source.Viewer, error)

// CommentAction is deliberately small and tab-local. Body is used only for a
// reply; Reaction is one of the finite GitHub values. Neither is persisted.
type CommentAction struct {
	Metadata source.Metadata
	Comment  source.ReviewComment
	Body     string
	Reaction string
	Delete   bool
}
type CommentActionSubmitter func(context.Context, CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error)

type CommentResult struct {
	Target     int
	Generation uint64
	Comment    source.ReviewComment
	Err        error
}

type CommentListResult struct {
	Target     int
	Generation uint64
	Comments   []source.ReviewComment
	Err        error
}

type BackgroundCommentListResult struct {
	Target     int
	Generation uint64
	Comments   []source.ReviewComment
	Err        error
}
type CommentActionResult struct {
	Target     int
	CommentID  int64
	Generation uint64
	Reply      source.ReviewComment
	Reaction   source.ReviewCommentReaction
	Delete     bool
	Err        error
}
type ViewerResult struct {
	Target int
	Viewer source.Viewer
	Err    error
}

type editorCursorTick struct{ generation uint64 }

func nextEditorCursorTick(generation uint64) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return editorCursorTick{generation: generation} })
}

type PullRequestListResult struct {
	PullRequests []source.PullRequest
	Repository   string
	Target       int
	Err          error
}

// PullRequestOpenResult carries the browser tab and immutable identity that
// initiated an asynchronous open. A result must never be applied to whichever
// review happens to be visible when the worker finishes.
type PullRequestOpenResult struct {
	Target   int
	Identity source.Identity
	Session  *review.Session
	Err      error
}

// PullRequestRefreshResult updates a review that was initially shown from a
// local frozen snapshot. It is intentionally separate from opening so the
// revision check never blocks the review surface.
type PullRequestRefreshResult struct {
	Target    int
	SessionID string
	Session   *review.Session
	Err       error
}

type switcherResult struct {
	identity source.Identity
	title    string
	open     bool
}

func (m *Model) SetLifecycle(store *session.Store, reader review.MetadataReader, fresh FreshLoader) {
	m.store, m.reader, m.fresh = store, reader, fresh
	if store != nil {
		m.listSessions, m.listRepositories = store.List, store.ListRepositories
	}
}

func (m *Model) SetPullRequestLifecycle(list PullRequestLoader, open PullRequestOpener) {
	m.listPullRequests, m.openPullRequest = list, open
}

// SetPullRequestRefresh supplies the background freshness check for a review
// opened from a local frozen snapshot.
func (m *Model) SetPullRequestRefresh(refresh PullRequestRefresher) { m.refreshPullRequest = refresh }

func (m *Model) refreshOpenedPullRequest(target int, opened *review.Session) tea.Cmd {
	if m.refreshPullRequest == nil || opened == nil || opened.RevisionStatus != session.Unchecked {
		return nil
	}
	refresh, sessionID := m.refreshPullRequest, opened.ID
	return func() tea.Msg {
		notify := m.notify
		if notify == nil {
			notify = func(string) {}
		}
		s, err := refresh(m.ctx, opened, notify)
		return PullRequestRefreshResult{Target: target, SessionID: sessionID, Session: s, Err: err}
	}
}

func (m *Model) SetGuideLifecycle(generate GuideLoader) { m.generateGuide = generate }

// SetCommentSubmitter installs the explicit write action used by the composer.
// It is intentionally separate from the read lifecycle dependencies.
func (m *Model) SetCommentSubmitter(submit CommentSubmitter) { m.submitComment = submit }
func (m *Model) SetCommentReader(read CommentReader)         { m.readComments = read }
func (m *Model) SetViewerReader(read ViewerReader)           { m.readViewer = read }
func (m *Model) SetCommentActionSubmitter(submit CommentActionSubmitter) {
	m.submitCommentAction = submit
}

func (m *Model) refreshComments() tea.Cmd {
	if m.readComments == nil || m.Session == nil {
		return nil
	}
	m.commentGeneration++
	generation, target, read := m.commentGeneration, m.activeTab, m.readComments
	metadata := m.Session.Inventory.Comparison.Metadata
	m.notice = "Refreshing pull request comments..."
	ctx := m.beginAction()
	return m.start(func() tea.Msg {
		comments, err := read(ctx, metadata)
		return CommentListResult{Target: target, Generation: generation, Comments: comments, Err: err}
	})
}

func (m *Model) refreshCommentsInBackground() tea.Cmd {
	if m.readComments == nil || m.Session == nil {
		return nil
	}
	m.commentGeneration++
	generation, target, read := m.commentGeneration, m.activeTab, m.readComments
	metadata := m.Session.Inventory.Comparison.Metadata
	return func() tea.Msg {
		comments, err := read(m.ctx, metadata)
		return BackgroundCommentListResult{Target: target, Generation: generation, Comments: comments, Err: err}
	}
}

func (m *Model) applyCommentListResult(target int, generation uint64, comments []source.ReviewComment, err error) {
	apply := func(state *reviewTabState) {
		if state == nil || state.commentGeneration != generation {
			return
		}
		if err != nil {
			state.ActionError = err
			return
		}
		state.Comments = commentOverlay(comments, state.Session)
	}
	if target == m.activeTab {
		state := &reviewTabState{Session: m.Session, Comments: m.Comments, commentGeneration: m.commentGeneration, ActionError: m.ActionError}
		apply(state)
		m.Comments, m.ActionError = state.Comments, state.ActionError
	} else if target >= 0 && target < len(m.tabs) {
		apply(m.tabs[target].review)
	}
}

func (m *Model) refreshViewer() tea.Cmd {
	if m.readViewer == nil {
		return nil
	}
	target, read := m.activeTab, m.readViewer
	ctx := m.beginAction()
	return m.start(func() tea.Msg {
		viewer, err := read(ctx)
		return ViewerResult{Target: target, Viewer: viewer, Err: err}
	})
}

func commentOverlay(comments []source.ReviewComment, session *review.Session) []source.ReviewComment {
	if session == nil {
		return nil
	}
	metadata := session.Inventory.Comparison.Metadata
	targets := map[source.ReviewCommentTarget]bool{}
	for i := range session.Inventory.Units {
		for _, line := range unitLines(session, i) {
			if line.target != nil {
				targets[*line.target] = true
			}
		}
	}
	out := make([]source.ReviewComment, 0, len(comments))
	for _, comment := range comments {
		if comment.Target.Identity == metadata.Identity && comment.Target.CommitID == metadata.HeadSHA && targets[comment.Target] {
			out = append(out, comment)
		}
	}
	return out
}

func (m *Model) commentComposerKey(key tea.KeyPressMsg) tea.Cmd {
	composer := m.Composer
	if composer == nil {
		m.pop()
		return nil
	}
	m.editorCursorVisible = true
	switch key.String() {
	case "esc":
		m.Composer = nil
		return nil
	case "left":
		composer.Cursor = max(0, composer.Cursor-1)
		return nil
	case "right":
		composer.Cursor = min(len([]rune(composer.Draft)), composer.Cursor+1)
		return nil
	case "home":
		composer.Cursor = 0
		return nil
	case "end":
		composer.Cursor = len([]rune(composer.Draft))
		return nil
	case "backspace":
		if composer.Cursor > 0 {
			r := []rune(composer.Draft)
			composer.Draft = string(append(r[:composer.Cursor-1], r[composer.Cursor:]...))
			composer.Cursor--
		}
		return nil
	case "delete":
		r := []rune(composer.Draft)
		if composer.Cursor < len(r) {
			composer.Draft = string(append(r[:composer.Cursor], r[composer.Cursor+1:]...))
		}
		return nil
	case "shift+enter":
		r := []rune(composer.Draft)
		composer.Draft = string(append(append(r[:composer.Cursor], '\n'), r[composer.Cursor:]...))
		composer.Cursor++
		return nil
	case "enter":
		if m.submitComment == nil {
			m.ActionError = errors.New("review comment submission unavailable")
			return nil
		}
		if composer.Draft == "" {
			m.ActionError = errors.New("review comment body is required")
			return nil
		}
		composer.generation++
		generation, target, submit := composer.generation, m.activeTab, m.submitComment
		if m.Session == nil {
			m.ActionError = errors.New("review comment submission unavailable")
			return nil
		}
		submission := CommentSubmission{
			Comment:  source.ReviewComment{Target: composer.Target, Body: composer.Draft},
			Metadata: m.Session.Inventory.Comparison.Metadata,
		}
		m.notice = "Submitting pull request comment..."
		ctx := m.beginAction()
		return m.start(func() tea.Msg {
			comment, err := submit(ctx, submission)
			return CommentResult{Target: target, Generation: generation, Comment: comment, Err: err}
		})
	}
	if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
		r := []rune(composer.Draft)
		insert := []rune(key.Text)
		composer.Draft = string(append(append(r[:composer.Cursor], insert...), r[composer.Cursor:]...))
		composer.Cursor += len(insert)
	}
	return nil
}

func (m *Model) commentActionKey(key tea.KeyPressMsg) tea.Cmd {
	menu := m.CommentMenu
	if menu == nil {
		return nil
	}
	if key.String() == "esc" {
		m.CommentMenu = nil
		return nil
	}
	if menu.mode == commentActionReply {
		m.editorCursorVisible = true
	}
	if menu.mode == commentActionPick {
		switch key.String() {
		case "r":
			menu.mode = commentActionReply
			m.editorCursorVisible = true
			m.editorCursorGeneration++
			m.ensureReplyEditorVisible()
			return nextEditorCursorTick(m.editorCursorGeneration)
		case "a":
			menu.mode = commentActionReact
		case "d":
			if menu.Author == m.Viewer && m.Viewer != "" {
				menu.mode = commentActionDeleteConfirm
			}
		}
		return nil
	}
	if menu.mode == commentActionReply {
		defer m.ensureReplyEditorVisible()
	}
	switch menu.mode {
	case commentActionReact:
		reactions := source.ReviewCommentReactions()
		picked := key.String()
		if len(picked) != 1 || picked[0] < '1' || int(picked[0]-'1') >= len(reactions) {
			return nil
		}
		menu.Reaction, menu.Draft = reactions[int(picked[0]-'1')], ""
	case commentActionDeleteConfirm:
		if key.String() != "enter" {
			return nil
		}
		menu.Draft = ""
	default:
		switch key.String() {
		case "enter":
			if menu.Draft == "" {
				m.ActionError = errors.New("review comment reply body is required")
				return nil
			}
		case "left":
			menu.Cursor = max(0, menu.Cursor-1)
			return nil
		case "right":
			menu.Cursor = min(len([]rune(menu.Draft)), menu.Cursor+1)
			return nil
		case "home":
			menu.Cursor = 0
			return nil
		case "end":
			menu.Cursor = len([]rune(menu.Draft))
			return nil
		case "backspace":
			r := []rune(menu.Draft)
			if menu.Cursor > 0 {
				menu.Draft = string(append(r[:menu.Cursor-1], r[menu.Cursor:]...))
				menu.Cursor--
			}
			return nil
		case "shift+enter":
			r := []rune(menu.Draft)
			menu.Draft = string(append(append(r[:menu.Cursor], '\n'), r[menu.Cursor:]...))
			menu.Cursor++
			return nil
		case "delete":
			r := []rune(menu.Draft)
			if menu.Cursor < len(r) {
				menu.Draft = string(append(r[:menu.Cursor], r[menu.Cursor+1:]...))
			}
			return nil
		default:
			if key.Text != "" && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
				r, add := []rune(menu.Draft), []rune(key.Text)
				menu.Draft = string(append(append(r[:menu.Cursor], add...), r[menu.Cursor:]...))
				menu.Cursor += len(add)
			}
			return nil
		}
	}
	if m.submitCommentAction == nil || m.Session == nil {
		m.ActionError = errors.New("review comment action unavailable")
		return nil
	}
	menu.generation++
	generation, target, submit := menu.generation, m.activeTab, m.submitCommentAction
	commentID := menu.CommentID
	if menu.mode == commentActionReply && menu.ReplyToID > 0 {
		commentID = menu.ReplyToID
	}
	action := CommentAction{Metadata: m.Session.Inventory.Comparison.Metadata, Comment: source.ReviewComment{ID: commentID, Author: menu.Author, Target: menu.Target}, Body: menu.Draft, Reaction: menu.Reaction, Delete: menu.mode == commentActionDeleteConfirm}
	m.notice = "Submitting review comment action..."
	ctx := m.beginAction()
	return m.start(func() tea.Msg {
		reply, reaction, err := submit(ctx, action)
		return CommentActionResult{Target: target, CommentID: menu.CommentID, Generation: generation, Reply: reply, Reaction: reaction, Delete: action.Delete, Err: err}
	})
}

func (m *Model) applyCommentResult(result CommentResult) {
	apply := func(state *reviewTabState) {
		if state == nil || state.Composer == nil || state.Composer.generation != result.Generation {
			return
		}
		state.Busy = false
		state.ActionError = result.Err
		if result.Err == nil {
			state.Comments = append(state.Comments, result.Comment)
			state.Composer = nil
		}
	}
	if result.Target == m.activeTab {
		state := &reviewTabState{Composer: m.Composer, Comments: m.Comments, Stack: m.Stack, Busy: m.Busy, ActionError: m.ActionError}
		apply(state)
		m.Composer, m.Comments, m.Stack, m.Busy, m.ActionError = state.Composer, state.Comments, state.Stack, state.Busy, state.ActionError
		return
	}
	if result.Target >= 0 && result.Target < len(m.tabs) {
		apply(m.tabs[result.Target].review)
	}
}

func (m *Model) applyCommentActionResult(result CommentActionResult) {
	apply := func(state *reviewTabState) {
		if state == nil || state.CommentMenu == nil || state.CommentMenu.CommentID != result.CommentID || state.CommentMenu.generation != result.Generation {
			return
		}
		state.Busy, state.ActionError = false, result.Err
		if result.Err != nil {
			return
		}
		switch {
		case result.Delete:
			for i, comment := range state.Comments {
				if comment.ID == result.CommentID {
					state.Comments = append(state.Comments[:i], state.Comments[i+1:]...)
					break
				}
			}
		case result.Reply.ID > 0:
			state.Comments = append(state.Comments, result.Reply)
		case result.Reaction.ID > 0:
			if state.CommentReactions == nil {
				state.CommentReactions = map[int64][]source.ReviewCommentReaction{}
			}
			state.CommentReactions[result.CommentID] = append(state.CommentReactions[result.CommentID], result.Reaction)
		}
		state.CommentMenu = nil
	}
	if result.Target == m.activeTab {
		state := &reviewTabState{CommentMenu: m.CommentMenu, Comments: m.Comments, CommentReactions: m.CommentReactions, Busy: m.Busy, ActionError: m.ActionError}
		apply(state)
		m.CommentMenu, m.Comments, m.CommentReactions, m.Busy, m.ActionError = state.CommentMenu, state.Comments, state.CommentReactions, state.Busy, state.ActionError
		return
	}
	if result.Target >= 0 && result.Target < len(m.tabs) {
		apply(m.tabs[result.Target].review)
	}
}

func (m *Model) start(work func() tea.Msg) tea.Cmd {
	m.Busy = true
	result := make(chan tea.Msg, 1)
	done := make(chan struct{})
	m.worker = done
	go func() { defer close(done); result <- work() }()
	return waitForLoading(result)
}

func (m *Model) Close() {
	m.cancel()
	if m.worker != nil {
		<-m.worker
	}
}

func (m *Model) lifecycleKey(k string) (tea.Cmd, bool) {
	if m.store == nil || m.Session == nil {
		return nil, false
	}
	if m.selectedReviewView() == viewDescription && (k == "m" || k == "c") {
		return nil, true
	}
	s := *m.Session
	switch k {
	case "m":
		if len(s.Slices) == 0 {
			return nil, true
		}
		// Guide rows only ever point at a frozen unit, so marking resolves
		// through UnitFiles exactly as the file plan does: the whole slice is
		// marked, including the units this file contributes to other guides.
		id := s.Slices[s.UnitFiles[m.Selected]].FileID
		m.notice = "Saving local reading progress..."
		return m.start(func() tea.Msg {
			err := review.Mark(m.store, &s, id, !slices.Contains(s.ReviewedSliceIDs, id))
			return ActionResult{Session: &s, Err: err}
		}), true
	case "r":
		if m.reader == nil {
			m.ActionError = errors.New("offline mode: metadata refresh is unavailable")
			return nil, true
		}
		m.notice = "Checking GitHub metadata only; source stays local..."
		ctx := m.beginAction()
		return m.start(func() tea.Msg {
			err := review.Refresh(ctx, m.store, &s, m.reader)
			return ActionResult{Session: &s, Err: err}
		}), true
	case "c":
		if m.readComments == nil {
			m.ActionError = errors.New("offline mode: comment refresh is unavailable")
			return nil, true
		}
		return m.refreshComments(), true
	case "N":
		if m.fresh == nil {
			return nil, true
		}
		m.notice = "Opening new comparison; old snapshot and progress retained..."
		ctx := m.beginAction()
		return m.start(func() tea.Msg {
			n := m.notify
			if n == nil {
				n = func(string) {}
			}
			fresh, err := m.fresh(ctx, &s, n)
			return ActionResult{Session: fresh, Err: err, Reset: true}
		}), true
	case "s":
		return m.loadSessions(), true
	case "b":
		return m.loadRepositories(), true
	case "g":
		if m.generateGuide == nil {
			return nil, true
		}
		m.push(pageGuideConsent)
		return nil, true
	}
	return nil, false
}

func (m *Model) guideConsentKey(k string) tea.Cmd {
	switch k {
	case "esc":
		m.pop()
	case "enter":
		if m.generateGuide == nil || m.Session == nil {
			return nil
		}
		m.pop()
		m.notice = "Sending bounded pinned source and evidence to OpenAI..."
		ctx := m.beginAction()
		s := *m.Session
		return m.start(func() tea.Msg {
			n := m.notify
			if n == nil {
				n = func(string) {}
			}
			derived, err := m.generateGuide(ctx, &s, n)
			return ActionResult{Session: derived, Err: err, Reset: true}
		})
	}
	return nil
}

func (m *Model) pickerKey(k string) tea.Cmd {
	m.SessionPicker.clamp(len(m.Entries))
	switch k {
	case "r":
		return m.loadSessions()
	case "esc":
		m.pop()
	case "n", "down", "j":
		m.SessionPicker.Index = min(max(0, len(m.Entries)-1), m.SessionPicker.Index+1)
	case "p", "up", "k":
		m.SessionPicker.Index = max(0, m.SessionPicker.Index-1)
	case "enter":
		if len(m.Entries) == 0 {
			return nil
		}
		id := m.Entries[m.SessionPicker.Index].ID
		m.notice = "Resuming frozen session; checking metadata freshness..."
		ctx := m.beginAction()
		return m.start(func() tea.Msg {
			s, err := review.Resume(ctx, m.store, id, m.reader)
			return ActionResult{Session: s, Err: err, Reset: true}
		})
	}
	return nil
}

func (m *Model) repositoryPickerKey(k string) tea.Cmd {
	m.RepositoryPicker.clamp(len(m.Repositories))
	switch k {
	case "r":
		return m.loadRepositories()
	case "esc":
		m.pop()
	case "n", "down", "j":
		m.RepositoryPicker.Index = min(max(0, len(m.Repositories)-1), m.RepositoryPicker.Index+1)
	case "p", "up", "k":
		m.RepositoryPicker.Index = max(0, m.RepositoryPicker.Index-1)
	case "enter":
		if len(m.Repositories) == 0 || m.listPullRequests == nil {
			return nil
		}
		repository := m.Repositories[m.RepositoryPicker.Index].Repository
		return m.loadPullRequests(repository)
	}
	return nil
}

func (m *Model) loadPullRequests(repository string) tea.Cmd {
	m.notice = "Listing open pull requests from GitHub..."
	ctx := m.beginAction()
	list := m.listPullRequests
	target := m.activeTab
	return m.start(func() tea.Msg {
		if list == nil {
			return PullRequestListResult{Target: target, Repository: repository, Err: errors.New("pull request listing unavailable")}
		}
		prs, err := list(ctx, repository)
		return PullRequestListResult{PullRequests: prs, Repository: repository, Target: target, Err: err}
	})
}

// openSwitcher keeps the active review visible until the overlay is rendered.
// Listing is deliberately scoped to the active repository; open reviews are
// immediately useful even while that cancellable request is in flight.
func (m *Model) openSwitcher() tea.Cmd {
	if m.Session == nil {
		return nil
	}
	m.SwitcherQuery = ""
	m.PullRequestPicker = pickerState{}
	m.push(pagePullRequestPicker)
	repository := m.Session.Inventory.Comparison.Metadata.Identity.Repository
	if repository == "" || m.listPullRequests == nil {
		return nil
	}
	return m.loadPullRequests(repository)
}

func (m *Model) switcherResults() []switcherResult {
	query := strings.ToLower(strings.TrimSpace(m.SwitcherQuery))
	matches := func(s string) bool { return query == "" || strings.Contains(strings.ToLower(s), query) }
	results := make([]switcherResult, 0, len(m.tabs)+len(m.PullRequests))
	opened := make(map[source.Identity]bool, len(m.tabs))
	for _, tab := range m.tabs {
		opened[tab.identity] = true
		label := fmt.Sprintf("%s#%d", tab.identity.Repository, tab.identity.Number)
		if matches(label) {
			results = append(results, switcherResult{identity: tab.identity, title: label, open: true})
		}
	}
	for _, pr := range m.PullRequests {
		if opened[pr.Identity] {
			continue
		}
		label := fmt.Sprintf("%s#%d %s", pr.Identity.Repository, pr.Identity.Number, pr.Title)
		if matches(label) {
			results = append(results, switcherResult{identity: pr.Identity, title: pr.Title})
		}
	}
	return results
}

func (m *Model) pullRequestRepository() string {
	if m.currentRepository != "" {
		return m.currentRepository
	}
	if len(m.Repositories) == 0 {
		return ""
	}
	m.RepositoryPicker.clamp(len(m.Repositories))
	return m.Repositories[m.RepositoryPicker.Index].Repository
}

func (m *Model) pullRequestPickerKey(k string) tea.Cmd {
	if m.Session != nil {
		return m.switcherKey(k)
	}
	m.PullRequestPicker.clamp(len(m.PullRequests))
	switch k {
	case "esc":
		if m.currentRepository != "" && len(m.Stack) == 1 {
			m.cancel()
			return tea.Quit
		}
		m.pop()
	case "r":
		if repository := m.pullRequestRepository(); repository != "" {
			return m.loadPullRequests(repository)
		}
	case "n", "down", "j":
		m.PullRequestPicker.Index = min(max(0, len(m.PullRequests)-1), m.PullRequestPicker.Index+1)
	case "p", "up", "k":
		m.PullRequestPicker.Index = max(0, m.PullRequestPicker.Index-1)
	case "enter":
		if len(m.PullRequests) == 0 || m.openPullRequest == nil {
			return nil
		}
		pr := m.PullRequests[m.PullRequestPicker.Index]
		for i, tab := range m.tabs {
			if tab.identity == pr.Identity {
				m.activateTab(i)
				return nil
			}
		}
		if len(m.tabs) >= maxTabs {
			m.ActionError = fmt.Errorf("maximum of %d tabs open; switch to an existing review", maxTabs)
			return nil
		}
		checkout := m.currentCheckout
		if checkout == "" {
			if len(m.Repositories) == 0 {
				return nil
			}
			checkout = m.Repositories[0].Checkout
			for _, repository := range m.Repositories {
				if repository.Repository == pr.Identity.Repository {
					checkout = repository.Checkout
					break
				}
			}
		}
		m.notice = "Opening selected pull request; source stays local..."
		ctx := m.beginAction()
		target := m.activeTab
		return m.start(func() tea.Msg {
			n := m.notify
			if n == nil {
				n = func(string) {}
			}
			s, err := m.openPullRequest(ctx, checkout, pr.Identity, n)
			return PullRequestOpenResult{Target: target, Identity: pr.Identity, Session: s, Err: err}
		})
	}
	return nil
}

func (m *Model) switcherKey(k string) tea.Cmd {
	results := m.switcherResults()
	m.PullRequestPicker.clamp(len(results))
	switch k {
	case "esc":
		m.pop()
		return nil
	case "backspace":
		if len(m.SwitcherQuery) > 0 {
			m.SwitcherQuery = m.SwitcherQuery[:len(m.SwitcherQuery)-1]
			m.PullRequestPicker.Index = 0
		}
		return nil
	case "up", "k":
		m.PullRequestPicker.Index = max(0, m.PullRequestPicker.Index-1)
		return nil
	case "down", "j":
		m.PullRequestPicker.Index = min(max(0, len(results)-1), m.PullRequestPicker.Index+1)
		return nil
	case "enter":
		if len(results) == 0 {
			return nil
		}
		selected := results[m.PullRequestPicker.Index]
		if selected.open {
			if m.Busy {
				m.cancelCurrentAction()
			}
			m.pop() // do not persist the overlay in either review's saved state
			for i, tab := range m.tabs {
				if tab.identity == selected.identity {
					m.activateTab(i)
					return nil
				}
			}
		}
		return m.openSelectedPullRequest(selected.identity)
	}
	if len(k) == 1 && k >= " " && k <= "~" {
		m.SwitcherQuery += k
		m.PullRequestPicker.Index = 0
	}
	return nil
}

func (m *Model) openSelectedPullRequest(identity source.Identity) tea.Cmd {
	if m.openPullRequest == nil {
		m.ActionError = errors.New("pull request opening unavailable")
		return nil
	}
	if len(m.tabs) >= maxTabs {
		m.ActionError = fmt.Errorf("maximum of %d reviews open; switch to an existing review", maxTabs)
		return nil
	}
	checkout := m.currentCheckout
	if checkout == "" {
		for _, repository := range m.Repositories {
			if repository.Repository == identity.Repository {
				checkout = repository.Checkout
				break
			}
		}
	}
	if checkout == "" {
		m.ActionError = errors.New("no pinned checkout available for selected pull request")
		return nil
	}
	m.notice = "Opening selected pull request; source stays local..."
	ctx := m.beginAction()
	target := m.activeTab
	return m.start(func() tea.Msg {
		n := m.notify
		if n == nil {
			n = func(string) {}
		}
		s, err := m.openPullRequest(ctx, checkout, identity, n)
		return PullRequestOpenResult{Target: target, Identity: identity, Session: s, Err: err}
	})
}

func (m *Model) pickerView() string {
	rows := make([]string, len(m.Entries))
	for i, entry := range m.Entries {
		text := entry.ID
		if entry.Err != nil {
			text += " UNREADABLE: " + entry.Err.Error()
		} else {
			meta := entry.Record.Inventory.Comparison.Metadata
			text += fmt.Sprintf(" %s #%d %d/%d read", meta.Identity.Repository, meta.Identity.Number, len(entry.Record.ReviewedSliceIDs), len(entry.Record.Slices))
		}
		rows[i] = Escape(text)
	}
	storage := ""
	if m.store != nil {
		storage = "Storage: " + Escape(m.store.Path())
	}
	return m.pickerScreen(&m.SessionPicker, []string{appHeader("Sessions", ""), "Stored checks are historical; resume checks freshness.", storage}, rows, "No saved sessions.", "enter: resume · r: reload · esc: back")
}

func (m *Model) repositoryPickerView() string {
	rows := make([]string, len(m.Repositories))
	for i, repository := range m.Repositories {
		rows[i] = Escape(repository.Repository)
	}
	return m.pickerScreen(&m.RepositoryPicker, []string{appHeader("Repositories", ""), "Select a repository to list its open pull requests."}, rows, "No remembered repositories. Open a PR with --repo first.", "enter: open · r: reload · esc: back")
}

func (m *Model) switcherView() string {
	results := m.switcherResults()
	m.PullRequestPicker.clamp(len(results))
	rows := make([]string, len(results))
	for i, result := range results {
		if result.open {
			rows[i] = "open " + Escape(result.title)
		} else {
			rows[i] = fmt.Sprintf("#%d %s", result.identity.Number, Escape(result.title))
		}
	}
	header := []string{appHeader("Switch pull requests", "type to filter"), "filter: " + Escape(m.SwitcherQuery)}
	footer := "type: filter · enter: switch/open · esc: cancel"
	return m.pickerScreen(&m.PullRequestPicker, header, rows, "No matching open reviews or pull requests.", footer)
}

func (m *Model) footer() string {
	if m.Busy {
		return Escape(m.notice) + " | q: cancel"
	}
	if m.ActionError != nil {
		return "Action failed; snapshot retained: " + Escape(m.ActionError.Error())
	}
	return strings.ReplaceAll(renderBindings(groupFooter), "\n", "  ")
}

func progress(s *review.Session) string {
	state := s.RevisionStatus
	if state == "" {
		state = session.Unchecked
	}
	text := fmt.Sprintf("freshness: %s | %d/%d read (local)", state, len(s.ReviewedSliceIDs), len(s.Slices))
	if state == session.Stale {
		text += " | keep reading old or N: new/reset"
	}
	if state == session.Unchecked || state == session.CheckFailed {
		text += " | freshness unknown"
	}
	return text
}

// progressClass warns on freshness states that are not confirmed current. The
// label text itself is unchanged, so the state survives without color.
func progressClass(s *review.Session) lineClass {
	if s.RevisionStatus == session.Stale || s.RevisionStatus == session.CheckFailed {
		return classWarning
	}
	return classTitle
}

func readMarker(s *review.Session, id string) string {
	if s.ID == "" {
		return ""
	}
	if slices.Contains(s.ReviewedSliceIDs, id) {
		return "[x] "
	}
	return "[ ] "
}
