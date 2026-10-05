package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
	"prui/internal/theme"
)

func TestPRPickerCarouselRevealsTitleAndResetsOnSelection(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 24
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 1}, Title: strings.Repeat("Long title ", 8) + "UNIQUE END", Author: "alice"}, {Identity: source.Identity{Number: 2}, Title: "Short"}}
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if cmd == nil {
		t.Fatal("overflow did not start carousel")
	}
	generation := m.prCarousel.generation
	for i := 0; i < 160; i++ {
		m.Update(prCarouselTick{generation})
		if strings.Contains(ansi.Strip(m.prPickerRow(m.PullRequests[0])), "UNIQUE END") {
			break
		}
		if i == 159 {
			t.Fatal("carousel never revealed title ending")
		}
	}
	key(m, 'j')
	if m.prCarousel.offset != 0 {
		t.Fatal("selection did not reset carousel")
	}
	_, cmd = m.Update(prCarouselTick{generation})
	if cmd != nil || m.prCarousel.offset != 0 {
		t.Fatal("stale tick advanced carousel")
	}
}

func TestPRPickerUsernameAndChecksUseDistinctAccents(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	palette, err := theme.Resolve(theme.Dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(palette)
	m.Width = 120
	pass := m.prPickerRow(source.PullRequest{Identity: source.Identity{Number: 1}, Title: "Title", Author: "alice", Checks: source.ChecksPassed})
	fail := m.prPickerRow(source.PullRequest{Identity: source.Identity{Number: 1}, Title: "Title", Author: "alice", Checks: source.ChecksFailed})
	for _, want := range []string{m.styleLine(classHunk, "@alice"), m.styleLine(classAdded, "✓")} {
		if !strings.Contains(pass, want) {
			t.Fatalf("missing accent %q in %q", want, pass)
		}
	}
	if !strings.Contains(fail, m.styleLine(classRemoved, "!")) {
		t.Fatal("failed check lacks failure accent")
	}
	if !strings.Contains(ansi.Strip(pass), "@alice ✓") {
		t.Fatal("accent styling changed row text")
	}
}

func TestPRPickerCarouselKeepsIdentityAndMetadataFixedAtNarrowAndWideWidths(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := New(context.Background(), nil)
		m.Stack = []page{pagePullRequestPicker}
		m.Width, m.Height = width, 24
		pr := source.PullRequest{Identity: source.Identity{Number: 42}, Title: strings.Repeat("界 retry ", 30) + "END", Author: "alice", TargetBranch: "release/next", Checks: source.ChecksPassed}
		m.PullRequests = []source.PullRequest{pr}
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		generation := m.prCarousel.generation
		for i := 0; i < 12; i++ {
			m.Update(prCarouselTick{generation})
		}
		row := ansi.Strip(m.prPickerRow(pr))
		if !strings.HasPrefix(row, "#42  ") || !strings.Contains(row, "@alice") {
			t.Fatalf("identity or metadata scrolled: %s", row)
		}
		rowWidth := width - 2
		if width >= 100 {
			rowWidth = m.prPickerGeometry(1).leftWidth - 2
		}
		if visibleWidth(row) > rowWidth {
			t.Fatal("carousel overflowed rail")
		}
		m.Update(tea.WindowSizeMsg{Width: 180, Height: 24})
		if m.prCarousel.offset != 0 || m.prCarousel.generation == generation {
			t.Fatal("resize did not reset carousel")
		}
		m.Stack = []page{pageHelp}
		_, cmd := m.Update(prCarouselTick{generation})
		if cmd != nil || m.prCarousel.target.width != 0 {
			t.Fatal("hidden picker retained carousel")
		}
		m.Close()
	}
}
