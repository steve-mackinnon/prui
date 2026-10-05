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
	"prui/internal/session"
	"prui/internal/source"
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
	for _, want := range []string{"✅ Checks pass", "#1  Passing", "Author alice  ·  Opened Sep 23, 2026", "Last commit bob", "❌ Checks fail"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in:\n%s", want, view)
		}
	}
	for _, tc := range []struct {
		index int
		label string
	}{{2, "⏳ Checks pending"}, {3, "? Checks unknown"}} {
		m.PullRequestPicker.Index = tc.index
		if got := ansi.Strip(m.View().Content); !strings.Contains(got, tc.label) {
			t.Fatalf("missing %q in:\n%s", tc.label, got)
		}
	}
	if lines := strings.Split(view, "\n"); len(lines) > m.Height {
		t.Fatalf("picker overflowed: %s", view)
	}
	if strings.ContainsAny(view, "╭╰") || strings.Count(view, "Author ") != 1 {
		t.Fatalf("expected compact rows and selected detail only:\n%s", view)
	}
}

func TestCompactPullRequestsBoundAndEscapeLongContent(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 10
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 42}, Title: "\x1b[31m Colorful change " + strings.Repeat("x", 120), Author: "alice", LastModifier: strings.Repeat("b", 100), Checks: source.ChecksFailed}}
	plain := ansi.Strip(m.View().Content)
	if strings.ContainsAny(plain, "╭╰") || !strings.Contains(plain, `\x1b[31m`) || (!strings.Contains(plain, "Last commit") || !strings.Contains(plain, strings.Repeat("b", 20))) || !strings.Contains(plain, "❌ Checks fail") {
		t.Fatalf("compact row, escaped title, or selected detail missing:\n%s", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if visibleWidth(line) > m.Width {
			t.Fatalf("row overflows: %q", line)
		}
	}
}

func TestCompactPullRequestsShowManyChoicesAndSelectedDetail(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 80, 15
	for i := 1; i <= 30; i++ {
		m.PullRequests = append(m.PullRequests, source.PullRequest{Identity: source.Identity{Number: i}, Title: fmt.Sprintf("Choice %02d", i), Author: fmt.Sprintf("author-%02d", i)})
	}
	m.PullRequestPicker.Index = 20
	view := ansi.Strip(m.View().Content)
	if strings.Count(view, "Choice ") < 6 || !strings.Contains(view, "› #21") || !strings.Contains(view, "Author author-21") || strings.Contains(view, "Author author-20") {
		t.Fatalf("picker did not prioritize compact choices and selected detail:\n%s", view)
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
				// The selected item takes priority when headers cannot fit.
				if !strings.Contains(content, "› ") || !strings.Contains(content, "20") {
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

func TestCompactSwitcherUsesOnlyAvailableSelectedMetadata(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	saved := largeSession(1, 1)
	saved.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	m.openReviewTab(saved)
	m.Stack = []page{pageReview, pagePullRequestPicker}
	m.Width, m.Height = 80, 12
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Repository: "owner/repo", Number: 2}, Title: "Available PR", Author: "alice", Checks: source.ChecksFailed}}
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "Author ") || strings.Contains(view, "Last commit ") || !strings.Contains(view, "open owner/repo#1") {
		t.Fatalf("saved review got invented metadata: %s", view)
	}
	m.PullRequestPicker.Index = 1
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Author alice") || !strings.Contains(view, "❌ Checks fail") {
		t.Fatalf("selected remote PR lacks available detail: %s", view)
	}
}

func TestCompactSwitcherKeepsSelectionVisibleAfterResize(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	saved := largeSession(1, 1)
	saved.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	m.openReviewTab(saved)
	m.Stack = []page{pageReview, pagePullRequestPicker}
	for i := 2; i < 32; i++ {
		m.PullRequests = append(m.PullRequests, source.PullRequest{Identity: source.Identity{Repository: "owner/repo", Number: i}, Title: "Long title " + strings.Repeat("界", 100), Checks: source.ChecksPending})
	}
	m.PullRequestPicker.Index = 20
	for _, width := range []int{24, 60, 120} {
		for _, height := range []int{1, 2, 4, 8, 20} {
			m.Update(tea.WindowSizeMsg{Width: width, Height: height})
			view := ansi.Strip(m.View().Content)
			if !strings.Contains(view, "› #21") {
				t.Fatalf("selection hidden at %dx%d:\\n%s", width, height, view)
			}
			lines := strings.Split(view, "\n")
			if len(lines) > height {
				t.Fatalf("height overflow at %dx%d", width, height)
			}
			for _, line := range lines {
				if visibleWidth(line) > width {
					t.Fatalf("width overflow: %q", line)
				}
			}
		}
	}
}

