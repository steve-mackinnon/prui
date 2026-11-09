// Package agentchat owns tab-local private conversation state independently of the TUI model.
package agentchat

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	chat "prui/internal/agentchat"
	"prui/internal/guideconfig"
	"prui/internal/source"
)

type Answerer func(context.Context, guideconfig.Selection, chat.Manifest) (chat.ResolvedAnswer, error)

// Thread is a detached metadata view. Turn payloads remain behind read-only
// accessors, so rendering can inspect bounded evidence without copying it.
type Thread struct {
	ID           uint64
	Anchor       source.ReviewCommentTarget
	Recipient    guideconfig.Selection
	Draft        string
	turns        []Turn
	Collapsed    bool
	Err          string
	Generation   uint64
	SourceOffset int
	SourceView   uint8
}

type Turn struct {
	Manifest chat.Manifest
	Answer   chat.ResolvedAnswer
}

type CardRowKind uint8

const (
	CardTitle CardRowKind = iota
	CardMetadata
	CardBody
	CardWarning
	CardSection
	CardSpacer
	CardDivider
	CardQuestion
)

type CardRow struct {
	Kind            CardRowKind
	Text            string
	Citation        bool
	Turn, Reference int
}

// CardRows owns the semantic sequence of questions, answers, citations and
// pending/error controls. The host supplies only Markdown line wrapping.
func CardRows(thread Thread, pending bool, answerLines func(string) []string) []CardRow {
	rows := []CardRow{{Kind: CardTitle, Text: "Agent Q&A · " + thread.Recipient.Provider + "/" + thread.Recipient.Model}}
	if thread.Collapsed {
		return append(rows, CardRow{Kind: CardMetadata, Text: "Q: expand · Enter: follow up"})
	}
	for turnIndex := 0; turnIndex < thread.TurnCount(); turnIndex++ {
		turn := thread.Turn(turnIndex)
		rows = append(rows, CardRow{Kind: CardSpacer})
		if turnIndex > 0 {
			rows = append(rows, CardRow{Kind: CardDivider}, CardRow{Kind: CardSpacer})
		}
		rows = append(rows, CardRow{Kind: CardQuestion, Text: "Question:"})
		rows = append(rows, CardRow{Kind: CardQuestion, Text: turn.Question()})
		rows = append(rows, CardRow{Kind: CardSpacer}, CardRow{Kind: CardSection, Text: "Answer:"})
		for _, text := range answerLines(turn.AnswerText()) {
			rows = append(rows, CardRow{Kind: CardBody, Text: text})
		}
		if turn.ReferenceCount() > 0 {
			rows = append(rows, CardRow{Kind: CardSpacer}, CardRow{Kind: CardSection, Text: "Source references · Enter on a reference to inspect"})
		}
		for i := 0; i < turn.ReferenceCount(); i++ {
			ref := turn.Reference(i)
			row := CardRow{Kind: CardWarning, Text: fmt.Sprintf("[%d] unavailable reference", i+1), Citation: true, Turn: turnIndex, Reference: i}
			if ref.Available() {
				row.Kind = CardMetadata
				row.Text = fmt.Sprintf("[%d] %s:%d-%d @ %.12s · Enter: inspect", i+1, ref.Path(), ref.StartLine(), ref.EndLine(), ref.Revision())
			}
			rows = append(rows, row)
		}
	}
	if pending {
		rows = append(rows, CardRow{Kind: CardSpacer}, CardRow{Kind: CardWarning, Text: "Agent is answering… · X: cancel"})
	}
	if thread.Err != "" {
		rows = append(rows, CardRow{Kind: CardSpacer}, CardRow{Kind: CardWarning, Text: thread.Err})
	}
	footer := "Enter: follow up · Enter on reference: inspect · Q: collapse"
	if pending {
		footer += " · X: cancel pending"
	}
	return append(rows, CardRow{Kind: CardSpacer}, CardRow{Kind: CardMetadata, Text: footer})
}

// ScopeLines describes the exact anchor, pinned recipient, upload, and lifetime
// beside the inline editor before explicit consent.
func ScopeLines(thread Thread, destination string) []string {
	start := thread.Anchor.StartLine
	if start == 0 {
		start = thread.Anchor.Line
	}
	lines := []string{
		fmt.Sprintf("Ask agent · %s:%d-%d %s", thread.Anchor.Path, start, thread.Anchor.Line, thread.Anchor.Side),
		"Recipient: " + thread.Recipient.Provider + "/" + thread.Recipient.Model + " · " + destination,
		"Upload on Send: selected hunk, up to 3 captured excerpts, recent turns (16 KiB).",
		"Scope: frozen PR only; no repository search or full-file source.",
		"Lifetime: private in this tab; discarded on close or comparison change.",
	}
	if thread.TurnCount() > 0 {
		lines = append(lines, fmt.Sprintf("Follow-up includes up to %d prior complete turns; earlier turns may be omitted.", thread.TurnCount()))
	}
	return lines
}

