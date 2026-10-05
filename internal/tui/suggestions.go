package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/commits"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
)

type SuggestionPreparer func(context.Context, source.Metadata, source.ReviewComment, string) (source.SuggestionApplication, error)
type SuggestionCommitter func(context.Context, source.SuggestionApplication) (source.SuggestionResult, error)
type suggestionPrepared struct {
	target      int
	application source.SuggestionApplication
	err         error
}
type suggestionApplied struct {
	target int
	result source.SuggestionResult
	err    error
}

func (m *Model) SetSuggestionActions(prepare SuggestionPreparer, apply SuggestionCommitter) {
	m.prepareSuggestion = prepare
	m.applySuggestion = apply
}
func composerBody(c *commentComposer) string {
	if c.Suggestion {
		body, _ := source.SuggestionBody(c.Draft)
		return body
	}
	return c.Draft
}
func (m *Model) targetSource(t source.ReviewCommentTarget) (string, bool) {
	if m.Session == nil {
		return "", false
	}
	i := m.Session.Inventory
	return commits.TargetSource(i.Files, i.Units, i.Patches, t)
}
func (m *Model) openSuggestionComposer() tea.Cmd {
	if m.commitFilter.subset {
		m.ActionError = errors.New("suggestions require the pinned PR diff; filtered commit comparisons are read-only")
		return nil
	}
	if m.selectedReviewView() == viewCommits {
		m.ActionError = errors.New("new historical suggestions unsupported; select PR Files")
		return nil
	}
	t := m.commentSelectionTarget()
	if t == nil {
		return nil
	}
	selected, err := m.completeCommentRange(*t)
	if err != nil {
		m.ActionError = err
		return nil
	}
	if selected.Side != "RIGHT" || selected.SubjectType != "" {
		m.ActionError = errors.New("suggestions require right-side lines")
		return nil
	}
	before, ok := m.targetSource(selected)
	if !ok {
		m.ActionError = errors.New("suggestion source unavailable")
		return nil
	}
	cmd := m.openCommentComposer()
	if m.Composer == nil {
		return cmd
	}
	c := m.Composer
	if c.Suggestion {
		return cmd
	}
	if c.Draft != "" {
		replacement, err := source.ParseSuggestion(c.Draft)
		if err != nil {
			m.ActionError = errors.New("existing comment is not a supported suggestion; retained")
			return cmd
		}
		c.Draft = replacement
	} else {
		c.Draft = before
	}
	c.Before = before
	c.Suggestion = true
	c.Cursor = len([]rune(c.Draft))
	return cmd
}
func (m *Model) suggestionComposerKey(key tea.KeyPressMsg) tea.Cmd {
	c := m.Composer
	switch key.String() {
	case "enter", "ctrl+p":
		body, err := source.SuggestionBody(c.Draft)
		if err != nil {
			m.ActionError = err
			return nil
		}
		// The ordinary submission seam stores/sends the exact generated Markdown.
		c.Suggestion = false
		replacement := c.Draft
		c.Draft = body
		cmd := m.commentComposerKey(key)
		if m.Composer == c {
			c.Draft = replacement
			c.Suggestion = true
			c.Cursor = min(c.Cursor, len([]rune(replacement)))
		}
		return cmd
	default:
		c.Suggestion = false
		cmd := m.commentComposerKey(key)
		if m.Composer == c {
			c.Suggestion = true
		}
		return cmd
	}
}
func suggestionPreview(before, after string) []diffLine {
	lines := []diffLine{{styledLine: styledLine{Class: classMetadata, Text: "Suggestion · Before"}}}
	for _, v := range strings.Split(before, "\n") {
		lines = append(lines, diffLine{styledLine: styledLine{Class: classPlain, Text: "- " + Escape(v)}})
	}
	lines = append(lines, diffLine{styledLine: styledLine{Class: classMetadata, Text: "After"}})
	if after == "" {
		lines = append(lines, diffLine{styledLine: styledLine{Class: classPlain, Text: "(delete selected lines)"}})
	} else {
		for _, v := range strings.Split(after, "\n") {
			lines = append(lines, diffLine{styledLine: styledLine{Class: classPlain, Text: "+ " + Escape(v)}})
		}
	}
	return lines
}
func (m *Model) prepareSelectedSuggestion() tea.Cmd {
	if m.prepareSuggestion == nil || m.Session == nil {
		m.ActionError = errors.New("suggestion application unavailable offline")
		return nil
	}
	var comment source.ReviewComment
	for _, c := range m.Comments {
		if c.ID == m.CommentMenu.CommentID {
			comment = c
			break
		}
	}
	before, ok := m.targetSource(comment.Target)
	if !ok {
		m.ActionError = errors.New("unsupported or stale suggestion: captured source unavailable")
		return nil
	}
	meta, prepare, target := m.Session.Inventory.Comparison.Metadata, m.prepareSuggestion, m.activeTab
	ctx := m.beginAction()
	return m.start(func() tea.Msg {
		a, err := prepare(ctx, meta, comment, before)
		return suggestionPrepared{target, a, err}
	})
}
func (m *Model) acceptSuggestionPrepared(v suggestionPrepared) {
	state := m.reviewStateForTarget(v.target)
	if state == nil {
		return
	}
	state.Busy = false
	state.ActionError = v.err
	if v.err == nil {
		state.SuggestionApply = &v.application
		state.SuggestionConfirm = false
		state.SuggestionScroll = 0
		state.CommentMenu = nil
		state.Stack = append(state.Stack, pageSuggestionApply)
	}
}
func (m *Model) suggestionApplyView() string {
	a := m.SuggestionApply
	if a == nil {
		return "Suggestion unavailable · esc: back"
	}
	content := []string{commentTargetLabel(a.Target), "Head repository: " + Escape(a.Metadata.HeadRepository), "Branch: " + Escape(a.Branch)}
	for _, l := range suggestionPreview(a.Before, a.Replacement) {
		content = append(content, l.Text)
	}
	width := max(1, min(96, m.Width-12))
	rows := strings.Split(ansi.Wrap(strings.Join(content, "\n"), width, ""), "\n")
	footer := "enter: review confirmation · esc: back"
	switch {
	case m.draft.attempt != "":
		footer = "Outcome uncertain · ctrl+r: reconcile · ctrl+d: discard"
	case m.SuggestionConfirm:
		footer = "enter: confirm remote commit · esc: edit confirmation"
	}
	available := max(1, m.Height-7)
	if m.ActionError != nil {
		available--
	}
	available = max(1, available)
	m.SuggestionScroll = min(m.SuggestionScroll, max(0, len(rows)-available))
	lines := []string{"Apply suggestion as a GitHub commit"}
	lines = append(lines, rows[m.SuggestionScroll:min(len(rows), m.SuggestionScroll+available)]...)
	lines = append(lines, fmt.Sprintf("j/k: scroll preview · %d/%d", m.SuggestionScroll+1, len(rows)))
	if m.ActionError != nil {
		lines = append(lines, "! "+clip(Escape(m.ActionError.Error()), width))
	}
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}

