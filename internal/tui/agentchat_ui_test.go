package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/agentchat"
	"prui/internal/guideconfig"
	"prui/internal/source"
	featureagent "prui/internal/tui/features/agentchat"
)

func agentTestModel(t *testing.T) *Model {
	t.Helper()
	m := rangeTestModel()
	m.Session.Inventory.Comparison.InventoryID = "agent-fixture-comparison"
	choice := guideconfig.Selection{Provider: "openai", Model: "fixture-model", BaseURL: "https://api.example.invalid/v1", Destination: "https://api.example.invalid", APIKeyEnv: "OPENAI_API_KEY"}
	m.SetGuideOptions([]guideconfig.Selection{choice}, choice, nil)
	selectRawTarget(t, m, "RIGHT", 1)
	return m
}

func fixtureAgentAnswer(_ context.Context, _ guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
	return agentchat.ValidateAnswer(manifest, agentchat.Answer{Text: "The selected code changes behavior.", References: []agentchat.Reference{{EvidenceID: manifest.Hunk.ID, StartLine: manifest.Anchor.Line, EndLine: manifest.Anchor.Line}}})
}

// These helpers drive the production inline hooks while keeping older routing
// and isolation regressions focused on their original assertions.
func openInlineAgentForTest(m *Model) {
	anchor, err := m.selectedCanonicalRange()
	if err != nil || anchor == nil {
		m.ActionError = errors.New("select a canonical PR diff line or valid range")
		return
	}
	draft := m.activateAgentComposer(*anchor)
	if m.ActionError != nil {
		return
	}
	m.Composer = &commentComposer{Target: *anchor, Agent: true, Draft: draft, Cursor: len([]rune(draft)), PendingIndex: -1}
}
func sendInlineAgentForTest(m *Model) tea.Cmd {
	if m.Composer == nil {
		return nil
	}
	if thread := m.agentActive(); thread != nil {
		m.Composer.Draft = thread.Draft
	}
	return m.sendAgentComposerQuestion(m.Composer)
}
func closeInlineAgentForTest(m *Model) {
	if m.Composer == nil {
		return
	}
	if thread := m.agentActive(); thread != nil {
		m.Composer.Draft = thread.Draft
	}
	m.saveAgentComposerDraft(m.Composer.Target, m.Composer.Draft)
	m.Composer = nil
}
func inlineAgentConsentForTest(m *Model) string {
	thread := m.agentActive()
	if thread == nil {
		return ""
	}
	return strings.Join(featureagent.QuestionLines(*thread, m.agent.HasPending(), safeAgentDestination(thread.Recipient.Destination)), "\n")
}

func TestAgentQuestionRequiresSendAndReopensTheSameAnchor(t *testing.T) {
	m := agentTestModel(t)
	calls := 0
	m.SetAgentAnswerer(func(ctx context.Context, choice guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		calls++
		if choice.Model != "fixture-model" || manifest.Anchor.Side != "RIGHT" {
			t.Fatalf("recipient/anchor = %#v %#v", choice, manifest.Anchor)
		}
		return fixtureAgentAnswer(ctx, choice, manifest)
	})
	openInlineAgentForTest(m)
	if calls != 0 || m.Composer == nil || !m.Composer.Agent || len(m.agent.Threads()) != 1 {
		t.Fatal("opening composer sent or failed to create thread")
	}
	if !strings.Contains(inlineAgentConsentForTest(m), "Lifetime: private") || !strings.Contains(inlineAgentConsentForTest(m), "api.example.invalid") {
		t.Fatal("consent scope missing", inlineAgentConsentForTest(m))
	}
	m.agent.SetDraft(m.agent.ActiveThread().ID, "What changed?")
	closeInlineAgentForTest(m)
	openInlineAgentForTest(m)
	if len(m.agent.Threads()) != 1 || m.agentActive().Draft != "What changed?" {
		t.Fatal("reopening anchor lost its draft")
	}
	cmd := sendInlineAgentForTest(m)
	if cmd == nil || calls != 0 || !m.agent.HasPending() || m.top() != pageReview {
		t.Fatalf("send did not enter local pending state: cmd=%v pending=%#v page=%v err=%q", cmd != nil, m.agent.PendingRequest(), m.top(), m.agentActive().Err)
	}
	m.Update(cmd())
	if calls != 1 || m.agent.HasPending() || m.agentActive().TurnCount() != 1 || m.agentActive().Draft != "" {
		t.Fatal("completed turn did not reach original thread")
	}
	if !strings.Contains(m.View().Content, "Agent Q&A") {
		t.Fatal("answer card missing from diff")
	}
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "Why?")
	cmd = sendInlineAgentForTest(m)
	m.Update(cmd())
	if calls != 2 || m.agentActive().TurnCount() != 2 || m.agentActive().Turn(1).HistoryCount() != 1 {
		t.Fatal("follow-up omitted bounded previous turn")
	}
}

