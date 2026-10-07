package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestOverviewConversationDiscoverableAndDirectJump(t *testing.T) {
	m := commitModel(t)
	body := strings.Repeat("long description\n", 50)
	m.Session.PullRequestDescription = &body
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Author: "alice", Body: "Please review this"}}}}
	key(m, '1')
	if !strings.Contains(ansi.Strip(m.View().Content), "Discussions [D]") {
		t.Fatal("conversation shortcut hidden by long description")
	}
	key(m, 'D')
	if m.top() != pageReview || m.selectedReviewView() != viewDescription || !strings.Contains(ansi.Strip(m.View().Content), "Please review this") {
		t.Fatal("D did not reveal conversation inside Overview", m.View().Content)
	}
	before := m.DescriptionScroll
	key(m, 'D')
	if m.DescriptionScroll != before {
		t.Fatal("repeated jump lost position")
	}
	key(m, '2')
	key(m, '1')
	if m.DescriptionScroll != before {
		t.Fatal("tab round trip lost position")
	}
}

func TestOverviewInlineContextFilesReplyAndReturn(t *testing.T) {
	m := rangeTestModel()
	t.Cleanup(m.Close)
	m.selectReviewView(viewFiles)
	var target source.ReviewCommentTarget
	for _, row := range m.cachedFileDetail(false) {
		if row.target != nil {
			target = *row.target
			break
		}
	}
	if target.Path == "" {
		t.Fatal("fixture lacks target")
	}
	no := false
	thread := source.Discussion{ID: "thread", CurrentAnchor: &target, Outdated: &no, Comments: []source.ReviewComment{{ID: 321, Target: target, Author: "alice", Body: "Check this"}, {ID: 322, ParentID: 321, Target: target, Author: "bob", Body: "Reply here"}}}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Threads: []source.Discussion{thread}}}})
	m.fileFilter = "hidden"
	m.fileFilterEditing = true
	m.selectReviewView(viewDescription)
	oldSelected := m.Selected
	key(m, 'D')
	namedKey(m, tea.KeyTab) // next activity is the reply
	text := ansi.Strip(m.View().Content)
	if !strings.Contains(text, "captured PR diff") || !strings.Contains(text, "Open in Files") {
		t.Fatal("missing inline context", text)
	}
	key(m, 'f')
	if m.selectedReviewView() != viewFiles || m.Focus != paneDiff || m.fileFilter != "" || m.fileFilterEditing {
		t.Fatal("Files jump failed")
	}
	_, id := m.cursorAnchor()
	if id != 322 {
		t.Fatalf("selected comment %d, want reply 322", id)
	}
	namedKey(m, tea.KeyEnter)
	if m.CommentMenu == nil {
		t.Fatal("selected reply cannot open actions")
	}
	namedKey(m, tea.KeyEscape)
	namedKey(m, tea.KeyEscape)
	if m.selectedReviewView() != viewDescription || m.discussions.selectedID != "inline:322" || m.fileFilter != "hidden" || !m.fileFilterEditing || m.Selected != oldSelected {
		t.Fatal("return lost origin or Files state")
	}
}

