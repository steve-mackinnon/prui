package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/inventory"
	"prui/internal/source"
)

func TestSuggestionNavigationSearchKeepsReadOnlyRowsAndCanonicalTargets(t *testing.T) {
	for _, mode := range []string{"OLD", "NEW", "expanded"} {
		t.Run(mode, func(t *testing.T) {
			m := navigationModel("@@ -2 +2 @@\n-old\n+new\n", "context\nold\ntail\n", "context\nnew\ntail\n", inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 2, Count: 1})
			defer m.Close()
			meta := &m.Session.Inventory.Comparison.Metadata
			meta.BaseSHA, meta.HeadSHA = strings.Repeat("a", 40), strings.Repeat("b", 40)
			m.Focus = paneDiff
			// Leave a real canonical target selected before visiting read-only source.
			selectRawTarget(t, m, "RIGHT", 2)
			canonical := *m.selectedDiffTarget()
			m.navigation.mode = mode
			m.fileCache = fileDetailCache{}
			searchInput(m, "context")
			if !m.searchOpen() || len(m.searchState().matches) != 1 {
				t.Fatal("source search missing", mode)
			}
			ctrlKey(m, 's')
			if m.Composer != nil {
				t.Fatal("search input opened replacement editor")
			}
			namedKey(m, tea.KeyEnter)
			if m.selectedDiffTarget() != nil {
				t.Fatal("read-only search reused earlier target")
			}
			ctrlKey(m, 's')
			if m.Composer != nil {
				t.Fatal("read-only search match became suggestion")
			}
			if mode == "expanded" {
				key(m, '/')
				for range len("context") {
					_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
					if cmd != nil {
						m.Update(cmd())
					}
				}
				_, cmd := m.Update(tea.PasteMsg{Content: "new"})
				if cmd != nil {
					m.Update(cmd())
				}
				namedKey(m, tea.KeyEnter)
				ctrlKey(m, 's')
				if m.Composer == nil || !m.Composer.Suggestion || m.Composer.Target != canonical || m.Composer.Before != "new" || m.Composer.Draft != "new" {
					t.Fatal("canonical searched suggestion lost raw provenance", m.Composer, m.ActionError)
				}
			}
		})
	}
}

func TestSuggestionEditorAndApplyOwnNavigationSearchKeys(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "RIGHT", 1)
	ctrlKey(m, 's')
	if m.Composer == nil {
		t.Fatal("suggestion editor unavailable")
	}
	target := m.Composer.Target
	before := m.Composer.Before
	key(m, '/')
	m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModAlt})
	ctrlKey(m, 'e')
	ctrlKey(m, 'w')
	if m.searchOpen() || m.navigation.mode != "" || m.navigation.whitespace || m.Composer.Target != target || m.Composer.Before != before || m.Composer.Draft != "first/" {
		t.Fatal("search/navigation intercepted replacement editor")
	}
	namedKey(m, tea.KeyEscape)
	a := source.SuggestionApplication{Target: target, Before: "first", Replacement: "new"}
	m.SuggestionApply = &a
	m.push(pageSuggestionApply)
	key(m, '/')
	m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	m.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModAlt})
	ctrlKey(m, 'e')
	if m.searchOpen() || m.navigation.mode != "" || m.SuggestionApply != &a || m.SuggestionConfirm {
		t.Fatal("navigation displaced confirmation")
	}
	namedKey(m, tea.KeyEnter)
	if !m.SuggestionConfirm || m.SuggestionApply.Target != target {
		t.Fatal("confirmation route lost")
	}
	m.navigation.mode = "expanded"
	ctrlKey(m, 'd')
	if m.SuggestionApply != nil || m.navigation.mode != "expanded" || m.top() == pageSuggestionApply {
		t.Fatal("application discard was consumed by navigation")
	}
}
