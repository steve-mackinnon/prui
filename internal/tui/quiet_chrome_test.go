package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/session"
	"prui/internal/source"
)

func TestQuietStatusPrioritizesAttentionAndPending(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Loading = false
	m.Session = screenSession()
	m.Session.Inventory.Complete = false
	m.Session.RevisionStatus = session.Stale
	m.Pending = []source.ReviewComment{{}}
	for _, width := range []int{40, 60, 99, 100, 120, 160} {
		got := ansi.Strip(m.healthStatus(width, m.Session))
		if visibleWidth(got) > width || !strings.Contains(got, "R ") || !strings.Contains(got, "1") || !strings.Contains(got, "!") {
			t.Fatalf("width %d lost review/pending/attention or overflowed: %q", width, got)
		}
		if !strings.Contains(got, "stale") && !strings.Contains(got, "2 issues") {
			t.Fatalf("multiple warnings silently dropped: %q", got)
		}
		if strings.Contains(got, "Inventory complete") || strings.Contains(got, "Guide available") {
			t.Fatalf("routine detail competes with attention: %q", got)
		}
	}
}

func TestQuietFooterHintsFollowContext(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Loading = false
	m.Session = screenSession()
	m.Width, m.Height = 120, 24
	m.Focus = paneList
	got := ansi.Strip(m.reviewStatus())
	if !strings.Contains(got, "enter: diff") || !strings.Contains(got, "m: mark file") || strings.Count(got, "\n") != 1 {
		t.Fatalf("list hints missing: %q", got)
	}
	m.Focus = paneDiff
	if got = ansi.Strip(m.reviewStatus()); !strings.Contains(got, "enter: comment") {
		t.Fatalf("diff hints missing: %q", got)
	}
	m.Composer = &commentComposer{}
	if got = ansi.Strip(m.reviewStatus()); !strings.Contains(got, "post now") || !strings.Contains(got, "pending") || !strings.Contains(got, "R Submit review") {
		t.Fatalf("composer lost explicit actions or status: %q", got)
	}
	m.Composer = nil
	m.Height = 8
	if got = m.reviewStatus(); strings.Contains(got, "\n") {
		t.Fatalf("short screen spent two footer rows: %q", got)
	}
}

func TestQuietHealthHelpRetainsOmittedHealth(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Loading = false
	m.Session = screenSession()
	m.Width, m.Height = 120, 24
	got := ansi.Strip(m.healthHelpView())
	for _, want := range []string{"Inventory complete", "Freshness unknown", "Guide available with g", "Reading progress is local"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help omitted %q: %s", want, got)
		}
	}
}
