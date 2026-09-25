package tui

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
)

var updateScreens = flag.Bool("update-golden", false, "explicitly replace TUI screen baselines")

// Presentation fixtures contain no Git timestamps, random IDs, or machine paths.
// Color semantics and escaping remain covered independently by color/style tests.
func screenSession() *review.Session {
	s := largeSession(2, 2)
	s.Inventory.Comparison.Metadata = source.Metadata{Identity: source.Identity{Repository: "example/review", Number: 42}, HeadSHA: strings.Repeat("a", 40)}
	s.Inventory.Files[0].NewPath = []byte("main.go")
	s.Inventory.Files[0].Status = "M"
	s.Inventory.Files[1].NewPath = []byte("README.md")
	s.Inventory.Units[0].Kind = inventory.TextHunk
	s.Inventory.Units[0].PatchReference = "patch"
	s.Inventory.Patches = map[string][]byte{"patch": []byte("@@ -1 +1 @@\n-old greeting\n+hello world\n")}
	return s
}

func TestScreenSnapshots(t *testing.T) {
	const descriptionMarkdown = "# Summary\r\n\r\n- review the loading path\r\n- confirm the fallback\r\n\r\n```go\r\nfmt.Println(\"ready\")\r\n```\r\n\r\n[Reference](https://example.com)\r\n\r\n<em>literal HTML</em>"
	for _, tc := range []struct {
		name          string
		width, height int
		setup         func(*Model)
	}{
		{"review_wide", 120, 12, func(m *Model) { m.Focus = paneDiff }},
		{"review_narrow", 60, 10, func(m *Model) { m.Focus = paneDiff }},
		{"review_side_by_side_wide", 160, 12, func(m *Model) {
			m.Focus = paneDiff
			key(m, 'S')
		}},
		{"review_side_by_side_narrow_fallback", 159, 12, func(m *Model) {
			m.Focus = paneDiff
			key(m, 'S')
		}},
		{"description_wide", 120, 18, func(m *Model) {
			description := descriptionMarkdown
			m.Session.PullRequestDescription = &description
			m.openReviewTab(m.Session)
			key(m, 'v')
		}},
		{"description_narrow", 60, 18, func(m *Model) {
			description := descriptionMarkdown
			m.Session.PullRequestDescription = &description
			m.openReviewTab(m.Session)
			key(m, 'v')
		}},
		{"comment_thread", 120, 16, func(m *Model) {
			m.Focus, m.cursorActive = paneDiff, true
			var target source.ReviewCommentTarget
			for _, line := range m.baseDetail() {
				if line.target != nil {
					target = *line.target
					break
				}
			}
			m.Comments = []source.ReviewComment{{ID: 7, Author: "reviewer", Target: target, Body: "parent message"}, {ID: 8, ParentID: 7, Author: "viewer", Target: target, Body: "threaded reply"}}
			m.CommentReactions = map[int64][]source.ReviewCommentReaction{7: {{Content: "heart"}, {Content: "heart"}, {Content: "+1"}}}
		}},
		{"review_form_wide", 120, 16, func(m *Model) {
			m.Pending = []source.ReviewComment{{Target: source.ReviewCommentTarget{Path: "main.go", Side: "RIGHT", Line: 1}, Body: "Please cover the empty case."}}
			m.openReviewForm()
		}},
		{"review_form_narrow_edit", 60, 10, func(m *Model) {
			m.openReviewForm()
			namedKey(m, tea.KeyDown)
			namedKey(m, tea.KeyTab)
		}},
		{"review_form_narrow", 60, 10, func(m *Model) {
			m.openReviewForm()
			m.ReviewForm.Event, m.ReviewForm.Body, m.ReviewForm.Confirm = 2, "Please address the edge case.", true
		}},
		{"loading", 80, 8, func(m *Model) { m.Loading, m.cancelAction = true, func() {} }},
		{"load_error", 80, 8, func(m *Model) { m.Err = errors.New("synthetic metadata failure") }},
		{"guides", 120, 14, func(m *Model) {
			m.Session.Guides = &guide.Bundle{Status: guide.Generated, Provider: "fixture", Model: "fixture", Items: []guide.Item{{
				Title: "Greeting", Description: "Update the greeting and its documentation.",
				Sections: []guide.Section{{Title: "Implementation", UnitIDs: []string{"unit-0"}}, {Title: "Documentation", UnitIDs: []string{"unit-1"}}},
			}}}
			m.Files = false
			m.begin()
		}},
		{"repositories", 60, 8, func(m *Model) {
			m.Stack = []page{pageRepositoryPicker}
			for i := 0; i < 20; i++ {
				m.Repositories = append(m.Repositories, session.Repository{Repository: fmt.Sprintf("example/repo-%02d", i+1)})
			}
			for i := 0; i < 19; i++ {
				key(m, 'j')
			}
		}},
		{"pull_requests", 60, 8, func(m *Model) {
			m.Session = nil
			m.Stack = []page{pagePullRequestPicker}
			for i := 0; i < 20; i++ {
				m.PullRequests = append(m.PullRequests, source.PullRequest{Identity: source.Identity{Repository: "example/review", Number: i + 1}, Title: fmt.Sprintf("Change %02d", i+1), Author: "alice", LastModifier: "bob", OpenedAt: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC), Checks: source.ChecksPassed})
			}
			for i := 0; i < 19; i++ {
				key(m, 'j')
			}
		}},
		{"switcher", 80, 10, func(m *Model) {
			other := screenSession()
			other.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "example/review", Number: 7}
			m.openReviewTab(other)
			m.activateTab(0)
			m.PullRequests = []source.PullRequest{{Identity: source.Identity{Repository: "example/review", Number: 99}, Title: "Unopened change"}}
			ctrlKey(m, 'p')
		}},
		{"action_error", 100, 10, func(m *Model) { m.ActionError = errors.New("synthetic save failure") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			m.Loading = false
			m.Session, m.Width, m.Height = screenSession(), tc.width, tc.height
			tc.setup(m)
			got := ansi.Strip(m.View().Content)
			lines := strings.Split(got, "\n")
			if len(lines) > tc.height {
				t.Fatalf("screen has %d lines, height is %d", len(lines), tc.height)
			}
			for i, line := range lines {
				if visibleWidth(line) > tc.width {
					t.Fatalf("screen exceeds width %d: %q", tc.width, line)
				}
				// Invisible right-edge padding is not part of the text baseline.
				lines[i] = strings.TrimRight(line, " ")
			}
			if tc.name == "repositories" && !strings.Contains(got, "› example/repo-20") || tc.name == "pull_requests" && (!strings.Contains(got, "› ╭") || !strings.Contains(got, "#20  Change 20")) {
				t.Fatalf("selected item is invisible; cannot accept this baseline:\n%s", got)
			}
			checkScreen(t, tc.name, strings.Join(lines, "\n")+"\n")
		})
	}
}

func checkScreen(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "screens", name+".golden")
	if *updateScreens {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read screen baseline: %v; to create it run go test ./internal/tui -run '^TestScreenSnapshots$' -args -update-golden", err)
	}
	if string(want) != got {
		t.Fatalf("screen differs from %s\n--- expected ---\n%s--- actual ---\n%s\nIf intentional, update with -update-golden and review the diff.", path, want, got)
	}
}
