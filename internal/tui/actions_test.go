package tui

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guideconfig"
	"prui/internal/review"
	"prui/internal/source"
)

func actionsModel(t *testing.T) *Model {
	t.Helper()
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(largeSession(3, 8))
	m.selectReviewView(viewFiles)
	m.Width, m.Height = 100, 30
	return m
}

func selectAction(t *testing.T, m *Model, query string) tea.Cmd {
	t.Helper()
	m.openActions()
	m.actionsInput(query)
	return m.actionsKey(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestActionsOpenFilterAndCancelPreserveReview(t *testing.T) {
	m := actionsModel(t)
	m.Selected, m.Row, m.Focus = 1, 1, paneDiff
	m.setOffset(3)
	m.openActions()
	if m.top() != pageActions {
		t.Fatal("actions did not open")
	}
	m.actionsInput("gItHuB")
	if view := ansi.Strip(m.actionsView()); !strings.Contains(view, "GitHub URL") || strings.Contains(view, "Switch pull request") {
		t.Fatalf("filter did not narrow names: %s", view)
	}
	m.actionsKey(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.actions.query != "gItHuBq" || m.top() != pageActions {
		t.Fatal("q escaped menu input ownership")
	}
	if cmd := m.actionsKey(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd != nil {
		t.Fatal("cancel returned work")
	}
	if m.top() != pageReview || m.Selected != 1 || m.Row != 1 || m.Focus != paneDiff || m.offset() != 3 {
		t.Fatal("cancel changed reading position")
	}
}

func TestActionsQueryIsUnicodeBoundedAndControlSafe(t *testing.T) {
	m := actionsModel(t)
	m.openActions()
	m.actionsInput("é界\x1b[31m\n\t\r")
	if !utf8.ValidString(m.actions.query) || strings.ContainsAny(m.actions.query, "\x1b\n\r\t") {
		t.Fatalf("unsafe query %q", m.actions.query)
	}
	m.actions.query = "é界"
	m.actionsKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.actions.query != "é" {
		t.Fatalf("backspace split a rune: %q", m.actions.query)
	}
	m.actionsInput(strings.Repeat("界", 10000))
	if utf8.RuneCountInString(m.actions.query) > 256 {
		t.Fatal("query not bounded")
	}
}

func TestActionsArrowSelectionAndEmptyResults(t *testing.T) {
	m := actionsModel(t)
	m.openActions()
	m.actionsInput("view")
	for i, action := range m.matchingActions() {
		if action.label == "View Files" {
			for range i {
				m.actionsKey(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			break
		}
	}
	m.actionsKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.top() != pageReview || m.selectedReviewView() != viewFiles {
		t.Fatal("down/enter did not select Files view")
	}
	m.openActions()
	m.actionsInput("no-such-action-12345")
	if !strings.Contains(m.actionsView(), "No matching actions") {
		t.Fatal("empty result not explained")
	}
	if cmd := m.actionsKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || m.top() != pageActions {
		t.Fatal("empty result dispatched work")
	}
}

func TestActionsGuideUsesExistingConsentWithoutGenerating(t *testing.T) {
	m := actionsModel(t)
	called := false
	m.SetGuideLifecycle(func(context.Context, *review.Session, guideconfig.Selection, func(string)) (*review.Session, error) {
		called = true
		return nil, nil
	})
	if cmd := selectAction(t, m, "Generate guide"); cmd != nil || called || m.top() != pageGuideConsent {
		t.Fatal("guide action bypassed existing consent")
	}
}

func TestActionsSubmitReviewOpensUnconfirmedFormFromCommits(t *testing.T) {
	m := actionsModel(t)
	m.selectReviewView(viewCommits)
	if cmd := selectAction(t, m, "Submit review"); cmd != nil || m.top() != pageReviewSubmit || m.ReviewForm == nil || m.ReviewForm.Confirm {
		t.Fatal("review action did not preserve confirmation")
	}
}

func TestActionsUnavailableOperationStaysInMenuWithReason(t *testing.T) {
	m := actionsModel(t)
	m.openActions()
	m.actionsInput("New comparison")
	if !strings.Contains(m.actionsView(), "unavailable") {
		t.Fatal("unavailable operation has no reason")
	}
	if cmd := m.actionsKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || m.top() != pageActions || m.Busy {
		t.Fatal("disabled action started work")
	}
}

func TestActionsFilterFilesFromOverviewAndFindFromGuide(t *testing.T) {
	m := actionsModel(t)
	m.selectReviewView(viewDescription)
	selectAction(t, m, "Filter files")
	if m.top() != pageReview || m.selectedReviewView() != viewFiles || !m.fileFilterEditing {
		t.Fatal("filter action did not focus filename field")
	}
	m.fileFilterEditing = false
	m.selectReviewView(viewGuide)
	selectAction(t, m, "Find code")
	if m.top() != pageReview || !m.searchOpen() {
		t.Fatal("find action did not open code search")
	}
}

func TestActionsViewportKeepsSelectedActionVisible(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {40, 12}, {20, 8}, {14, 5}, {1, 1}} {
		m := actionsModel(t)
		m.Width, m.Height = size[0], size[1]
		m.openActions()
		for range 30 {
			m.actionsKey(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		view := m.actionsView()
		assertFitsViewport(t, view, m.Width, m.Height)
		if m.Width >= 20 && !strings.Contains(ansi.Strip(view), "›") {
			t.Fatalf("selection disappeared at %dx%d: %s", m.Width, m.Height, view)
		}
	}
}

func TestActionsExposeContextDisplayToolsWithoutChangingSourceOnOpen(t *testing.T) {
	m := actionsModel(t)
	m.openActions()
	m.actionsInput("whitespace")
	if !strings.Contains(m.actionsView(), "Toggle whitespace") || m.navigation.whitespace {
		t.Fatal("display action missing or opening changed whitespace")
	}
	m.actionsKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.navigation.whitespace || m.top() != pageReview {
		t.Fatal("whitespace action did not dispatch")
	}
	m.selectReviewView(viewDescription)
	m.openActions()
	m.actionsInput("whitespace")
	if !strings.Contains(m.actionsView(), "No matching actions") {
		t.Fatal("context-only action shown outside diff")
	}
}

func TestActionsMouseActivatesVisibleRowAndIgnoresChrome(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {40, 12}, {14, 5}} {
		m := actionsModel(t)
		m.Width, m.Height = size[0], size[1]
		m.openActions()
		m.actionsInput("GitHub URL")
		view := ansi.Strip(m.actionsView())
		rows := strings.Split(view, "\n")
		x, y := -1, -1
		for row, line := range rows {
			if col := strings.LastIndex(line, "›"); col >= 0 && strings.Contains(line, "GitHub URL") {
				x, y = utf8.RuneCountInString(line[:col]), row
				break
			}
		}
		if x < 0 {
			t.Fatalf("no visible mouse target: %s", view)
		}
		m.actionsMouseClick(x, max(0, y-1))
		if m.top() != pageActions {
			t.Fatal("query chrome activated action")
		}
		m.actionsMouseClick(x, y)
		if m.top() != pageURL {
			t.Fatalf("visible action did not activate at %dx%d (%d,%d):\n%s", m.Width, m.Height, x, y, view)
		}
	}
}

func TestActionsCommentEntriesPreserveCanonicalTargetsAndNeverPost(t *testing.T) {
	for _, action := range []string{"Add line comment", "Compose suggestion", "Add file comment"} {
		t.Run(action, func(t *testing.T) {
			m := rangeTestModel()
			t.Cleanup(m.Close)
			selectRawTarget(t, m, "RIGHT", 1)
			target := *m.commentSelectionTarget()
			m.SetCommentSubmitter(func(context.Context, CommentSubmission) (source.ReviewComment, error) {
				t.Fatal("opening a composer posted a comment")
				return source.ReviewComment{}, nil
			})
			selectAction(t, m, action)
			if m.Composer == nil {
				t.Fatalf("%s did not open a composer: %v", action, m.ActionError)
			}
			if action == "Add file comment" {
				if m.Composer.Target.SubjectType != "file" || m.Composer.Target.Path != target.Path {
					t.Fatal("file target changed")
				}
			} else if m.Composer.Target != target {
				t.Fatal("comment target changed")
			}
			if (action == "Compose suggestion") != m.Composer.Suggestion {
				t.Fatal("wrong composer mode")
			}
		})
	}
}

func TestActionsCommentRangeAndReadOnlySubsetGuards(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "RIGHT", 1)
	selectAction(t, m, "Start or cancel comment range")
	if m.rangeStart == nil || m.rangeStart.Line != 1 {
		t.Fatal("range start was not captured")
	}
	selectRawTarget(t, m, "RIGHT", 2)
	selectAction(t, m, "Add line comment")
	if m.Composer == nil || m.Composer.Target.StartLine != 1 || m.Composer.Target.Line != 2 {
		t.Fatal("range provenance lost")
	}
	m.Composer = nil
	m.commitFilter.subset = true
	m.openActions()
	m.actionsInput("Add line comment")
	if !strings.Contains(m.actionsView(), "No matching actions") {
		t.Fatal("read-only subset exposed comment action")
	}
}

func TestActionsGeneralCommentOpensEditorWithoutPosting(t *testing.T) {
	m := actionsModel(t)
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		t.Fatal("opening PR comment posted")
		return source.ConversationEvent{}, nil
	})
	selectAction(t, m, "Add PR comment")
	if m.top() != pageReview || m.selectedReviewView() != viewDescription || m.discussions.editor == nil {
		t.Fatal("general comment did not open beside Overview discussions")
	}
}
