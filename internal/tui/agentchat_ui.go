package tui

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/agentchat"
	"prui/internal/commits"
	"prui/internal/guideconfig"
	"prui/internal/privacy"
	"prui/internal/review"
	"prui/internal/source"
	featureagent "prui/internal/tui/features/agentchat"
)

// AgentAnswerer sends exactly one explicit turn to the thread's pinned guide
// recipient. Its invocation must not access the currently active review tab.
type AgentAnswerer = featureagent.Answerer
type agentThread = featureagent.Thread

type agentAnswerResult struct {
	tab                  *reviewTabState
	session              *review.Session
	comparisonID         string
	threadID, generation uint64
	manifest             agentchat.Manifest
	answer               agentchat.ResolvedAnswer
	err                  error
}

func (m *Model) SetAgentAnswerer(answer AgentAnswerer) { m.agentAnswerer = answer }

func (m *Model) agentThread(id uint64) *agentThread {
	thread := m.agent.Thread(id)
	if thread.ID == 0 {
		return nil
	}
	return &thread
}

func (m *Model) agentActive() *agentThread {
	thread := m.agent.ActiveThread()
	if thread.ID == 0 {
		return nil
	}
	return &thread
}

func (m *Model) currentAgentRecipient() guideconfig.Selection {
	if m.guideOptionsSet {
		return m.guideConfirmed
	}
	return m.currentGuideSelection()
}

// activateAgentComposer is called only when the inline editor selects Agent
// question mode. It retains the pinned recipient and unsent draft for an
// already opened anchor without starting a provider request.
func (m *Model) activateAgentComposer(anchor source.ReviewCommentTarget) string {
	if m.agentAnswerer == nil {
		m.ActionError = errors.New("agent answering is unavailable")
		return ""
	}
	if m.Session == nil || source.ValidateReviewCommentTarget(anchor) != nil || !commits.InventoryContainsTarget(m.Session.Inventory.Files, m.Session.Inventory.Units, m.Session.Inventory.Patches, anchor) {
		m.ActionError = errors.New("select a canonical PR diff line or valid range in Files or Guide")
		return ""
	}
	choice := m.currentAgentRecipient()
	if choice.Provider == "" || !guideconfig.ValidModel(choice.Model) {
		m.ActionError = errors.New("configure and confirm a guide model before asking")
		return ""
	}
	if current := m.agent.ActiveThread(); current.ID != 0 && current.Anchor == anchor {
		m.ActionError = nil
		return current.Draft
	}
	thread, _ := m.agent.Open(anchor, choice, m.offset(), uint8(m.selectedReviewView()))
	m.ActionError = nil
	return thread.Draft
}

func (m *Model) saveAgentComposerDraft(anchor source.ReviewCommentTarget, draft string) {
	m.agent.SetDraftAtAnchor(anchor, draft)
}

// newAgentComposerThread makes recipient changes explicit while retaining the
// old pinned conversation and its unsent question in this tab.
func (m *Model) newAgentComposerThread(c *commentComposer) string {
	if c == nil || !c.Agent || m.agent.HasPending() {
		m.ActionError = errors.New("finish or cancel the pending agent request first")
		return ""
	}
	old := m.agentActive()
	if old == nil || old.Anchor != c.Target {
		m.ActionError = errors.New("agent thread is unavailable for this source anchor")
		return ""
	}
	choice := m.currentAgentRecipient()
	if choice.Provider == "" || !guideconfig.ValidModel(choice.Model) {
		m.ActionError = errors.New("configure and confirm a guide model before asking")
		return ""
	}
	if choice == old.Recipient {
		m.ActionError = errors.New("select and confirm a different guide model before starting a new thread")
		return ""
	}
	m.agent.SetDraft(old.ID, c.Draft)
	m.agent.New(c.Target, choice, m.offset(), uint8(m.selectedReviewView()))
	m.ActionError = nil
	return ""
}

// openAgentCardComposer routes Enter on a non-citation card row back into the
// shared inline editor, preloaded with this thread's private draft.
func (m *Model) openAgentCardComposer(thread *agentThread) tea.Cmd {
	if thread == nil || m.Composer != nil {
		return nil
	}
	if !m.agent.Activate(thread.ID) {
		return nil
	}
	m.Focus = paneDiff
	m.Composer = &commentComposer{Target: thread.Anchor, Agent: true, Draft: thread.Draft, Cursor: len([]rune(thread.Draft)), PendingIndex: -1}
	m.ActionError = nil
	m.editorCursorVisible = true
	m.editorCursorGeneration++
	m.ensureInlineEditorVisible()
	return nextEditorCursorTick(m.editorCursorGeneration)
}