func TestAgentCancelAndLateResultCannotDuplicateOrRetarget(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(fixtureAgentAnswer)
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "Why?")
	old := sendInlineAgentForTest(m)
	m.cancelAgentRequest()
	m.Update(old())
	if m.agentActive().TurnCount() != 0 || m.agent.HasPending() || m.agentActive().Draft != "Why?" {
		t.Fatal("cancelled answer was accepted")
	}
	openInlineAgentForTest(m)
	retry := sendInlineAgentForTest(m)
	if retry == nil || !m.agent.HasPending() {
		t.Fatal("retry was unavailable")
	}
	m.Update(old())
	if !m.agent.HasPending() || m.agentActive().TurnCount() != 0 {
		t.Fatal("old generation consumed retry")
	}
	m.Update(retry())
	if m.agentActive().TurnCount() != 1 {
		t.Fatal("retry answer missing")
	}
}

func TestAgentResultBelongsToOriginTabAndComparison(t *testing.T) {
	m := agentTestModel(t)
	m.openReviewTab(m.Session)
	m.SetAgentAnswerer(fixtureAgentAnswer)
	m.selectReviewView(viewFiles)
	m.Focus = paneDiff
	selectRawTarget(t, m, "RIGHT", 1)
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "Why?")
	cmd := sendInlineAgentForTest(m)
	origin := m.reviewTabState
	secondSession := *m.Session
	secondSession.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/other", Number: 2}
	second := newReviewTabState(&secondSession)
	m.tabs = append(m.tabs, workspaceTab{identity: secondSession.Inventory.Comparison.Metadata.Identity, review: second})
	m.activateTab(1)
	m.Update(cmd())
	if origin.agent.Threads()[0].TurnCount() != 1 || len(second.agent.Threads()) != 0 {
		t.Fatal("answer routed to active tab")
	}
	m.activateTab(0)
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "Again?")
	late := sendInlineAgentForTest(m)
	m.Session = &secondSession
	m.Update(late())
	if origin.agent.Threads()[0].TurnCount() != 1 {
		t.Fatal("changed comparison accepted old answer")
	}
}

func TestAgentAnchorsShareFilesAndGuideButKeepDifferentThreads(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(fixtureAgentAnswer)
	openInlineAgentForTest(m)
	first := m.agentActive()
	closeInlineAgentForTest(m)
	m.selectReviewView(viewGuide)
	m.Focus = paneDiff
	selectRawTarget(t, m, "RIGHT", 1)
	openInlineAgentForTest(m)
	if m.agentActive().ID != first.ID || len(m.agent.Threads()) != 1 {
		t.Fatal("Guide repeated canonical anchor made another thread")
	}
	closeInlineAgentForTest(m)
	selectRawTarget(t, m, "RIGHT", 2)
	openInlineAgentForTest(m)
	if m.agentActive().ID == first.ID || len(m.agent.Threads()) != 2 {
		t.Fatal("different source anchor reused first thread")
	}
}

