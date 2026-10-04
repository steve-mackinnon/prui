package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
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
	m.selectReviewView(viewFiles)
	m.openReviewTab(second)
	m.selectReviewView(viewFiles)

	key(m, 'P')
	if m.top() != pagePullRequestPicker {
		t.Fatalf("P did not open the PR switcher: %#v", m.Stack)
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
	m.selectReviewView(viewFiles)
	m.openReviewTab(second)
	m.selectReviewView(viewFiles)
	m.PullRequests = []source.PullRequest{
		{Identity: first.Inventory.Comparison.Metadata.Identity, Title: "duplicate"},
		{Identity: source.Identity{Repository: "owner/repo", Number: 3}, Title: "database migration"},
	}

	key(m, 'P')
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
	m.selectReviewView(viewFiles)

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
	m.selectReviewView(viewFiles)
	m.openReviewTab(second)
	m.selectReviewView(viewFiles)
	m.activateTab(0)

	guided, _, _ := guidedSession(t, groupingAnalyzer{path: "a.go"})
	m.Session = guided
	m.Stack = []page{pageReview}
	m.Files = false
	m.begin()
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
	m.selectReviewView(viewFiles)
	firstErr := errors.New("first failed")
	m.Selected, m.Row, m.Files = 1, 3, true
	m.collapsed.guides[0] = true
	m.Scroll, m.GuideScroll = map[int]int{1: 9}, map[int]int{0: 4}
	m.Horizontal, m.Inventory, m.Focus = 12, true, paneDiff
	m.Stack = []page{pageReview, pageHelp}
	m.Err, m.Loading, m.Busy, m.ActionError, m.notice = firstErr, true, true, firstErr, "first notice"

	m.openReviewTab(second)
	m.selectReviewView(viewFiles)
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
	m.selectReviewView(viewFiles)
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

func TestFilesViewAndGuideTabNeedsOptIn(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(screenSession())
	m.selectReviewView(viewFiles)
	if !m.Files || !strings.Contains(ansi.Strip(m.View().Content), "› Files [2]") {
		t.Fatal("review did not open on Files")
	}
	key(m, 'G')
	if m.Files || !strings.Contains(ansi.Strip(m.View().Content), "No guide yet. Press g") {
		t.Fatal("empty Guide tab did not offer explicit generation")
	}
	key(m, 'F')
	if !m.Files || !strings.Contains(ansi.Strip(m.View().Content), "main.go") {
		t.Fatal("F did not restore the changed-file picker")
	}
}

func TestBackgroundComparisonRefreshPreservesReadingAndTabOwnership(t *testing.T) {
	m := New(context.Background(), nil)
	old := largeSession(2, 2)
	old.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	old.Inventory.Comparison.Metadata.BaseSHA = strings.Repeat("a", 40)
	old.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("b", 40)
	old.ReviewedSliceIDs = []string{"file-1"}
	old.ID = "cached"
	m.openReviewTab(old)
	m.selectReviewView(viewFiles)
	m.Selected = 1
	fresh := *old
	fresh.RevisionStatus = session.Current
	m.Update(PullRequestRefreshResult{Target: 0, SessionID: old.ID, Freshness: PullRequestFreshness{Status: fresh.RevisionStatus}})
	if m.Session != old || m.Selected != 1 || len(m.Session.ReviewedSliceIDs) != 1 || m.Session.RevisionStatus != session.Current {
		t.Fatal("same-revision refresh lost selection or progress")
	}
	other := largeSession(1, 1)
	other.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 2}
	m.openReviewTab(other)
	m.selectReviewView(viewFiles)
	changed := largeSession(2, 2)
	changed.Inventory.Comparison.Metadata = old.Inventory.Comparison.Metadata
	changed.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("c", 40)
	changed.Inventory.Comparison.InventoryID = "changed"
	m.Update(PullRequestRefreshResult{Target: 0, SessionID: old.ID, Freshness: PullRequestFreshness{Session: changed}})
	if m.Session != other || m.tabs[0].review.Session != changed || m.tabs[0].review.Selected != 0 {
		t.Fatal("late refresh changed the active review or kept a stale selection")
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

func TestWideReviewSeparatesPanesAndIdentifiesFocus(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Loading = false
	m.Session = screenSession()
	m.Width, m.Height = 120, 12
	var listHeader string
	for _, tc := range []struct {
		focus pane
	}{{paneList}, {paneDiff}} {
		m.Focus = tc.focus
		rendered := m.View().Content
		if tc.focus == paneList {
			listHeader = strings.Split(rendered, "\n")[2]
		} else if listHeader == strings.Split(rendered, "\n")[2] {
			t.Fatal("pane focus did not change header styling")
		}
		view := ansi.Strip(rendered)
		lines := strings.Split(view, "\n")
		if !strings.HasPrefix(lines[2], "┌ Files ") || !strings.Contains(lines[2], "┬ Diff · FILES") {
			t.Fatalf("missing pane headers:\n%s", view)
		}
		if !strings.HasPrefix(lines[3], "│› main.go") || strings.Count(lines[3], "│") != 3 || !strings.HasPrefix(lines[3+m.bodyHeight()], "└") {
			t.Fatalf("wide review should frame both panes:\n%s", view)
		}
	}
}

func TestSelectedFilePathScrollsAcrossFullName(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Loading = false
	m.Session = screenSession()
	path := "internal/very/long/path/to/the/selected/file/overflow_test.go"
	m.Session.Inventory.Files[0].NewPath = []byte(path)
	m.Width, m.Height, m.Focus = 120, 12, paneList
	if !m.guidePathScrollEligible() {
		t.Fatal("selected overflowing file path is not eligible to scroll")
	}
	first := ansi.Strip(m.View().Content)
	_, cmd := m.Update(guidePathTick{generation: m.guidePathGeneration})
	if cmd == nil {
		t.Fatal("file path carousel stopped before showing the full path")
	}
	if m.guidePathOffset != 1 || ansi.Strip(m.View().Content) == first {
		t.Fatal("selected file path did not advance")
	}
	_, _, pathWidth, _ := m.guidePathScrollTarget()
	for m.guidePathOffset < visibleWidth(fileDirectory(m.Session.Inventory.Files[0]))-pathWidth {
		m.Update(guidePathTick{generation: m.guidePathGeneration})
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "overflow_test.go") {
		t.Fatal("carousel did not reveal the end of the file name")
	}
}

func TestReviewWorkspaceUsesCompactHealthStatusInsteadOfShortcutFooter(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.Session.ID = "fixture"
	m.Width, m.Height, m.Focus = 120, 12, paneDiff

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "0/2 read") || !strings.Contains(view, "Freshness unknown") || !strings.Contains(view, "?: Health & help") {
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
	if !strings.Contains(view, "0/2 read") || !strings.Contains(view, "R Review (0)") || !strings.Contains(view, "Inventory incomplete") || !strings.Contains(view, "?: Help") {
		t.Fatalf("narrow health status hid progress or the highest-severity signal:\n%s", view)
	}
}

func TestHealthHelpGroupsEveryBinding(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.Width, m.Height = 160, 90
	key(m, '?')

	view := ansi.Strip(m.healthHelpView())
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
	m.selectReviewView(viewFiles)
	if m.Session == nil || m.Err != nil {
		t.Fatal("load failed", m.Err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	key(m, 'i') // Raw-unit traversal belongs to full inventory, not Files.
	for i := range m.Session.Inventory.Units {
		if m.Selected != i {
			t.Fatal("unit inaccessible", i)
		}
		if !strings.Contains(m.View().Content, string(m.Session.Inventory.Units[i].Kind)) {
			t.Fatal("kind absent")
		}
		key(m, 'n')
	}
	key(m, 'i') // Return to the continuous Files workspace.
	key(m, 'n')
	key(m, 'j')
	selected, scroll := m.Selected, m.Scroll[m.Selected]
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	if m.Selected != selected || m.Scroll[m.Selected] != scroll {
		t.Fatal("resize lost reading position")
	}
	key(m, 'i')
	if !strings.Contains(m.View().Content, "Full inventory (i)") {
		t.Fatalf("inventory unavailable: %q", ansi.Strip(m.View().Content))
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
	key(m, '?')
	if !strings.Contains(ansi.Strip(m.View().Content), "Guide available with g") {
		t.Fatal("health help hides the analysis decision")
	}
	namedKey(m, tea.KeyEscape)
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
	m.selectReviewView(viewFiles)
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
	if got, want := m.offset(), m.pageStep(); got != want {
		t.Fatalf("d page-down offset = %d, want %d", got, want)
	}
	key(m, 'u')
	if got := m.offset(); got != 0 {
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

func TestDiffCursorMovesBetweenCommentTargetsAndKeepsThemVisible(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.Selected, m.Focus = 1, paneDiff
	m.Width, m.Height = 120, 7 // two framed detail rows: force cursor-following scroll.
	m.cursorActive = true
	m.ensureCursorVisible()

	first := m.cursor()
	if got := m.detail()[first].target; got == nil || got.Side != "RIGHT" || got.Line != 1 {
		t.Fatalf("initial cursor target = %#v, want first commentable context line", got)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "›  context") {
		t.Fatalf("initial comment target is not visibly marked:\n%s", view)
	}
	if !strings.Contains(view, "│  ── text") {
		t.Fatalf("unselected detail line does not retain the cursor gutter:\n%s", view)
	}

	key(m, 'n')
	second := m.cursor()
	if second <= first || m.detail()[second].target == nil || m.detail()[second].target.Side != "LEFT" {
		t.Fatalf("n cursor = %d (%#v), want deletion target after %d", second, m.detail()[second].target, first)
	}
	if second < m.offset() || second >= m.offset()+m.bodyHeight() {
		t.Fatalf("cursor %d fell outside visible detail [%d,%d)", second, m.offset(), m.offset()+m.bodyHeight())
	}

	key(m, 'p')
	if got := m.cursor(); got != first {
		t.Fatalf("p cursor = %d, want %d", got, first)
	}
}

func TestZZCentersFocusedDiffCursor(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 11)
	m.Focus = paneDiff
	m.cursorActive = true
	for i, line := range m.displayDetail() {
		if i >= 25 && line.target != nil {
			m.setCursor(i)
			break
		}
	}
	cursor, selected := m.cursor(), m.Selected
	if cursor < 25 {
		t.Fatalf("fixture has no distant comment target: %d", cursor)
	}
	m.setOffset(cursor - 1)
	key(m, 'z')
	if got := m.offset(); got != cursor-1 {
		t.Fatalf("first z moved offset to %d", got)
	}
	key(m, 'z')
	if want := m.clampOffset(cursor - m.bodyHeight()/2); m.offset() != want {
		t.Fatalf("zz offset = %d, want %d", m.offset(), want)
	}
	if m.cursor() != cursor || m.Selected != selected {
		t.Fatalf("zz changed cursor or unit: cursor=%d selected=%d", m.cursor(), m.Selected)
	}

	for i := len(m.displayDetail()) - 1; i >= 0; i-- {
		line := m.displayDetail()[i]
		if line.target != nil || line.commentID > 0 {
			m.setCursor(i)
			break
		}
	}
	m.setOffset(0)
	m.cursorActive = false
	key(m, 'z')
	key(m, 'z')
	if want := m.clampOffset(m.cursor() - m.bodyHeight()/2); m.offset() != want {
		t.Fatalf("zz near end offset = %d, want clamped %d", m.offset(), want)
	}
	if !m.cursorActive {
		t.Fatal("zz did not reveal the selected line marker")
	}
}

func TestZZRequiresConsecutiveKeysInFocusedChangesDiff(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 11)
	m.Focus = paneDiff
	m.cursorActive = true
	for i, line := range m.displayDetail() {
		if i >= 25 && line.target != nil {
			m.setCursor(i)
			break
		}
	}
	m.setOffset(m.cursor() - 1)
	initial := m.offset()
	key(m, 'z')
	namedKey(m, tea.KeyRight) // Interrupt without moving the file or vertical cursor.
	key(m, 'z')
	if got := m.offset(); got != initial {
		t.Fatalf("interrupted z sequence changed offset to %d", got)
	}
	m.Focus = paneList
	key(m, 'z')
	m.Focus = paneDiff
	key(m, 'z')
	if got := m.offset(); got != initial {
		t.Fatalf("list z leaked into diff sequence: offset %d", got)
	}
}

func TestDiffCursorTracksScrollingAndTabStateWithoutChangingReviewSelection(t *testing.T) {
	m := New(context.Background(), nil)
	first, second := kindsSession(), kindsSession()
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 2}
	m.openReviewTab(first)
	m.selectReviewView(viewFiles)
	m.Width, m.Height, m.Selected, m.Focus, m.Horizontal = 120, 5, 1, paneDiff, 8
	key(m, 'n')
	key(m, 'J')
	wantCursor := m.cursor()
	if m.Horizontal != 8 {
		t.Fatalf("diff scrolling changed horizontal position: %d", m.Horizontal)
	}

	m.openReviewTab(second)
	m.selectReviewView(viewFiles)
	m.Selected, m.Focus = 1, paneDiff
	key(m, 'n')
	secondCursor := m.cursor()
	m.activateTab(0)
	if got := m.cursor(); got != wantCursor {
		t.Fatalf("first tab cursor was not restored: got %d, want %d", got, wantCursor)
	}
	m.activateTab(1)
	if got := m.cursor(); got != secondCursor {
		t.Fatalf("second tab cursor = %d, want %d", got, secondCursor)
	}
}

func TestInactiveTabRestoresCursorTargetAfterWorkspaceResize(t *testing.T) {
	m := New(context.Background(), nil)
	first, second := kindsSession(), kindsSession()
	first.Inventory.Comparison.Metadata.Identity.Number = 1
	second.Inventory.Comparison.Metadata.Identity.Number = 2
	m.Width, m.Height = 160, 30
	m.openReviewTab(first)
	m.selectReviewView(viewFiles)
	m.Selected, m.Focus = 1, paneDiff
	var target *source.ReviewCommentTarget
	for i, line := range m.displayDetail() {
		if line.target != nil {
			m.setCursor(i)
			target, _ = m.cursorAnchor()
			break
		}
	}
	if target == nil {
		t.Fatal("fixture has no commentable cursor target")
	}
	m.openReviewTab(second)
	m.selectReviewView(viewFiles)
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 18})
	m.activateTab(0)
	got, _ := m.cursorAnchor()
	if got == nil || *got != *target {
		t.Fatalf("resized inactive tab cursor = %#v, want %#v", got, target)
	}
}

func TestSideBySideTabPreferenceIsIsolatedAcrossOpenReviews(t *testing.T) {
	m := New(context.Background(), nil)
	first, second := kindsSession(), kindsSession()
	first.Inventory.Comparison.Metadata.Identity.Number = 1
	second.Inventory.Comparison.Metadata.Identity.Number = 2
	m.openReviewTab(first)
	m.selectReviewView(viewFiles)
	m.Width, m.Height = sideBySideMinimumWidth, 12

	key(m, 'S')
	if m.diffLayout() != diffLayoutSideBySide {
		t.Fatal("S did not enable the first tab's side-by-side preference")
	}
	m.openReviewTab(second)
	m.selectReviewView(viewFiles)
	if m.diffLayout() != diffLayoutUnified {
		t.Fatal("first tab's side-by-side preference leaked to newly opened tab")
	}
	key(m, 'S')
	m.activateTab(0)
	if m.diffLayout() != diffLayoutSideBySide {
		t.Fatal("first tab did not restore its side-by-side preference")
	}
	m.activateTab(1)
	if m.diffLayout() != diffLayoutSideBySide {
		t.Fatal("second tab did not retain its own side-by-side preference")
	}
}

func TestDiffLayoutDefaultsToUnifiedAndSTogglesOnlyChanges(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.openReviewTab(kindsSession())
	m.selectReviewView(viewFiles)
	m.Width, m.Height = sideBySideMinimumWidth, 12

	if got := m.diffLayout(); got != diffLayoutUnified {
		t.Fatalf("initial diff layout = %v, want unified", got)
	}
	key(m, 'S')
	if got := m.diffLayout(); got != diffLayoutSideBySide {
		t.Fatalf("S in Changes layout = %v, want side-by-side", got)
	}
	m.selectReviewView(viewDescription)
	key(m, 'S')
	if got := m.diffLayout(); got != diffLayoutSideBySide {
		t.Fatalf("S outside Changes changed layout to %v", got)
	}
}

func TestDiffLayoutRestoresPreferredSideBySideAtMinimumWidth(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.openReviewTab(kindsSession())
	m.selectReviewView(viewFiles)
	m.Width, m.Height = sideBySideMinimumWidth, 12
	key(m, 'S')

	m.Update(tea.WindowSizeMsg{Width: sideBySideMinimumWidth - 1, Height: 12})
	if got := m.diffLayout(); got != diffLayoutSideBySide {
		t.Fatalf("narrow width reset preference to %v", got)
	}
	if m.sideBySideEnabled() {
		t.Fatal("side-by-side enabled below its minimum width")
	}
	m.Update(tea.WindowSizeMsg{Width: sideBySideMinimumWidth, Height: 12})
	if !m.sideBySideEnabled() {
		t.Fatal("side-by-side did not resume at its minimum width")
	}
}

func TestSideBySideResizePreservesPreferenceScrollCursorAndDraft(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.openReviewTab(kindsSession())
	m.selectReviewView(viewFiles)
	m.Selected, m.Focus = 1, paneDiff
	m.Update(tea.WindowSizeMsg{Width: sideBySideMinimumWidth, Height: 5})
	key(m, 'S')
	m.cursorActive = true
	key(m, 'j')
	wantTarget := *m.displayDetail()[m.cursor()].target
	m.setOffset(1)
	key(m, ']')
	if got := m.displayDetail()[m.cursor()].target; got == nil || *got != wantTarget {
		t.Fatalf("pane width change cursor target = %#v, want %#v", got, wantTarget)
	}
	m.Composer = &commentComposer{Target: wantTarget, Draft: "keep this draft", Cursor: 4}

	m.Update(tea.WindowSizeMsg{Width: sideBySideMinimumWidth - 1, Height: 5})
	if m.diffLayout() != diffLayoutSideBySide || m.offset() != 1 {
		t.Fatalf("narrow fallback reset preference or scroll: layout=%v offset=%d", m.diffLayout(), m.offset())
	}
	if m.Composer == nil || m.Composer.Draft != "keep this draft" {
		t.Fatalf("narrow fallback lost draft: %#v", m.Composer)
	}
	if got := m.displayDetail()[m.cursor()].target; got == nil || *got != wantTarget {
		t.Fatalf("narrow fallback cursor target = %#v, want %#v", got, wantTarget)
	}

	m.Update(tea.WindowSizeMsg{Width: sideBySideMinimumWidth, Height: 5})
	if m.diffLayout() != diffLayoutSideBySide || m.offset() != 1 {
		t.Fatalf("wide resize reset preference or scroll: layout=%v offset=%d", m.diffLayout(), m.offset())
	}
	if got := m.displayDetail()[m.cursor()].target; got == nil || *got != wantTarget {
		t.Fatalf("wide resize cursor target = %#v, want %#v", got, wantTarget)
	}
}

func TestSideBySideCursorPrefersRightTargetOnPairedRows(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.Selected, m.Focus = 1, paneDiff
	m.Width, m.Height, m.layout = sideBySideMinimumWidth, 12, diffLayoutSideBySide

	for i, line := range m.displayDetail() {
		if line.target != nil && line.target.Line == 2 {
			m.setCursor(i)
			break
		}
	}
	if got := m.displayDetail()[m.cursor()].target; got == nil || got.Side != "RIGHT" || got.Line != 2 {
		t.Fatalf("paired row cursor target = %#v, want RIGHT line 2", got)
	}
	namedKey(m, tea.KeyEnter)
	if m.Composer == nil || m.Composer.Target.Side != "RIGHT" || m.Composer.Target.Line != 2 {
		t.Fatalf("Enter target = %#v, want RIGHT line 2", m.Composer)
	}
}

func TestDiffEnterOpensACommentComposerOnlyForTheCursorTarget(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.Selected, m.Focus = 1, paneList

	namedKey(m, tea.KeyEnter)
	if m.Focus != paneDiff || m.top() != pageReview {
		t.Fatalf("list enter did not retain its focus-diff meaning: focus=%v page=%v", m.Focus, m.top())
	}
	namedKey(m, tea.KeyEnter)
	if m.top() != pageReview || m.Composer == nil || m.Composer.Target.Line != 1 || m.Composer.Target.Side != "RIGHT" {
		t.Fatalf("diff enter did not open composer for cursor target: page=%v composer=%#v", m.top(), m.Composer)
	}

	m = New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.Selected, m.Focus = 0, paneDiff // Full inventory isolates the metadata unit.
	m.Inventory = true
	namedKey(m, tea.KeyEnter)
	if m.top() != pageReview || m.Composer != nil {
		t.Fatalf("non-commentable detail opened composer: page=%v composer=%#v", m.top(), m.Composer)
	}
}

func TestDiffEnterRejectsCommentTargetsWithUnsafePaths(t *testing.T) {
	for name, path := range map[string][]byte{
		"empty":        {},
		"invalid utf8": {0xff},
	} {
		t.Run(name, func(t *testing.T) {
			m := New(context.Background(), nil)
			m.Loading = false
			m.Session = kindsSession()
			m.Session.Inventory.Files[0].NewPath = path
			m.Selected, m.Focus = 1, paneDiff

			namedKey(m, tea.KeyEnter)
			if m.top() != pageReview || m.Composer != nil {
				t.Fatalf("unsafe target path opened composer: page=%v composer=%#v", m.top(), m.Composer)
			}
		})
	}
}

func TestCommentComposerSubmitsOnEnterAndShiftEnterAddsNewline(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	submissions := 0
	m.SetCommentSubmitter(func(context.Context, CommentSubmission) (source.ReviewComment, error) {
		submissions++
		return source.ReviewComment{}, nil
	})
	namedKey(m, tea.KeyEnter)
	if m.Composer == nil {
		t.Fatal("composer did not open")
	}
	key(m, 'a')
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	key(m, 'b')
	if got, want := m.Composer.Draft, "a\nb"; got != want {
		t.Fatalf("composer draft = %q, want %q", got, want)
	}
	_, submit := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, submit)
	if submissions != 1 || m.Composer != nil {
		t.Fatalf("enter did not submit: submissions=%d composer=%#v", submissions, m.Composer)
	}
	// Reopen to prove Escape still discards a draft without a write.
	namedKey(m, tea.KeyEnter)
	key(m, 'x')
	namedKey(m, tea.KeyEscape)
	if submissions != 1 || m.Composer != nil || m.top() != pageReview {
		t.Fatalf("escape did not discard composer: page=%v composer=%#v", m.top(), m.Composer)
	}
}

func TestInlineCommentEditorDeletesRunesOnBothSidesOfCursor(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	namedKey(m, tea.KeyEnter)
	for _, r := range "a界🙂b" {
		key(m, r)
	}
	// a界🙂b; remove the preceding emoji, then the following b.
	namedKey(m, tea.KeyLeft)
	namedKey(m, tea.KeyBackspace)
	namedKey(m, tea.KeyDelete)
	if got, want := m.Composer.Draft, "a界"; got != want || m.Composer.Cursor != 2 {
		t.Fatalf("draft/cursor = %q/%d, want %q/2", got, m.Composer.Cursor, want)
	}
}

func TestInlineCommentEditorBlinkTickTogglesCaretOnlyForActiveEditor(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	namedKey(m, tea.KeyEnter)
	if !m.editorCursorVisible || m.editorCursorGeneration == 0 {
		t.Fatalf("editor cursor was not initialized: visible=%v generation=%d", m.editorCursorVisible, m.editorCursorGeneration)
	}
	generation := m.editorCursorGeneration
	m.Update(editorCursorTick{generation: generation})
	if m.editorCursorVisible {
		t.Fatal("blink tick did not hide caret")
	}
	m.Update(editorCursorTick{generation: generation - 1})
	if m.editorCursorVisible {
		t.Fatal("stale blink tick changed caret")
	}
}

func TestInlineCommentsRenderOnlyAtExactFrozenTargets(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	var target source.ReviewCommentTarget
	for _, line := range m.baseDetail() {
		if line.target != nil {
			target = *line.target
			break
		}
	}
	if target.Path == "" {
		t.Fatal("fixture has no comment target")
	}
	m.Comments = []source.ReviewComment{
		{ID: 1, Author: "reviewer\x1b[31m", Target: target, Body: "exact\x1b[2J"},
		{ID: 2, Author: "stale", Target: source.ReviewCommentTarget{Identity: target.Identity, CommitID: "0000000000000000000000000000000000000000", Path: target.Path, Side: target.Side, Line: target.Line}, Body: "wrong"},
	}
	boxed := m.reviewCommentLines(m.Comments[0])
	if got := boxed[1]; got.Class != classMetadata || !strings.Contains(got.Text, "@reviewer\\x1b[31m") {
		t.Fatalf("author line = %#v, want escaped @author in metadata color", got)
	}
	if got := boxed[2]; got.Class != classPlain || !strings.Contains(got.Text, "exact\\x1b[2J") {
		t.Fatalf("body line = %#v, want escaped plain/white text", got)
	}
	view := ansi.Strip(m.reviewView())
	for _, want := range []string{"+---", "| @reviewer\\x1b[31m", "| exact\\x1b[2J"} {
		if !strings.Contains(view, want) {
			t.Fatalf("inline overlay missing bordered comment part %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "wrong") {
		t.Fatalf("inline overlay did not exactly/securely render:\n%s", view)
	}
}

func TestReviewCommentUsesAvailableDetailWidthAndWrapsLongBody(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.Width, m.Height = 180, 24
	comment := source.ReviewComment{
		ID:     1,
		Author: "reviewer",
		Body:   "This comment should wrap inside the available review pane instead of being clipped after a narrow fixed-width box. The final words must remain visible.",
	}

	lines := m.reviewCommentLines(comment)
	if inner := m.overlayInnerWidth(0); inner <= 68 {
		t.Fatalf("comment inner width = %d, want more than the previous 68-column cap", inner)
	}
	if len(lines) < 5 { // border, author, at least two body rows, border
		t.Fatalf("long comment body was not wrapped: %#v", lines)
	}
	rendered := strings.Join(func() []string {
		out := make([]string, len(lines))
		for i := range lines {
			out[i] = lines[i].Text
		}
		return out
	}(), "\n")
	if !strings.Contains(rendered, "The final words") || !strings.Contains(rendered, "must remain visible.") {
		t.Fatalf("wrapped comment lost trailing text:\n%s", rendered)
	}
}

func TestCommentCursorOpensLocalActionMenuAndEscapeDoesNotWrite(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	var target source.ReviewCommentTarget
	for _, line := range m.baseDetail() {
		if line.target != nil {
			target = *line.target
			break
		}
	}
	m.Comments = []source.ReviewComment{{ID: 7, Author: "other", Target: target, Body: "note"}}
	m.cursorActive = true
	for attempts := 0; attempts < len(m.detail()) && m.detail()[m.cursor()].commentID == 0; attempts++ {
		key(m, 'n')
	}
	if m.detail()[m.cursor()].commentID != 7 {
		t.Fatal("comment navigation did not reach the loaded comment")
	}
	namedKey(m, tea.KeyEnter)
	if m.CommentMenu == nil || m.Composer != nil || m.CommentMenu.CommentID != 7 {
		t.Fatalf("comment enter did not open local menu: %#v", m.CommentMenu)
	}
	namedKey(m, tea.KeyEscape)
	if m.CommentMenu != nil {
		t.Fatal("escape did not close local menu")
	}
}

func TestCommentActionResultOnlyChangesOriginatingTabAndPreservesFailureDraft(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	first, second := kindsSession(), kindsSession()
	second.Inventory.Comparison.Metadata.Identity.Number = 2
	m.openReviewTab(first)
	m.selectReviewView(viewFiles)
	m.openReviewTab(second)
	m.selectReviewView(viewFiles)
	m.activateTab(0)
	m.CommentMenu = &commentActionMenu{CommentID: 7, generation: 1, Draft: "reply"}
	m.Comments = []source.ReviewComment{{ID: 7}}
	m.activateTab(1)
	m.Update(CommentActionResult{Target: 0, CommentID: 7, Generation: 1, Reply: source.ReviewComment{ID: 8}, Err: errors.New("rejected")})
	if m.CommentMenu != nil {
		t.Fatal("stale first-tab failure leaked into active tab")
	}
	if got := m.tabs[0].review.CommentMenu; got == nil || got.Draft != "reply" {
		t.Fatalf("failure did not retain first-tab draft: %#v", got)
	}
}

func TestReplyEditorAndCanonicalReplyRenderAsIndentedThread(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	var target source.ReviewCommentTarget
	for _, line := range m.baseDetail() {
		if line.target != nil {
			target = *line.target
			break
		}
	}
	m.Comments = []source.ReviewComment{{ID: 7, Target: target, Body: "parent"}, {ID: 8, Target: target, ParentID: 7, Body: "reply"}}
	m.CommentMenu = &commentActionMenu{CommentID: 7, Target: target, mode: commentActionReply, Draft: "draft"}
	view := ansi.Strip(m.reviewView())
	for _, want := range []string{"  | parent", "      | reply", "      | draft"} {
		if !strings.Contains(view, want) {
			t.Fatalf("threaded reply missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(m.reviewStatus(), "draft") {
		t.Fatalf("reply draft leaked into status: %s", m.reviewStatus())
	}
}

func TestOpeningReplyScrollsEntireEditorIntoView(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width int
		split bool
	}{
		{"narrow", 80, false},
		{"wide", 120, false},
		{"side by side", 160, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background(), nil)
			m.Loading = false
			m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
			m.Width, m.Height = tc.width, 10
			if tc.split {
				key(m, 'S')
			}
			var target source.ReviewCommentTarget
			for _, line := range m.baseDetail() {
				if line.target != nil {
					target = *line.target
					break
				}
			}
			m.Comments = []source.ReviewComment{{ID: 7, Target: target, Body: "parent"}}
			commentRow := -1
			for i, line := range m.displayDetail() {
				if line.commentID == 7 {
					commentRow = i
				}
			}
			if commentRow < 0 {
				t.Fatal("comment missing from detail")
			}
			m.setCursor(commentRow)
			m.setOffset(max(0, commentRow-m.bodyHeight()+1))
			if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
				t.Fatal("opening comment actions unexpectedly returned a command")
			}
			key(m, 'r')
			if m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply {
				t.Fatal("reply editor did not open")
			}
			assertEditorVisible := func() {
				t.Helper()
				start, end := -1, -1
				for i, line := range m.displayDetail() {
					if line.editor {
						if start < 0 {
							start = i
						}
						end = i + 1
					}
				}
				if start < 0 || start < m.offset() || end > m.offset()+m.bodyHeight() {
					t.Fatalf("reply editor rows [%d,%d) outside viewport [%d,%d)", start, end, m.offset(), m.offset()+m.bodyHeight())
				}
			}
			assertEditorVisible()
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
			assertEditorVisible()
		})
	}
}

func TestReactionPickerDigitsAndBottomBorderCounts(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.CommentMenu = &commentActionMenu{CommentID: 7, mode: commentActionReact}
	key(m, '5')
	if m.CommentMenu == nil || m.CommentMenu.Reaction != "heart" {
		t.Fatalf("5 selected %#v, want heart", m.CommentMenu)
	}
	m.CommentReactions = map[int64][]source.ReviewCommentReaction{7: {{Content: "heart"}, {Content: "heart"}, {Content: "+1"}}}
	parts := []string{}
	for _, line := range m.reviewCommentLines(source.ReviewComment{ID: 7, Body: "body"}) {
		parts = append(parts, line.Text)
	}
	box := strings.Join(parts, "\n")
	if !strings.Contains(box, "[👍 1] [❤️ 2]") {
		t.Fatalf("bottom border lacks reaction counts:\n%s", box)
	}
}

func TestReactionEmojiDisplayFallsBackForASCII(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.reactionEmoji = true
	m.CommentReactions = map[int64][]source.ReviewCommentReaction{7: {
		{Content: "+1"}, {Content: "-1"}, {Content: "laugh"}, {Content: "confused"},
		{Content: "heart"}, {Content: "hooray"}, {Content: "rocket"}, {Content: "eyes"},
	}}
	emojiBox := m.commentBottomBorder("", 120, 7)
	for _, want := range []string{"[👍 1]", "[👎 1]", "[😄 1]", "[😕 1]", "[❤️ 1]", "[🎉 1]", "[🚀 1]", "[👀 1]"} {
		if !strings.Contains(emojiBox, want) {
			t.Fatalf("emoji reaction chip %q missing from %q", want, emojiBox)
		}
	}
	m.CommentMenu = &commentActionMenu{CommentID: 7, mode: commentActionReact}
	if status := m.reviewStatus(); !strings.Contains(status, "1 👍") || !strings.Contains(status, "8 👀") {
		t.Fatalf("emoji reaction picker = %q", status)
	}

	m.reactionEmoji = false
	asciiBox := m.commentBottomBorder("", 120, 7)
	if !strings.Contains(asciiBox, "[+1 1]") || !strings.Contains(asciiBox, "[eyes 1]") || strings.Contains(asciiBox, "👍") {
		t.Fatalf("ASCII fallback reaction chips = %q", asciiBox)
	}
	if status := m.reviewStatus(); !strings.Contains(status, "1 +1") || !strings.Contains(status, "8 eyes") || strings.Contains(status, "👍") {
		t.Fatalf("ASCII fallback reaction picker = %q", status)
	}
}

func TestReplyUsesSharedBlinkingEditorAndSubmits(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	var target source.ReviewCommentTarget
	for _, line := range m.baseDetail() {
		if line.target != nil {
			target = *line.target
			break
		}
	}
	m.Comments = []source.ReviewComment{{ID: 7, Target: target, Body: "parent"}}
	m.CommentMenu = &commentActionMenu{CommentID: 7, Target: target, mode: commentActionPick}
	called := 0
	m.SetCommentActionSubmitter(func(_ context.Context, action CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
		called++
		if action.Body != "reply" {
			t.Fatalf("reply body = %q", action.Body)
		}
		return source.ReviewComment{ID: 8, ParentID: 7, Target: target, Body: action.Body}, source.ReviewCommentReaction{}, nil
	})
	key(m, 'r')
	if !m.editorCursorVisible || m.editorCursorGeneration == 0 {
		t.Fatalf("reply did not start shared blinking editor: visible=%v generation=%d", m.editorCursorVisible, m.editorCursorGeneration)
	}
	generation := m.editorCursorGeneration
	m.Update(editorCursorTick{generation: generation})
	if m.editorCursorVisible {
		t.Fatal("reply editor tick did not blink the shared caret")
	}
	m.editorCursorVisible = true
	for _, r := range "reply" {
		key(m, r)
	}
	_, submit := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, submit)
	if called != 1 || m.CommentMenu != nil || len(m.Comments) != 2 {
		t.Fatalf("reply submission = calls %d menu %#v comments %#v", called, m.CommentMenu, m.Comments)
	}
}

