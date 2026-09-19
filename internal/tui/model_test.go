package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/review"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

type fakeGitHub struct{ m source.Metadata }

func (g fakeGitHub) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.m, nil
}
func (g fakeGitHub) ListPullRequests(context.Context, string) ([]source.PullRequest, error) {
	return nil, nil
}
func (g fakeGitHub) Token(context.Context) (string, error) { panic("no live network allowed") }
func key(m *Model, k rune)                                 { m.Update(tea.KeyPressMsg{Code: k, Text: string(k)}) }
func namedKey(m *Model, k rune)                            { m.Update(tea.KeyPressMsg{Code: k}) }
func ctrlKey(m *Model, k rune)                             { m.Update(tea.KeyPressMsg{Code: k, Mod: tea.ModCtrl}) }

// completeAction follows the loading command chain exactly as Bubble Tea does.
// A single command invocation may only return a loading tick when the action
// takes longer than its animation interval.
func completeAction(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	for m.Busy && cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if m.Busy {
		t.Fatal("action ended without clearing busy state")
	}
}

func TestCommandSwitcherOpensOverReviewAndListsOpenTabsFirst(t *testing.T) {
	m := New(context.Background(), nil)
	first := largeSession(1, 1)
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	second := largeSession(1, 1)
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 2}
	m.openReviewTab(first)
	m.openReviewTab(second)

	ctrlKey(m, 'p')
	if m.top() != pagePullRequestPicker {
		t.Fatalf("ctrl+p did not open the PR switcher: %#v", m.Stack)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Switch pull requests") || !strings.Contains(view, "open owner/repo#1") || !strings.Contains(view, "open owner/repo#2") {
		t.Fatalf("switcher did not list open reviews first:\n%s", view)
	}
	namedKey(m, tea.KeyEscape)
	if m.top() != pageReview || m.Session != second {
		t.Fatal("escape did not dismiss the switcher without changing the active review")
	}
}

func TestCommandSwitcherFiltersAndActivatesExistingReviewWithoutOpening(t *testing.T) {
	m := New(context.Background(), nil)
	first := largeSession(1, 1)
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	second := largeSession(1, 1)
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 2}
	m.openReviewTab(first)
	m.openReviewTab(second)
	m.PullRequests = []source.PullRequest{
		{Identity: first.Inventory.Comparison.Metadata.Identity, Title: "duplicate"},
		{Identity: source.Identity{Repository: "owner/repo", Number: 3}, Title: "database migration"},
	}

	ctrlKey(m, 'p')
	key(m, 'm')
	key(m, 'i')
	key(m, 'g')
	if view := ansi.Strip(m.View().Content); strings.Contains(view, "owner/repo#1") || !strings.Contains(view, "database migration") {
		t.Fatalf("filter did not use the query or deduplicate opened PRs:\n%s", view)
	}
	namedKey(m, tea.KeyBackspace)
	namedKey(m, tea.KeyBackspace)
	namedKey(m, tea.KeyBackspace)
	m.PullRequestPicker.Index = 0
	namedKey(m, tea.KeyEnter)
	if m.Session != first || m.top() != pageReview {
		t.Fatal("selecting an open review did not restore it and dismiss the switcher")
	}
}

func TestWorkspaceReviewsStartEmptyAndActivateLoadedReview(t *testing.T) {
	m := New(context.Background(), nil)
	if len(m.tabs) != 0 || m.activeTab != -1 {
		t.Fatalf("new model did not start with no review tabs: %#v, active=%d", m.tabs, m.activeTab)
	}

	s := largeSession(1, 1)
	s.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 17}
	m.Update(Loaded{Session: s})
	if len(m.tabs) != 1 || m.activeTab != 0 || m.tabs[0].identity != s.Inventory.Comparison.Metadata.Identity {
		t.Fatalf("loaded review was not activated: %#v, active=%d", m.tabs, m.activeTab)
	}
}

