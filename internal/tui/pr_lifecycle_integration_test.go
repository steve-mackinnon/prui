package tui

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
