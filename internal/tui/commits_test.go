package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/source"
	"strings"
	"testing"
)

func commitModel(t *testing.T) *Model {
	t.Helper()
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	s.Commits = &commits.Bundle{Status: commits.Captured, Complete: true}
	for i, subject := range []string{"First commit", "Second commit"} {
		sha := strings.Repeat(string(rune('a'+i)), 40)
		f := inventory.FileChange{ID: "file", NewPath: []byte("changed.go"), NewMode: "100644"}
		patch := "@@ -1,1 +1,30 @@\n-old\n" + strings.Repeat("+"+subject+"\n", 30)
		s.Commits.Entries = append(s.Commits.Entries, commits.Entry{SHA: sha, Subject: subject, Author: "Alice", Status: commits.Captured, Diff: &commits.Diff{Complete: true, Files: []inventory.FileChange{f}, Units: []inventory.ReviewUnit{{FileChangeID: "file", Kind: inventory.TextHunk, PatchReference: "patch"}}, Patches: map[string][]byte{"patch": []byte(patch)}}})
	}
	m.openReviewTab(s)
	m.Width, m.Height = 120, 18
	key(m, '3')
	return m
}

func TestCommitsSelectionScrollAndIsolation(t *testing.T) {
	m := commitModel(t)
	m.Selected, m.Focus, m.Horizontal = 1, paneDiff, 7
	m.Scroll[1] = 5
	key(m, 'j')
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "Second commit") || strings.Contains(got, "+First commit") {
		t.Fatalf("selection did not replace diff: %s", got)
	}
	namedKey(m, tea.KeyEnter)
	key(m, 'd')
	saved := m.commit.offsets[strings.Repeat("b", 40)]
	if saved == 0 {
		t.Fatal("detail did not scroll")
	}
	key(m, 'p')
	key(m, 'n')
	if m.commit.offsets[strings.Repeat("b", 40)] != saved {
		t.Fatal("reading offset not restored")
	}
	for _, k := range []rune{'m', 'S', 'F', 'G', 'i', 'c', 'e'} {
		key(m, k)
	}
	namedKey(m, tea.KeyEnter)
	namedKey(m, tea.KeyTab)
	if m.Composer != nil || m.Selected != 1 || m.Focus != paneDiff || m.Horizontal != 7 || m.Scroll[1] != 5 || len(m.Session.ReviewedSliceIDs) != 0 {
		t.Fatal("commit controls changed main review state")
	}
	namedKey(m, tea.KeyEscape)
	if m.commit.focus != paneList || m.selectedReviewView() != viewCommits {
		t.Fatal("escape did not return to rail")
	}
	key(m, '1')
	if m.Focus != paneDiff || m.Scroll[1] != 5 {
		t.Fatal("return to diff lost state")
	}
}

func TestCommitsMouseAndNarrowEmptyMain(t *testing.T) {
	m := commitModel(t)
	m.Session.Inventory.Units = nil
	m.Update(tea.MouseClickMsg{X: 3, Y: 5, Button: tea.MouseLeft})
	if m.commit.selected != 1 {
		t.Fatal("click did not select second commit")
	}
	m.Update(tea.MouseClickMsg{X: 70, Y: 5, Button: tea.MouseLeft})
	if m.commit.focus != paneDiff {
		t.Fatal("click did not focus detail")
	}
	m.Update(tea.MouseWheelMsg{X: 70, Y: 5, Button: tea.MouseWheelDown})
	if m.commit.offsets[strings.Repeat("b", 40)] == 0 {
		t.Fatal("detail wheel did not scroll")
	}
	for _, w := range []int{60, 99, 100, 120} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 18})
		for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if visibleWidth(line) > w {
				t.Fatalf("width %d exceeded", w)
			}
		}
	}
	m.Width = 60
	m.commit.focus = paneList
	if !strings.Contains(ansi.Strip(m.View().Content), "Second commit") {
		t.Fatal("narrow rail missing")
	}
	namedKey(m, tea.KeyEnter)
	if !strings.Contains(ansi.Strip(m.View().Content), "+Second commit") {
		t.Fatal("narrow detail missing")
	}
}