func TestReviewControlsExcludeLegacyPlanActions(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))

	for _, control := range []rune{'a', 'v', 'o'} {
		key(m, control)
		if m.top() != pageReview {
			t.Fatalf("legacy plan control %q opened page %v", control, m.top())
		}
	}
	if help := renderHealth(); strings.Contains(help, "accepted plan") || strings.Contains(help, "move selected unit") || strings.Contains(help, "reorder slices") {
		t.Fatalf("legacy plan controls remain in help:\n%s", help)
	}
}

func TestCommandSwitcherDoesNotTakeOverGuideTab(t *testing.T) {
	m := New(context.Background(), nil)
	first := largeSession(1, 1)
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	second := largeSession(1, 1)
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 2}
	m.openReviewTab(first)
	m.openReviewTab(second)
	m.activateTab(0)

	guided, _, _ := guidedSession(t, groupingAnalyzer{path: "a.go"})
	m.Session = guided
	m.Stack = []page{pageReview}
	rows := m.rows()
	if len(rows) == 0 {
		t.Fatal("fixture has no guide rows")
	}
	before := len(rows)
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.activeTab != 0 || len(m.rows()) >= before {
		t.Fatal("guide tab key was repurposed for workspace navigation")
	}
}

func TestWorkspaceTabsRestoreIndependentReviewState(t *testing.T) {
	m := New(context.Background(), nil)
	first := largeSession(2, 2)
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	second := largeSession(2, 2)
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 2}

	m.openReviewTab(first)
	firstErr := errors.New("first failed")
	m.Selected, m.Row, m.Files = 1, 3, true
	m.collapsed.guides[0] = true
	m.Scroll, m.GuideScroll = map[int]int{1: 9}, map[int]int{0: 4}
	m.Horizontal, m.Inventory, m.Focus = 12, true, paneDiff
	m.Stack = []page{pageReview, pageHelp}
	m.Err, m.Loading, m.Busy, m.ActionError, m.notice = firstErr, true, true, firstErr, "first notice"

	m.openReviewTab(second)
	secondErr := errors.New("second failed")
	m.Selected, m.Row, m.Files = 0, 1, false
	m.Scroll, m.GuideScroll = map[int]int{0: 2}, map[int]int{1: 7}
	m.Horizontal, m.Inventory, m.Focus = 3, false, paneList
	m.Stack = []page{pageReview, pageEvidence}
	m.Err, m.Loading, m.Busy, m.ActionError, m.notice = secondErr, false, false, secondErr, "second notice"

	m.activateTab(0)
	if m.Session != first || m.Selected != 1 || m.Row != 3 || !m.Files || !m.collapsed.guides[0] || m.Scroll[1] != 9 || m.GuideScroll[0] != 4 || m.Horizontal != 12 || !m.Inventory || m.Focus != paneDiff || m.Err != firstErr || !m.Loading || !m.Busy || m.ActionError != firstErr || m.notice != "first notice" {
		t.Fatalf("tab 2 did not restore first review state: %#v", m)
	}
	if len(m.Stack) != 2 || m.Stack[1] != pageHelp {
		t.Fatalf("first review stack = %#v, want help page", m.Stack)
	}

	m.activateTab(1)
	if m.Session != second || m.Selected != 0 || m.Row != 1 || m.Files || m.Scroll[0] != 2 || m.GuideScroll[1] != 7 || m.Horizontal != 3 || m.Inventory || m.Focus != paneList || m.Err != secondErr || m.Loading || m.Busy || m.ActionError != secondErr || m.notice != "second notice" {
		t.Fatalf("tab 3 did not restore second review state: %#v", m)
	}
	if len(m.Stack) != 2 || m.Stack[1] != pageEvidence {
		t.Fatalf("second review stack = %#v, want evidence page", m.Stack)
	}
}

