package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/session"
	"pr-review/internal/theme"
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
	for _, want := range []string{"╭ Description [2] ╮", "Frozen from GitHub when this review opened.", "Summary", `\x1b]52;c;unsafe\x07`} {
		if !strings.Contains(view, want) {
			t.Fatalf("description view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "\x1b]") || strings.Contains(view, "main.go") {
		t.Fatalf("description view leaked terminal controls or diff UI:\n%s", view)
	}
}

func TestDescriptionViewKeepsGlobalActionsAndDisablesProgressCommentActions(t *testing.T) {
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, metadata, _ := guidedSession(t, splitAnalyzer{})
	description := "Frozen text"
	s.PullRequestDescription = &description
	saved, err := store.Create(s.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.SetLifecycle(store, fakeGitHub{metadata}, nil)
	m.openReviewTab(saved)
	key(m, 'v')

	key(m, 'e')
	if m.top() != pageEvidence {
		t.Fatalf("e did not open evidence from Description: %v", m.Stack)
	}
	namedKey(m, tea.KeyEscape)
	key(m, 'U')
	if m.top() != pageURL {
		t.Fatalf("U did not open the GitHub URL from Description: %v", m.Stack)
	}
	namedKey(m, tea.KeyEscape)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil || !m.Busy {
		t.Fatal("r did not start metadata refresh from Description")
	}
	completeAction(t, m, cmd)
	before := len(m.Session.ReviewedSliceIDs)
	key(m, 'm')
	key(m, 'c')
	if len(m.Session.ReviewedSliceIDs) != before || m.Busy {
		t.Fatal("Description progress or comment action mutated review state")
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

func TestDescriptionViewCachesRenderedLinesForScrollAndRebuildsOnResize(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	description := "# Heading\n\n" + strings.Repeat("a description line that needs rendering\n", 20)
	s.PullRequestDescription = &description
	m.openReviewTab(s)
	m.Width, m.Height = 60, 7
	key(m, 'v')

	first := m.descriptionLines()
	if m.descriptionCache.body != description || m.descriptionCache.width != 60 || m.descriptionCache.themeName != theme.Terminal || len(first) == 0 {
		t.Fatalf("description render was not cached: %#v", m.descriptionCache)
	}
	key(m, 'j')
	if got := m.descriptionLines(); len(got) == 0 || &got[0] != &first[0] {
		t.Fatal("description scroll did not reuse cached rendered lines")
	}

	m.Update(tea.WindowSizeMsg{Width: 36, Height: 7})
	resized := m.descriptionLines()
	if m.descriptionCache.width != 36 {
		t.Fatalf("description cache width = %d, want 36", m.descriptionCache.width)
	}
	if &resized[0] == &first[0] {
		t.Fatal("description resize reused stale rendered lines")
	}
}

func TestDescriptionViewInvalidatesEveryTabCacheWhenThemeChanges(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	first, second := screenSession(), screenSession()
	firstDescription, secondDescription := "# First", "# Second"
	first.PullRequestDescription, second.PullRequestDescription = &firstDescription, &secondDescription
	second.Inventory.Comparison.Metadata.Identity.Number = 2
	m.openReviewTab(first)
	key(m, 'v')
	m.descriptionLines()
	m.openReviewTab(second)
	key(m, 'v')
	beforeTheme := m.descriptionLines()

	dark, err := theme.Resolve(theme.Dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(dark)
	if m.descriptionCache.valid || m.tabs[0].review.descriptionCache.valid || m.tabs[1].review.descriptionCache.valid {
		t.Fatal("theme change retained a stale description render cache")
	}
	afterTheme := m.descriptionLines()
	if m.descriptionCache.themeName != theme.Dark {
		t.Fatalf("description cache theme = %q, want %q", m.descriptionCache.themeName, theme.Dark)
	}
	if len(afterTheme) == 0 || &afterTheme[0] == &beforeTheme[0] {
		t.Fatal("theme change did not rebuild description rendering")
	}
}