func TestCommitsSafeStatesAndReadonlyRows(t *testing.T) {
	m := commitModel(t)
	for _, tc := range []struct {
		bundle *commits.Bundle
		want   string
	}{
		{nil, "Commits were not captured for this session."},
		{&commits.Bundle{Status: commits.Captured, Complete: true}, "No commits captured for this PR."},
		{&commits.Bundle{Status: commits.Unavailable, Reason: "bounded failure"}, "Commit list unavailable: bounded failure"},
	} {
		m.Session.Commits = tc.bundle
		m.commit = commitState{}
		if got := ansi.Strip(m.View().Content); !strings.Contains(got, tc.want) {
			t.Fatalf("safe state missing: %s", got)
		}
	}
	m.Session.Commits = &commits.Bundle{Status: commits.Captured, Entries: []commits.Entry{{SHA: strings.Repeat("a", 40), Status: commits.Unavailable, Reason: "object missing"}}}
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "Commit diff unavailable: object missing") || !strings.Contains(got, "more may exist") {
		t.Fatalf("unavailable/capped state missing: %s", got)
	}
	e := m.Session.Commits.Entries[0]
	e.Status = commits.Captured
	e.Diff = &commits.Diff{Complete: false, Files: []inventory.FileChange{{ID: "binary", NewPath: []byte("untrusted\x1b.go")}}, Units: []inventory.ReviewUnit{{FileChangeID: "binary", Kind: inventory.Binary}}}
	e.Parents = []string{strings.Repeat("c", 40), strings.Repeat("d", 40)}
	m.Session.Commits.Entries[0] = e
	m.commit.cache = commitRenderCache{}
	got := ansi.Strip(m.View().Content)
	if !strings.Contains(got, "Compared with first parent") || !strings.Contains(got, "INCOMPLETE commit diff") || !strings.Contains(got, "Binary content changed") || !strings.Contains(got, `untrusted\x1b.go`) {
		t.Fatalf("partial/merge/escaping state missing: %s", got)
	}
	for _, row := range m.commitRows() {
		if row.target != nil || row.commentID != 0 {
			t.Fatal("commit rows acquired PR comment targets")
		}
	}
}

func TestCommitsWorkspaceAndShortHeight(t *testing.T) {
	m := commitModel(t)
	first := m.Session
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	key(m, 'd')
	key(m, ']')
	saved := m.commit.offsets[strings.Repeat("b", 40)]
	width := m.commit.width
	second := screenSession()
	second.Inventory.Comparison.Metadata.Identity.Number = 999
	m.openReviewTab(second)
	key(m, '3')
	if m.commit.selected != 0 || m.commit.focus != paneList {
		t.Fatal("new tab inherited commits state")
	}
	m.activateTab(0)
	if m.Session != first || m.commit.selected != 1 || m.commit.focus != paneDiff || m.commit.width != width || m.commit.offsets[strings.Repeat("b", 40)] != saved {
		t.Fatal("workspace switch lost commit state")
	}
	m.Height = 7
	m.commit.focus = paneList
	m.commitRail()
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "› Second commit") {
		t.Fatalf("short height lost selected subject: %s", got)
	}
}

func TestProgramCommitNavigation(t *testing.T) {
	fixture := commitModel(t).Session
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return fixture, nil })
	h := runProgram(t, m)
	h.expect("loaded review", func(f programFrame) bool { return !f.loading && strings.Contains(f.text, "main.go") })
	h.key('3')
	h.expect("first commit diff", func(f programFrame) bool { return strings.Contains(f.text, "+First commit") })
	h.key('j')
	h.expect("second commit diff", func(f programFrame) bool {
		return strings.Contains(f.text, "+Second commit") && !strings.Contains(f.text, "+First commit")
	})
	h.key(tea.KeyEnter)
	h.key(tea.KeyEnter)
	h.expect("read-only commit detail", func(f programFrame) bool {
		return f.read == 0 && strings.Contains(f.text, "+Second commit") && !strings.Contains(f.text, "post now")
	})
	h.key('1')
	h.expect("main review restored", func(f programFrame) bool { return strings.Contains(f.text, "hello world") && f.read == 0 })
	h.quit()
}

