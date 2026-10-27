package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestActionsHasNoFindFieldAndUsesDisplayedHotkeys(t *testing.T) {
	m := actionsModel(t)
	key(m, ' ')
	if strings.Contains(ansi.Strip(m.View().Content), "Find:") {
		t.Fatal("Actions still shows a Find field")
	}
	key(m, '4')
	if m.top() != pageReview || m.selectedReviewView() != viewCommits {
		t.Fatal("displayed 4 hotkey did not open Commits")
	}
	key(m, ' ')
	key(m, 'R')
	if m.top() != pageReviewSubmit || m.ReviewForm == nil || m.ReviewForm.Confirm {
		t.Fatal("R did not open the existing unconfirmed review form")
	}
}

func TestActionsKeepsUppercaseLowercaseAndModifierHotkeysDistinct(t *testing.T) {
	m := actionsModel(t)
	m.openActions()
	key(m, 's')
	if m.top() != pageActions || m.layout != diffLayoutUnified {
		t.Fatal("disabled saved-reviews hotkey changed the diff layout")
	}
	key(m, 'S')
	if m.top() != pageReview || m.layout != diffLayoutSideBySide {
		t.Fatal("S did not toggle side-by-side layout")
	}
	m.openActions()
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if m.top() != pageReadiness {
		t.Fatal("Alt+R did not open readiness")
	}
}

func TestActionsPasteCannotExecuteHotkeysOrRequestClipboard(t *testing.T) {
	m := actionsModel(t)
	m.openActions()
	for _, text := range []string{"R", "4", "ctrl+v", "é\nq"} {
		_, cmd := m.Update(tea.PasteMsg{Content: text})
		if cmd != nil || m.top() != pageActions || m.ReviewForm != nil {
			t.Fatal("paste activated an action")
		}
	}
	if field, _ := m.textInput(); field != nil {
		t.Fatal("Actions still owns a text field")
	}
	key(m, 'q')
	if m.top() != pageActions {
		t.Fatal("unknown key escaped menu ownership")
	}
}

func TestActionsRangeHotkeyRetainsCanonicalTarget(t *testing.T) {
	m := rangeTestModel()
	t.Cleanup(m.Close)
	selectRawTarget(t, m, "RIGHT", 1)
	target := *m.commentSelectionTarget()
	m.openActions()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	if cmd != nil || m.top() != pageReview || m.rangeStart == nil || *m.rangeStart != target {
		t.Fatal("Ctrl+V requested clipboard input or lost the captured range target")
	}
}

func TestActionsNarrowMenuKeepsHotkeyBeforeClippedLabel(t *testing.T) {
	m := actionsModel(t)
	m.Width, m.Height = 24, 10
	m.openActions()
	highlightAction(t, m, "Add file comment")
	view := ansi.Strip(m.actionsView())
	if !strings.Contains(view, "› [Ctrl+F]") {
		t.Fatalf("narrow menu clipped the action hotkey: %s", view)
	}
}

func TestActionsJKScrollSelectionAndClampAtBoundaries(t *testing.T) {
	m := actionsModel(t)
	m.Width, m.Height = 100, 10
	m.openActions()
	actions := m.reviewActions()
	for _, navigation := range []struct {
		key          rune
		start, delta int
	}{
		{key: 'j', start: 0, delta: 1},
		{key: 'k', start: len(actions) - 1, delta: -1},
	} {
		for step := 1; step <= len(actions)+1; step++ {
			want := min(max(0, navigation.start+step*navigation.delta), len(actions)-1)
			key(m, navigation.key)
			if m.top() != pageActions || m.actions.index != want {
				t.Fatalf("%c: page = %v, index = %d; want Actions index %d", navigation.key, m.top(), m.actions.index, want)
			}
			label := actions[want].label
			if actions[want].shortcut != "" {
				label = "[" + actions[want].shortcut + "] " + label
			}
			if !strings.Contains(ansi.Strip(m.actionsBody()), "› "+label) {
				t.Fatalf("%c: selected action %q is not visible", navigation.key, label)
			}
		}
	}
}
