package tui

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"reflect"
	"strings"
	"testing"
)

// Lifecycle uses live identity/head evidence, while the unrelated private review
// attempt and source/navigation stay tied to their original immutable pins.
func TestPRLifecycleCombinedFreshPreflightRetainsPrivateAttemptAndPins(t *testing.T) {
	for _, change := range []struct {
		name     string
		selected int
		mutate   func(*source.Lifecycle)
	}{
		{"head", 11, func(s *source.Lifecycle) { s.HeadSHA = strings.Repeat("c", 40) }},
		{"permission", 11, func(s *source.Lifecycle) { s.CanClose = false }},
		{"draft", 11, func(s *source.Lifecycle) { s.Draft = true }},
		{"policy", 1, func(s *source.Lifecycle) { s.PolicyKnown = false }},
	} {
		t.Run(change.name, func(t *testing.T) {
			store, e := session.Open(t.TempDir() + "/private")
			if e != nil {
				t.Fatal(e)
			}
			defer store.Close()
			m := draftTestModel(t, store)
			defer m.Close()
			m.Width, m.Height = 120, 24
			meta := m.Session.Inventory.Comparison.Metadata
			target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}
			m.Pending = []source.ReviewComment{{Target: target, Body: "private pending review"}}
			m.ReviewForm = &reviewForm{Body: "immutable attempted review", Event: 0}
			if !m.prepareDraftAttempt("review") {
				t.Fatal(m.ActionError)
			}
			original, e := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
			if e != nil {
				t.Fatal(e)
			}
			pins, _ := json.Marshal(m.Session)
			m.navigation = codeNavigation{mode: "NEW", whitespace: true}
			m.Horizontal = 3
			m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
				return source.Readiness{Identity: id, HeadSHA: strings.Repeat("f", 40)}, nil
			})
			live := tuiLifecycleState(m)
			live.Permission = "WRITE"
			live.PolicyKnown = true
			live.Methods = []string{"SQUASH"}
			live.Readiness = source.Readiness{Identity: live.Identity, HeadSHA: live.HeadSHA, BaseSHA: live.BaseSHA, State: "OPEN", HeadVerified: true, RequirementsKnown: true, RequiredReviews: 0, ChecksComplete: true, ReviewsComplete: true, Mergeable: "MERGEABLE", MergeState: "CLEAN"}
			mutations := 0
			m.SetPRLifecycle(func(context.Context, source.Identity) (source.Lifecycle, error) { return live, nil }, func(_ context.Context, a source.LifecycleAction) (source.LifecycleOutcome, error) {
				current := live
				change.mutate(&current)
				err := current.ValidateAction(a)
				if err == nil {
					mutations++
				}
				return source.LifecycleOutcome{Snapshot: current, Refreshed: true}, err
			})
			_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
			if cmd == nil || m.top() != pageReadiness {
				t.Fatal("Alt+R displaced by lifecycle")
			}
			m.Update(cmd())
			_, cmd = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
			if cmd == nil || m.top() != pageLifecycle {
				t.Fatal("lifecycle entry")
			}
			m.Update(cmd())
			m.lifecycle.selected = change.selected
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("confirmation not submitted")
			}
			m.Update(cmd())
			if mutations != 0 || m.lifecycle.uncertain != nil {
				t.Fatal("stale lifecycle preflight wrote or entered attempted state")
			}
			after, _ := json.Marshal(m.Session)
			retained, e := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
			if e != nil || !bytes.Equal(pins, after) || !reflect.DeepEqual(original, retained) || m.draft.attempt != "review" || m.draft.attempted.Review.CommitID != meta.HeadSHA {
				t.Fatal("live lifecycle changed private attempt/source pins", e)
			}
			if m.navigation.mode != "NEW" || !m.navigation.whitespace || m.Horizontal != 3 {
				t.Fatal("lifecycle disturbed navigation")
			}
			m.prLifecycleKey("esc")
			m.readinessKey("esc")
			key(m, '2')
			key(m, 'C')
			if !m.commitFilter.open {
				t.Fatal("C no longer owns captured commit filtering")
			}
		})
	}
}