func TestOverviewExtendedTargetsAndUnavailableContexts(t *testing.T) {
	for _, kind := range []string{"LEFT", "RIGHT", "range", "file", "outdated", "mismatch", "retained"} {
		for _, split := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: " unified", true: " split"}[split], func(t *testing.T) {
				m := rangeTestModel()
				t.Cleanup(m.Close)
				if split {
					m.layout = diffLayoutSideBySide
				}
				side := "RIGHT"
				if kind == "LEFT" {
					side = "LEFT"
				}
				selectRawTarget(t, m, side, 2)
				target := *m.selectedDiffTarget()
				if kind == "range" {
					target.StartLine = 1
					target.StartSide = side
				}
				if kind == "file" {
					target.SubjectType = "file"
					target.Line = 0
					target.Side = ""
				}
				no, yes := false, true
				thread := source.Discussion{ID: "thread", CurrentAnchor: &target, Outdated: &no, Resolved: &no, Comments: []source.ReviewComment{{ID: 901, Target: target, Body: "Inspect the target"}}, DiffHunk: "+historical only"}
				if kind == "outdated" {
					thread.Outdated = &yes
				}
				if kind == "retained" {
					thread.Retained = true
				}
				m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: DiscussionSnapshot{CurrentVerified: kind != "mismatch", Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Threads: []source.Discussion{thread}}}})
				key(m, 'D')
				text := ansi.Strip(m.View().Content)
				key(m, 'f')
				if kind == "outdated" || kind == "mismatch" || kind == "retained" {
					if m.selectedReviewView() != viewDescription || strings.Contains(text, "captured PR diff") || !strings.Contains(text, "Historical snippet") {
						t.Fatal("unverified thread relocated or lost history", text)
					}
				} else {
					if m.selectedReviewView() != viewFiles {
						t.Fatal("extended target cannot open Files", text)
					}
					_, id := m.cursorAnchor()
					if id != 901 {
						t.Fatalf("selected %d instead of target comment", id)
					}
					if kind == "file" && !strings.Contains(text, "File comment") {
						t.Fatal("file comment has no path context", text)
					}
					if kind == "range" && (strings.Contains(text, "start 1 → end 2") || !strings.Contains(text, target.Path)) {
						t.Fatal("range context label should show only the path", text)
					}
				}
			})
		}
	}
}

func TestOverviewMouseActionsAndRefreshIdentity(t *testing.T) {
	m := rangeTestModel()
	t.Cleanup(m.Close)
	selectRawTarget(t, m, "RIGHT", 1)
	target := *m.selectedDiffTarget()
	no := false
	thread := source.Discussion{ID: "thread", CurrentAnchor: &target, Outdated: &no, Comments: []source.ReviewComment{{ID: 801, Target: target, Body: "Mouse navigation"}}}
	snapshot := DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Threads: []source.Discussion{thread}}}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: snapshot})
	key(m, '1')
	m.overviewMouseClick(2)
	if !m.discussions.overviewFocus {
		t.Fatal("visible section shortcut cannot be clicked")
	}
	for i, row := range m.overviewRows() {
		if row.action == "files" {
			m.DescriptionScroll = max(0, i-2)
			m.overviewMouseClick(3 + i - m.DescriptionScroll)
			break
		}
	}
	if m.selectedReviewView() != viewFiles {
		t.Fatal("Files action mouse hit missed")
	}
	namedKey(m, tea.KeyEscape)
	snapshot.Snapshot.Events = []source.ConversationEvent{{ID: "PR comment:early", Kind: "PR comment", Body: "New earlier activity"}}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: snapshot})
	if m.discussions.selectedID != "inline:801" {
		t.Fatal("refresh replaced selected identity")
	}
	// Empty/incomplete and unavailable must remain visibly different from no activity.
	m.discussions.snapshot.Snapshot = source.DiscussionSnapshot{Complete: false, Timeline: true}
	m.discussions.selectedID = ""
	m.DescriptionScroll = 0
	if !strings.Contains(m.overviewView(), "No activity loaded") || strings.Contains(m.overviewView(), "No discussions.") {
		t.Fatal("partial data presented as empty")
	}
}

func TestProgramOverviewFilesReplyReturn(t *testing.T) {
	m := rangeTestModel()
	selectRawTarget(t, m, "RIGHT", 1)
	target := *m.selectedDiffTarget()
	no := false
	thread := source.Discussion{ID: "thread", CurrentAnchor: &target, Outdated: &no, Comments: []source.ReviewComment{{ID: 777, Author: "alice", Target: target, Body: "Program navigation"}}}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Threads: []source.Discussion{thread}}}})
	h := runProgram(t, m)
	h.expect("Files", func(f programFrame) bool { return f.page == pageReview && strings.Contains(f.text, "› Files [2]") })
	h.key('D')
	h.expect("Overview activity", func(f programFrame) bool {
		return f.page == pageReview && strings.Contains(f.text, "Open in Files [f]")
	})
	h.key('f')
	h.key(tea.KeyEnter)
	h.expect("selected comment menu", func(f programFrame) bool { return strings.Contains(f.text, "Comment actions:") })
	h.key('r')
	h.expect("reply editor", func(f programFrame) bool { return strings.Contains(f.text, "Reply editor open") })
	h.key(tea.KeyEscape)
	h.key(tea.KeyEscape)
	h.expect("returned to activity", func(f programFrame) bool {
		return strings.Contains(f.text, "› Overview [1]") && strings.Contains(f.text, "Program navigation")
	})
	h.quit()
}

