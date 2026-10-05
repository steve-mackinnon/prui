package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"prui/internal/session"
	"strings"
	"testing"
)

func TestCommentTypeSelectorCyclesAndPreservesDrafts(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "RIGHT", 1)
	namedKey(m, tea.KeyEnter)
	c := m.Composer
	if c == nil {
		t.Fatal("Enter did not open editor")
	}
	target := c.Target
	view := m.View().Content
	for _, label := range []string{"[Comment]", "Suggestion", "File comment", "tab: type"} {
		if !strings.Contains(view, label) {
			t.Fatalf("missing %q in %s", label, view)
		}
	}
	c.Draft, c.Cursor = "comment draft", 13
	namedKey(m, tea.KeyTab)
	if !c.Suggestion || c.Draft != "first" || c.Target != target {
		t.Fatalf("suggestion: %+v", c)
	}
	c.Draft, c.Cursor = "replacement", 11
	namedKey(m, tea.KeyTab)
	if c.Suggestion || c.Target.SubjectType != "file" || c.Draft != "" {
		t.Fatalf("file: %+v", c)
	}
	c.Draft = "file draft"
	namedKey(m, tea.KeyTab)
	if c.Suggestion || c.Target != target || c.Draft != "comment draft" {
		t.Fatalf("comment restored: %+v", c)
	}
	namedKey(m, tea.KeyTab)
	if !c.Suggestion || c.Draft != "replacement" {
		t.Fatalf("replacement lost: %+v", c)
	}
	namedKey(m, tea.KeyTab)
	if c.Draft != "file draft" {
		t.Fatal("file draft lost")
	}
}

func TestCommentTypeSelectorSkipsUnsupportedSuggestions(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "LEFT", 1)
	m.openCommentComposer()
	if m.Composer == nil {
		t.Fatal("comment unavailable")
	}
	if strings.Contains(m.View().Content, "[Comment] · Suggestion") {
		t.Fatal("old-side suggestion offered")
	}
	namedKey(m, tea.KeyTab)
	if m.Composer.Suggestion || m.Composer.Target.SubjectType != "file" {
		t.Fatal("did not skip unsupported suggestion")
	}
}

func TestCommentTypeSelectorCannotChangeUncertainDelivery(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "RIGHT", 1)
	m.openCommentComposer()
	m.draft.attempt = "comment"
	namedKey(m, tea.KeyTab)
	if m.Composer.Suggestion || m.Composer.Target.SubjectType != "" || m.ActionError == nil {
		t.Fatal("uncertain payload changed")
	}
}

func TestCommentTypeDraftsSurviveRestart(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := rangeTestModel()
	defer m.Close()
	m.Session.ID = strings.Repeat("d", 32)
	meta := &m.Session.Inventory.Comparison.Metadata
	meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
	m.SetLifecycle(store, nil, nil)
	selectRawTarget(t, m, "RIGHT", 1)
	m.openCommentComposer()
	original := m.Composer.Target
	m.Composer.Draft = "comment draft"
	namedKey(m, tea.KeyTab)
	m.Composer.Draft = "replacement"
	namedKey(m, tea.KeyTab)
	m.Composer.Draft = "file draft"
	if !m.persistDraft(m.reviewTabState) {
		t.Fatal(m.ActionError)
	}
	restored := New(context.Background(), nil)
	defer restored.Close()
	restored.SetLifecycle(store, nil, nil)
	restored.openReviewTab(m.Session)
	namedKey(restored, tea.KeyEnter)
	if restored.Composer == nil || restored.Composer.Draft != "file draft" {
		t.Fatal("file editor lost")
	}
	namedKey(restored, tea.KeyTab)
	if restored.Composer.Target != original || restored.Composer.Draft != "comment draft" {
		t.Fatal("line draft lost")
	}
	namedKey(restored, tea.KeyTab)
	if !restored.Composer.Suggestion || restored.Composer.Draft != "replacement" {
		t.Fatal("suggestion lost")
	}
	namedKey(restored, tea.KeyTab)
	restored.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !restored.Composer.Suggestion {
		t.Fatal("Shift+Tab did not cycle backwards")
	}
}
