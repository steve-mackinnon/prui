package tui

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	"prui/internal/source"
)

func TestMouseReviewSelectsScrolledListWithoutActivation(t *testing.T) {
	m := largeModel(largeSession(30, 30), 100, 10)
	m.Selected = 20
	list, _ := m.reviewListPresentation()
	want := list[1].row
	m.mouseReviewClick(2, 4)
	if m.Selected != want || m.Focus != paneList {
		t.Fatalf("selection=%d focus=%v", m.Selected, m.Focus)
	}
	if m.Composer != nil || m.CommentMenu != nil {
		t.Fatal("click activated editor")
	}
}

func TestMouseReviewSplitCellsPreserveExactTarget(t *testing.T) {
	s := screenSession()
	s.Inventory.Files[0].OldPath = []byte("old-main.go")
	m := largeModel(s, 160, 16)
	m.layout = diffLayoutSideBySide
	lines := m.displayDetail()
	index := -1
	for i, line := range lines {
		if line.sideBySide != nil && len(rowTargets(*line.sideBySide)) == 2 {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("missing paired fixture")
	}
	targets := rowTargets(*lines[index].sideBySide)
	g := m.workspaceGeometry()
	for _, tc := range []struct {
		x      int
		target source.ReviewCommentTarget
	}{{g.Detail.Min.X, targets[0]}, {g.Detail.Max.X - 2, targets[1]}} {
		m.mouseReviewClick(tc.x, g.Detail.Min.Y+index)
		target, _ := m.cursorAnchor()
		if target == nil || *target != tc.target {
			t.Fatalf("click %d target=%+v want=%+v", tc.x, target, tc.target)
		}
		if m.Composer != nil || m.CommentMenu != nil {
			t.Fatal("click activated")
		}
		key(m, 'S')
		target, _ = m.cursorAnchor()
		if target == nil || *target != tc.target {
			t.Fatalf("unified reflow lost target: %+v", target)
		}
		key(m, 'S')
	}
	m.mouseReviewClick(g.Detail.Min.X, g.Detail.Min.Y+index)
	namedKey(m, tea.KeyEnter)
	if m.Composer == nil || m.Composer.Target != targets[0] {
		t.Fatalf("composer wrong target: %+v", m.Composer)
	}
}

func TestMouseReviewStructuralAndEmptyCellsDoNotSelect(t *testing.T) {
	m := largeModel(screenSession(), 160, 16)
	m.layout = diffLayoutSideBySide
	before, _ := m.cursorAnchor()
	g := m.workspaceGeometry()
	m.mouseReviewClick(g.Detail.Min.X, g.Detail.Min.Y)
	after, _ := m.cursorAnchor()
	if *before != *after || m.Focus != paneList {
		t.Fatal("structural row changed selection")
	}
}

func TestMouseReviewContextEmptySeparatorAndCommentRows(t *testing.T) {
	s := screenSession()
	s.Inventory.Files[0].OldPath = []byte("main.go")
	s.Inventory.Patches["patch"] = []byte("@@ -1,2 +1,3 @@\n context\n-old\n+new\n+extra\n")
	m := largeModel(s, 160, 24)
	m.layout = diffLayoutSideBySide
	g := m.workspaceGeometry()
	old, new := splitCellBounds(m.detailWidth())
	lines := m.displayDetail()
	for i, line := range lines {
		if line.sideBySide == nil || line.sideBySide.full != nil {
			continue
		}
		row := line.sideBySide
		x := g.Detail.Min.X + new.Min.X
		m.mouseReviewClick(x, g.Detail.Min.Y+i)
		target, _ := m.cursorAnchor()
		if target == nil || row.new == nil || *target != *row.new.line.target {
			t.Fatal("right click missed actual target")
		}
		if row.old == nil || row.old.line.target == nil {
			m.mouseReviewClick(g.Detail.Min.X, g.Detail.Min.Y+i)
			after, _ := m.cursorAnchor()
			if *after != *target {
				t.Fatal("empty/context old cell invented a target")
			}
		}
		m.mouseReviewClick(g.Detail.Min.X+old.Max.X+1, g.Detail.Min.Y+i)
		after, _ := m.cursorAnchor()
		if *after != *target {
			t.Fatal("separator selected a source target")
		}
	}
	target, _ := m.cursorAnchor()
	m.Comments = []source.ReviewComment{{ID: 42, Target: *target, Body: "comment body", Author: "reviewer"}}
	for i, line := range m.displayDetail() {
		if line.commentID == 42 {
			m.mouseReviewClick(g.Detail.Min.X+5, g.Detail.Min.Y+i)
			got, id := m.cursorAnchor()
			if got != nil || id != 42 || m.CommentMenu != nil {
				t.Fatalf("comment click got %v %d", got, id)
			}
			namedKey(m, tea.KeyEnter)
			if m.CommentMenu == nil || m.CommentMenu.CommentID != 42 {
				t.Fatal("Enter did not open selected comment menu")
			}
			return
		}
	}
	t.Fatal("missing comment row")
}

func TestMouseReviewVisibleTabsAndNarrowPane(t *testing.T) {
	m := largeModel(screenSession(), 99, 12)
	m.openReviewTab(m.Session)
	m.selectReviewView(viewFiles)
	m.mouseReviewClick(15, 1)
	if m.selectedReviewView() != viewDescription {
		t.Fatal("description tab missed")
	}
	m.mouseReviewClick(44, 1)
	if m.Files || m.Inventory {
		t.Fatal("guide tab missed")
	}
	m.mouseReviewClick(33, 1)
	if !m.Files {
		t.Fatal("file tab missed")
	}
	m.Focus = paneDiff
	before := m.Selected
	m.mouseReviewClick(2, 3)
	if m.Selected != before || m.Focus != paneDiff {
		t.Fatal("hidden rail intercepted click")
	}
}

func TestMouseReviewUnifiedOffsetAndHorizontalDoNotRetarget(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 159, 12)
	m.Focus = paneDiff
	m.setOffset(10)
	m.Horizontal = 3
	lines := m.displayDetail()
	for i := 10; i < 10+m.bodyHeight(); i++ {
		if lines[i].target == nil {
			continue
		}
		m.mouseReviewClick(m.workspaceGeometry().Detail.Min.X+2, 3+i-10)
		got, _ := m.cursorAnchor()
		if got == nil || *got != *lines[i].target {
			t.Fatalf("wrong offset target %+v", got)
		}
		return
	}
	t.Fatal("missing visible target")
}