// QuestionLines includes the draft and explicit Send control for alternate
// presentations while reusing the same consent scope as the inline editor.
func QuestionLines(thread Thread, pending bool, destination string) []string {
	lines := ScopeLines(thread, destination)
	lines = append(lines, "Question: "+thread.Draft)
	if thread.Err != "" {
		lines = append(lines, "! "+thread.Err)
	}
	if pending {
		return append(lines, "One request is pending in this tab · ctrl+x: cancel · esc: keep reviewing")
	}
	return append(lines, "enter: Send and consent to this upload · shift+enter: newline · esc: keep draft · ctrl+n: new recipient thread")
}

func OmissionLabel(omitted chat.Omission) string {
	label := fmt.Sprintf("Omitted: %s (%d)", omitted.Reason, omitted.Count)
	if omitted.StartLine > 0 {
		label += fmt.Sprintf(" · lines %d-%d", omitted.StartLine, omitted.EndLine)
	}
	return label
}

// EvidenceLines returns the exact captured excerpt selected by a citation row.
func EvidenceLines(thread Thread, selection EvidenceSelection) ([]string, bool) {
	if selection.Turn < 0 || selection.Turn >= thread.TurnCount() {
		return nil, false
	}
	turn := thread.Turn(selection.Turn)
	if selection.Reference < 0 || selection.Reference >= turn.ReferenceCount() {
		return nil, false
	}
	ref := turn.Reference(selection.Reference)
	if !ref.Available() {
		return []string{fmt.Sprintf("Reference %d is unavailable; it did not match supplied evidence.", selection.Reference+1)}, true
	}
	lines := []string{fmt.Sprintf("Sent evidence %d/%d · %s:%d-%d @ %s", selection.Reference+1, turn.ReferenceCount(), ref.Path(), ref.StartLine(), ref.EndLine(), ref.Revision()), "Exact sent excerpt; cited range is shown above."}
	if ref.BlobID() != "" {
		lines = append(lines, "Blob: "+ref.BlobID())
	}
	if ref.LineCount() > 0 {
		for i := 0; i < ref.LineCount(); i++ {
			line := ref.Line(i)
			lines = append(lines, fmt.Sprintf("%d %s", line.Line, line.Text))
		}
	} else {
		lines = append(lines, strings.Split(ref.Text(), "\n")...)
	}
	for i := 0; i < turn.OmissionCount(); i++ {
		lines = append(lines, OmissionLabel(turn.Omission(i)))
	}
	return lines, true
}

// TurnView exposes sent material as read-only scalar accessors. It is cheap to
// copy even when a manifest contains the full bounded source upload.
type TurnView struct{ turn *Turn }
type ReferenceView struct{ reference *chat.ResolvedReference }

