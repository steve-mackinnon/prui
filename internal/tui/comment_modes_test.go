package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"prui/internal/agentchat"
	"prui/internal/guideconfig"
	"prui/internal/session"
	"strings"
	"testing"
)

func TestAgentComposerCyclesSecondAndSendsPrivately(t *testing.T) {
	m := agentTestModel(t)
	defer m.Close()
	calls := 0
	m.SetAgentAnswerer(func(ctx context.Context, choice guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		calls++
		return fixtureAgentAnswer(ctx, choice, manifest)
	})
	m.openCommentComposer()
	if m.Composer == nil || commentType(m.Composer) != "Comment" {
		t.Fatal("comment is not the default")
	}
	namedKey(m, tea.KeyTab)
	if !m.Composer.Agent || commentType(m.Composer) != "Ask agent" || !strings.Contains(m.commentTypeSelector(), "[Ask agent]") {
		t.Fatal("Ask agent is not second", m.commentTypeSelector())
	}
	for _, r := range "Why?" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if m.Composer.Draft != "Why?\n" {
		t.Fatalf("Shift+Enter lost newline: %q", m.Composer.Draft)
	}
	m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if len(m.Pending) != 0 || m.Composer == nil || m.Composer.Draft != "Why?\n" {
		t.Fatal("Agent Ctrl+P queued or discarded draft")
	}
	if d := draftContent(m.reviewTabState); d.Composer != nil && strings.Contains(d.Composer.Body, "Why?") {
		t.Fatal("private Ask agent entered durable composer")
	}
	namedKey(m, tea.KeyTab)
	if !m.Composer.Suggestion {
		t.Fatal("Suggestion did not follow Agent")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !m.Composer.Agent || m.Composer.Draft != "Why?\n" {
		t.Fatalf("Agent draft not restored: %+v", m.Composer)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || calls != 0 {
		t.Fatalf("Enter did not dispatch one explicit private request: cmd=%v calls=%d", cmd != nil, calls)
	}
	m.Update(cmd())
	if calls != 1 || len(m.Pending) != 0 {
		t.Fatalf("private Send crossed GitHub queue: calls=%d pending=%d", calls, len(m.Pending))
	}
}

func TestAgentComposerLeftSideAndEscapeRetainsPrivateDraft(t *testing.T) {
	m := agentTestModel(t)
	defer m.Close()
	m.SetAgentAnswerer(fixtureAgentAnswer)
	selectRawTarget(t, m, "LEFT", 1)
	m.openCommentComposer()
	namedKey(m, tea.KeyTab)
	if m.Composer == nil || !m.Composer.Agent || m.Composer.Target.Side != "LEFT" || m.Composer.Suggestion {
		t.Fatal("LEFT deletion did not offer Agent second")
	}
	for _, r := range "private question" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	namedKey(m, tea.KeyEscape)
	if m.Composer != nil || len(m.Pending) != 0 {
		t.Fatal("Escape submitted or retained inline editor")
	}
	selectRawTarget(t, m, "LEFT", 1)
	m.openCommentComposer()
	namedKey(m, tea.KeyTab)
	if m.Composer == nil || !m.Composer.Agent || m.Composer.Draft != "private question" {
		t.Fatalf("private draft lost on reopen: %+v", m.Composer)
	}
	if d := draftContent(m.reviewTabState); d.Composer != nil && strings.Contains(d.Composer.Body, "private question") {
		t.Fatal("Agent draft persisted into GitHub comment")
	}
}

func TestAgentComposerFailedSendRetainsDraftForRetry(t *testing.T) {
	m := agentTestModel(t)
	defer m.Close()
	m.SetAgentAnswerer(func(context.Context, guideconfig.Selection, agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
		return agentchat.ResolvedAnswer{}, errors.New("synthetic provider failure")
	})
	m.openCommentComposer()
	namedKey(m, tea.KeyTab)
	if m.Composer == nil || !m.Composer.Agent {
		t.Fatal("Agent mode unavailable")
	}
	m.Composer.Draft = "What changed?"
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("failed request was not dispatched")
	}
	m.Update(cmd())
	if m.Composer != nil {
		t.Fatal("completed failed request left input overlay open")
	}
	selectRawTarget(t, m, "RIGHT", 1)
	m.openCommentComposer()
	namedKey(m, tea.KeyTab)
	if m.Composer == nil || !m.Composer.Agent || m.Composer.Draft != "What changed?" {
		t.Fatalf("failed request lost retry draft: %+v", m.Composer)
	}
}

func TestCommentTypeSelectorCyclesAndPreservesDrafts(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "RIGHT", 1)
	namedKey(m, tea.KeyEnter)
	c := m.Composer
	if c == nil {
		t.Fatal("Enter did not open editor")
	}
	target := c.Target
	view := m.View().Content
	for _, label := range []string{"[Comment]", "Suggestion", "File comment", "tab: type"} {
		if !strings.Contains(view, label) {
			t.Fatalf("missing %q in %s", label, view)
		}
	}
	c.Draft, c.Cursor = "comment draft", 13
	namedKey(m, tea.KeyTab)
	if !c.Suggestion || c.Draft != "first" || c.Target != target {
		t.Fatalf("suggestion: %+v", c)
	}
	c.Draft, c.Cursor = "replacement", 11
	namedKey(m, tea.KeyTab)
	if c.Suggestion || c.Target.SubjectType != "file" || c.Draft != "" {
		t.Fatalf("file: %+v", c)
	}
	c.Draft = "file draft"
	namedKey(m, tea.KeyTab)
	if c.Suggestion || c.Target != target || c.Draft != "comment draft" {
		t.Fatalf("comment restored: %+v", c)
	}
	namedKey(m, tea.KeyTab)
	if !c.Suggestion || c.Draft != "replacement" {
		t.Fatalf("replacement lost: %+v", c)
	}
	namedKey(m, tea.KeyTab)
	if c.Draft != "file draft" {
		t.Fatal("file draft lost")
	}
}