func TestOverviewInactiveStoredDraftDoesNotClaimNavigation(t *testing.T) {
	m := rangeTestModel()
	t.Cleanup(m.Close)
	m.selectReviewView(viewDescription)
	m.discussions.editor = &generalCommentEditor{draft: "stored draft", cursor: 12}
	key(m, '2')
	if m.selectedReviewView() != viewFiles || m.discussions.editor.draft != "stored draft" {
		t.Fatal("inactive stored draft intercepted tab selection")
	}
	key(m, 'D')
	key(m, 't')
	key(m, 'q')
	key(m, '2')
	if m.selectedReviewView() != viewDescription || m.discussions.editor == nil || m.discussions.editor.draft != "stored drafttq2" {
		t.Fatal("active editor released text keys", m.discussions.editor)
	}
}

func TestOverviewOriginalJumpBeforeBrowsingCommits(t *testing.T) {
	m := commitModel(t)
	key(m, '1')
	m.commit = commitState{}
	sha := m.commitEntries()[1].SHA
	target := source.ReviewCommentTarget{Identity: m.Session.Inventory.Comparison.Metadata.Identity, CommitID: sha, Path: "changed.go", Line: 12, Side: "RIGHT"}
	thread := testDiscussion("historical", sha)
	thread.OriginalAnchor = &target
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{thread}}}
	key(m, 'D')
	key(m, 'o')
	if m.selectedReviewView() != viewCommits || m.commit.selectedSHA != sha || m.commit.focus != paneDiff {
		t.Fatal("did not open original commit diff")
	}
	cursor := m.commitCursor()
	if row := m.commitRows()[cursor]; row.target == nil || *row.target != target {
		t.Fatal("did not select original comment anchor")
	}
	if m.commit.offsets[sha] != max(0, cursor-2) {
		t.Fatal("did not scroll to original comment anchor")
	}
	namedKey(m, tea.KeyEscape)
	if m.selectedReviewView() != viewDescription || !m.discussions.overviewFocus {
		t.Fatal("did not restore overview discussion")
	}
}

func TestOverviewOriginalReturnWithFilteredFiles(t *testing.T) {
	m := commitModel(t)
	m.commitFilter.subset = true
	sha := m.commitEntries()[1].SHA
	thread := testDiscussion("historical", sha)
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{thread}}}
	key(m, 'D')
	key(m, 'o')
	if m.selectedReviewView() != viewCommits {
		t.Fatal("original jump failed")
	}
	namedKey(m, tea.KeyEscape)
	if m.selectedReviewView() != viewDescription || !m.commitFilter.subset {
		t.Fatal("filtered Files state prevented original return")
	}
}

func TestOverviewThreadFallbackShowsReplyAuthors(t *testing.T) {
	m := commitModel(t)
	thread := testDiscussion("thread", m.commitEntries()[0].SHA)
	thread.Comments = append(thread.Comments, source.ReviewComment{ID: 2, ParentID: 1, Author: "bob", Body: "Follow-up concern"})
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{thread}}}
	key(m, 'D')
	var text strings.Builder
	for _, row := range m.overviewRows() {
		text.WriteString(ansi.Strip(row.text) + "\n")
	}
	if !strings.Contains(text.String(), "@bob") || !strings.Contains(text.String(), "Follow-up concern") {
		t.Fatal("thread-only fallback lost reply attribution")
	}
}