func TestCommitsDividerWidthIndependent(t *testing.T) {
	m := commitModel(t)
	m.listWidthPreference = 28
	g := m.commitGeometry()
	initial := m.commitListWidth()
	m.Update(tea.MouseClickMsg{X: g.Divider.Min.X, Y: 4, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: g.Divider.Min.X + 7, Y: 4, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: g.Divider.Min.X + 7, Y: 4, Button: tea.MouseLeft})
	if m.commitListWidth() != initial+7 || m.listWidthPreference != 28 {
		t.Fatal("commit divider did not resize independently")
	}
	key(m, '[')
	if m.commitListWidth() != initial+5 {
		t.Fatal("keyboard and mouse do not share preference")
	}
	m.Width = 99
	key(m, ']')
	m.Width = 120
	if m.commitListWidth() != initial+5 {
		t.Fatal("hidden divider changed preference")
	}
}

func TestCommitsSHASelectionAndShortCappedNotice(t *testing.T) {
	m := commitModel(t)
	key(m, 'j')
	sha := m.commit.selectedSHA
	m.Session.Commits.Entries[0], m.Session.Commits.Entries[1] = m.Session.Commits.Entries[1], m.Session.Commits.Entries[0]
	m.commitSelection()
	if m.commit.selected != 0 || m.commit.selectedSHA != sha {
		t.Fatal("selection identity did not follow SHA")
	}
	m.Height = 7
	m.Session.Commits.Complete = false
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "more may exist") {
		t.Fatalf("short capped notice missing: %s", got)
	}
}

func TestCommitDiffRowsPreservesFileAndUnitOrder(t *testing.T) {
	d := &commits.Diff{Files: []inventory.FileChange{{ID: "b", NewPath: []byte("b.go")}, {ID: "a", NewPath: []byte("a.go")}}, Units: []inventory.ReviewUnit{{FileChangeID: "a", Kind: inventory.Unavailable, UnavailableReason: "a first"}, {FileChangeID: "b", Kind: inventory.Unavailable, UnavailableReason: "b first"}, {FileChangeID: "a", Kind: inventory.Unavailable, UnavailableReason: "a second"}, {FileChangeID: "b", Kind: inventory.Unavailable, UnavailableReason: "b second"}}}
	var text []string
	for _, row := range commitDiffRows(d) {
		text = append(text, row.Text)
	}
	got := strings.Join(text, "\n")
	previous := -1
	for _, want := range []string{"── b.go", "b first", "b second", "── a.go", "a first", "a second"} {
		index := strings.Index(got, want)
		if index <= previous {
			t.Fatalf("file/unit order changed: %s", got)
		}
		previous = index
	}
	m := commitModel(t)
	m.Width = 60
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "Commit diff · 2/2") {
		t.Fatalf("narrow detail selection count missing: %s", got)
	}
}

func TestCommitPatchAnchorsUseRawCounters(t *testing.T) {
	d := &commits.Diff{Complete: true, Files: []inventory.FileChange{{ID: "f", OldPath: []byte("old.go"), NewPath: []byte("new.go")}}, Units: []inventory.ReviewUnit{{FileChangeID: "f", Kind: inventory.TextHunk, PatchReference: "p"}}, Patches: map[string][]byte{"p": []byte("@@ -4,2 +7,2 @@\n-before\n+after\n context\n\\ No newline at end of file\n@@ -20 +30 @@\n+next\n")}}
	rows := commitDiffRowsFor(d, source.Identity{Repository: "o/r", Number: 1}, strings.Repeat("a", 40))
	var got []source.ReviewCommentTarget
	for _, row := range rows {
		if row.target != nil {
			got = append(got, *row.target)
		}
	}
	if len(got) != 4 || got[0].Path != "old.go" || got[0].Side != "LEFT" || got[0].Line != 4 || got[1].Path != "new.go" || got[1].Line != 7 || got[2].Line != 8 || got[3].Line != 30 {
		t.Fatalf("wrong raw anchors: %+v", got)
	}
	if commitPatchAnchor(source.Identity{}, "sha", []byte("bad\npath"), "RIGHT", 1) != nil {
		t.Fatal("unsafe path anchor")
	}
}