func TestCommentTypeCyclingKeepsEditorVisible(t *testing.T) {
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		for _, reverse := range []bool{false, true} {
			m := rangeTestModel()
			m.layout, m.Height = layout, 14
			selectRawTarget(t, m, "RIGHT", 3)
			namedKey(m, tea.KeyEnter)
			for i := 0; i < 6; i++ {
				msg := tea.KeyPressMsg{Code: tea.KeyTab}
				if reverse {
					msg.Mod = tea.ModShift
				}
				m.Update(msg)
				start, end := -1, -1
				for row, line := range m.displayDetail() {
					if line.editor {
						if start < 0 {
							start = row
						}
						end = row + 1
					}
				}
				if start < 0 || end > m.offset()+m.bodyHeight() || end-start <= m.bodyHeight() && start < m.offset() {
					t.Fatalf("layout=%v reverse=%v type=%s: editor [%d,%d) outside viewport [%d,%d)", layout, reverse, commentType(m.Composer), start, end, m.offset(), m.offset()+m.bodyHeight())
				}
			}
			m.Close()
		}
	}
}

func TestFileCommentReturnsToOriginalLine(t *testing.T) {
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		for _, tc := range []struct {
			draft     string
			startLine int
		}{{"short draft", 0}, {strings.Repeat("draft\n", 12), 0}, {strings.Repeat("draft\n", 12), 2}} {
			m := rangeTestModel()
			defer m.Close()
			m.layout, m.Height = layout, 14
			selectRawTarget(t, m, "RIGHT", 3)
			namedKey(m, tea.KeyEnter)
			if tc.startLine != 0 {
				m.Composer.Target.StartLine, m.Composer.Target.StartSide = tc.startLine, "RIGHT"
			}
			target := m.Composer.Target
			m.Composer.Draft = tc.draft
			m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
			if m.Composer.Target.SubjectType != "file" {
				t.Fatal("did not switch to file comment")
			}
			namedKey(m, tea.KeyTab)
			if m.Composer.Target != target || m.Composer.Draft != tc.draft {
				t.Fatal("original comment was not restored")
			}
			row := -1
			endpoint := target
			endpoint.StartLine, endpoint.StartSide = 0, ""
			for i, line := range m.displayDetail() {
				if diffLineHasTarget(line, endpoint) {
					row = i
					break
				}
			}
			if row < m.offset() || row >= m.offset()+m.bodyHeight() || row == m.offset() && stickyFileHeader(m.displayDetail(), m.offset()) >= 0 {
				t.Fatalf("layout=%v: target row %d hidden by viewport [%d,%d)", layout, row, m.offset(), m.offset()+m.bodyHeight())
			}
			if m.cursor() != row {
				t.Fatalf("layout=%v: cursor %d did not return to target row %d", layout, m.cursor(), row)
			}
		}
	}
}

func TestCommentTypeSelectorSkipsUnsupportedSuggestions(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "LEFT", 1)
	m.openCommentComposer()
	if m.Composer == nil {
		t.Fatal("comment unavailable")
	}
	if strings.Contains(m.View().Content, "[Comment] · Suggestion") {
		t.Fatal("old-side suggestion offered")
	}
	namedKey(m, tea.KeyTab)
	if m.Composer.Suggestion || m.Composer.Target.SubjectType != "file" {
		t.Fatal("did not skip unsupported suggestion")
	}
}

func TestCommentTypeSelectorCannotChangeUncertainDelivery(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "RIGHT", 1)
	m.openCommentComposer()
	m.draft.attempt = "comment"
	namedKey(m, tea.KeyTab)
	if m.Composer.Suggestion || m.Composer.Target.SubjectType != "" || m.ActionError == nil {
		t.Fatal("uncertain payload changed")
	}
}

func TestCommentTypeDraftsSurviveRestart(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := rangeTestModel()
	defer m.Close()
	m.Session.ID = strings.Repeat("d", 32)
	meta := &m.Session.Inventory.Comparison.Metadata
	meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
	m.SetLifecycle(store, nil, nil)
	selectRawTarget(t, m, "RIGHT", 1)
	m.openCommentComposer()
	original := m.Composer.Target
	m.Composer.Draft = "comment draft"
	namedKey(m, tea.KeyTab)
	m.Composer.Draft = "replacement"
	namedKey(m, tea.KeyTab)
	m.Composer.Draft = "file draft"
	if !m.persistDraft(m.reviewTabState) {
		t.Fatal(m.ActionError)
	}
	restored := New(context.Background(), nil)
	defer restored.Close()
	restored.SetLifecycle(store, nil, nil)
	restored.openReviewTab(m.Session)
	namedKey(restored, tea.KeyEnter)
	if restored.Composer == nil || restored.Composer.Draft != "file draft" {
		t.Fatal("file editor lost")
	}
	namedKey(restored, tea.KeyTab)
	if restored.Composer.Target != original || restored.Composer.Draft != "comment draft" {
		t.Fatal("line draft lost")
	}
	namedKey(restored, tea.KeyTab)
	if !restored.Composer.Suggestion || restored.Composer.Draft != "replacement" {
		t.Fatal("suggestion lost")
	}
	namedKey(restored, tea.KeyTab)
	restored.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !restored.Composer.Suggestion {
		t.Fatal("Shift+Tab did not cycle backwards")
	}
}
