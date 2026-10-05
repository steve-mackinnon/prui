package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"path/filepath"
	"prui/internal/layoutprefs"
	"testing"
)

func TestLayoutPreferencesRestartNarrowAndTabs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	p := layoutprefs.Preferences{Version: 1, Split: true, RailWidth: 80, CommitWidth: 44, GroupFiles: true, CollapseGenerated: true}
	if err := layoutprefs.Save(path, p); err != nil {
		t.Fatal(err)
	}
	stored, err := layoutprefs.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := largeModel(largeTextSession(2, 2), 220, 24)
	m.SetLayoutPreferences(stored, func(p layoutprefs.Preferences) error { return layoutprefs.Save(path, p) })
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.sideBySideEnabled() || m.listWidthPreference != 80 || m.layout != diffLayoutSideBySide {
		t.Fatal("narrow fallback lost desired layout")
	}
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 24})
	if !m.sideBySideEnabled() || m.listWidth() != 80 {
		t.Fatal("wide preference not restored")
	}
	key(m, 'S')
	key(m, ']')
	key(m, 'B')
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	stored, err = layoutprefs.Load(path)
	if err != nil || stored.Split || stored.RailWidth != 82 || stored.GroupFiles || stored.CollapseGenerated {
		t.Fatalf("%+v %v", stored, err)
	}
	fresh := New(context.Background(), nil)
	fresh.SetLayoutPreferences(stored, nil)
	tab := fresh.newDraftReviewTab(m.Session)
	if tab.listWidthPreference != 82 || tab.commit.width != 44 || tab.layout != diffLayoutUnified {
		t.Fatal("new tab lost global initial preferences")
	}
	// Existing tabs own their sizing even after another tab changes defaults.
	m.listWidthPreference = 91
	m.restoreReviewTab(tab)
	if m.listWidthPreference != 82 {
		t.Fatal("tab restoration changed saved width")
	}
}
