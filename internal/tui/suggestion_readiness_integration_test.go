package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/source"
)

func TestSuggestionReadinessAndCommitFilterKeepFrozenTargets(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	m.openReviewTab(m.Session)
	m.Loading = false
	m.selectReviewView(viewFiles)
	m.Selected, m.Focus = 1, paneDiff
	selectRawTarget(t, m, "RIGHT", 1)
	frozen := m.Session
	target := *m.selectedDiffTarget()
	reads, writes := 0, 0
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		reads++
		return source.Readiness{Identity: id, HeadSHA: strings.Repeat("c", 40), HeadVerified: true}, nil
	})
	m.SetSuggestionActions(nil, func(context.Context, source.SuggestionApplication) (source.SuggestionResult, error) {
		writes++
		return source.SuggestionResult{}, nil
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if cmd == nil || m.top() != pageReadiness {
		t.Fatal("Alt+R readiness lost")
	}
	m.Update(cmd())
	ctrlKey(m, 's')
	ctrlKey(m, 'a')
	namedKey(m, tea.KeyEnter)
	if reads != 1 || writes != 0 || m.Session != frozen || m.Composer != nil || m.SuggestionApply != nil || !m.readiness.loaded {
		t.Fatal("readiness changed frozen suggestion or wrote")
	}
	namedKey(m, tea.KeyEscape)
	key(m, 'C')
	if !m.commitFilter.open || m.top() != pageReview || m.Session != frozen {
		t.Fatal("C filter replaced readiness or lost source")
	}
	namedKey(m, tea.KeyEscape)
	selectRawTarget(t, m, "RIGHT", 1)
	ctrlKey(m, 's')
	if m.Composer == nil || m.Composer.Target != target || m.Composer.Before != "first" {
		t.Fatal("readiness changed canonical replacement target")
	}
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	key(m, 'C')
	if m.top() != pageReview || m.commitFilter.open || reads != 1 || m.Composer.Draft != "firstC" {
		t.Fatal("readiness/filter intercepted editor input")
	}
	namedKey(m, tea.KeyEscape)
	a := source.SuggestionApplication{Target: target, Metadata: frozen.Inventory.Comparison.Metadata, Replacement: "retained replacement"}
	m.SuggestionApply = &a
	m.push(pageSuggestionApply)
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	key(m, 'C')
	namedKey(m, tea.KeyEnter)
	if m.top() != pageSuggestionApply || !m.SuggestionConfirm || m.SuggestionApply != &a || reads != 1 || writes != 0 || m.commitFilter.open {
		t.Fatal("readiness/filter displaced application confirmation")
	}
}