func TestMouseReviewPaddingDoesNotSelect(t *testing.T) {
	m := largeModel(screenSession(), 120, 16)
	before, _ := m.cursorAnchor()
	for i, line := range m.displayDetail() {
		if line.target == nil {
			continue
		}
		m.mouseReviewClick(119, 3+i)
		after, _ := m.cursorAnchor()
		if *before != *after || m.Focus != paneList {
			t.Fatal("trailing padding changed selection")
		}
	}
}

func TestMouseReviewSplitSelectionIsTabOwned(t *testing.T) {
	s := screenSession()
	s.Inventory.Files[0].OldPath = []byte("main.go")
	m := largeModel(s, 160, 16)
	m.openReviewTab(s)
	m.selectReviewView(viewFiles)
	m.layout = diffLayoutSideBySide
	g := m.workspaceGeometry()
	for i, line := range m.displayDetail() {
		if line.sideBySide == nil || len(rowTargets(*line.sideBySide)) != 2 {
			continue
		}
		want := rowTargets(*line.sideBySide)[0]
		m.mouseReviewClick(g.Detail.Min.X, g.Detail.Min.Y+i)
		other := screenSession()
		other.Inventory.Comparison.Metadata.Identity.Number = 43
		m.openReviewTab(other)
		m.selectReviewView(viewFiles)
		m.activateTab(0)
		got, _ := m.cursorAnchor()
		if got == nil || *got != want {
			t.Fatalf("tab restore lost target: %+v", got)
		}
		return
	}
	t.Fatal("missing paired row")
}

func TestMouseReviewInactiveTabResizeKeepsSplitTarget(t *testing.T) {
	s := screenSession()
	s.Inventory.Files[0].OldPath = []byte("main.go")
	m := largeModel(s, 160, 16)
	m.openReviewTab(s)
	m.selectReviewView(viewFiles)
	m.layout = diffLayoutSideBySide
	g := m.workspaceGeometry()
	_, right := splitCellBounds(m.detailWidth())
	for i, line := range m.displayDetail() {
		if line.sideBySide == nil || len(rowTargets(*line.sideBySide)) != 2 {
			continue
		}
		want := rowTargets(*line.sideBySide)[1]
		m.mouseReviewClick(g.Detail.Min.X+right.Min.X, g.Detail.Min.Y+i)
		other := screenSession()
		other.Inventory.Comparison.Metadata.Identity.Number = 43
		m.openReviewTab(other)
		m.selectReviewView(viewFiles)
		m.Update(tea.WindowSizeMsg{Width: 159, Height: 16})
		m.activateTab(0)
		got, _ := m.cursorAnchor()
		if got == nil || *got != want {
			t.Fatalf("inactive resize lost RIGHT target: %+v; want %+v", got, want)
		}
		namedKey(m, tea.KeyEnter)
		if m.Composer == nil || m.Composer.Target != want {
			t.Fatalf("inactive resize composer target: %+v", m.Composer)
		}
		return
	}
	t.Fatal("missing paired row")
}

