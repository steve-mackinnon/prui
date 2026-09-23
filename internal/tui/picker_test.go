package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/session"
	"pr-review/internal/source"
)

func TestPickerListingCancellationAndRetry(t *testing.T) {
	for _, keyCode := range []rune{'s', 'b'} {
		t.Run(string(keyCode), func(t *testing.T) {
			m := New(context.Background(), nil)
			m.Session = largeSession(1, 1)
			original := m.Session
			m.SetLifecycle(pickerStore(t), nil, nil)
			release := make(chan struct{})
			started := make(chan struct{})
			t.Cleanup(m.Close)
			// Registered after Close so a failed assertion cannot strand a worker.
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			calls := 0
			read := func() error {
				calls++
				if calls == 1 {
					close(started)
					<-release
				}
				if calls == 2 {
					return errors.New("synthetic storage failure")
				}
				return nil
			}
			m.listSessions = func() ([]session.Entry, error) {
				return []session.Entry{{ID: "fixture", Err: errors.New("unreadable fixture")}}, read()
			}
			m.listRepositories = func() ([]session.Repository, error) {
				return []session.Repository{{Repository: "owner/fixture"}}, read()
			}
			returned := make(chan tea.Cmd, 1)
			go func() { _, cmd := m.Update(tea.KeyPressMsg{Code: keyCode, Text: string(keyCode)}); returned <- cmd }()
			var cmd tea.Cmd
			select {
			case cmd = <-returned:
			case <-time.After(time.Second):
				t.Fatal("Update blocked on storage")
			}
			if !m.Busy || !strings.Contains(m.View().Content, "Loading") {
				t.Fatal("missing loading state")
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("storage worker did not start")
			}
			namedKey(m, tea.KeyEscape)
			close(release)
			m.Update(cmd())
			if !errors.Is(m.ActionError, context.Canceled) || m.Session != original || len(m.Entries)+len(m.Repositories) != 0 {
				t.Fatal("canceled listing replaced the review or adopted late results")
			}
			action(t, m, 'r')
			if m.ActionError == nil || !strings.Contains(m.ActionError.Error(), "storage failure") {
				t.Fatal("listing failure hidden")
			}
			action(t, m, 'r')
			if m.ActionError != nil || m.Busy || len(m.Entries)+len(m.Repositories) != 1 {
				t.Fatal("listing could not retry")
			}
			namedKey(m, tea.KeyEscape)
			if m.top() != pageReview || m.Session != original {
				t.Fatal("back lost original review")
			}
		})
	}
}

func TestEmptyPickersIgnoreEnter(t *testing.T) {
	for _, screen := range []page{pagePicker, pageRepositoryPicker, pagePullRequestPicker} {
		m := New(context.Background(), nil)
		m.Stack = []page{screen}
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd != nil || m.Busy {
			t.Fatal("empty picker started an operation")
		}
		_ = m.View()
		m.Close()
	}
}

func pickerStore(t *testing.T) *session.Store {
	t.Helper()
	s, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestBrowserInitializationListsLocalRepositories(t *testing.T) {
	store := pickerStore(t)
	if err := store.RememberRepository("owner/repo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	m := NewPullRequestBrowser(context.Background(), store, nil, nil)
	defer m.Close()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("browser initialization did not load local repositories")
	}
	m.Update(cmd())
	if m.Busy || m.top() != pageRepositoryPicker || !strings.Contains(ansi.Strip(m.View().Content), "› owner/repo") {
		t.Fatalf("browser not ready: %s", m.View().Content)
	}
	if m.GuideScroll == nil {
		t.Fatal("browser did not initialize guide scrolling")
	}
}

func TestPickerUsesSharedSelectionAndHealthChrome(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Stack = []page{pageRepositoryPicker}
	m.Repositories = []session.Repository{{Repository: "owner/repo"}}
	m.Width, m.Height = 80, 8

	view := m.View().Content
	if !strings.Contains(view, "› owner/repo") || !strings.Contains(view, "?: Health & help") {
		t.Fatalf("picker does not use shared chrome:\n%s", view)
	}
}

func TestPullRequestPickerOpensHelpAndReturns(t *testing.T) {
	m := NewCurrentRepositoryBrowser(context.Background(), pickerStore(t), "owner/repo", t.TempDir(), nil, nil)
	defer m.Close()
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Repository: "owner/repo", Number: 42}, Title: "Open me"}}

	key(m, '?')
	if m.top() != pageHelp || !strings.Contains(ansi.Strip(m.View().Content), "Health & help") {
		t.Fatalf("pull request picker did not open help: %s", m.View().Content)
	}
	namedKey(m, tea.KeyEscape)
	if m.top() != pagePullRequestPicker || !strings.Contains(ansi.Strip(m.View().Content), "Open me") {
		t.Fatalf("help did not return to pull request picker: %s", m.View().Content)
	}
}

