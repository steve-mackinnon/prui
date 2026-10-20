package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func TestPRPickerTwoLineRows(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	for _, width := range []int{40, 80, 120, 180} {
		m.Width, m.Height = width, 24
		pr := source.PullRequest{Identity: source.Identity{Number: 42}, Title: strings.Repeat("Title ", 30), Author: "alice", Checks: source.ChecksPassed, StackPosition: 2, StackSize: 4}
		m.PullRequests = []source.PullRequest{pr}
		lines := strings.Split(ansi.Strip(m.prPickerRow(pr)), "\n")
		if len(lines) != 2 || !strings.Contains(lines[1], "2/4") || !strings.Contains(lines[1], "@alice") {
			t.Fatalf("width %d: %q", width, lines)
		}
		limit := width - 2
		if width >= 100 {
			limit = m.prPickerGeometry(1).leftWidth - 2
		}
		if visibleWidth(lines[0]) != limit {
			t.Fatalf("title not using available width: %q", lines[0])
		}
		for _, line := range lines {
			if visibleWidth(line) > limit {
				t.Fatal("row overflow")
			}
		}
	}
}

func TestPullRequestRowsPinAuthorsBeforeChecks(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Width = 80
	first := strings.Split(ansi.Strip(m.prPickerRow(source.PullRequest{Identity: source.Identity{Number: 1}, Title: strings.Repeat("long title ", 20), Author: "alice", Checks: source.ChecksPassed})), "\n")
	second := strings.Split(ansi.Strip(m.prPickerRow(source.PullRequest{Identity: source.Identity{Number: 2}, Title: "Short", Author: "bob", Checks: source.ChecksPending})), "\n")
	if !strings.HasPrefix(first[1], "@alice") || !strings.HasPrefix(second[1], "@bob") {
		t.Fatal("authors must start metadata line independently of title length")
	}
	for _, width := range []int{20, 32, 40, 60, 80, 120, 240} {
		m.Width = width
		row := m.prPickerRow(source.PullRequest{Identity: source.Identity{Number: 42}, Title: strings.Repeat("x", 100), Author: strings.Repeat("a", 100), Checks: source.ChecksPassed})
		if !strings.Contains(row, "#42") {
			t.Fatal("missing identity")
		}
		for _, line := range strings.Split(row, "\n") {
			if visibleWidth(line) > m.prPickerRowWidth() {
				t.Fatalf("width %d: %q", width, row)
			}
		}
	}
}

func TestPRPickerTwoLineScrolledMouseSelection(t *testing.T) {
	for _, width := range []int{80, 120} {
		for _, height := range []int{4, 8, 12, 24} {
			m := New(context.Background(), nil)
			m.Stack = []page{pagePullRequestPicker}
			m.Width, m.Height = width, height
			for i := 1; i <= 30; i++ {
				m.PullRequests = append(m.PullRequests, source.PullRequest{Identity: source.Identity{Number: i}, Title: "Title", Author: "alice"})
			}
			m.PullRequestPicker.Index = 25
			g := m.prPickerGeometry(1)
			rowHeight := min(2, g.listHeight)
			capacity := max(1, g.listHeight/rowHeight)
			start := 25 - capacity + 1
			for part := 0; part < rowHeight; part++ {
				m.PullRequestPicker.Index = 25
				m.mousePickerClick(1, g.listTop+part)
				if m.PullRequestPicker.Index != start {
					t.Fatalf("%dx%d line %d selected %d; want %d", width, height, part, m.PullRequestPicker.Index, start)
				}
			}
			m.Close()
		}
	}
}