func (v Thread) TurnCount() int { return len(v.turns) }
func (v Thread) Turn(i int) TurnView {
	if i < 0 || i >= len(v.turns) {
		return TurnView{}
	}
	return TurnView{turn: &v.turns[i]}
}
func (v TurnView) Question() string {
	if v.turn == nil {
		return ""
	}
	return v.turn.Manifest.Question
}
func (v TurnView) AnswerText() string {
	if v.turn == nil {
		return ""
	}
	return v.turn.Answer.Text
}
func (v TurnView) HistoryCount() int {
	if v.turn == nil {
		return 0
	}
	return len(v.turn.Manifest.History)
}
func (v TurnView) ReferenceCount() int {
	if v.turn == nil {
		return 0
	}
	return len(v.turn.Answer.References)
}
func (v TurnView) Reference(i int) ReferenceView {
	if v.turn == nil || i < 0 || i >= len(v.turn.Answer.References) {
		return ReferenceView{}
	}
	return ReferenceView{reference: &v.turn.Answer.References[i]}
}
func (v TurnView) OmissionCount() int {
	if v.turn == nil {
		return 0
	}
	return len(v.turn.Manifest.Omissions)
}
func (v TurnView) Omission(i int) chat.Omission {
	if v.turn == nil || i < 0 || i >= len(v.turn.Manifest.Omissions) {
		return chat.Omission{}
	}
	return v.turn.Manifest.Omissions[i]
}
func (v ReferenceView) Available() bool { return v.reference != nil && v.reference.Available }
func (v ReferenceView) StartLine() int {
	if v.reference == nil {
		return 0
	}
	return v.reference.StartLine
}
func (v ReferenceView) EndLine() int {
	if v.reference == nil {
		return 0
	}
	return v.reference.EndLine
}
func (v ReferenceView) Path() string {
	if v.reference == nil {
		return ""
	}
	return v.reference.Excerpt.Path
}
func (v ReferenceView) Revision() string {
	if v.reference == nil {
		return ""
	}
	return v.reference.Excerpt.Revision
}
func (v ReferenceView) BlobID() string {
	if v.reference == nil {
		return ""
	}
	return v.reference.Excerpt.BlobID
}
func (v ReferenceView) Text() string {
	if v.reference == nil {
		return ""
	}
	return v.reference.Excerpt.Text
}
func (v ReferenceView) LineCount() int {
	if v.reference == nil {
		return 0
	}
	return len(v.reference.Excerpt.Lines)
}
func (v ReferenceView) Line(i int) chat.SourceLine {
	if v.reference == nil || i < 0 || i >= len(v.reference.Excerpt.Lines) {
		return chat.SourceLine{}
	}
	return v.reference.Excerpt.Lines[i]
}

type Request struct {
	ThreadID, Generation uint64
	AnchorRow            int
	View                 uint8
	GuideIndex           int
}

type EvidenceSelection struct {
	ThreadID        uint64
	Turn, Reference int
	Scroll          int
}

type thread struct{ Thread }
type request struct {
	Request
	cancel context.CancelFunc
}

type State struct {
	threads  []*thread
	active   uint64
	pending  *request
	evidence EvidenceSelection
	nextID   uint64
}

func cloneTurn(v Turn) Turn {
	v.Manifest.History = append([]chat.Turn(nil), v.Manifest.History...)
	v.Manifest.Omissions = append([]chat.Omission(nil), v.Manifest.Omissions...)
	v.Manifest.Evidence = append([]chat.Excerpt(nil), v.Manifest.Evidence...)
	v.Manifest.Hunk.Lines = append([]chat.SourceLine(nil), v.Manifest.Hunk.Lines...)
	for i := range v.Manifest.Evidence {
		v.Manifest.Evidence[i].Lines = append([]chat.SourceLine(nil), v.Manifest.Evidence[i].Lines...)
	}
	v.Answer.References = append([]chat.ResolvedReference(nil), v.Answer.References...)
	for i := range v.Answer.References {
		v.Answer.References[i].Excerpt.Lines = append([]chat.SourceLine(nil), v.Answer.References[i].Excerpt.Lines...)
	}
	return v
}

func snapshot(v *thread) Thread {
	if v == nil {
		return Thread{}
	}
	out := v.Thread
	// The backing turns are private. Callers can only read scalar values through
	// TurnView, so source payloads are never cloned on a diff render.
	return out
}

func (s *State) own(id uint64) *thread {
	for _, v := range s.threads {
		if v.ID == id {
			return v
		}
	}
	return nil
}

func (s *State) Thread(id uint64) Thread { return snapshot(s.own(id)) }
func (s *State) ActiveThread() Thread    { return s.Thread(s.active) }
func (s *State) PendingRequest() Request {
	if s.pending == nil {
		return Request{}
	}
	return s.pending.Request
}
func (s *State) HasPending() bool            { return s.pending != nil }
func (s *State) Evidence() EvidenceSelection { return s.evidence }
func (s *State) Threads() []Thread {
	out := make([]Thread, len(s.threads))
	for i, v := range s.threads {
		out[i] = snapshot(v)
	}
	return out
}
func (s *State) ThreadCount() int { return len(s.threads) }

// SetDraftAtAnchor updates only an existing thread, never creating a thread
// while the inline composer is being dismissed or another mode is selected.
func (s *State) SetDraftAtAnchor(anchor source.ReviewCommentTarget, draft string) bool {
	if current := s.own(s.active); current != nil && current.Anchor == anchor {
		s.SetDraft(current.ID, draft)
		return true
	}
	for i := len(s.threads) - 1; i >= 0; i-- {
		if s.threads[i].Anchor == anchor {
			s.SetDraft(s.threads[i].ID, draft)
			return true
		}
	}
	return false
}

func (s *State) Activate(id uint64) bool {
	if s.own(id) == nil {
		return false
	}
	s.active = id
	return true
}