func TestPullRequestPickerShowsMetadataAndCheckStates(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 80, 15
	m.PullRequests = []source.PullRequest{
		{Identity: source.Identity{Number: 1}, Title: "Passing", Author: "alice", LastModifier: "bob", OpenedAt: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC), Checks: source.ChecksPassed},
		{Identity: source.Identity{Number: 2}, Title: "Failing", Checks: source.ChecksFailed},
		{Identity: source.Identity{Number: 3}, Title: "Running", Checks: source.ChecksPending},
		{Identity: source.Identity{Number: 4}, Title: "No status", Checks: source.ChecksUnknown},
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"╭─ ✓ Checks pass", "╰", "#1  Passing", "Author alice  ·  Opened Sep 23, 2026", "Last commit bob", "✗ Checks fail"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in:\n%s", want, view)
		}
	}
	for _, tc := range []struct {
		index int
		label string
	}{{2, "… Checks pending"}, {3, "? Checks unknown"}} {
		m.PullRequestPicker.Index = tc.index
		if got := ansi.Strip(m.View().Content); !strings.Contains(got, tc.label) {
			t.Fatalf("missing %q in:\n%s", tc.label, got)
		}
	}
	if lines := strings.Split(view, "\n"); len(lines) > m.Height {
		t.Fatalf("picker overflowed: %s", view)
	}
	if strings.Count(view, "╭") != strings.Count(view, "╰") {
		t.Fatalf("partial card rendered:\n%s", view)
	}
}

func TestPullRequestCardsKeepBordersAndSemanticColorAtWideWidth(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 10
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 42}, Title: "\x1b[31m Colorful change " + strings.Repeat("x", 120), Author: "alice", LastModifier: strings.Repeat("b", 100), Checks: source.ChecksFailed}}
	colored := m.View().Content
	if !strings.Contains(colored, m.styleLine(classRemoved, "✗ Checks fail")) || !strings.Contains(colored, m.styleLine(classPaneBorderFocused, "╭─ ")) {
		t.Fatalf("card lost semantic status or selected border color: %q", colored)
	}
	plain := ansi.Strip(colored)
	if strings.Count(plain, "╭") != 1 || strings.Count(plain, "╰") != 1 || !strings.Contains(plain, `\x1b[31m`) || !strings.Contains(plain, "Last commit b") {
		t.Fatalf("card border or escaped title missing:\n%s", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if visibleWidth(line) > m.Width {
			t.Fatalf("wide card overflows: %q", line)
		}
	}
}

func TestPickerBackNavigationRestoresRepositorySelection(t *testing.T) {
	for _, count := range []int{1, 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			m := New(context.Background(), nil)
			defer m.Close()
			m.Loading = false
			m.Stack = []page{pageRepositoryPicker}
			for i := 0; i < count; i++ {
				m.Repositories = append(m.Repositories, session.Repository{Repository: fmt.Sprintf("owner/repo%d", i)})
			}
			selected := ""
			m.listPullRequests = func(_ context.Context, repository string) ([]source.PullRequest, error) {
				selected = repository
				return []source.PullRequest{{Title: "First"}, {Title: "Second"}, {Title: "Third"}, {Title: "Fourth"}}, nil
			}
			if count > 1 {
				namedKey(m, tea.KeyDown)
			}
			enter := func() {
				_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if cmd != nil {
					m.Update(cmd())
				}
			}
			enter()
			want := selected
			namedKey(m, tea.KeyDown)
			namedKey(m, tea.KeyDown)
			namedKey(m, tea.KeyDown)
			namedKey(m, tea.KeyEscape)
			if !strings.Contains(ansi.Strip(m.View().Content), "› "+want) {
				t.Fatalf("parent selection lost after back: %s", m.View().Content)
			}
			enter()
			if selected != want {
				t.Fatalf("reopened %q, want %q", selected, want)
			}
		})
	}
}

func TestPickerViewportKeepsSelectionVisible(t *testing.T) {
	for _, screen := range []page{pagePicker, pageRepositoryPicker, pagePullRequestPicker} {
		t.Run(fmt.Sprint(screen), func(t *testing.T) {
			m := New(context.Background(), nil)
			defer m.Close()
			m.Loading = false
			m.store = pickerStore(t)
			m.Stack = []page{screen}
			for i := 0; i < 30; i++ {
				m.Entries = append(m.Entries, session.Entry{ID: fmt.Sprintf("entry-%02d", i), Err: fmt.Errorf("fixture")})
				m.Repositories = append(m.Repositories, session.Repository{Repository: fmt.Sprintf("repo-%02d", i)})
				m.PullRequests = append(m.PullRequests, source.PullRequest{Identity: source.Identity{Number: i}, Title: fmt.Sprintf("pr-%02d", i)})
			}
			for i := 0; i < 20; i++ {
				namedKey(m, tea.KeyDown)
			}
			for _, height := range []int{10, 4, 2, 1, 8} {
				m.Update(tea.WindowSizeMsg{Width: 24, Height: height})
				content := ansi.Strip(m.View().Content)
				// One-line terminals show the fixed workspace strip; there is no
				// remaining row for a picker item. At two rows and above, the
				// selected item must stay visible below that strip.
				if height >= 2 && (!strings.Contains(content, "› ") || !strings.Contains(content, "20")) {
					t.Fatalf("selection hidden at height %d: %s", height, content)
				}
				if len(strings.Split(content, "\n")) > height {
					t.Fatalf("height overflow: %s", content)
				}
				for _, line := range strings.Split(content, "\n") {
					if visibleWidth(line) > 24 {
						t.Fatalf("width overflow: %q", line)
					}
				}
			}
		})
	}
}