func TestAgentCitationsInspectOnlySuppliedEvidence(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(func(_ context.Context, _ guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		return agentchat.ValidateAnswer(manifest, agentchat.Answer{Text: "See **changed** source \x1b]unsafe", References: []agentchat.Reference{{EvidenceID: manifest.Hunk.ID, StartLine: 1, EndLine: 1}, {EvidenceID: "invented", StartLine: 1, EndLine: 1}}})
	})
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "Explain.")
	m.Update(sendInlineAgentForTest(m)())
	thread := m.agentActive()
	if thread.TurnCount() != 1 || !thread.Turn(0).Reference(0).Available() || thread.Turn(0).Reference(1).Available() {
		t.Fatal("citation resolution changed")
	}
	card := m.agentCardLines(thread)
	joined := ""
	for _, row := range card {
		joined += row.Text + "\n"
	}
	if !strings.Contains(joined, "unavailable reference") || strings.Contains(joined, "\x1b]unsafe") {
		t.Fatal("card did not distinguish invented reference or neutralize terminal control", joined)
	}
	m.openAgentEvidence(thread)
	if !strings.Contains(m.agentEvidenceView(), "Sent evidence") || !strings.Contains(m.agentEvidenceView(), "first") {
		t.Fatal("exact sent hunk evidence missing", m.agentEvidenceView())
	}
	if got := featureagent.OmissionLabel(agentchat.Omission{Reason: "nearby hunk rows omitted", Count: 2, StartLine: 8, EndLine: 9}); !strings.Contains(got, "lines 8-9") {
		t.Fatal("omitted source range is not visible", got)
	}
	m.agentEvidenceKey("right")
	if !strings.Contains(m.agentEvidenceView(), "unavailable") {
		t.Fatal("invented citation became navigable")
	}
}

func TestAgentPendingCardAppearsBeforeAnswerAndReplacementCancels(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(fixtureAgentAnswer)
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "Why?")
	cmd := sendInlineAgentForTest(m)
	if cmd == nil || !strings.Contains(m.View().Content, "Agent is answering") {
		t.Fatal("pending state is absent beside diff")
	}
	old := m.reviewTabState
	newSession := *m.Session
	newSession.Inventory.Comparison.InventoryID = "replacement"
	m.Update(ActionResult{Session: &newSession, Reset: true})
	if len(m.agent.Threads()) != 0 || m.agent.HasPending() {
		t.Fatal("replacement retained memory-only conversation")
	}
	m.Update(cmd())
	if len(m.agent.Threads()) != 0 || old.agent.HasPending() {
		t.Fatal("late result was routed into replacement")
	}
}

func TestAgentChangedGuideRecipientNeedsNewThreadAndExplicitSend(t *testing.T) {
	m := agentTestModel(t)
	calls := 0
	m.SetAgentAnswerer(func(ctx context.Context, choice guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		calls++
		if choice.Model != "new-model" {
			t.Fatal("recipient was not pinned to new thread", choice.Model)
		}
		return fixtureAgentAnswer(ctx, choice, manifest)
	})
	openInlineAgentForTest(m)
	first := m.agentActive()
	closeInlineAgentForTest(m)
	newChoice := guideconfig.Selection{Provider: "openai", Model: "new-model", BaseURL: "https://api.example.invalid/v1", Destination: "https://api.example.invalid", APIKeyEnv: "OPENAI_API_KEY"}
	m.SetGuideOptions([]guideconfig.Selection{newChoice}, newChoice, nil)
	openInlineAgentForTest(m)
	if m.agentActive().ID != first.ID || len(m.agent.Threads()) != 1 {
		t.Fatal("existing anchor did not reopen pinned recipient")
	}
	m.newAgentComposerThread(m.Composer)
	if len(m.agent.Threads()) != 2 || m.agentActive().ID == first.ID || m.agentActive().Recipient.Model != "new-model" || calls != 0 {
		t.Fatal("new recipient thread sent or reused old model")
	}
	m.agent.SetDraft(m.agent.ActiveThread().ID, "What now?")
	m.Update(sendInlineAgentForTest(m)())
	if calls != 1 {
		t.Fatal("explicit Send did not call selected provider once")
	}
}