func TestOverviewCardsKeyboardSelection(t *testing.T) {
	m := commitModel(t)
	m.selectReviewView(viewDescription)
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "a", Kind: "PR comment", Body: strings.Repeat("First line\n", 20)}, {ID: "b", Kind: "PR comment", Body: "Second"}}}}
	key(m, 'D')
	start := m.discussions.overviewCursor
	key(m, 'j')
	if m.discussions.overviewCursor != start+1 || m.discussions.selectedID != "a" {
		t.Fatal("cursor skipped a row or activity")
	}
	for m.discussions.selectedID != "b" {
		key(m, 'j')
		if m.discussions.overviewCursor > 200 {
			t.Fatal("never selected second activity")
		}
	}
	text := ansi.Strip(m.overviewView())
	if !strings.Contains(text, "▸") || !strings.Contains(text, "Selected") {
		t.Fatal("cursor or selection missing", text)
	}
	namedKey(m, tea.KeyEnter)
	if !m.discussions.overviewExpanded["b"] || m.discussions.selectedID != "b" {
		t.Fatal("Enter did not open cursor activity")
	}
	namedKey(m, tea.KeyEscape)
	key(m, 'k')
	if m.discussions.overviewFocus {
		t.Fatal("blank row retained actionable selection")
	}
	for _, row := range m.overviewRows() {
		if ansi.StringWidth(row.text) > m.Width {
			t.Fatal("card exceeds viewport")
		}
	}
}

func TestOverviewPageScrollPreservesCursorScreenRow(t *testing.T) {
	m := commitModel(t)
	m.selectReviewView(viewDescription)
	body := strings.Repeat("description line\n", 100)
	m.Session.PullRequestDescription = &body
	m.Width, m.Height = 80, 20
	m.DescriptionScroll = 10
	m.discussions.overviewCursor, m.discussions.overviewCursorActive = 14, true
	screenRow := 4
	for _, input := range []string{"d", "u", "pgdown", "pgup", "u", "u", "d"} {
		before := m.DescriptionScroll
		cursor := m.discussions.overviewCursor
		m.overviewKey(input)
		if m.discussions.overviewCursor-cursor != m.DescriptionScroll-before {
			t.Fatal("cursor and viewport moved different amounts", input)
		}
		if m.discussions.overviewCursor-m.DescriptionScroll != screenRow {
			t.Fatal("page scrolling moved cursor on screen", input)
		}
	}
	m.DescriptionScroll = max(0, len(m.overviewRows())-m.overviewHeight()-2)
	m.discussions.overviewCursor = m.DescriptionScroll + screenRow
	m.overviewKey("d")
	if m.discussions.overviewCursor-m.DescriptionScroll != screenRow {
		t.Fatal("clamped page scroll moved cursor on screen")
	}
	m.overviewKey("d")
	if m.discussions.overviewCursor-m.DescriptionScroll != screenRow {
		t.Fatal("bottom boundary moved cursor")
	}
}

func TestOverviewCardBackgroundFillsStyledRows(t *testing.T) {
	m := commitModel(t)
	m.Width = 80
	m.colorProfile = colorprofile.TrueColor
	m.discussions.selectedID, m.discussions.overviewFocus = "comment", true
	for _, id := range []string{"", "comment"} {
		rows := m.overviewCards([]overviewRow{{text: "Heading \x1b[31mcode\x1b[0m tail \x1b[49mend", id: id}})
		cells := themeCanvasBuffer(rows[1].text, m.Width, 1)
		expected := cells.CellAt(1, 0).Style.Bg
		if expected == nil {
			t.Fatal("card has no background")
		}
		for x := 1; x < m.Width-1; x++ {
			bg := cells.CellAt(x, 0).Style.Bg
			if bg == nil {
				t.Fatalf("background missing at column %d", x)
			}
			r, g, b, a := bg.RGBA()
			er, eg, eb, ea := expected.RGBA()
			if r != er || g != eg || b != eb || a != ea {
				t.Fatalf("background differs at column %d", x)
			}
		}
		if cells.CellAt(10, 0).Style.Fg == nil {
			t.Fatal("Markdown foreground lost")
		}
	}
}