func TestReviewHeaderExposesSwitcherAndFitsViewport(t *testing.T) {
	m := New(context.Background(), nil)
	s := largeSession(1, 1)
	s.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repository", Number: 42}
	m.openReviewTab(s)
	m.Update(tea.WindowSizeMsg{Width: 15, Height: 8})

	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if len(lines) == 0 || strings.Contains(lines[0], "PRs") {
		t.Fatalf("first view row retained a permanent PR strip: %q", lines[0])
	}
	for _, line := range lines {
		if visibleWidth(line) > 15 {
			t.Fatalf("tab workspace viewport exceeded width: %q", line)
		}
	}
}

func TestReviewSelectionUsesPersistentChevronOutsideFocusedPane(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.Width, m.Height, m.Focus = 120, 12, paneDiff

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "› main.go") {
		t.Fatalf("unfocused file selection lacks persistent chevron:\n%s", view)
	}
}

func TestReviewWorkspaceUsesCompactHealthStatusInsteadOfShortcutFooter(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.Session.ID = "fixture"
	m.Width, m.Height, m.Focus = 120, 12, paneDiff

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "0/2 read") || !strings.Contains(view, "Inventory complete") || !strings.Contains(view, "?: Health & help") {
		t.Fatalf("review lacks compact health status:\n%s", view)
	}
	if strings.Contains(view, "ctrl+h/ctrl+l: focus list/diff") {
		t.Fatalf("review retained the verbose shortcut footer:\n%s", view)
	}
}

func TestNarrowReviewHealthStatusKeepsProgressAndAttention(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.Session.ID = "fixture"
	m.Session.Inventory.Complete = false
	m.Width, m.Height, m.Focus = 60, 10, paneDiff

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "0/2 read") || !strings.Contains(view, "Inventory incomplete") || !strings.Contains(view, "?: Health & help") {
		t.Fatalf("narrow health status hid progress or the highest-severity signal:\n%s", view)
	}
}

func TestHealthHelpGroupsEveryBinding(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.Width, m.Height = 160, 60
	key(m, '?')

	view := ansi.Strip(m.View().Content)
	for _, heading := range []string{"Navigate", "Review", "Views", "Diagnostics", "App"} {
		if !strings.Contains(view, heading) {
			t.Fatalf("Health & help is missing %q:\n%s", heading, view)
		}
	}
	for _, b := range bindings {
		line := b.keys + ": " + b.desc
		if count := strings.Count(view, line); count != 1 {
			t.Fatalf("binding %q appears %d times in Health & help:\n%s", line, count, view)
		}
	}
}

func TestRawReviewMockedEndToEnd(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\x1b]52;c;attack\a\n"+strings.Repeat("long line\n", 50))
	r.Write("b", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 42}, BaseRepository: "owner/repo", HeadRepository: "fork/repo", BaseSHA: base, HeadSHA: head}
	m := New(context.Background(), func(c context.Context, n func(string)) (*review.Session, error) {
		return review.Open(c, r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), n)
	})
	if !strings.Contains(m.View().Content, "Loading") {
		t.Fatal("loading absent")
	}
	completeAction(t, m, m.Init())
	if m.Session == nil || m.Err != nil {
		t.Fatal("load failed", m.Err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	for i := range m.Session.Inventory.Units {
		if m.Selected != i {
			t.Fatal("unit inaccessible", i)
		}
		if !strings.Contains(m.View().Content, string(m.Session.Inventory.Units[i].Kind)) {
			t.Fatal("kind absent")
		}
		key(m, 'n')
	}
	key(m, 'n')
	key(m, 'j')
	selected, scroll := m.Selected, m.Scroll[m.Selected]
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	if m.Selected != selected || m.Scroll[m.Selected] != scroll {
		t.Fatal("resize lost reading position")
	}
	key(m, 'i')
	if !strings.Contains(m.View().Content, "INVENTORY") {
		t.Fatal("inventory unavailable")
	}
	key(m, 'i')
	key(m, '?')
	if !strings.Contains(m.View().Content, "Health & help") {
		t.Fatal("help unavailable")
	}
	// Pages leave only through esc; '?' no longer toggles help closed.
	namedKey(m, tea.KeyEscape)
	plain := Plain(m.Session)
	if strings.ContainsAny(plain, "\x1b\a") || !strings.Contains(plain, `\x1b]52;c;attack\a`) {
		t.Fatal("unsafe terminal content")
	}
	if !strings.Contains(plain, "inventory complete") || !strings.Contains(plain, "guides: unavailable (analysis not requested)") {
		t.Fatal("status missing")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if !strings.Contains(ansi.Strip(m.View().Content), "Guides unavailable") {
		t.Fatal("terminal status hides the analysis decision")
	}
	hunk := -1
	for i, u := range m.Session.Inventory.Units {
		if u.Kind == inventory.TextHunk {
			hunk = i
			break
		}
	}
	if hunk < 0 {
		t.Fatal("no text hunk generated")
	}
	m.Selected, m.Focus = hunk, paneDiff
	if !strings.Contains(m.View().Content, "\x1b[") {
		t.Fatal("interactive diff unstyled")
	}
	if stripped := ansi.Strip(m.View().Content); strings.Contains(stripped, "\x1b") {
		t.Fatal("styled view leaked raw escapes")
	}
	m.Focus = paneList
	for _, w := range []int{1, 20, 60, 99, 100, 120} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 10})
		for _, line := range strings.Split(m.View().Content, "\n") {
			if visibleWidth(line) > w {
				t.Fatalf("viewport exceeded %d: %q", w, line)
			}
		}
	}
}