func TestAgentCardFollowsRangeEndInUnifiedAndSplit(t *testing.T) {
	for _, split := range []bool{false, true} {
		m := agentTestModel(t)
		m.SetAgentAnswerer(fixtureAgentAnswer)
		if split {
			m.layout = diffLayoutSideBySide
		}
		selectRawTarget(t, m, "RIGHT", 1)
		origin := *m.selectedDiffTarget()
		selectRawTarget(t, m, "RIGHT", 2)
		end := *m.selectedDiffTarget()
		m.rangeStart, m.rangeEnd = &origin, &end
		openInlineAgentForTest(m)
		if m.agentActive() == nil {
			t.Fatal("range did not open Agent Q&A", m.ActionError)
		}
		m.agent.SetDraft(m.agent.ActiveThread().ID, "What does this range change?")
		cmd := sendInlineAgentForTest(m)
		if cmd == nil {
			t.Fatal("range question was not sent", m.ActionError)
		}
		m.Update(cmd())
		rows := m.displayDetail()
		endRow, cardRow := -1, -1
		for i, row := range rows {
			if row.agentThreadID == m.agentActive().ID && cardRow < 0 {
				cardRow = i
			}
			if row.sideBySide != nil {
				for _, target := range rowTargets(*row.sideBySide) {
					if target.Side == "RIGHT" && target.Line == 2 {
						endRow = i
					}
				}
			} else if row.target != nil && row.target.Side == "RIGHT" && row.target.Line == 2 {
				endRow = i
			}
		}
		if endRow < 0 || cardRow != endRow+1 {
			t.Fatalf("split=%v end row=%d card row=%d", split, endRow, cardRow)
		}
	}
}

func TestAgentLeftDeletionLine857SendsSourceAndAnchorsCard(t *testing.T) {
	for _, split := range []bool{false, true} {
		name := "unified"
		if split {
			name = "split"
		}
		t.Run(name, func(t *testing.T) {
			s := kindsSession()
			s.Inventory.Files[0].OldPath = []byte("src/mod.rs")
			s.Inventory.Files[0].NewPath = []byte("src/mod.rs")
			s.Inventory.Patches["p"] = []byte("@@ -857 +857 @@\n-deleted_behavior();\n+replacement_behavior();\n")
			s.Inventory.Comparison.Metadata.BaseSHA = strings.Repeat("a", 40)
			s.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("b", 40)
			s.Inventory.Comparison.MergeBaseSHA = strings.Repeat("a", 40)
			s.Inventory.Comparison.InventoryID = "agent-left-deletion"
			m := New(context.Background(), nil)
			m.Loading, m.Session = false, s
			m.Selected, m.Focus, m.Width, m.Height = 1, paneDiff, 180, 24
			m.selectReviewView(viewFiles)
			if split {
				m.layout = diffLayoutSideBySide
			}
			choice := guideconfig.Selection{Provider: "openai", Model: "fixture-model", Destination: "https://api.example.invalid", APIKeyEnv: "OPENAI_API_KEY"}
			m.SetGuideOptions([]guideconfig.Selection{choice}, choice, nil)
			selectRawTarget(t, m, "LEFT", 857)
			called := false
			m.SetAgentAnswerer(func(_ context.Context, _ guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
				called = true
				if manifest.Anchor.Side != "LEFT" || manifest.Anchor.Line != 857 || manifest.Anchor.Path != "src/mod.rs" || manifest.Hunk.Revision != s.Inventory.Comparison.MergeBaseSHA || len(manifest.Hunk.Lines) != 1 || !manifest.Hunk.Lines[0].Selected || manifest.Hunk.Lines[0].Text != "deleted_behavior();" {
					t.Fatalf("old-side source absent from request: anchor=%+v hunk=%+v", manifest.Anchor, manifest.Hunk)
				}
				return fixtureAgentAnswer(context.Background(), choice, manifest)
			})
			openInlineAgentForTest(m)
			if m.agentActive() == nil {
				t.Fatal("LEFT deletion did not open Agent Q&A", m.ActionError)
			}
			m.agent.SetDraft(m.agent.ActiveThread().ID, "Why was this removed?")
			cmd := sendInlineAgentForTest(m)
			if cmd == nil {
				t.Fatal("LEFT deletion Send unavailable", m.agentActive().Err)
			}
			m.Update(cmd())
			if !called {
				t.Fatal("Send did not reach answerer")
			}
			endRow, cardRow := -1, -1
			for i, row := range m.displayDetail() {
				if row.agentThreadID == m.agentActive().ID && cardRow < 0 {
					cardRow = i
				}
				if row.sideBySide != nil {
					for _, target := range rowTargets(*row.sideBySide) {
						if target.Side == "LEFT" && target.Line == 857 {
							endRow = i
						}
					}
				} else if row.target != nil && row.target.Side == "LEFT" && row.target.Line == 857 {
					endRow = i
				}
			}
			if endRow < 0 || cardRow != endRow+1 {
				t.Fatalf("card row=%d, deleted source row=%d", cardRow, endRow)
			}
		})
	}
}

