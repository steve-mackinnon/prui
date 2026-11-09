package agentchat

import (
	"context"
	"testing"

	chat "prui/internal/agentchat"
	"prui/internal/guideconfig"
	"prui/internal/source"
)

func TestConversationTransitionsWithoutRootModel(t *testing.T) {
	var state State
	anchor := source.ReviewCommentTarget{Path: "a.go", Side: "RIGHT", Line: 4}
	first, created := state.Open(anchor, guideconfig.Selection{Provider: "openai", Model: "one"}, 7, 0)
	if !created || first.ID == 0 {
		t.Fatal("first anchor was not opened")
	}
	state.Insert("question")
	if state.ActiveThread().Draft != "question" {
		t.Fatal("draft not retained")
	}
	again, created := state.Open(anchor, guideconfig.Selection{Provider: "openai", Model: "two"}, 9, 1)
	if created || again.ID != first.ID || again.Recipient.Model != "one" || again.Draft != "question" {
		t.Fatal("same anchor did not reopen pinned thread")
	}
	second := state.New(anchor, guideconfig.Selection{Provider: "openai", Model: "two"}, 9, 1)
	if second.ID == first.ID || state.ActiveThread().ID != second.ID {
		t.Fatal("explicit new recipient thread reused existing identity")
	}
	state.Insert("follow up")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := state.Begin(second.ID, cancel, 4, 1, 0)
	state.Cancel()
	if ctx.Err() == nil || state.HasPending() || state.Thread(second.ID).Draft != "follow up" {
		t.Fatal("cancel failed to preserve draft and invalidate request")
	}
	if state.Accept(second.ID, request.Generation, chat.Manifest{}, chat.ResolvedAnswer{}, "") {
		t.Fatal("late cancelled result was accepted")
	}
	_, retryCancel := context.WithCancel(context.Background())
	defer retryCancel()
	retry := state.Begin(second.ID, retryCancel, 4, 1, 0)
	if state.Accept(second.ID, request.Generation, chat.Manifest{}, chat.ResolvedAnswer{}, "") || state.PendingRequest() != retry {
		t.Fatal("stale generation consumed retry")
	}
	if !state.Accept(second.ID, retry.Generation, chat.Manifest{Question: "follow up"}, chat.ResolvedAnswer{Text: "answer"}, "") || state.Thread(second.ID).TurnCount() != 1 || state.Thread(second.ID).Draft != "" {
		t.Fatal("result did not complete only its originating thread")
	}
	if state.Thread(first.ID).TurnCount() != 0 {
		t.Fatal("another anchor thread was modified")
	}
}

func TestConversationReadViewsCannotMutateOwnedEvidence(t *testing.T) {
	var state State
	thread := state.New(source.ReviewCommentTarget{Path: "a.go", Side: "RIGHT", Line: 1}, guideconfig.Selection{Provider: "openai", Model: "one"}, 0, 0)
	state.SetDraft(thread.ID, "sent question")
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := state.Begin(thread.ID, cancel, 0, 0, -1)
	manifest := chat.Manifest{Question: "sent question", Hunk: chat.Excerpt{Lines: []chat.SourceLine{{Line: 1, Text: "original"}}}}
	answer := chat.ResolvedAnswer{Text: "sent answer", References: []chat.ResolvedReference{{Available: true, Excerpt: chat.Excerpt{Path: "a.go", Lines: []chat.SourceLine{{Line: 1, Text: "original"}}}}}}
	if !state.Accept(thread.ID, request.Generation, manifest, answer, "") {
		t.Fatal("answer rejected")
	}
	manifest.Hunk.Lines[0].Text = "mutated input"
	answer.References[0].Excerpt.Lines[0].Text = "mutated input"
	view := state.Thread(thread.ID)
	view.Draft = "mutated view"
	view.Recipient.Model = "mutated model"
	line := view.Turn(0).Reference(0).Line(0)
	line.Text = "mutated line"
	owned := state.Thread(thread.ID)
	if owned.Draft != "" || owned.Recipient.Model != "one" || owned.Turn(0).Reference(0).Line(0).Text != "original" {
		t.Fatal("read view or request payload mutated owner state")
	}
}

func TestConversationSemanticRowsKeepTurnCitationAndOmittedRange(t *testing.T) {
	var state State
	thread := state.New(source.ReviewCommentTarget{Path: "a.go", Side: "RIGHT", Line: 1}, guideconfig.Selection{Provider: "openai", Model: "one"}, 0, 0)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := state.Begin(thread.ID, cancel, 0, 0, -1)
	manifest := chat.Manifest{Question: "why", Omissions: []chat.Omission{{Reason: "nearby hunk rows omitted", Count: 2, StartLine: 8, EndLine: 9}}}
	answer := chat.ResolvedAnswer{Text: "answer", References: []chat.ResolvedReference{{Reference: chat.Reference{StartLine: 1, EndLine: 1}, Available: true, Excerpt: chat.Excerpt{Path: "a.go", Revision: "abcdef", Lines: []chat.SourceLine{{Line: 1, Text: "sent"}}}}}}
	state.Accept(thread.ID, request.Generation, manifest, answer, "")
	view := state.Thread(thread.ID)
	rows := CardRows(view, false, func(s string) []string { return []string{s} })
	found := false
	for _, row := range rows {
		if row.Citation && row.Turn == 0 && row.Reference == 0 && row.Kind == CardMetadata {
			found = true
		}
	}
	if !found {
		t.Fatal("citation row lost exact turn/reference metadata")
	}
	lines, ok := EvidenceLines(view, EvidenceSelection{ThreadID: thread.ID, Turn: 0, Reference: 0})
	if !ok || len(lines) < 4 || lines[2] != "1 sent" || lines[3] != "Omitted: nearby hunk rows omitted (2) · lines 8-9" {
		t.Fatalf("exact evidence/omission missing: %v", lines)
	}
}

func TestConversationDraftAtSameAnchorFollowsExplicitActiveThread(t *testing.T) {
	var state State
	anchor := source.ReviewCommentTarget{Path: "a.go", Side: "RIGHT", Line: 1}
	first := state.New(anchor, guideconfig.Selection{Provider: "openai", Model: "one"}, 0, 0)
	second := state.New(anchor, guideconfig.Selection{Provider: "openai", Model: "two"}, 0, 0)
	if !state.Activate(first.ID) || !state.SetDraftAtAnchor(anchor, "old recipient draft") {
		t.Fatal("could not edit old thread")
	}
	if state.Thread(first.ID).Draft != "old recipient draft" || state.Thread(second.ID).Draft != "" {
		t.Fatal("same-anchor drafts leaked across recipient threads")
	}
}
