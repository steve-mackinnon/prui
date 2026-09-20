package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDescriptionViewRendersFrozenEscapedTextAndProvenance(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	description := "Summary\n\x1b]52;c;unsafe\a\nA deliberately long line that must wrap inside a narrow description view."
	s.PullRequestDescription = &description
	m.openReviewTab(s)
	m.Width, m.Height = 48, 10
	m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"View: [Description]", "Frozen from GitHub when this review opened.", "Summary", `\x1b]52;c;unsafe\a`} {
		if !strings.Contains(view, want) {
			t.Fatalf("description view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "\x1b]") || strings.Contains(view, "main.go") {
		t.Fatalf("description view leaked terminal controls or diff UI:\n%s", view)
	}
}

func TestDescriptionViewDistinguishesEmptyAndLegacyDescriptions(t *testing.T) {
	for _, tc := range []struct {
		name, want  string
		description *string
	}{
		{"empty captured", "No description provided.", func() *string { text := ""; return &text }()},
		{"legacy absent", "Description was not captured for this session.", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			s := screenSession()
			s.PullRequestDescription = tc.description
			m.openReviewTab(s)
			key(m, 'v')
			if view := ansi.Strip(m.View().Content); !strings.Contains(view, tc.want) {
				t.Fatalf("description state missing %q:\n%s", tc.want, view)
			}
		})
	}
}

func TestDescriptionViewScrollIsTabOwnedAndLeavesChangesStateUntouched(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	description := strings.Repeat("description line\n", 20)
	s.PullRequestDescription = &description
	m.openReviewTab(s)
	m.Width, m.Height = 60, 7
	m.Selected, m.Row, m.Focus = 1, 1, paneDiff
	m.Scroll[1], m.Horizontal = 3, 5
	key(m, 'v')
	key(m, 'j')
	namedKey(m, tea.KeyEscape)
	if m.DescriptionScroll == 0 {
		t.Fatal("description navigation did not scroll")
	}
	if m.Selected != 1 || m.Row != 1 || m.Focus != paneDiff || m.Scroll[1] != 3 || m.Horizontal != 5 {
		t.Fatalf("description navigation changed source review state: %#v", m)
	}
	key(m, 'V')
	if got := m.selectedReviewView(); got != viewChanges || m.DescriptionScroll == 0 {
		t.Fatalf("switching back lost view or description scroll: view=%v scroll=%d", got, m.DescriptionScroll)
	}
}