func TestPullRequestRowsPinAuthorsBeforeChecks(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Width = 80
	first := m.pullRequestRow(source.PullRequest{Identity: source.Identity{Number: 1}, Title: strings.Repeat("long title ", 20), Author: "alice", Checks: source.ChecksPassed})
	second := m.pullRequestRow(source.PullRequest{Identity: source.Identity{Number: 2}, Title: "Short", Author: "bob", Checks: source.ChecksPending})
	if !strings.Contains(first, "@alice") || !strings.Contains(second, "@bob") {
		t.Fatalf("authors missing from rows: %q / %q", first, second)
	}
	if visibleWidth(first[:strings.Index(first, "@alice")]) != visibleWidth(second[:strings.Index(second, "@bob")]) {
		t.Fatalf("author columns do not align: %q / %q", first, second)
	}
	for _, width := range []int{20, 32, 40, 60, 80} {
		m.Width = width
		row := m.pullRequestRow(source.PullRequest{Identity: source.Identity{Number: 42}, Title: strings.Repeat("x", 100), Author: strings.Repeat("a", 100), Checks: source.ChecksPassed})
		if visibleWidth(row) > width-2 || !strings.Contains(row, "#42") {
			t.Fatalf("invalid row at width %d: %q", width, row)
		}
	}
}

func TestPullRequestRowsKeepColumnsTogetherOnWideTerminals(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	pr := source.PullRequest{Identity: source.Identity{Number: 42}, Title: strings.Repeat("long title ", 20), Author: "alice", Checks: source.ChecksPassed}
	m.Width = 122
	want := m.pullRequestRow(pr)
	for _, width := range []int{160, 240, 400} {
		m.Width = width
		if got := m.pullRequestRow(pr); got != want {
			t.Fatalf("columns spread at width %d: %q; want %q", width, got, want)
		}
	}
}

func TestPullRequestRowsShowViewerReview(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Width = 80
	for _, tc := range []struct{ state, marker, detail string }{
		{"APPROVED", "✅", "Your review: Approved"},
		{"CHANGES_REQUESTED", "🔄", "Your review: Requested Changes"},
		{"COMMENTED", "💬", "Your review: Commented"},
		{"DISMISSED", "○", "Your review: None"},
		{"PENDING", "✎", "Your review: None"},
		{"", "○", "Your review: None"},
	} {
		pr := source.PullRequest{Identity: source.Identity{Number: 1}, Title: "Change", Author: "alice", ViewerReview: tc.state}
		if row := m.pullRequestRow(pr); !strings.Contains(row, tc.marker) || visibleWidth(row) > m.Width-2 {
			t.Fatalf("invalid row: %q", row)
		}
		if detail := m.pullRequestDetail(pr)[2]; detail != tc.detail {
			t.Fatalf("invalid detail: %q", detail)
		}
	}
}

func TestPullRequestTargetBranchInListAndPreview(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	pr := source.PullRequest{Identity: source.Identity{Number: 42}, Title: "Change", Author: "alice", TargetBranch: "release/next"}
	for _, width := range []int{62, 80, 120, 240} {
		m.Width = width
		row := ansi.Strip(m.prPickerRow(pr))
		if !strings.Contains(row, "target:") || !strings.Contains(row, "release") {
			t.Fatalf("target absent at %d: %q", width, row)
		}
	}
	if detail := strings.Join(m.pullRequestDetail(pr), "\n"); !strings.Contains(detail, "Target branch: release/next") {
		t.Fatal(detail)
	}
	for _, width := range []int{20, 32, 40, 60, 80, 100, 120} {
		m.Width = width
		pr.TargetBranch = strings.Repeat("界", 100) + "\x1b]52;unsafe\a"
		row := ansi.Strip(m.prPickerRow(pr))
		limit := width - 2
		if width >= 100 {
			limit = m.prPickerGeometry(1).leftWidth - 2
		}
		if visibleWidth(row) > limit || strings.Contains(row, "\x1b") {
			t.Fatalf("unsafe/oversized row at %d: %q", width, row)
		}
	}
}