func TestOverviewDetailHintsMatchActions(t *testing.T) {
	m := publishedModel(t)
	entries := m.discussionEntries()
	var inline, general source.Discussion
	for _, entry := range entries {
		if entry.Kind == "PR comment" {
			general = entry
		} else {
			inline = entry
		}
	}
	hints := strings.Join(m.overviewDiscussionActions(inline), " · ")
	if !strings.Contains(hints, "e: edit") || !strings.Contains(hints, "z: resolve") || strings.Contains(hints, "f: Files") {
		t.Fatal("inline actions incorrect", hints)
	}
	m.Viewer = "bob"
	if strings.Contains(strings.Join(m.overviewDiscussionActions(inline), " · "), "e: edit") {
		t.Fatal("edit advertised for another author")
	}
	yes := true
	inline.Resolved, inline.CanUnresolve = &yes, &yes
	if !strings.Contains(strings.Join(m.overviewDiscussionActions(inline), " · "), "z: reopen") {
		t.Fatal("reopen missing")
	}
	if strings.Contains(strings.Join(m.overviewDiscussionActions(general), " · "), "f: Files") {
		t.Fatal("general comment advertises Files")
	}
	m.discussions.overviewExpanded = map[string]bool{inline.ID: true}
	m.discussions.overviewFocus = true
	m.discussions.selectedID = inline.ID
	m.revealOverviewActivity()
	if !strings.Contains(ansi.Strip(m.overviewView()), "Actions ·") {
		t.Fatal("detail action row missing")
	}
	m.submitPublished = nil
	if strings.Contains(strings.Join(m.overviewDiscussionActions(inline), " · "), "z:") {
		t.Fatal("offline resolve advertised")
	}
}

func TestOverviewGeneralComposerKeepsFeedVisible(t *testing.T) {
	m := commitModel(t)
	m.selectReviewView(viewDescription)
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		return source.ConversationEvent{}, nil
	})
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Author: "alice", Body: "Surrounding discussion"}}}}
	key(m, 'D')
	key(m, 'r')
	text := ansi.Strip(m.overviewView())
	if !strings.Contains(text, "Surrounding discussion") || !strings.Contains(text, "@alice") || m.discussions.detail {
		t.Fatal("reply replaced feed", text)
	}
	namedKey(m, tea.KeyEscape)
	if m.discussions.editor != nil || m.selectedReviewView() != viewDescription {
		t.Fatal("cancel left feed")
	}
	key(m, 'n')
	if !strings.Contains(ansi.Strip(m.overviewView()), "Surrounding discussion") {
		t.Fatal("new comment replaced feed")
	}
	editor := m.discussions.editor
	m.applyGeneralCommentResult(GeneralCommentResult{Target: m.activeTab, Session: m.Session, Editor: editor, Event: source.ConversationEvent{ID: "PR comment:2", Kind: "PR comment", Author: "me", Body: "Posted"}})
	if m.discussions.detail || m.discussions.editor != nil {
		t.Fatal("posting switched to detail")
	}
}

func TestOverviewInlineReplyStaysInFeedAndUsesRoot(t *testing.T) {
	m := rangeTestModel()
	t.Cleanup(m.Close)
	var target source.ReviewCommentTarget
	for _, row := range m.cachedFileDetail(false) {
		if row.target != nil {
			target = *row.target
			break
		}
	}
	no := false
	root := source.ReviewComment{ID: 501, Author: "alice", Body: "Root comment", Target: target}
	reply := source.ReviewComment{ID: 502, ParentID: 501, Author: "bob", Body: "Existing reply", Target: target}
	m.SetCommentActionSubmitter(func(context.Context, CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, nil
	})
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Threads: []source.Discussion{{ID: "thread", CurrentAnchor: &target, Outdated: &no, Comments: []source.ReviewComment{root, reply}}}}}})
	m.selectReviewView(viewDescription)
	key(m, 'D')
	namedKey(m, tea.KeyTab)
	key(m, 'r')
	if m.selectedReviewView() != viewDescription || m.discussions.detail || m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply {
		t.Fatal("reply left feed")
	}
	if m.CommentMenu.ReplyToID != 501 || m.CommentMenu.RootAnchor == nil {
		t.Fatal("reply lost root")
	}
	if text := ansi.Strip(m.overviewView()); !strings.Contains(text, "Existing reply") || !strings.Contains(text, "Reply to code thread") {
		t.Fatal("composer lost context", text)
	}
	key(m, 'x')
	if m.CommentMenu.Draft != "x" {
		t.Fatal("editor did not receive input")
	}
	namedKey(m, tea.KeyEscape)
	if m.CommentMenu != nil || m.discussions.selectedID != "inline:502" {
		t.Fatal("cancel lost selection")
	}
}