func (m *Model) suggestionApplyKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "j", "down":
		m.SuggestionScroll++
		return nil
	case "k", "up":
		m.SuggestionScroll = max(0, m.SuggestionScroll-1)
		return nil
	}
	if k.String() == "ctrl+r" && m.draft.attempt != "" {
		return m.reconcileDraftCommand()
	}
	if k.String() == "ctrl+d" {
		m.SuggestionApply = nil
		m.draft.attempt = ""
		m.draft.attempted = nil
		m.pop()
		return nil
	}
	if m.draft.attempt != "" {
		m.ActionError = errors.New("outcome uncertain: reconcile before retry")
		return nil
	}
	if k.String() == "esc" {
		if m.SuggestionConfirm {
			m.SuggestionConfirm = false
		} else {
			m.pop()
		}
		return nil
	}
	if k.String() != "enter" || m.SuggestionApply == nil {
		return nil
	}
	if m.Width < 30 || m.Height < 10 {
		m.ActionError = errors.New("enlarge terminal to review suggestion confirmation")
		return nil
	}
	if !m.SuggestionConfirm {
		m.SuggestionConfirm = true
		return nil
	}
	if m.applySuggestion == nil {
		m.ActionError = errors.New("suggestion application unavailable offline")
		return nil
	}
	if !m.prepareDraftAttempt("suggestion") {
		return nil
	}
	// Even memory-only sessions retain an exact attempted payload.
	if m.draft.attempt == "" {
		copy := *m.SuggestionApply
		m.draft.attempt = "suggestion"
		m.draft.attempted = &session.DraftAttempt{Kind: "suggestion", Application: &copy}
	}
	a, apply, target := *m.SuggestionApply, m.applySuggestion, m.activeTab
	ctx := m.beginAction()
	return m.start(func() tea.Msg { result, err := apply(ctx, a); return suggestionApplied{target, result, err} })
}
func (m *Model) acceptSuggestionApplied(v suggestionApplied) {
	state := m.reviewStateForTarget(v.target)
	if state == nil {
		return
	}
	state.Busy = false
	state.ActionError = v.err
	state.SuggestionConfirm = false
	if v.err == nil {
		state.draft.attempt = ""
		state.draft.attempted = nil
		state.SuggestionApply = nil
		state.notice = "Suggestion committed · " + Escape(v.result.SHA) + " · open a new comparison"
		if state.Stack[len(state.Stack)-1] == pageSuggestionApply {
			state.Stack = state.Stack[:len(state.Stack)-1]
		}
	}
}

func (m *Model) restoreSuggestionEditor(c *commentComposer) {
	if c == nil || c.Suggestion {
		return
	}
	replacement, err := source.ParseSuggestion(c.Draft)
	if err != nil {
		return
	}
	before, ok := m.targetSource(c.Target)
	if !ok {
		return
	}
	c.Suggestion = true
	c.Before = before
	c.Draft = replacement
	c.Cursor = len([]rune(replacement))
}
