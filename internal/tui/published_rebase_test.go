package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestPublishedEditorOwnsFilesInputWithCommitFilterAvailable(t *testing.T) {
	m := publishedModel(t)
	m.Loading = false
	m.Width, m.Height = 120, 24
	key(m, '2')
	frozen := m.Session
	key(m, 'C')
	if !m.commitFilter.open || !strings.Contains(ansi.Strip(m.View().Content), "Select commits") {
		t.Fatal("main commit-filter modal lost")
	}
	namedKey(m, tea.KeyEscape)
	if m.commitFilter.open || m.Session != frozen {
		t.Fatal("closing filter changed pinned comparison")
	}
	if !m.openPublished(1, "", false) {
		t.Fatal("published edit unavailable")
	}
	e := m.discussions.published
	draft := e.draft
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if m.fileFilterEditing || e.draft != draft+"/" {
		t.Fatal("files filter intercepted editor input")
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	if m.discussions.published != e || m.Session != frozen || !strings.Contains(ansi.Strip(m.View().Content), "Edit published comment") {
		t.Fatal("resize displaced editor or frozen comparison")
	}
	namedKey(m, tea.KeyEscape)
	if m.discussions.published != nil || m.fileFilter != "" || m.Session != frozen {
		t.Fatal("escape changed background state")
	}
	key(m, 'C')
	if !m.commitFilter.open {
		t.Fatal("published edit broke commit filter")
	}
}
