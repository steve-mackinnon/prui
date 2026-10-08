package tui

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"fmt"
	"prui/internal/source"
	"strings"
	"testing"
)

func commitCacheComposer(t *testing.T, lines int) *Model {
	t.Helper()
	m := commitModel(t)
	m.Session.Commits.Entries[0].Diff.Patches["patch"] = []byte(fmt.Sprintf("@@ -0,0 +1,%d @@\n", lines) + strings.Repeat("+source\n", lines))
	m.commit.sourceCache = commitSourceCache{}
	m.commit.cache = commitRenderCache{}
	for _, row := range m.commitRows() {
		if row.target != nil {
			m.Composer = &commentComposer{Target: *row.target, CommitSHA: row.target.CommitID, Draft: "draft", PendingIndex: -1}
			break
		}
	}
	if m.Composer == nil {
		t.Fatal("missing fixture anchor")
	}
	m.commitRows()
	return m
}

func TestCommitEditorAllocationsDoNotScaleWithPatchLines(t *testing.T) {
	small, large := commitCacheComposer(t, 30), commitCacheComposer(t, 3000)
	allocations := func(m *Model) float64 {
		return testing.AllocsPerRun(10, func() {
			m.editorCursorVisible = !m.editorCursorVisible
			m.commitRows()
		})
	}
	smallAllocs, largeAllocs := allocations(small), allocations(large)
	t.Logf("editor redraw allocations: 30 lines %.0f; 3000 lines %.0f", smallAllocs, largeAllocs)
	if largeAllocs > smallAllocs+50 {
		t.Fatalf("editor redraw reparses patch rows: 30 lines %.0f allocations, 3000 lines %.0f", smallAllocs, largeAllocs)
	}
}

func TestCommitSourceCacheRetainsRowsAcrossPresentationChanges(t *testing.T) {
	m := commitCacheComposer(t, 30)
	sourceRows := m.commit.sourceCache.rows
	target := m.Composer.Target
	m.Composer = nil
	m.Width = 80
	m.theme.Name = "synthetic theme"
	m.discussions.generation++
	thread := testDiscussion("cache-thread", target.CommitID)
	thread.OriginalAnchor = &target
	thread.Comments[0].Body = "fresh discussion"
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{thread}}}})
	rows := m.commitRows()
	found := false
	for _, row := range rows {
		found = found || strings.Contains(row.Text, "fresh discussion")
	}
	if !found || &sourceRows[0] != &m.commit.sourceCache.rows[0] {
		t.Fatal("refresh failed to update overlay while retaining source rows")
	}
	for _, row := range sourceRows {
		if row.editor || strings.Contains(row.Text, "fresh discussion") || strings.Contains(row.Text, "Comment on") {
			t.Fatal("transient content mutated cached source")
		}
	}
	m.discussions.generation++
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: true}}})
	rows = m.commitRows()
	if &rows[0] != &sourceRows[0] {
		t.Fatal("overlay-free rows should reuse source storage")
	}
}

func TestCommitSourceCacheReplacesSelectedSnapshot(t *testing.T) {
	m := commitModel(t)
	m.commitRows()
	first := m.commit.sourceCache.rows
	m.selectCommit(1)
	rows := m.commitRows()
	if &rows[0] == &first[0] || !strings.Contains(rows[0].Text, "Second commit") {
		t.Fatal("selected commit retained previous source")
	}
	previous := m.Session
	replacement := *previous
	m.Session = &replacement
	m.commitRows()
	if m.commit.sourceCache.session != &replacement || &rows[0] == &m.commit.sourceCache.rows[0] {
		t.Fatal("session change retained previous source")
	}
	entry := &m.Session.Commits.Entries[1]
	diff := *entry.Diff
	diff.Patches = map[string][]byte{"patch": []byte("@@ -0,0 +1 @@\n+replacement snapshot\n")}
	entry.Diff = &diff
	found := false
	for _, row := range m.commitRows() {
		found = found || strings.Contains(row.Text, "replacement snapshot")
	}
	if !found {
		t.Fatal("overlay cache hid source snapshot replacement")
	}
}

func TestCommitCacheKeepsEditorFreshAndRemovesCancelledDraft(t *testing.T) {
	m := commitCacheComposer(t, 30)
	text := func() string {
		var lines []string
		for _, row := range m.commitRows() {
			lines = append(lines, row.Text)
		}
		return strings.Join(lines, "\n")
	}
	m.Composer.Draft, m.Composer.Cursor = "updated", 3
	m.editorCursorVisible = true
	m.ActionError = errors.New("synthetic error")
	if got := text(); !strings.Contains(got, "upd▏ated") || !strings.Contains(got, "synthetic error") {
		t.Fatalf("editor changes stale: %s", got)
	}
	m.editorCursorVisible = false
	if got := text(); strings.Contains(got, "▏") || !strings.Contains(got, "upd ated") {
		t.Fatalf("cursor blink stale: %s", got)
	}
	m.commentComposerKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := text(); strings.Contains(got, "updated") || strings.Contains(got, "Comment on") || strings.Contains(got, "synthetic error") {
		t.Fatalf("cancelled editor retained in cache: %s", got)
	}
}