func TestMouseReviewInactiveTabResizeKeepsCommentID(t *testing.T) {
	s := screenSession()
	m := largeModel(s, 160, 16)
	m.openReviewTab(s)
	m.selectReviewView(viewFiles)
	m.layout = diffLayoutSideBySide
	target, _ := m.cursorAnchor()
	m.Comments = []source.ReviewComment{{ID: 42, Target: *target, Body: "saved comment", Author: "reviewer"}}
	g := m.workspaceGeometry()
	for i, line := range m.displayDetail() {
		if line.commentID != 42 {
			continue
		}
		m.mouseReviewClick(g.Detail.Min.X+5, g.Detail.Min.Y+i)
		other := screenSession()
		other.Inventory.Comparison.Metadata.Identity.Number = 43
		m.openReviewTab(other)
		m.selectReviewView(viewFiles)
		m.Update(tea.WindowSizeMsg{Width: 159, Height: 16})
		m.activateTab(0)
		got, id := m.cursorAnchor()
		if got != nil || id != 42 {
			t.Fatalf("inactive resize lost comment: %+v %d", got, id)
		}
		namedKey(m, tea.KeyEnter)
		if m.CommentMenu == nil || m.CommentMenu.CommentID != 42 {
			t.Fatalf("inactive resize comment menu: %+v", m.CommentMenu)
		}
		return
	}
	t.Fatal("missing comment row")
}

func TestMouseReviewDoubleClickCommentAndRailActivation(t *testing.T) {
	m := largeModel(screenSession(), 120, 24)
	m.selectReviewView(viewFiles)
	g := m.workspaceGeometry()
	list, _ := m.reviewListPresentation()
	for i, item := range list {
		if item.row < 0 {
			continue
		}
		m.mouseReviewClick(g.Rail.Min.X+2, g.Rail.Min.Y+i)
		if m.Focus != paneList {
			t.Fatal("single click focused detail")
		}
		m.mouseReviewClick(g.Rail.Min.X+2, g.Rail.Min.Y+i)
		if m.Focus != paneDiff {
			t.Fatal("double click did not focus detail")
		}
		break
	}
	m.resetMouseActivation()
	for i, line := range m.displayDetail() {
		if line.target == nil || line.target.Side != "RIGHT" || i >= g.Detail.Dy() {
			continue
		}
		x, y := g.Detail.Min.X+2, g.Detail.Min.Y+i
		m.mouseReviewClick(x, y)
		if m.Composer != nil {
			t.Fatal("single click opened editor")
		}
		m.mouseReviewClick(x, y)
		if m.Composer == nil || m.Composer.Target != *line.target {
			t.Fatal("double click failed exact target editor")
		}
		return
	}
	t.Fatal("fixture lacks visible target")
}

func TestMouseReviewHorizontalWheelSplitAndFiltered(t *testing.T) {
	m := largeModel(screenSession(), 160, 24)
	m.selectReviewView(viewFiles)
	m.layout = diffLayoutSideBySide
	g := m.workspaceGeometry()
	m.mouseWheel(tea.MouseWheelMsg{X: g.Detail.Min.X, Y: g.Detail.Min.Y, Button: tea.MouseWheelRight})
	if m.Horizontal != mouseWheelStep {
		t.Fatal("split diff did not pan")
	}
	m.mouseWheel(tea.MouseWheelMsg{X: g.Detail.Min.X, Y: g.Detail.Min.Y, Button: tea.MouseWheelLeft})
	if m.Horizontal != 0 {
		t.Fatal("left wheel did not restore pan")
	}
}

func TestMouseRangeFooterComposeAndCancel(t *testing.T) {
	for _, action := range []string{"enter: compose", "esc: cancel"} {
		m := rangeTestModel()
		selectRawTarget(t, m, "RIGHT", 1)
		target := *m.selectedDiffTarget()
		m.rangeStart = &target
		m.Width, m.Height = 160, 24
		hints := ansi.Strip(m.reviewHints())
		at := strings.Index(hints, action)
		if at < 0 {
			t.Fatal("missing range action")
		}
		y := len(strings.Split(m.reviewView(), "\n")) - 1
		m.Update(tea.MouseClickMsg{X: visibleWidth(hints[:at]) + 1, Y: y, Button: tea.MouseLeft})
		if m.rangeStart != nil {
			t.Fatal("range control did not activate guarded handler")
		}
		if action == "enter: compose" && m.Composer == nil {
			t.Fatal("compose did not open editor")
		}
		if action == "esc: cancel" && m.Composer != nil {
			t.Fatal("cancel opened editor")
		}
	}
}

func TestMouseRangeFooterDoesNotInterpretErrorAsControls(t *testing.T) {
	m := rangeTestModel()
	selectRawTarget(t, m, "RIGHT", 1)
	target := *m.selectedDiffTarget()
	m.rangeStart = &target
	m.Width, m.Height = 180, 24
	m.ActionError = errors.New("enter: compose · esc: cancel")
	hints := ansi.Strip(m.reviewHints())
	at := strings.Index(hints, "enter: compose")
	y := len(strings.Split(m.reviewView(), "\n")) - 1
	m.Update(tea.MouseClickMsg{X: visibleWidth(hints[:at]) + 1, Y: y, Button: tea.MouseLeft})
	if m.rangeStart == nil || m.Composer != nil {
		t.Fatal("error text activated range controls")
	}
}