func TestAgentExpandedUnanchoredSourceIsUnavailable(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(fixtureAgentAnswer)
	m.navigation.mode = "NEW"
	m.openCommentComposer()
	if m.Composer != nil || m.agentActive() != nil {
		t.Fatal("expanded source became an agent anchor", m.ActionError)
	}
}

func TestAgentQuestionDoesNotChangeGuideProgressOrGitHubWrites(t *testing.T) {
	m := agentTestModel(t)
	m.Session.ReviewedSliceIDs = []string{"existing"}
	guideBefore := m.Session.Guides
	writes := 0
	m.SetCommentSubmitter(func(context.Context, CommentSubmission) (source.ReviewComment, error) {
		writes++
		return source.ReviewComment{}, nil
	})
	m.SetAgentAnswerer(fixtureAgentAnswer)
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "Why?")
	m.Update(sendInlineAgentForTest(m)())
	m.cancelAgentRequest()
	if writes != 0 || m.Session.Guides != guideBefore || len(m.Session.ReviewedSliceIDs) != 1 || m.Session.ReviewedSliceIDs[0] != "existing" {
		t.Fatal("asking changed guide/progress/GitHub state")
	}
}

func TestAgentSetupErrorsAreUsefulWithoutEchoingProviderDiagnostics(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(func(context.Context, guideconfig.Selection, agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		return agentchat.ResolvedAnswer{}, errors.New("OPENAI_API_KEY is required to ask the agent")
	})
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "Why?")
	m.Update(sendInlineAgentForTest(m)())
	if !strings.Contains(m.agentActive().Err, "OPENAI_API_KEY") || !strings.Contains(m.agentActive().Err, "retry") {
		t.Fatal("setup guidance hidden", m.agentActive().Err)
	}
	m.SetAgentAnswerer(func(context.Context, guideconfig.Selection, agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		return agentchat.ResolvedAnswer{}, errors.New("request echoed secret-key and source")
	})
	openInlineAgentForTest(m)
	m.Update(sendInlineAgentForTest(m)())
	if strings.Contains(m.agentActive().Err, "secret-key") || strings.Contains(m.agentActive().Err, "source") {
		t.Fatal("provider diagnostic leaked", m.agentActive().Err)
	}
}

func TestAgentProviderFailuresKeepSafeActionableStatus(t *testing.T) {
	for _, tc := range []struct {
		failure string
		want    string
	}{
		{"agent provider request failed: HTTP 400", "HTTP 400"},
		{"agent provider request failed: HTTP 401", "API key"},
		{"agent provider request failed: HTTP 403", "access"},
		{"agent provider request failed: HTTP 404", "model ID"},
		{"agent provider request failed: HTTP 429", "rate limit"},
		{"agent provider request failed: HTTP 503", "provider unavailable"},
		{"agent provider returned incomplete output", "incomplete"},
	} {
		got := safeAgentFailure(errors.New(tc.failure))
		if !strings.Contains(got, tc.want) || !strings.Contains(got, "retry") {
			t.Errorf("failure %q became %q; want %q and retry", tc.failure, got, tc.want)
		}
	}
	for _, failure := range []string{
		"agent provider request failed: HTTP 400 source excerpt",
		"agent provider request failed: HTTP 400\x1b]secret",
		"agent provider request failed: HTTP -1",
	} {
		if got := safeAgentFailure(errors.New(failure)); strings.Contains(got, "source") || strings.Contains(got, "secret") || strings.Contains(got, "HTTP") {
			t.Errorf("untrusted status was relayed: %q", got)
		}
	}
}