func TestFullDiffDoesNotScrollPastViewport(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n"+strings.Repeat("long line\n", 50))
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 42}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
	m := New(context.Background(), func(c context.Context, n func(string)) (*review.Session, error) {
		return review.Open(c, r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), n)
	})
	completeAction(t, m, m.Init())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 100})
	for i := range m.Session.Inventory.Units {
		if strings.Contains(unitText(m.Session, i), "long line") {
			m.Selected = i
			break
		}
	}
	ctrlKey(m, 'l')
	if m.Focus != paneDiff {
		t.Fatal("diff did not receive focus")
	}

	lastLine := "long line"
	if m.Scroll[m.Selected] != 0 || !strings.Contains(m.View().Content, lastLine) {
		t.Fatal("full diff is not initially visible")
	}
	for _, k := range []rune{'j', 'J'} {
		key(m, k)
		if m.Scroll[m.Selected] != 0 || !strings.Contains(m.View().Content, lastLine) {
			t.Fatalf("%q scrolled past a fully visible diff", k)
		}
	}
	namedKey(m, tea.KeyDown)
	namedKey(m, tea.KeyPgDown)
	if m.Scroll[m.Selected] != 0 || !strings.Contains(m.View().Content, lastLine) {
		t.Fatal("downward paging scrolled past a fully visible diff")
	}
}

func TestDUPageFocusedDiff(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 10)
	m.Focus = paneDiff

	key(m, 'd')
	if got, want := m.Scroll[m.Selected], m.pageStep(); got != want {
		t.Fatalf("d page-down offset = %d, want %d", got, want)
	}
	key(m, 'u')
	if got := m.Scroll[m.Selected]; got != 0 {
		t.Fatalf("u did not restore the initial offset: %d", got)
	}
}

func TestHLFocusesReviewPanes(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.Focus = paneList

	key(m, 'l')
	if m.Focus != paneDiff {
		t.Fatal("l did not focus the diff pane")
	}
	key(m, 'h')
	if m.Focus != paneList {
		t.Fatal("h did not focus the list pane")
	}
}

func TestRawReviewCancelAndFailure(t *testing.T) {
	canceled := make(chan struct{})
	m := New(context.Background(), func(c context.Context, _ func(string)) (*review.Session, error) {
		<-c.Done()
		close(canceled)
		return nil, c.Err()
	})
	cmd := m.Init()
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	key(m, 'q')
	<-canceled
	m.Update(<-done)
	if !errors.Is(m.Err, context.Canceled) {
		t.Fatal("loading cancellation lost", m.Err)
	}
	failed := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) {
		return nil, errors.New("synthetic failure")
	})
	failed.Update(failed.Init()())
	if !strings.Contains(failed.View().Content, "synthetic failure") || strings.Contains(failed.View().Content, "inventory complete") {
		t.Fatal("failure masquerades as empty")
	}
}