func TestReplyToReplyTargetsTopLevelComment(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	var target source.ReviewCommentTarget
	for _, line := range m.baseDetail() {
		if line.target != nil {
			target = *line.target
			break
		}
	}
	m.Comments = []source.ReviewComment{{ID: 7, Target: target, Body: "parent"}, {ID: 8, ParentID: 7, Target: target, Body: "reply"}}
	m.CommentMenu = &commentActionMenu{CommentID: 8, ReplyToID: 7, Target: target, mode: commentActionReply, Draft: "follow up", Cursor: len([]rune("follow up"))}
	m.SetCommentActionSubmitter(func(_ context.Context, action CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
		if action.Comment.ID != 7 {
			t.Fatalf("reply target = %d, want top-level 7", action.Comment.ID)
		}
		return source.ReviewComment{ID: 9, ParentID: 7, Target: target, Body: action.Body}, source.ReviewCommentReaction{}, nil
	})
	_, submit := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, submit)
	if len(m.Comments) != 3 || m.Comments[2].ParentID != 7 {
		t.Fatalf("reply was not added to top-level thread: %#v", m.Comments)
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
	for _, wording := range []string{"focus the diff; open a line editor or selected-comment action menu", "reset active diff scroll"} {
		if !strings.Contains(help, wording) {
			t.Fatalf("updated help wording missing %q", wording)
		}
	}
	for _, key := range []string{"P", "j/k", "J/K", "h/l, ctrl+h/ctrl+l", "esc", "q/ctrl+c"} {
		if !strings.Contains(help, key) {
			t.Fatalf("binding %q missing from help", key)
		}
	}
	for _, key := range []string{"P", "h/l, ctrl+h/ctrl+l", "esc", "q/ctrl+c", "m", "N"} {
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
	m.selectReviewView(viewFiles)
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
		if u.Kind == inventory.FileMetadata {
			if !strings.Contains(stripped, fileDivider(s.Inventory.Files[s.UnitFiles[i]])) {
				t.Fatalf("%s omitted its file divider", u.Kind)
			}
			for _, noisy := range []string{"old object:", "new object:", "status "} {
				if strings.Contains(stripped, noisy) {
					t.Fatalf("%s leaked metadata %q", u.Kind, noisy)
				}
			}
			continue
		}
		for _, line := range strings.Split(strings.TrimSuffix(unitText(s, i), "\n"), "\n") {
			if u.Kind == inventory.TextHunk && strings.HasPrefix(line, "── ") {
				if !strings.Contains(stripped, fileDivider(s.Inventory.Files[s.UnitFiles[i]])) {
					t.Fatalf("%s omitted its file divider", u.Kind)
				}
				continue
			}
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
	m.selectReviewView(viewFiles)
	for _, w := range []int{1, 20, 60, 99, 100, 120} {
		for i := range s.Inventory.Units {
			for _, focus := range []pane{paneList, paneDiff} {
				m.Selected, m.Focus = i, focus
				m.Update(tea.WindowSizeMsg{Width: w, Height: 12})
				colored := m.View().Content
				plain := withoutStyles(m, func() string { return m.View().Content })
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
	if !strings.Contains(content, "GUIDES") || !strings.Contains(content, `1.1 Add greeting\x1b]52`) {
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
	for steps := 0; steps < len(m.rows()) && m.rows()[m.Row].kind != portionRow; steps++ {
		key(m, 'n')
	}
	if m.rows()[m.Row].kind != portionRow {
		t.Fatal("guide navigation did not reach a portion row")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focus != paneDiff || !strings.Contains(ansi.Strip(m.View().Content), "· Diff") || !strings.Contains(ansi.Strip(m.View().Content), fileDivider(s.Inventory.Files[m.Session.UnitFiles[m.Selected]])) {
		t.Fatal("narrow tab did not switch to the diff pane")
	}
	namedKey(m, tea.KeyEscape)
	if m.Focus != paneList {
		t.Fatal("esc did not switch back to the hierarchy")
	}
	for steps := 0; steps < len(m.rows()) && m.Row > 0; steps++ {
		key(m, 'p')
	}
	if m.Row != 0 {
		t.Fatal("guide navigation did not return to the first row")
	}
	before := len(m.rows())
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focus != paneList || len(m.rows()) >= before {
		t.Fatal("tab on a guide row did not collapse it")
	}
}
