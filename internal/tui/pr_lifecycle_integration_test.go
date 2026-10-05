package tui

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
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