func TestHistoricalCommitComposerGateAndQueueIsolation(t *testing.T) {
	m := commitModel(t)
	m.commit.focus = paneDiff
	for i, row := range m.commitRows() {
		if row.target != nil {
			m.commitCursor()
			m.commit.cursors[m.commit.selectedSHA] = i
			break
		}
	}
	m.openCommitComposer()
	if m.Composer != nil || m.ActionError == nil {
		t.Fatal("unverified historical target must remain read-only")
	}
	target := source.ReviewCommentTarget{CommitID: strings.Repeat("a", 40), Path: "file.go", Side: "RIGHT", Line: 1}
	m.Composer = &commentComposer{Target: target, CommitSHA: target.CommitID, Draft: "draft", PendingIndex: -1}
	m.commentComposerKey(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if len(m.Pending) != 0 || m.Composer == nil || m.Composer.Draft != "draft" || m.ActionError == nil {
		t.Fatal("historical queue mutated draft or pending review")
	}
}

func TestCommitCommentUncertainDeliveryRetainsDraft(t *testing.T) {
	m := commitModel(t)
	m.Composer = &commentComposer{Target: source.ReviewCommentTarget{CommitID: strings.Repeat("a", 40)}, CommitSHA: strings.Repeat("a", 40), Draft: "retain this", PendingIndex: -1, generation: 3}
	m.applyCommentResult(CommentResult{Target: m.activeTab, Generation: 3, Err: source.ErrCommentDeliveryUnknown})
	if m.Composer == nil || m.Composer.Draft != "retain this" || !strings.Contains(m.notice, "refresh discussions before retrying") || len(m.Pending) != 0 {
		t.Fatal("uncertain delivery lost draft or refresh guidance")
	}
}

func TestCommitComposerKeepsEditorVisibleWithoutMainScroll(t *testing.T) {
	m := commitModel(t)
	m.Height = 12
	m.commit.focus = paneDiff
	inv := &m.Session.Inventory
	m.Session.Commits.Entries[0].SHA = inv.Comparison.Metadata.HeadSHA
	m.Session.Commits.Entries[0].Diff = &commits.Diff{Files: inv.Files, Units: inv.Units, Patches: inv.Patches, Complete: true}
	m.commit.selectedSHA = inv.Comparison.Metadata.HeadSHA
	m.commit.cache = commitRenderCache{}
	for i, row := range m.commitRows() {
		if row.target != nil {
			m.commitCursor()
			m.commit.cursors[m.commit.selectedSHA] = i
			m.commitOffset()
			m.commit.offsets[m.commit.selectedSHA] = max(0, i-m.bodyHeight()+1)
			break
		}
	}
	m.Scroll[0] = 3
	m.openCommitComposer()
	if m.Composer == nil {
		t.Fatal("head fixture composer not opened")
	}
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "▏") {
		t.Fatalf("editor hidden below viewport: %s", got)
	}
	for i := 0; i < 10; i++ {
		m.commentComposerKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	}
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "▏") {
		t.Fatalf("multiline editor cursor hidden: %s", got)
	}
	if m.Scroll[0] != 3 {
		t.Fatal("commit editor changed main scroll")
	}
}

func TestCommitKeyboardAfterWheelPreservesViewport(t *testing.T) {
	m := commitModel(t)
	m.commit.focus = paneDiff
	m.commitCursor()
	m.commit.cursors[m.commit.selectedSHA] = 0
	m.commitScroll(10)
	before := m.commitOffset()
	m.commitCursorMove(1)
	if m.commitOffset() < before {
		t.Fatalf("keyboard jumped from offset %d to %d", before, m.commitOffset())
	}
	if m.commitCursor() < m.commitOffset() {
		t.Fatal("cursor remains above viewport")
	}
}

func TestCommitComposerCanRefreshWithoutLosingUncertainDraft(t *testing.T) {
	m := commitModel(t)
	m.Composer = &commentComposer{CommitSHA: m.commitEntries()[0].SHA, Draft: "retain after uncertain post", PendingIndex: -1}
	reads := 0
	m.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
		reads++
		return DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: true}}, nil
	})
	cmd := m.commentComposerKey(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("composer cannot refresh before retrying")
	}
	m.Update(cmd())
	if reads != 1 || m.Composer == nil || m.Composer.Draft != "retain after uncertain post" {
		t.Fatal("refresh lost draft or posted")
	}
}