// sendAgentComposerQuestion is the explicit Enter path for the inline Agent
// mode. It never routes through comment submission or persists a draft.
func (m *Model) sendAgentComposerQuestion(c *commentComposer) tea.Cmd {
	if c == nil || m.Composer != c {
		return nil
	}
	thread := m.agentActive()
	if thread == nil || thread.Anchor != c.Target {
		m.activateAgentComposer(c.Target)
		thread = m.agentActive()
		if m.ActionError != nil || thread == nil || thread.Anchor != c.Target {
			return nil
		}
	}
	m.agent.SetDraft(thread.ID, c.Draft)
	cmd := m.dispatchAgentQuestion(func() { m.Composer = nil })
	if cmd == nil {
		if m.agentAnswerer == nil || m.Session == nil {
			m.ActionError = errors.New("agent answering is unavailable")
			return nil
		}
		if active := m.agentActive(); active != nil && active.Err != "" {
			m.ActionError = errors.New(active.Err)
		}
		return nil
	}
	m.ActionError = nil
	return cmd
}

func safeAgentDestination(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return "configured destination"
	}
	return u.Scheme + "://" + u.Host
}

func safeAgentFailure(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY"} {
		if message == name+" is required to ask the agent" || message == name+" is required to send source for analysis" {
			return name + " is required to ask the agent; question kept for retry"
		}
	}
	if strings.HasPrefix(message, "offline mode:") {
		return "Offline mode blocks agent requests; question kept for retry"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Agent request timed out; question kept for retry"
	}
	const providerHTTP = "agent provider request failed: HTTP "
	if statusText, ok := strings.CutPrefix(message, providerHTTP); ok {
		if status, parseErr := strconv.Atoi(statusText); parseErr == nil && status >= 100 && status <= 599 {
			switch status {
			case 401:
				return "Agent request failed (HTTP 401); check API key; question kept for retry"
			case 403:
				return "Agent request failed (HTTP 403); check provider access; question kept for retry"
			case 404:
				return "Agent request failed (HTTP 404); check model ID and endpoint; question kept for retry"
			case 429:
				return "Agent request hit a rate limit (HTTP 429); question kept for retry"
			}
			if status >= 500 {
				return "Agent provider unavailable (HTTP " + statusText + "); question kept for retry"
			}
			return "Agent request failed (HTTP " + statusText + "); check provider/model settings; question kept for retry"
		}
	}
	switch message {
	case "agent model selection is invalid", "agent credential configuration is invalid", "agent provider selection is invalid":
		return message + "; check global guide configuration"
	case "agent provider returned incomplete output":
		return "Agent provider returned incomplete output; question kept for retry"
	case "agent provider returned invalid output":
		return "Agent provider returned invalid output; question kept for retry"
	}
	return "Agent request failed; question kept for retry"
}