func TestAgentCitationRowSelectsItsExactOlderTurn(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(func(_ context.Context, _ guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		line := 1
		if manifest.Question == "second" {
			line = 2
		}
		return agentchat.ValidateAnswer(manifest, agentchat.Answer{Text: manifest.Question, References: []agentchat.Reference{{EvidenceID: manifest.Hunk.ID, StartLine: line, EndLine: line}}})
	})
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "first")
	m.Update(sendInlineAgentForTest(m)())
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "second")
	m.Update(sendInlineAgentForTest(m)())
	thread := m.agentActive()
	for i, row := range m.displayDetail() {
		if row.agentThreadID == thread.ID && row.agentCitation && row.agentTurn == 0 {
			m.Focus, m.cursorActive = paneDiff, true
			m.setCursor(i)
			m.openAgentEvidence(thread)
			if m.agent.Evidence().Turn != 0 || m.agent.Evidence().Reference != 0 || !strings.Contains(m.agentEvidenceView(), "text:1-1") {
				t.Fatal("older citation opened wrong turn", m.agentEvidenceView())
			}
			return
		}
	}
	t.Fatal("first citation row not rendered")
}

func TestAgentInlineActivationUsesCommittedValidatedRange(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(fixtureAgentAnswer)
	start := *m.selectedDiffTarget()
	selectRawTarget(t, m, "LEFT", 1)
	invalid := *m.selectedDiffTarget()
	m.rangeStart, m.rangeEnd = &start, &invalid
	if _, err := m.selectedCanonicalRange(); err == nil {
		t.Fatal("invalid cross-side range was accepted")
	}
	selectRawTarget(t, m, "RIGHT", 2)
	valid := *m.selectedDiffTarget()
	m.rangeEnd = &valid
	anchor, err := m.selectedCanonicalRange()
	if err != nil || anchor == nil {
		t.Fatal("valid range unavailable", err)
	}
	if draft := m.activateAgentComposer(*anchor); draft != "" || m.ActionError != nil || m.agentActive() == nil || m.agentActive().Anchor != *anchor {
		t.Fatal("valid range did not activate inline Agent mode", draft, m.ActionError)
	}
}