func (s *State) Open(anchor source.ReviewCommentTarget, recipient guideconfig.Selection, offset int, view uint8) (Thread, bool) {
	for i := len(s.threads) - 1; i >= 0; i-- {
		if s.threads[i].Anchor == anchor {
			v := s.threads[i]
			v.SourceOffset, v.SourceView = offset, view
			s.active = v.ID
			return snapshot(v), false
		}
	}
	return s.New(anchor, recipient, offset, view), true
}

func (s *State) New(anchor source.ReviewCommentTarget, recipient guideconfig.Selection, offset int, view uint8) Thread {
	s.nextID++
	v := &thread{Thread: Thread{ID: s.nextID, Anchor: anchor, Recipient: recipient, SourceOffset: offset, SourceView: view}}
	s.threads = append(s.threads, v)
	s.active = v.ID
	return snapshot(v)
}

func (s *State) SetDraft(id uint64, draft string) {
	if v := s.own(id); v != nil && utf8.ValidString(draft) && len(draft) <= chat.MaxQuestionBytes {
		v.Draft = draft
	}
}
func (s *State) Insert(text string) {
	if v := s.own(s.active); v != nil && utf8.ValidString(text) && len(v.Draft)+len(text) <= chat.MaxQuestionBytes {
		v.Draft += text
	}
}
func (s *State) DeleteLastRune() {
	if v := s.own(s.active); v != nil && v.Draft != "" {
		_, n := utf8.DecodeLastRuneInString(v.Draft)
		v.Draft = v.Draft[:len(v.Draft)-n]
	}
}
func (s *State) Fail(id uint64, message string) {
	if v := s.own(id); v != nil {
		v.Err = message
	}
}
func (s *State) History(id uint64) []chat.Turn {
	v := s.own(id)
	if v == nil {
		return nil
	}
	history := make([]chat.Turn, 0, len(v.turns))
	for _, turn := range v.turns {
		history = append(history, chat.Turn{Question: turn.Manifest.Question, Answer: turn.Answer.Text})
	}
	return history
}
func (s *State) Begin(id uint64, cancel context.CancelFunc, anchorRow int, view uint8, guideIndex int) Request {
	v := s.own(id)
	if v == nil || s.pending != nil {
		return Request{}
	}
	v.Err = ""
	v.Generation++
	s.pending = &request{Request: Request{ThreadID: id, Generation: v.Generation, AnchorRow: anchorRow, View: view, GuideIndex: guideIndex}, cancel: cancel}
	return s.pending.Request
}
func (s *State) Cancel() {
	if r := s.pending; r != nil {
		r.cancel()
		if v := s.own(r.ThreadID); v != nil {
			v.Generation++
			v.Err = "Request cancelled; question kept for retry"
		}
		s.pending = nil
	}
}
func (s *State) Accept(id, generation uint64, manifest chat.Manifest, answer chat.ResolvedAnswer, failure string) bool {
	r := s.pending
	if r == nil || r.ThreadID != id || r.Generation != generation {
		return false
	}
	v := s.own(id)
	s.pending = nil
	if v == nil || v.Generation != generation {
		return false
	}
	v.Err = failure
	if failure == "" {
		v.turns = append(v.turns, cloneTurn(Turn{Manifest: manifest, Answer: answer}))
		v.Draft = ""
	}
	return true
}
func (s *State) Toggle(id uint64) bool {
	v := s.own(id)
	if v == nil {
		return false
	}
	v.Collapsed = !v.Collapsed
	return v.Collapsed
}
func (s *State) SelectEvidence(id uint64, turn, reference int) {
	s.evidence = EvidenceSelection{ThreadID: id, Turn: turn, Reference: reference}
}
func (s *State) NavigateEvidence(key string) {
	v := s.own(s.evidence.ThreadID)
	if v == nil || s.evidence.Turn < 0 || s.evidence.Turn >= len(v.turns) {
		return
	}
	refs := v.turns[s.evidence.Turn].Answer.References
	if len(refs) == 0 {
		return
	}
	switch key {
	case "right", "l":
		if s.evidence.Reference < len(refs)-1 {
			s.evidence.Reference++
		}
		s.evidence.Scroll = 0
	case "left", "h":
		if s.evidence.Reference > 0 {
			s.evidence.Reference--
		}
		s.evidence.Scroll = 0
	case "j", "down":
		s.evidence.Scroll++
	case "k", "up":
		if s.evidence.Scroll > 0 {
			s.evidence.Scroll--
		}
	}
}