func TestModalPagesOwnInputAndBack(t *testing.T) {
	m := New(context.Background(), nil)
	m.Session = &review.Session{}
	m.Loading = false
	m.Selected = 3

	key(m, '?')
	if m.top() != pageHelp {
		t.Fatal("help did not open")
	}
	key(m, 'n')
	if m.Selected != 3 {
		t.Fatal("help leaked review input")
	}
	key(m, 'g')
	if m.top() != pageHelp {
		t.Fatal("help accepted another page opener")
	}
	key(m, 'q')

	namedKey(m, tea.KeyEscape)
	if m.top() != pageReview {
		t.Fatal("esc did not leave help")
	}
	key(m, 'U')
	if m.top() != pageURL || !strings.Contains(m.View().Content, "esc: back") {
		t.Fatal("URL page or back hint missing")
	}
	namedKey(m, tea.KeyEscape)
	if m.top() != pageReview {
		t.Fatal("esc did not leave URL page")
	}

	m.Focus = paneDiff
	namedKey(m, tea.KeyEscape)
	if m.Focus != paneList || m.top() != pageReview {
		t.Fatal("esc did not return to list focus")
	}
}

func TestBindingsRenderHelpAndFooter(t *testing.T) {
	help := renderBindings(groupHelp)
	footer := renderBindings(groupFooter)
	for _, wording := range []string{"focus the diff; on a file, jump to its place in the guide diff", "reset selected guide scroll, or selected unit's without guides"} {
		if !strings.Contains(help, wording) {
			t.Fatalf("updated help wording missing %q", wording)
		}
	}
	for _, key := range []string{"ctrl+p", "j/k", "J/K", "h/l, ctrl+h/ctrl+l", "esc", "q/ctrl+c"} {
		if !strings.Contains(help, key) {
			t.Fatalf("binding %q missing from help", key)
		}
	}
	for _, key := range []string{"ctrl+p", "h/l, ctrl+h/ctrl+l", "esc", "q/ctrl+c", "m", "N"} {
		if !strings.Contains(footer, key) {
			t.Fatalf("binding %q missing from footer", key)
		}
	}
}

func TestRawReviewIncomplete(t *testing.T) {
	r := testutil.NewRepo(t)
	base := r.Commit()
	r.Write("large", "too much text\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	l := source.Defaults()
	l.BlobBytes = 1
	s, e := review.Open(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), l, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(Plain(s), "INCOMPLETE") || !strings.Contains(Plain(s), "per-blob limit") || strings.Contains(Plain(s), "inventory complete") {
		t.Fatal("incomplete review hidden")
	}
}

func TestReviewStyledUnitKindsKeepWording(t *testing.T) {
	s := kindsSession()
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return s, nil })
	m.Update(m.Init()())
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	for i, u := range s.Inventory.Units {
		m.Selected, m.Focus = i, paneDiff
		content := m.View().Content
		if !strings.Contains(content, "\x1b[") {
			t.Fatalf("%s rendered without a style", u.Kind)
		}
		stripped := ansi.Strip(content)
		if strings.Contains(stripped, "\x1b") {
			t.Fatalf("%s leaked raw escapes", u.Kind)
		}
		for _, line := range strings.Split(strings.TrimSuffix(unitText(s, i), "\n"), "\n") {
			if line != "" && !strings.Contains(stripped, line) {
				t.Fatalf("%s reworded line %q", u.Kind, line)
			}
		}
	}
}