func TestAgentInlineComposerSendAndCardFollowUp(t *testing.T) {
	m := agentTestModel(t)
	calls := 0
	m.SetAgentAnswerer(func(ctx context.Context, choice guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		calls++
		return fixtureAgentAnswer(ctx, choice, manifest)
	})
	anchor := *m.selectedDiffTarget()
	if draft := m.activateAgentComposer(anchor); draft != "" || m.ActionError != nil || calls != 0 {
		t.Fatal("activation sent or failed", draft, m.ActionError)
	}
	m.Composer = &commentComposer{Target: anchor, Agent: true, Draft: "", PendingIndex: -1}
	visible := m.View().Content
	for _, want := range []string{"[Ask agent]", "fixture-model"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("inline editor did not render %q", want)
		}
	}
	if cmd := m.sendAgentComposerQuestion(m.Composer); cmd != nil || m.Composer == nil || m.ActionError == nil {
		t.Fatal("blank question was sent or editor closed")
	}
	m.Composer.Draft = "Why does this line change?"
	cmd := m.sendAgentComposerQuestion(m.Composer)
	if cmd == nil || m.Composer != nil || calls != 0 || m.top() != pageReview || !m.agent.HasPending() {
		t.Fatal("explicit inline Send failed or opened another page")
	}
	m.Update(cmd())
	if calls != 1 || m.agentActive().TurnCount() != 1 {
		t.Fatal("answer did not reach inline thread")
	}
	thread := m.agentActive()
	if m.openAgentCardComposer(thread) == nil || m.Composer == nil || !m.Composer.Agent || m.Composer.Target != anchor {
		t.Fatal("card Enter did not reopen inline Agent editor")
	}
	cardEnd, editorStart := -1, -1
	for i, row := range m.displayDetail() {
		if row.agentThreadID == thread.ID {
			cardEnd = i
		}
		if row.editor && editorStart < 0 {
			editorStart = i
		}
	}
	if cardEnd < 0 || editorStart != cardEnd+1 {
		t.Fatalf("follow-up editor should follow answer card: card end=%d editor start=%d", cardEnd, editorStart)
	}
	m.Composer.Draft = "Follow up?"
	cmd = m.sendAgentComposerQuestion(m.Composer)
	if cmd == nil {
		t.Fatal("follow-up Send failed")
	}
	m.Update(cmd())
	if calls != 2 || m.agentActive().Turn(1).HistoryCount() != 1 {
		t.Fatal("follow-up history was lost")
	}
}

func TestAgentFramedCardWrapsAtNarrowWidthWithCitationMetadata(t *testing.T) {
	m := agentTestModel(t)
	m.Width = 46
	m.SetAgentAnswerer(func(_ context.Context, _ guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		return agentchat.ValidateAnswer(manifest, agentchat.Answer{Text: "A longer answer that should wrap inside the comment-style frame at a narrow terminal width.", References: []agentchat.Reference{{EvidenceID: manifest.Hunk.ID, StartLine: manifest.Anchor.Line, EndLine: manifest.Anchor.Line}}})
	})
	anchor := *m.selectedDiffTarget()
	m.activateAgentComposer(anchor)
	m.Composer = &commentComposer{Target: anchor, Agent: true, Draft: "A lengthy question that should wrap instead of being clipped at the right frame edge", PendingIndex: -1}
	m.Update(m.sendAgentComposerQuestion(m.Composer)())
	card := m.agentCardLines(m.agentActive())
	if len(card) < 8 || !strings.HasPrefix(card[0].Text, "  +") || card[0].Text != card[len(card)-1].Text {
		t.Fatal("full inline frame missing")
	}
	questionRows, citationRows, inQuestion := 0, 0, false
	for _, row := range card {
		if visibleWidth(row.Text) > m.detailWidth() {
			t.Fatalf("card exceeds narrow detail width: %q", row.Text)
		}
		if strings.Contains(row.Text, "Question:") {
			inQuestion = true
		}
		if strings.Contains(row.Text, "Answer") {
			inQuestion = false
		}
		if inQuestion {
			questionRows++
		}
		if row.agentCitation {
			citationRows++
			if row.agentTurn != 0 || row.agentReference != 0 {
				t.Fatal("wrapped citation lost turn/reference")
			}
		}
	}
	if questionRows < 2 || citationRows == 0 {
		t.Fatal("question did not wrap or citation disappeared", questionRows, citationRows)
	}
}

