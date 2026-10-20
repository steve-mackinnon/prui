package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func pageMouseModel(t *testing.T, p page) *Model {
	t.Helper()
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Session = largeSession(3, 20)
	m.Width, m.Height = 120, 24
	m.Stack = []page{pageReview, p}
	return m
}
func pageClick(m *Model, x, y int) tea.Cmd {
	cmd, _ := m.mousePage(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	return cmd
}
func TestPageMouseInboxSelectionFiltersAndClippedControls(t *testing.T) {
	m := pageMouseModel(t, pageInbox)
	m.inbox.data.Items = []source.InboxItem{{PullRequest: source.PullRequest{Identity: source.Identity{Repository: "owner/repo", Number: 1}}}, {PullRequest: source.PullRequest{Identity: source.Identity{Repository: "owner/repo", Number: 2}}}}
	pageClick(m, 3, 5)
	if m.inbox.picker.Index != 1 {
		t.Fatal("inbox row not selected")
	}
	pageClick(m, 31, 23)
	if m.top() != pageInboxFilters {
		t.Fatal("filter control not opened")
	}
	pageClick(m, 2, 2)
	if m.inbox.field != 1 {
		t.Fatal("repository text field not selected")
	}
	pageClick(m, 2, 1)
	if m.inbox.draftOptions.View != "authored" {
		t.Fatalf("choice did not cycle: %s", m.inbox.draftOptions.View)
	}
	if key := pageMouseFooterKey(inboxMouseControls, " · ", 2, 5); key != "" {
		t.Fatal("clipped control activated")
	}
}
func TestPageMouseHelpWheelToggleAndBack(t *testing.T) {
	m := pageMouseModel(t, pageHelp)
	m.Height = 8
	m.mousePage(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 5, Y: 3})
	if m.helpScroll == 0 {
		t.Fatal("help wheel did not scroll")
	}
	pageClick(m, 20, 7)
	if !m.helpExpanded {
		t.Fatal("help shortcut toggle did not activate")
	}
	pageClick(m, ansi.StringWidth("↑/↓: scroll · s: getting started · ")+1, 7)
	if m.top() != pageReview {
		t.Fatal("help back did not activate")
	}
}
func TestPageMouseIncrementalWheelPanAndBack(t *testing.T) {
	m := pageMouseModel(t, pageIncremental)
	m.mousePage(tea.MouseWheelMsg{Button: tea.MouseWheelRight, X: 5, Y: 3})
	if m.incremental.horizontal != 15 {
		t.Fatal("horizontal wheel did not pan")
	}
	pageClick(m, 2, 23)
	if m.incremental.file != 0 {
		t.Fatal("previous file not selected")
	}
	pageClick(m, 55, 23)
	if !m.incremental.drafts {
		t.Fatal("draft toggle did not activate")
	}
}
func TestPageMouseReadOnlyAuxiliaryFooters(t *testing.T) {
	for _, p := range []page{pageEvidence, pageURL, pageIssueContext, pageReadiness} {
		t.Run(string(rune('a'+p)), func(t *testing.T) {
			m := pageMouseModel(t, p)
			x, y := 1, 0
			switch p {
			case pageEvidence:
				y = strings.Count(m.evidenceView(), "\n")
			case pageURL:
				y = 2
			case pageIssueContext:
				y = strings.Count(m.issueContextView(), "\n")
				x = 35
			case pageReadiness:
				y = strings.Count(m.readinessView(), "\n")
				x = ansi.StringWidth(pageMouseFooter(m.readinessMouseControls()[:3], " | ")+" | ") + 1
			}
			pageClick(m, x, y)
			if m.top() != pageReview {
				t.Fatalf("page %v back control did not activate", p)
			}
		})
	}
}
func TestPageMouseLifecycleSelectionAndWheel(t *testing.T) {
	m := pageMouseModel(t, pageLifecycle)
	m.lifecycle.loaded = true
	m.Height = 30
	pageClick(m, 1, 7)
	if m.lifecycle.selected != 1 {
		t.Fatalf("selected %d", m.lifecycle.selected)
	}
	m.mousePage(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 3, Y: 10})
	if m.lifecycle.selected != 4 {
		t.Fatal("lifecycle wheel failed")
	}
	if m.lifecycle.confirmation != nil || m.lifecycle.busy {
		t.Fatal("selection dispatched lifecycle action")
	}
}
func TestPageMouseChromeOwnsEvents(t *testing.T) {
	m := pageMouseModel(t, pageInbox)
	for _, msg := range []tea.MouseMsg{tea.MouseClickMsg{Button: tea.MouseLeft, X: -1, Y: 4}, tea.MouseClickMsg{Button: tea.MouseRight, X: 2, Y: 2}, tea.MouseWheelMsg{Button: tea.MouseWheelLeft, X: 2, Y: 2}} {
		if cmd, owned := m.mousePage(msg); cmd != nil || !owned || m.top() != pageInbox {
			t.Fatal("invalid input escaped page ownership")
		}
	}
}

func TestPageMouseLifecycleConfirmationRequiresVisibleExplicitControl(t *testing.T) {
	m := pageMouseModel(t, pageLifecycle)
	m.Width, m.Height = 120, 30
	s := tuiLifecycleState(m)
	m.lifecycle.snapshot = s
	m.lifecycle.loaded = true
	writes := 0
	m.submitLifecycle = func(context.Context, source.LifecycleAction) (source.LifecycleOutcome, error) {
		writes++
		return source.LifecycleOutcome{}, nil
	}
	action := source.LifecycleAction{Kind: "close", Expected: s}
	m.lifecycle.confirmation = &action
	body, fits := m.lifecycleConfirmationBody()
	if !fits {
		t.Fatal("fixture confirmation does not fit")
	}
	width := min(96, m.Width-8)
	left := max(0, (m.Width-width-4)/2) + 2
	top := max(0, (m.Height-(strings.Count(body, "\n")+3))/2) + 1
	lines := m.lifecycleConfirmationLines()
	prefix := ansi.Hardwrap(ansi.Wrap(strings.Join(lines[:len(lines)-2], "\n"), width, ""), width, false)
	y := top + strings.Count(prefix, "\n") + 1
	if cmd := pageClick(m, left, top); cmd != nil || writes != 0 || m.lifecycle.busy {
		t.Fatal("confirmation text triggered write")
	}
	cmd := pageClick(m, left+1, y)
	if cmd == nil || writes != 0 || !m.lifecycle.busy {
		t.Fatal("explicit confirm did not prepare guarded command")
	}
	cmd()
	if writes != 1 {
		t.Fatal("confirm did not submit exactly once")
	}
	if pageClick(m, left+1, y) != nil {
		t.Fatal("busy confirmation repeated command")
	}
}