func TestPRLifecycleEntryCannotDisplaceSuggestionConfirmation(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	a := source.SuggestionApplication{Metadata: m.Session.Inventory.Comparison.Metadata, Replacement: "private replacement"}
	m.SuggestionApply = &a
	m.push(pageSuggestionApply)
	reads, writes := 0, 0
	m.SetPRLifecycle(func(context.Context, source.Identity) (source.Lifecycle, error) {
		reads++
		return source.Lifecycle{}, nil
	}, func(context.Context, source.LifecycleAction) (source.LifecycleOutcome, error) {
		writes++
		return source.LifecycleOutcome{}, nil
	})
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	key(m, 'l')
	key(m, 'C')
	namedKey(m, tea.KeyEnter)
	if m.top() != pageSuggestionApply || !m.SuggestionConfirm || m.SuggestionApply != &a || reads != 0 || writes != 0 || m.commitFilter.open {
		t.Fatal("lifecycle displaced suggestion or commit-filter ownership")
	}
}

func TestPRLifecycleUnknownOutcomeAndResetPreserveActualGeneralV4SuggestionAttempt(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	defer m.Close()
	m.Width, m.Height = 120, 24
	old := m.Session
	meta := old.Inventory.Comparison.Metadata
	body, _ := source.SuggestionBody("replacement")
	a := source.SuggestionApplication{Metadata: meta, Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}, CommentID: 9, Branch: "feature", Before: "old", Replacement: "replacement", Content: "replacement\n", CommentBody: body}
	input := map[string]any{"branch": map[string]string{"repositoryNameWithOwner": meta.HeadRepository, "refName": a.Branch}, "expectedHeadOid": meta.HeadSHA, "message": map[string]string{"headline": "Apply review suggestion #9"}, "fileChanges": map[string]any{"additions": []map[string]string{{"path": "text", "contents": base64.StdEncoding.EncodeToString([]byte(a.Content))}}}}
	a.Payload, _ = json.Marshal(map[string]any{"query": `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid url} ref{target{oid}}}}`, "variables": map[string]any{"input": input}})
	m.SuggestionApply = &a
	m.discussions.editor = &generalCommentEditor{draft: "edited general", cursor: 3, attemptedBody: "original general", attemptedIDs: map[string]bool{"PR comment:12": true}, uncertain: true}
	if !m.prepareDraftAttempt("suggestion") {
		t.Fatal(m.ActionError)
	}
	original, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	if err != nil || original.Version != 4 || original.Attempted.Application == nil || original.General.AttemptedBody != "original general" {
		t.Fatal("actual v4 codec lost v3 attempt", err)
	}
	pinned, _ := json.Marshal(old)
	live := tuiLifecycleState(m)
	writes := 0
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		return source.Readiness{Identity: id, HeadSHA: live.HeadSHA}, nil
	})
	var readContext context.Context
	m.SetPRLifecycle(func(ctx context.Context, _ source.Identity) (source.Lifecycle, error) {
		readContext = ctx
		return live, nil
	}, func(context.Context, source.LifecycleAction) (source.LifecycleOutcome, error) {
		writes++
		return source.LifecycleOutcome{Attempted: true, Refreshed: true, Uncertain: true, Snapshot: live}, errors.New("synthetic delivery uncertainty")
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	m.Update(cmd())
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m.Update(cmd())
	m.lifecycle.selected = 11
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(cmd())
	if writes != 1 || m.lifecycle.uncertain == nil || m.discussions.editor.attemptedBody != "original general" || !m.discussions.editor.uncertain {
		t.Fatal("lifecycle delivery reconciled unrelated general intent")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m.Update(cmd())
	if writes != 1 || m.lifecycle.uncertain == nil || m.prLifecycleKey("enter") != nil {
		t.Fatal("refresh repeated/unlocked unknown lifecycle")
	}
	retained, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	after, _ := json.Marshal(old)
	if err != nil || !reflect.DeepEqual(original, retained) || !bytes.Equal(pinned, after) {
		t.Fatal("live attempt mutated private original or source", err)
	}
	readinessContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.readiness.cancel = cancel
	stale := LifecycleResult{Target: m.activeTab, Session: old, Generation: m.lifecycle.generation, Snapshot: live}
	fresh := *old
	fresh.ID = strings.Repeat("e", 32)
	fresh.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("c", 40)
	m.Update(ActionResult{Session: &fresh, Reset: true})
	m.Update(stale)
	if readContext.Err() != context.Canceled || readinessContext.Err() != context.Canceled || m.lifecycle.loaded || m.lifecycle.uncertain != nil || m.SuggestionApply != nil || m.discussions.editor != nil || m.draft.attempt != "" {
		t.Fatal("reset carried old remote evidence or private intent")
	}
	empty, err := store.LoadDraft(context.Background(), session.DraftKeyFor(fresh.Inventory.Comparison.Metadata))
	if err != nil || empty.Generation != 0 {
		t.Fatal("reset retargeted private attempts", err)
	}
	retained, err = store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	if err != nil || !reflect.DeepEqual(original, retained) {
		t.Fatal("reset changed original v4 attempts", err)
	}
	recovered := draftTestModel(t, store)
	defer recovered.Close()
	if recovered.draft.attempted == nil || recovered.draft.attempted.Application == nil || !bytes.Equal(recovered.draft.attempted.Application.Payload, a.Payload) || recovered.discussions.editor == nil || recovered.discussions.editor.attemptedBody != "original general" || !recovered.discussions.editor.uncertain {
		t.Fatal("offline recovery lost immutable v4/general-v3/suggestion work")
	}
	if recovered.suggestionApplyKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil || recovered.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil {
		t.Fatal("private attempt retried after lifecycle reset")
	}
}

// Context source rows are read-only anchors, even when a live lifecycle attempt
// is unresolved and the user leaves and returns through the account inbox.
func TestPRLifecycleContextSourceInboxRoundTripAndModalOwnership(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := inboxIntegrationSession(t, store, "o/first")
	original := inboxIntegrationDraft(t, store, first, "PRIVATE ORIGINAL", false)
	rawRows := inboxPrivateRows(t, store.Path())
	m := New(context.Background(), nil)
	defer m.Close()
	m.SetLifecycle(store, nil, nil)
	m.openReviewTab(first)
	namedKey(m, tea.KeyEnter)
	inboxLeaveApplicationForFixture(t, m, false) // prepared application can be closed via Escape
	key(m, '2')
	m.Width, m.Height = 160, 24
	for i, f := range m.Session.Inventory.Files {
		if string(f.NewPath) == "code.txt" {
			m.selectFile(i)
			break
		}
	}
	wordState := func() []byte {
		var rows []any
		highlighted := 0
		for unit := range m.Session.UnitFiles {
			for _, row := range unitLines(m.Session, unit) {
				if len(row.wordChanges) > 0 {
					highlighted++
				}
				rows = append(rows, []any{row.Text, row.rawSource, row.searchID, row.wordChanges, row.target, row.oldTarget})
			}
		}
		if highlighted == 0 {
			t.Fatal("fixture has no actual changed-word emphasis")
		}
		b, e := json.Marshal(rows)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	words := wordState()
	m.Focus = paneDiff
	m.cursorActive = true
	m.navigation = codeNavigation{mode: "NEW", whitespace: true}
	m.fileCache = fileDetailCache{}
	searchInput(m, "context")
	namedKey(m, tea.KeyEnter)
	anchor := m.displayDetail()[m.cursor()].searchID
	if anchor.Unit == "" || m.selectedDiffTarget() != nil {
		t.Fatal("context search is not a read-only source anchor")
	}
	m.search = [2]*diffSearchState{}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	key(m, 'S')
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	if !layoutRowHasSourceID(m.displayDetail()[m.cursor()], anchor) {
		t.Fatal("layout lost context source")
	}
	pin, _ := json.Marshal(first)
	contextReads := 0
	m.SetIssueContextReader(func(_ context.Context, id source.Identity, refresh bool) (source.IssueContext, error) {
		if id != first.Inventory.Comparison.Metadata.Identity {
			t.Fatal("context borrowed another PR")
		}
		if refresh {
			contextReads++
		}
		return source.IssueContext{Identity: id}, nil
	})
	_, cmdContext := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	if cmdContext == nil || m.top() != pageIssueContext {
		t.Fatal("context screen unavailable")
	}
	m.Update(cmdContext())
	_, cmdContext = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m.Update(cmdContext())
	namedKey(m, tea.KeyEscape)
	if contextReads != 1 || !bytes.Equal(words, wordState()) || !layoutRowHasSourceID(m.displayDetail()[m.cursor()], anchor) {
		t.Fatal("context refresh changed words or source anchor")
	}
	live := tuiLifecycleState(m)
	writes := 0
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		return source.Readiness{Identity: id, HeadSHA: live.HeadSHA}, nil
	})
	m.SetPRLifecycle(func(context.Context, source.Identity) (source.Lifecycle, error) { return live, nil }, func(context.Context, source.LifecycleAction) (source.LifecycleOutcome, error) {
		writes++
		return source.LifecycleOutcome{Attempted: true, Refreshed: true, Uncertain: true, Snapshot: live}, errors.New("synthetic uncertainty")
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	m.Update(cmd())
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m.Update(cmd())
	m.lifecycle.selected = 11
	namedKey(m, tea.KeyEnter)
	// A confirmation owns these keys: none may open a switcher, path/search
	// control, collapse tree, or another readiness request behind the modal.
	for _, k := range []tea.KeyPressMsg{{Code: 'I', Text: "I"}, {Code: 'P', Text: "P"}, {Code: 'o', Mod: tea.ModCtrl}, {Code: 'F', Text: "F"}, {Code: '/', Text: "/"}, {Code: 'C', Text: "C"}, {Code: 'c', Mod: tea.ModAlt}, {Code: 'r', Mod: tea.ModAlt}} {
		_, c := m.Update(k)
		if c != nil || m.top() != pageLifecycle || m.lifecycle.confirmation == nil {
			t.Fatal("lifecycle confirmation lost key ownership")
		}
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("visible confirmation unavailable")
	}
	m.Update(cmd())
	if writes != 1 || m.lifecycle.uncertain == nil {
		t.Fatal("missing uncertainty lock")
	}
	namedKey(m, tea.KeyEscape)
	namedKey(m, tea.KeyEscape)
	m.SetInbox(func(_ context.Context, o source.InboxOptions, _ bool) (source.Inbox, error) {
		if o.Account != "" && o.Account != "account-a" {
			t.Fatal("account changed")
		}
		return source.Inbox{Viewer: "account-a", Complete: true, Items: []source.InboxItem{{PullRequest: source.PullRequest{Identity: live.Identity}}}}, nil
	}, nil, func(context.Context, string, source.Identity, func(string)) (*review.Session, error) {
		t.Fatal("existing tab reopened")
		return nil, nil
	}, source.InboxOptions{}, false, false)
	key(m, 'P')
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if cmd == nil || m.top() != pageInbox {
		t.Fatal("inbox unreachable after leaving lifecycle")
	}
	m.Update(cmd())
	_, late := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	namedKey(m, tea.KeyEnter)
	m.Update(late())
	if m.top() != pagePullRequestPicker {
		t.Fatal("same active tab should return to switcher")
	}
	namedKey(m, tea.KeyEscape)
	if m.Session != first || m.top() != pageReview || m.inbox.options.Account != "account-a" || !layoutRowHasSourceID(m.displayDetail()[m.cursor()], anchor) || m.selectedDiffTarget() != nil || m.lifecycle.uncertain == nil {
		t.Fatalf("inbox return lost source/account/lifecycle ownership: session=%t page=%v account=%q anchor=%t target=%v uncertain=%t", m.Session == first, m.top(), m.inbox.options.Account, layoutRowHasSourceID(m.displayDetail()[m.cursor()], anchor), m.selectedDiffTarget(), m.lifecycle.uncertain != nil)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	m.Update(cmd())
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m.Update(cmd())
	namedKey(m, tea.KeyEnter)
	if writes != 1 || m.lifecycle.confirmation != nil {
		t.Fatal("return repeated uncertain write")
	}
	after, _ := json.Marshal(first)
	retained, err := store.LoadDraft(context.Background(), session.DraftKeyFor(first.Inventory.Comparison.Metadata))
	if err != nil || !bytes.Equal(pin, after) || !reflect.DeepEqual(original, retained) || !bytes.Equal(rawRows, inboxPrivateRows(t, store.Path())) || !bytes.Equal(words, wordState()) {
		t.Fatal("lifecycle/inbox changed private source or attempt", err)
	}
}