func TestStrippedViewMatchesUnstyledRender(t *testing.T) {
	s := kindsSession()
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return s, nil })
	m.Update(m.Init()())
	styled := palette
	defer func() { palette = styled }()
	for _, w := range []int{1, 20, 60, 99, 100, 120} {
		for i := range s.Inventory.Units {
			for _, focus := range []pane{paneList, paneDiff} {
				m.Selected, m.Focus = i, focus
				m.Update(tea.WindowSizeMsg{Width: w, Height: 12})
				colored := m.View().Content
				palette = map[lineClass]lipgloss.Style{}
				plain := m.View().Content
				palette = styled
				if ansi.Strip(colored) != plain {
					t.Fatalf("width %d unit %d focus %v changed content:\n%q\n%q", w, i, focus, ansi.Strip(colored), plain)
				}
				for _, line := range strings.Split(colored, "\n") {
					if visibleWidth(line) > w {
						t.Fatalf("styled viewport exceeded %d: %q", w, line)
					}
				}
			}
		}
	}
}

func TestRawReviewReportsGuideStatusWithoutLegacyPlanStatus(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 7}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	s, e := review.Open(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if s.Guides == nil || s.Guides.Status != guide.Unavailable {
		t.Fatal("session carries no analysis decision")
	}
	s.Guides = nil
	// A session stored before guides existed reports no guide decision and no
	// obsolete provider-plan status.
	if pre := Plain(s); strings.Contains(pre, "analysis:") || strings.Contains(pre, "guides:") {
		t.Fatal("pre-guide session reports obsolete analysis state")
	}
	hostile := guide.Bundle{Status: guide.Generated, Provider: "openai", Model: "m\x1b]52;c;attack\a", Items: []guide.Item{{Title: "Authentication flow"}}}
	s.Guides = &hostile
	plain := Plain(s)
	if !strings.Contains(plain, "guides: generated (openai/") || !strings.Contains(plain, "1 guides") {
		t.Fatal("generated status missing")
	}
	if strings.ContainsAny(plain, "\x1b\a") {
		t.Fatal("unescaped model-authored status")
	}
}

// groupingAnalyzer groups one path's units and leaves the rest, so plain output
// is tested against both the model-authored and the synthesized guides.
type groupingAnalyzer struct{ path string }

func (a groupingAnalyzer) Analyze(_ context.Context, in guide.Input) (guide.Bundle, error) {
	section := guide.Section{Title: "Add greeting\x1b]52;c;section\a", Description: "Section description."}
	for _, u := range in.Units {
		if string(u.Path) == a.path {
			section.UnitIDs = append(section.UnitIDs, u.ID)
		}
	}
	return guide.Bundle{Status: guide.Generated, Provider: "openai", Model: "test-model", Items: []guide.Item{
		{Title: "Greeting flow\x1b]52;c;title\a", Description: "Adds a greeting.\x1b]52;c;desc\a", Sections: []guide.Section{section}},
	}}, nil
}

