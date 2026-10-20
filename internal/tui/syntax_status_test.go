package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/syntax"
)

func TestSyntaxFailureVisibleInDiffAndHealth(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Session = screenSession()
	f := m.Session.Inventory.Files[0]
	m.Session.Inventory.SyntaxStatus = map[string]syntax.FileStatus{f.ID: {Old: syntax.Ready, New: syntax.Timeout}}
	for _, width := range []int{40, 60, 100, 160} {
		got := ansi.Strip(m.healthStatus(width, m.Session))
		if strings.Contains(got, "Syntax unavailable") || strings.Contains(got, "!") || visibleWidth(got) > width {
			t.Fatalf("width %d: %q", width, got)
		}
	}
	if got := m.healthHelpView(); !strings.Contains(got, "capture timed out") || !strings.Contains(got, "open a new comparison") {
		t.Fatalf("missing reason/recovery: %s", got)
	}
	for i, u := range m.Session.Inventory.Units {
		if u.FileChangeID != f.ID || u.Kind != inventory.TextHunk {
			continue
		}
		rows := unitLines(m.Session, i)
		if len(rows) == 0 || rows[0].Class != classWarning || !strings.Contains(rows[0].Text, "capture timed out") {
			t.Fatalf("missing diff warning: %#v", rows)
		}
		if rows[0].target != nil || rows[0].rawSource != "" {
			t.Fatal("warning acquired a source/comment identity")
		}
	}
}

func TestLegacyEmptySyntaxIsVisibleWithoutGuessingReason(t *testing.T) {
	s := screenSession()
	s.Inventory.Files[0].NewPath = []byte("x.ts")
	s.Inventory.Files[0].OldPath = []byte("x.ts")
	s.Inventory.Syntax = map[string]syntax.Patch{}
	for _, u := range s.Inventory.Units {
		if u.Kind == inventory.TextHunk {
			s.Inventory.Syntax[u.ID] = syntax.Patch{}
		}
	}
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Session = s
	got := m.healthHelpView()
	if !strings.Contains(got, "reason not recorded") {
		t.Fatalf("legacy omissions silent: %s", got)
	}
	if strings.Contains(got, "capture timed out") {
		t.Fatal("invented a historical failure reason")
	}
}

func TestSuccessfulTokenFreeSyntaxDoesNotWarn(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status syntax.Status
	}{
		{"token free", syntax.Ready}, {"unsupported", syntax.Unsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			m.Session = screenSession()
			f := m.Session.Inventory.Files[0]
			m.Session.Inventory.Syntax = map[string]syntax.Patch{}
			for _, u := range m.Session.Inventory.Units {
				if u.Kind == inventory.TextHunk {
					m.Session.Inventory.Syntax[u.ID] = syntax.Patch{}
				}
			}
			m.Session.Inventory.SyntaxStatus = map[string]syntax.FileStatus{f.ID: {Old: tc.status, New: tc.status}}
			if got := m.healthHelpView(); strings.Contains(got, "Syntax unavailable") {
				t.Fatalf("successful/unsupported capture reported as failure: %s", got)
			}
			if got := m.healthStatus(160, m.Session); strings.Contains(got, "Syntax unavailable") {
				t.Fatalf("unexpected footer warning: %s", got)
			}
			if tc.status == syntax.Unsupported && !strings.Contains(m.healthHelpView(), "unsupported file type") {
				t.Fatal("unsupported reason missing")
			}
		})
	}
}

func TestCommitSyntaxFailureVisibleWithoutChangingAnchors(t *testing.T) {
	s := screenSession()
	d := &commits.Diff{Files: s.Inventory.Files, Units: s.Inventory.Units, Patches: s.Inventory.Patches,
		SyntaxStatus: map[string]syntax.FileStatus{s.Inventory.Files[0].ID: {Old: syntax.Ready, New: syntax.InventoryTimeLimit}}}
	rows := commitDiffRowsFor(d, source.Identity{Repository: "example/review", Number: 42}, strings.Repeat("a", 40))
	warned, sourceSeen := false, false
	for _, row := range rows {
		if strings.Contains(row.Text, "comparison syntax time budget exhausted") {
			warned = true
			if row.Class != classWarning || row.target != nil {
				t.Fatal("warning acquired source attributes")
			}
		}
		if row.rawSource == "hello world" {
			sourceSeen = true
			if row.target == nil || row.target.Line != 1 || row.target.Side != "RIGHT" {
				t.Fatal("source anchor changed")
			}
		}
	}
	if !warned || !sourceSeen {
		t.Fatalf("warning=%t source=%t", warned, sourceSeen)
	}
	s.Commits = &commits.Bundle{Entries: []commits.Entry{{SHA: strings.Repeat("a", 40), Diff: d}}}
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Session = s
	if got := m.healthHelpView(); !strings.Contains(got, "Commit syntax unavailable") {
		t.Fatalf("commit omissions silent: %s", got)
	}
	if got := m.healthStatus(160, m.Session); strings.Contains(got, "Syntax unavailable") || strings.Contains(got, "syntax unavailable") {
		t.Fatalf("unexpected commit syntax footer warning: %s", got)
	}
}

func TestSyntaxHealthTracksReviewSwitches(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	first, second := screenSession(), screenSession()
	first.Inventory.SyntaxStatus = map[string]syntax.FileStatus{first.Inventory.Files[0].ID: {Old: syntax.Timeout, New: syntax.Timeout}}
	second.Inventory.SyntaxStatus = map[string]syntax.FileStatus{second.Inventory.Files[0].ID: {Old: syntax.Ready, New: syntax.Ready}}
	m.Session = first
	if got := m.healthHelpView(); !strings.Contains(got, "capture timed out") {
		t.Fatal("first review reason missing")
	}
	m.Session = second
	if got := m.healthHelpView(); strings.Contains(got, "capture timed out") || strings.Contains(got, "Syntax unavailable") {
		t.Fatal("stale diagnostics crossed review tabs")
	}
	m.Session = first
	if got := m.healthHelpView(); !strings.Contains(got, "capture timed out") {
		t.Fatal("restored review reason missing")
	}
}