func TestOverviewNewCommentComposerAtEnd(t *testing.T) {
	m := commitModel(t)
	m.Width, m.Height = 80, 20
	m.selectReviewView(viewDescription)
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		return source.ConversationEvent{}, nil
	})
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Body: strings.Repeat("First comment\n", 20)}, {ID: "PR comment:2", Kind: "PR comment", Body: "Final discussion"}}}}
	key(m, 'D')
	key(m, 'n')
	rows := m.overviewRows()
	lastDiscussion, firstEditor := -1, -1
	for i, row := range rows {
		if row.id == "PR comment:2" {
			lastDiscussion = i
		}
		if row.action == "editor" && firstEditor < 0 {
			firstEditor = i
		}
	}
	if firstEditor <= lastDiscussion {
		t.Fatal("new PR composer is not after all discussions")
	}
	if m.DescriptionScroll != max(0, len(rows)-m.overviewHeight()) {
		t.Fatal("n did not scroll to bottom")
	}
	if !strings.Contains(ansi.Strip(m.overviewView()), "New PR comment") {
		t.Fatal("composer not visible")
	}
}

func TestOverviewEnterExpandsInPlace(t *testing.T) {
	m := commitModel(t)
	m.Width, m.Height = 100, 60
	m.selectReviewView(viewDescription)
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Body: "Before discussion"}, {ID: "PR comment:2", Kind: "PR comment", Body: "<details><summary>More</summary>Hidden content</details>"}, {ID: "PR comment:3", Kind: "PR comment", Body: "After discussion"}}}}
	key(m, 'D')
	namedKey(m, tea.KeyTab)
	scroll, cursor := m.DescriptionScroll, m.discussions.overviewCursor
	namedKey(m, tea.KeyEnter)
	text := ansi.Strip(strings.Join(func() []string {
		var lines []string
		for _, row := range m.overviewRows() {
			lines = append(lines, row.text)
		}
		return lines
	}(), "\n"))
	for _, want := range []string{"Description", "Before discussion", "Hidden content", "After discussion"} {
		if !strings.Contains(text, want) {
			t.Fatal("expansion removed surrounding content", want, text)
		}
	}
	if m.discussions.detail || m.DescriptionScroll != scroll || m.discussions.overviewCursor != cursor {
		t.Fatal("expansion changed page or position")
	}
	namedKey(m, tea.KeyEnter)
	for _, row := range m.overviewRows() {
		if strings.Contains(row.text, "Hidden content") {
			t.Fatal("Enter did not collapse")
		}
	}
}

func TestOverviewMouseDescriptionClearsDiscussionSelection(t *testing.T) {
	m := commitModel(t)
	m.selectReviewView(viewDescription)
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Body: "Comment"}}}}
	key(m, 'D')
	m.DescriptionScroll = 0
	m.overviewMouseClick(3)
	if m.discussions.overviewFocus {
		t.Fatal("description cursor still targets a discussion")
	}
	namedKey(m, tea.KeyEnter)
	if m.discussions.overviewExpanded["PR comment:1"] {
		t.Fatal("Enter expanded a discussion away from cursor")
	}
}

func TestOverviewRefreshKeepsCursorOnActivity(t *testing.T) {
	m := commitModel(t)
	m.selectReviewView(viewDescription)
	snapshot := source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:2", Kind: "PR comment", Body: "Selected comment"}}}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: snapshot}})
	key(m, 'D')
	key(m, 'j')
	relative := m.discussions.overviewCursor - m.DescriptionScroll
	snapshot.Events = append(snapshot.Events, source.ConversationEvent{ID: "PR comment:1", Kind: "PR comment", Body: "Earlier comment"})
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: snapshot}})
	rows := m.overviewRows()
	if rows[m.discussions.overviewCursor].id != "PR comment:2" || m.discussions.overviewCursor-m.DescriptionScroll != relative {
		t.Fatal("refresh detached cursor from selected activity")
	}
}