func TestAgentFramedCardSeparatesQuestionAnswerAndFollowUpTurns(t *testing.T) {
	m := agentTestModel(t)
	m.Width = 110
	m.SetAgentAnswerer(fixtureAgentAnswer)
	anchor := *m.selectedDiffTarget()
	for _, question := range []string{"first question", "second question"} {
		m.activateAgentComposer(anchor)
		m.Composer = &commentComposer{Target: anchor, Agent: true, Draft: question, PendingIndex: -1}
		cmd := m.sendAgentComposerQuestion(m.Composer)
		if cmd == nil {
			t.Fatal("send unavailable", m.ActionError)
		}
		m.Update(cmd())
	}
	card := m.agentCardLines(m.agentActive())
	joined := ""
	spacers, dividers := 0, 0
	questionStyled, answerStyled := false, false
	for _, row := range card {
		joined += row.Text + "\n"
		if strings.Contains(row.Text, "first question") || strings.Contains(row.Text, "second question") {
			questionStyled = questionStyled || row.Class == classAgentQuestion
		}
		if strings.Contains(row.Text, "The selected code changes behavior.") {
			answerStyled = answerStyled || row.Class == classPlain
		}
		if strings.TrimSpace(strings.Trim(row.Text, "| ")) == "" {
			spacers++
		}
		if strings.Contains(row.Text, "| "+strings.Repeat("-", m.overlayInnerWidth(0))) {
			dividers++
		}
	}
	for _, label := range []string{"Question:", "first question", "second question", "Answer", "Source references"} {
		if !strings.Contains(joined, label) {
			t.Fatalf("missing visual hierarchy %q: %s", label, joined)
		}
	}
	if strings.Contains(joined, "Turn 1") || strings.Contains(joined, "RIGHT 1-1") {
		t.Fatalf("card still shows redundant turn or anchor labels: %s", joined)
	}
	if spacers < 4 || dividers < 1 {
		t.Fatal("turns read as one unbroken block", spacers, dividers)
	}
	if !questionStyled || !answerStyled {
		t.Fatal("question and answer do not have distinct semantic styles")
	}
}

func TestAgentCollapseRestoresCanonicalSourcePosition(t *testing.T) {
	m := agentTestModel(t)
	m.SetAgentAnswerer(fixtureAgentAnswer)
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "why?")
	m.Update(sendInlineAgentForTest(m)())
	thread := m.agentActive()
	for i, row := range m.displayDetail() {
		if row.agentThreadID == thread.ID {
			m.Focus, m.cursorActive = paneDiff, true
			m.setCursor(i)
			m.setOffset(i)
			m.toggleAgentCard(thread)
			if !m.agent.Thread(thread.ID).Collapsed || m.cursor() != m.agentAnchorRow(thread.Anchor) || m.cursor() < m.offset() || m.cursor() >= m.offset()+m.bodyHeight() {
				t.Fatal("collapse did not restore source", m.cursor(), m.offset())
			}
			collapsed := m.agent.Thread(thread.ID)
			rows := m.agentCardLines(&collapsed)
			if len(rows) < 3 || !strings.Contains(rows[2].Text, "Q: expand · Enter: follow up") {
				t.Fatal("collapsed card advertises a removed action")
			}
			return
		}
	}
	t.Fatal("card not found")
}

func TestAgentInactiveOriginAnswerPreservesViewportBelowCard(t *testing.T) {
	m := agentTestModel(t)
	m.openReviewTab(m.Session)
	m.selectReviewView(viewFiles)
	m.Focus = paneDiff
	selectRawTarget(t, m, "RIGHT", 1)
	m.SetAgentAnswerer(fixtureAgentAnswer)
	openInlineAgentForTest(m)
	m.agent.SetDraft(m.agent.ActiveThread().ID, "why?")
	cmd := sendInlineAgentForTest(m)
	origin, thread := m.reviewTabState, m.agentActive()
	beforeCard := len(m.agentCardLinesFor(thread, true))
	origin.Scroll[-1] = m.agentAnchorRow(thread.Anchor) + beforeCard + 1
	beforeOffset := origin.Scroll[-1]
	secondSession := *m.Session
	secondSession.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/other", Number: 2}
	m.tabs = append(m.tabs, workspaceTab{identity: secondSession.Inventory.Comparison.Metadata.Identity, review: newReviewTabState(&secondSession)})
	m.activateTab(1)
	m.Update(cmd())
	updated := origin.agent.Thread(thread.ID)
	delta := len(m.agentCardLinesFor(&updated, false)) - beforeCard
	if delta <= 0 || origin.Scroll[-1] != beforeOffset+delta {
		t.Fatal("inactive source viewport shifted", beforeOffset, origin.Scroll[-1], delta)
	}
}

func keyPress(name string) tea.KeyPressMsg {
	switch name {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{}
}
