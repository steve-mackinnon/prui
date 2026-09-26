package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/theme"
)

func TestThemePickerOpensNavigatesAndCancels(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	base, err := theme.Resolve(theme.Dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(base)

	key(m, 't')
	if m.top() != pageThemePicker || m.ThemePicker.Index != 2 {
		t.Fatalf("t did not open at active theme: page=%v index=%d", m.top(), m.ThemePicker.Index)
	}
	view := ansi.Strip(m.View().Content)
	for _, name := range theme.BuiltInNames() {
		if !strings.Contains(view, name) {
			t.Fatalf("picker omitted %q:\n%s", name, view)
		}
	}
	if !strings.Contains(view, "› dark") {
		t.Fatalf("picker does not mark active theme without color:\n%s", view)
	}
	if !strings.Contains(view, "enter: apply & save") || !strings.Contains(view, "esc/t: cancel") {
		t.Fatalf("picker lacks color-independent controls:\n%s", view)
	}

	namedKey(m, tea.KeyDown)
	if m.ThemePicker.Index != 3 {
		t.Fatalf("down index = %d, want 3", m.ThemePicker.Index)
	}
	key(m, 'k')
	if m.ThemePicker.Index != 2 {
		t.Fatalf("k index = %d, want 2", m.ThemePicker.Index)
	}
	key(m, 't')
	if m.top() != pageReview || m.theme.Name != theme.Dark {
		t.Fatalf("t did not cancel picker without changing theme: page=%v theme=%q", m.top(), m.theme.Name)
	}
}

func TestThemePickerPersistsBeforeApplyingAndKeepsOverrides(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	base, err := theme.Resolve(theme.Dark, map[theme.Token]string{theme.Added: "#112233"})
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(base)
	var saved string
	m.SetThemeSelectionSaver(func(name string) (theme.PersistResult, error) { saved = name; return theme.PersistResult{}, nil })

	key(m, 't')
	m.ThemePicker.Index = 1 // light
	namedKey(m, tea.KeyEnter)
	if saved != theme.Light || m.theme.Name != theme.Light || m.top() != pageReview {
		t.Fatalf("selection was not persisted then applied: saved=%q theme=%q page=%v", saved, m.theme.Name, m.top())
	}
	if got := m.theme.Syntax(theme.Added); got != "#112233" {
		t.Fatalf("selection lost valid override: added=%q", got)
	}
}

func TestThemePickerRetainsExplicitOverrideThatMatchesItsOriginalBase(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	// This is dark's built-in Added color. Its provenance still matters: when
	// switching to light, it must remain an explicit override rather than
	// silently adopting light's Added color.
	base, err := theme.Resolve(theme.Dark, map[theme.Token]string{theme.Added: "#7ee787"})
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(base)
	m.SetThemeSelectionSaver(func(string) (theme.PersistResult, error) { return theme.PersistResult{}, nil })

	key(m, 't')
	m.ThemePicker.Index = 1 // light
	namedKey(m, tea.KeyEnter)
	if got := m.theme.Syntax(theme.Added); got != "#7ee787" {
		t.Fatalf("switch lost explicit same-as-base override: added=%q", got)
	}
}

func TestThemePickerPersistsButDoesNotReplaceCLISelectedTheme(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	active, err := theme.Resolve(theme.Dark, map[theme.Token]string{theme.Added: "#112233"})
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(active)
	m.SetThemeSelectionLocked(true)
	var saved string
	m.SetThemeSelectionSaver(func(name string) (theme.PersistResult, error) { saved = name; return theme.PersistResult{}, nil })

	key(m, 't')
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "enter: save for next launch") || strings.Contains(view, "enter: apply & save") {
		t.Fatalf("locked picker advertised the wrong action:\n%s", view)
	}
	m.ThemePicker.Index = 1 // light, persisted for the next launch
	namedKey(m, tea.KeyEnter)
	if saved != theme.Light {
		t.Fatalf("picker persisted %q, want %q", saved, theme.Light)
	}
	if m.theme.Name != theme.Dark || m.theme.Syntax(theme.Added) != "#112233" {
		t.Fatalf("picker replaced CLI-selected live theme: %#v", m.theme)
	}
}

func TestThemePickerAppliesSelectionAfterDurabilityWarning(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	m.SetThemeSelectionSaver(func(string) (theme.PersistResult, error) {
		return theme.PersistResult{DurabilityWarning: errors.New("private filesystem detail")}, nil
	})

	key(m, 't')
	m.ThemePicker.Index = 1 // light
	namedKey(m, tea.KeyEnter)
	if m.theme.Name != theme.Light {
		t.Fatalf("durability warning did not apply completed selection: %q", m.theme.Name)
	}
	if !strings.Contains(m.notice, "durability could not be confirmed") || strings.Contains(m.notice, "private filesystem") {
		t.Fatalf("durability warning leaked details or was absent: %q", m.notice)
	}
}

func TestThemePickerPersistenceFailureKeepsThemeAndShowsSafeFeedback(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	base, err := theme.Resolve(theme.Dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(base)
	m.SetThemeSelectionSaver(func(string) (theme.PersistResult, error) {
		return theme.PersistResult{}, errors.New("private config contents must not be shown")
	})

	key(m, 't')
	m.ThemePicker.Index = 1
	namedKey(m, tea.KeyEnter)
	if m.theme.Name != theme.Dark || m.top() != pageReview {
		t.Fatalf("failed save changed active theme: theme=%q page=%v", m.theme.Name, m.top())
	}
	if m.ActionError == nil || strings.Contains(m.ActionError.Error(), "private config") {
		t.Fatalf("failed save did not provide safe feedback: %v", m.ActionError)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "theme selection was not saved") {
		t.Fatalf("failed save feedback not visible:\n%s", view)
	}
}

func TestThemePickerYieldsToExistingModalInputAndFitsTinyTerminals(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	m.Composer = &commentComposer{}
	key(m, 't')
	if m.top() != pageReview {
		t.Fatalf("theme picker stole composer input: page=%v", m.top())
	}
	m.Composer = nil
	m.CommentMenu = &commentActionMenu{}
	key(m, 't')
	if m.top() != pageReview {
		t.Fatalf("theme picker stole action-menu input: page=%v", m.top())
	}
	m.CommentMenu = nil
	m.Busy = true
	key(m, 't')
	if m.top() != pageReview {
		t.Fatalf("theme picker stole loading input: page=%v", m.top())
	}
	m.Busy = false
	key(m, 't')
	for _, size := range [][2]int{{10, 2}, {19, 5}, {20, 6}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(m.View().Content)
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Fatalf("%dx%d picker has %d lines:\n%s", size[0], size[1], len(lines), view)
		}
		for _, line := range lines {
			if visibleWidth(line) > size[0] {
				t.Fatalf("%dx%d picker overflow: %q", size[0], size[1], line)
			}
		}
	}
}

func TestThemePickerIsAvailableOnNavigationPickersOnly(t *testing.T) {
	m := New(context.Background(), nil)
	for _, p := range []page{pagePicker, pageRepositoryPicker, pagePullRequestPicker} {
		m.Stack = []page{p}
		key(m, 't')
		if m.top() != pageThemePicker {
			t.Fatalf("t did not open from picker page %v: stack=%v", p, m.Stack)
		}
		key(m, 't')
	}
	for _, p := range []page{pageHelp, pageURL, pageEvidence, pageGuideConsent} {
		m.Stack = []page{p}
		key(m, 't')
		if m.top() != p {
			t.Fatalf("t unexpectedly opened from page %v: stack=%v", p, m.Stack)
		}
	}
}