func TestPlainGuides(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a.go", "old\n")
	base := r.Commit()
	r.Write("a.go", "new\n")
	r.Write("README.md", "docs\n")
	r.Write(".env", "TOKEN=abcdef\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 42}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	s, e := review.OpenWithConfig(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), nil, review.Config{Analyzer: groupingAnalyzer{path: "a.go"}})
	if e != nil {
		t.Fatal(e)
	}
	plain := Plain(s)
	if strings.ContainsAny(plain, "\x1b\a") || !strings.Contains(plain, `\x1b]52;c;title\a`) || !strings.Contains(plain, `\x1b]52;c;section\a`) {
		t.Fatal("model-authored guide text is not escaped like patch content")
	}
	order := []string{
		"Guides interpret the diff; file slices remain the unit of reading progress.",
		`1. Greeting flow\x1b]52;c;title\a`,
		"   1.1 Add greeting",
		"       a.go [",
		"2. Ungrouped changes (not grouped by analysis)",
		"Analysis scope: ",
		"Evidence: ",
	}
	at := -1
	for _, want := range order {
		i := strings.Index(plain, want)
		if i <= at {
			t.Fatalf("plain guide ordering lost %q (%d after %d)", want, i, at)
		}
		at = i
	}
	if strings.Index(plain, "Guides interpret") > strings.Index(plain, "[file_metadata]") {
		t.Fatal("guides render after the raw units they interpret")
	}
	ungroupedStart := strings.Index(plain, "2. Ungrouped changes")
	if ungroupedStart < 0 {
		t.Fatal("ungrouped guide is missing")
	}
	ungrouped := plain[ungroupedStart:]
	analysisScopeStart := strings.Index(ungrouped, "Analysis scope")
	if analysisScopeStart < 0 {
		t.Fatal("analysis scope is missing")
	}
	for _, want := range []string{"README.md [", ".env ["} {
		if !strings.Contains(ungrouped[:analysisScopeStart], want) {
			t.Fatalf("ungrouped guide hides %q", want)
		}
	}
	total := len(s.Inventory.Units)
	if !strings.Contains(plain, fmt.Sprintf("Analysis scope: %d/%d units sent", total-len(s.Guides.WithheldPaths), total)) {
		t.Fatal("analysis scope not stated", plain)
	}
	if !strings.Contains(plain, ".env (credential-like filename)") {
		t.Fatal("withheld input not disclosed")
	}
	if strings.Count(plain, ".env (credential-like filename)") != 1 {
		t.Fatal("withheld disclosure repeats one path per unit")
	}
	if !strings.Contains(plain, "guides: generated (openai/test-model, 2 guides)") {
		t.Fatal("status does not report the generated bundle")
	}
	s.Guides = nil
	if strings.Contains(Plain(s), "Guides interpret") {
		t.Fatal("a session without analysis renders a guide block")
	}
}

// TestRawReviewGuideHierarchy keeps the escaping, clipping, and narrow-width
// contracts true once model-authored titles reach the left pane.
func TestRawReviewGuideHierarchy(t *testing.T) {
	s, _, _ := guidedSession(t, groupingAnalyzer{path: "a.go"})
	m := loaded(t, s, 120, 40)
	// Styling wraps whole lines, so the escaping contract is checked on the
	// stripped render: the title must already be escaped underneath the style.
	content := ansi.Strip(m.View().Content)
	// The left pane is narrow, so a long hostile title is clipped; what must
	// hold is that it is escaped first and clipped second.
	if strings.ContainsAny(content, "\x1b\a") || !strings.Contains(content, `1. Greeting flow\x1b]52`) {
		t.Fatal("guide titles are not escaped like patch content", content)
	}
	if !strings.Contains(content, "Guides") || !strings.Contains(content, `1.1 Add greeting\x1b]52`) {
		t.Fatal("guide hierarchy missing from the left pane", content)
	}
	for _, w := range []int{1, 20, 60, 99, 100, 120} {
		for _, h := range []int{5, 10, 24} {
			m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) > h {
				t.Fatalf("viewport height exceeded %d: %d lines", h, len(lines))
			}
			for _, line := range lines {
				if visibleWidth(line) > w {
					t.Fatalf("viewport exceeded %d: %q", w, line)
				}
				// Styles are the only escapes allowed; the text under them must
				// still be free of control bytes from the model or the patch.
				if strings.ContainsAny(ansi.Strip(line), "\x1b\a") {
					t.Fatalf("unescaped control byte at width %d", w)
				}
			}
		}
	}
	// Below 100 columns one pane is visible at a time; tab on a file portion
	// hands it to the diff, while tab on a guide or section expands it. esc is
	// the way back, exactly as it is for the file plan.
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	for m.rows()[m.Row].kind != portionRow {
		key(m, 'n')
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focus != paneDiff || !strings.Contains(ansi.Strip(m.View().Content), "focus: diff") {
		t.Fatal("narrow tab did not switch to the diff pane")
	}
	namedKey(m, tea.KeyEscape)
	if m.Focus != paneList {
		t.Fatal("esc did not switch back to the hierarchy")
	}
	for m.Row > 0 {
		key(m, 'p')
	}
	before := len(m.rows())
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focus != paneList || len(m.rows()) >= before {
		t.Fatal("tab on a guide row did not collapse it")
	}
}