func (m *Model) dispatchAgentQuestion(closeComposer func()) tea.Cmd {
	thread := m.agentActive()
	if thread == nil || m.Session == nil || m.agentAnswerer == nil {
		return nil
	}
	if m.agent.HasPending() {
		m.agent.Fail(thread.ID, "finish or cancel the pending request first")
		return nil
	}
	history := m.agent.History(thread.ID)
	manifest, err := agentchat.Build(m.Session.Inventory, m.Session.Context, privacy.Policy{}, thread.Anchor, thread.Draft, history)
	if err != nil {
		m.agent.Fail(thread.ID, err.Error())
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	guideIndex := -1
	if guide, ok := m.activeGuide(); ok {
		guideIndex = guide
	}
	request := m.agent.Begin(thread.ID, cancel, m.agentAnchorRow(thread.Anchor), uint8(m.selectedReviewView()), guideIndex)
	tab, session, answer := m.reviewTabState, m.Session, m.agentAnswerer
	choice := thread.Recipient
	comparisonID := session.Inventory.Comparison.InventoryID
	closeComposer()
	return func() tea.Msg {
		result, err := answer(ctx, choice, manifest)
		cancel()
		return agentAnswerResult{tab: tab, session: session, comparisonID: comparisonID, threadID: thread.ID, generation: request.Generation, manifest: manifest, answer: result, err: err}
	}
}

func (m *Model) cancelAgentRequest() {
	m.agent.Cancel()
}

func (m *Model) agentAnchorRow(anchor source.ReviewCommentTarget) int {
	end := overlayEndpoint(anchor)
	last := -1
	for i, row := range m.displayDetail() {
		for _, target := range sourceLineTargets(row) {
			if target == end {
				last = i
			}
		}
		if row.sideBySide != nil {
			for _, target := range rowTargets(*row.sideBySide) {
				if target == end {
					last = i
				}
			}
		}
	}
	return last
}

func (m *Model) toggleAgentCard(thread *agentThread) {
	if thread == nil {
		return
	}
	if !m.agent.Toggle(thread.ID) {
		return
	}
	anchorRow := m.agentAnchorRow(thread.Anchor)
	if anchorRow < 0 {
		return
	}
	m.Focus, m.cursorActive = paneDiff, true
	m.setCursor(anchorRow)
	if thread.SourceView == uint8(m.selectedReviewView()) {
		m.setOffset(m.clampOffset(thread.SourceOffset))
	} else {
		m.setOffset(m.clampOffset(max(0, anchorRow-2)))
	}
	if anchorRow < m.offset() || anchorRow >= m.offset()+m.bodyHeight() {
		m.setOffset(m.clampOffset(max(0, anchorRow-2)))
	}
}

func (m *Model) applyAgentAnswer(result agentAnswerResult) {
	if result.tab == nil || result.session == nil {
		return
	}
	owned := false
	for i := range m.tabs {
		if m.tabs[i].review == result.tab {
			owned = true
			break
		}
	}
	if !owned && result.tab != m.reviewTabState {
		return
	}
	state := result.tab
	if state.Session != result.session || state.Session.Inventory.Comparison.InventoryID != result.comparisonID {
		return
	}
	pending := state.agent.PendingRequest()
	if pending.ThreadID == 0 || pending.ThreadID != result.threadID || pending.Generation != result.generation {
		return
	}
	view := state.agent.Thread(result.threadID)
	thread := &view
	if thread.ID == 0 || thread.Generation != result.generation {
		state.agent.Accept(result.threadID, result.generation, result.manifest, result.answer, safeAgentFailure(result.err))
		return
	}
	oldCardLength := len(m.agentCardLinesFor(thread, true))
	oldOffset, oldLength, oldCursor, anchorBefore, cardEnd := 0, 0, -1, -1, -1
	if state == m.reviewTabState && m.diffReviewView() {
		rows := m.displayDetail()
		oldOffset, oldLength, oldCursor = m.offset(), len(rows), m.cursorInDetail(rows)
		end := overlayEndpoint(thread.Anchor)
		for i, row := range rows {
			if row.agentThreadID == thread.ID {
				cardEnd = i
			}
			for _, target := range sourceLineTargets(row) {
				if target == end {
					anchorBefore = i
				}
			}
			if row.sideBySide != nil {
				for _, target := range rowTargets(*row.sideBySide) {
					if target == end {
						anchorBefore = i
					}
				}
			}
		}
	}
	state.agent.Accept(result.threadID, result.generation, result.manifest, result.answer, safeAgentFailure(result.err))
	updated := state.agent.Thread(result.threadID)
	thread = &updated
	if anchorBefore >= 0 {
		delta := len(m.displayDetail()) - oldLength
		if anchorBefore < oldOffset {
			m.setOffset(oldOffset + delta)
		}
		if oldCursor > cardEnd && cardEnd >= 0 {
			m.setCursor(oldCursor + delta)
		}
	} else if state != m.reviewTabState && pending.AnchorRow >= 0 && uint8(state.ContextView) == pending.View {
		delta := len(m.agentCardLinesFor(thread, false)) - oldCardLength
		if pending.View == uint8(viewFiles) && state.Scroll[-1] > pending.AnchorRow {
			state.Scroll[-1] = max(0, state.Scroll[-1]+delta)
		}
		if pending.View == uint8(viewGuide) && pending.GuideIndex >= 0 && state.GuideScroll[pending.GuideIndex] > pending.AnchorRow {
			state.GuideScroll[pending.GuideIndex] = max(0, state.GuideScroll[pending.GuideIndex]+delta)
		}
	}
}

func (m *Model) agentCardLines(thread *agentThread) []diffLine {
	pending := m.agent.HasPending() && m.agent.PendingRequest().ThreadID == thread.ID
	return m.agentCardLinesFor(thread, pending)
}

func (m *Model) agentCardLinesFor(thread *agentThread, pending bool) []diffLine {
	inner := m.overlayInnerWidth(0)
	semantic := featureagent.CardRows(*thread, pending, func(answer string) []string {
		return m.conversationLinesAt(answer, inner, true)
	})
	prefix := "  "
	border := prefix + "+" + strings.Repeat("-", inner+2) + "+"
	lines := []diffLine{{styledLine: styledLine{Class: classMetadata, Text: border}, agentThreadID: thread.ID}}
	for _, item := range semantic {
		class := classMetadata
		switch item.Kind {
		case featureagent.CardTitle, featureagent.CardSection:
			class = classTitle
		case featureagent.CardBody:
			class = classPlain
		case featureagent.CardWarning:
			class = classWarning
		case featureagent.CardQuestion:
			class = classAgentQuestion
		}
		text := item.Text
		switch item.Kind {
		case featureagent.CardDivider:
			text = strings.Repeat("-", inner)
		case featureagent.CardBody:
			if m.colorProfile <= colorprofile.Ascii {
				text = ansi.Strip(text)
			}
		default:
			text = Escape(text)
		}
		for _, part := range strings.Split(ansi.Wrap(text, inner, ""), "\n") {
			part += strings.Repeat(" ", max(0, inner-visibleWidth(part)))
			lines = append(lines, diffLine{styledLine: styledLine{Class: class, Text: prefix + "| " + part + " |"}, agentThreadID: thread.ID, agentCitation: item.Citation, agentTurn: item.Turn, agentReference: item.Reference})
		}
	}
	return append(lines, diffLine{styledLine: styledLine{Class: classMetadata, Text: border}, agentThreadID: thread.ID})
}

func (m *Model) agentCardAtCursor() *agentThread {
	if !m.diffReviewView() || m.Focus != paneDiff || !m.cursorActive {
		return nil
	}
	rows := m.displayDetail()
	i := m.cursorInDetail(rows)
	if i < 0 || i >= len(rows) {
		return nil
	}
	return m.agentThread(rows[i].agentThreadID)
}

func (m *Model) openAgentEvidence(thread *agentThread) {
	if thread == nil || thread.TurnCount() == 0 {
		m.ActionError = errors.New("no sent evidence to inspect")
		return
	}
	turn, reference := thread.TurnCount()-1, 0
	if m.diffReviewView() && m.Focus == paneDiff && m.cursorActive {
		rows := m.displayDetail()
		if cursor := m.cursorInDetail(rows); cursor >= 0 && cursor < len(rows) && rows[cursor].agentThreadID == thread.ID && rows[cursor].agentCitation {
			turn, reference = rows[cursor].agentTurn, rows[cursor].agentReference
		}
	}
	if thread.Turn(turn).ReferenceCount() == 0 {
		m.ActionError = errors.New("answer has no source references")
		return
	}
	m.agent.SelectEvidence(thread.ID, turn, reference)
	m.push(pageAgentEvidence)
}

func (m *Model) agentEvidenceView() string {
	s := m.agent.Evidence()
	thread := m.agentThread(s.ThreadID)
	if thread == nil {
		return "Evidence unavailable · esc: back"
	}
	lines, ok := featureagent.EvidenceLines(*thread, s)
	if !ok {
		return "Evidence unavailable · esc: back"
	}
	for i := range lines {
		lines[i] = Escape(lines[i])
	}
	available := max(1, m.Height-8)
	start := min(s.Scroll, max(0, len(lines)-available))
	end := min(len(lines), start+available)
	return strings.Join(append(lines[start:end], "j/k: scroll · left/right: other reference · esc: return to thread"), "\n")
}

func (m *Model) agentEvidenceKey(k string) {
	if k == "esc" {
		m.pop()
		return
	}
	m.agent.NavigateEvidence(k)
}
