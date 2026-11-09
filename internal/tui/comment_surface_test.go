package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
	"prui/internal/theme"
)

func TestSelectedCommentUsesBorderInsteadOfRowHighlight(t *testing.T) {
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		m := largeModel(screenSession(), 160, 30)
		m.layout = layout
		palette, _ := theme.Resolve(theme.CatppuccinMocha, nil)
		m.SetTheme(palette)
		m.colorProfile = colorprofile.TrueColor
		var target *source.ReviewCommentTarget
		for _, line := range m.displayDetail() {
			if line.target != nil {
				target = line.target
				break
			}
		}
		if target == nil {
			t.Fatal("missing source target")
		}
		m.Comments = []source.ReviewComment{{ID: 42, Target: *target, Body: "selected body", Author: "reviewer"}}
		for i, line := range m.displayDetail() {
			if strings.Contains(line.Text, "selected body") {
				m.setCursor(i)
				m.cursorActive = true
				break
			}
		}
		view := m.View().Content
		if !strings.Contains(view, "┌") || !strings.Contains(view, "└") {
			t.Fatal("selected comment lacks solid corners")
		}
		cells := themeCanvasBuffer(view, m.Width, m.Height)
		found := false
		for y, row := range strings.Split(ansi.Strip(view), "\n") {
			body := strings.Index(row, "selected body")
			if body < 0 {
				continue
			}
			found = true
			x := visibleWidth(row[:body])
			if cells.CellAt(x, y).Style.Attrs&uv.AttrReverse != 0 {
				t.Fatal("selected comment still uses reverse row highlight")
			}
			border := strings.LastIndex(row[:body], "│")
			if border < 0 {
				t.Fatal("selected comment lacks solid side border")
			}
			left := visibleWidth(row[:border])
			if cells.CellAt(left, y).Content != "│" {
				t.Fatal("selected comment lacks solid side border")
			}
			if cells.CellAt(left, y-2).Content != "┌" || cells.CellAt(left, y+1).Content != "└" {
				t.Fatal("selected comment lacks solid corners")
			}
			if !sameCanvasColor(cells.CellAt(left, y).Style.Fg, themeBaseColor(palette, theme.FocusedBorder)) {
				t.Fatal("selected border lacks focus color")
			}
			if !sameCanvasColor(cells.CellAt(x, y).Style.Bg, modalBackground(palette)) {
				t.Fatal("comment background changed on selection")
			}
		}
		if !found {
			t.Fatal("comment body not rendered")
		}
		m.Close()
	}
}

func TestFocusedAgentCardUsesSolidHighlightedBorder(t *testing.T) {
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		m := agentTestModel(t)
		m.layout = layout
		m.Height = 40
		palette, _ := theme.Resolve(theme.CatppuccinMocha, nil)
		m.SetTheme(palette)
		m.colorProfile = colorprofile.TrueColor
		m.SetAgentAnswerer(fixtureAgentAnswer)
		selectRawTarget(t, m, "RIGHT", 1)
		openInlineAgentForTest(m)
		if m.Composer == nil || m.agent.ActiveThread().ID == 0 {
			t.Fatalf("layout %v: agent composer did not open: %v", layout, m.ActionError)
		}
		m.agent.SetDraft(m.agent.ActiveThread().ID, "What changed?")
		cmd := sendInlineAgentForTest(m)
		if cmd == nil {
			t.Fatalf("layout %v: agent question was not sent: %v", layout, m.ActionError)
		}
		m.Update(cmd())
		threadID := m.agent.ActiveThread().ID
		for i, row := range m.displayDetail() {
			if row.agentThreadID == threadID && strings.Contains(row.Text, "Answer") {
				m.setCursor(i)
				m.cursorActive = true
				break
			}
		}
		view := m.View().Content
		if !strings.Contains(ansi.Strip(view), "┌") || !strings.Contains(ansi.Strip(view), "└") {
			t.Fatalf("layout %v: focused agent card lacks solid corners", layout)
		}
		cells := themeCanvasBuffer(view, m.Width, m.Height)
		found := false
		for y, row := range strings.Split(ansi.Strip(view), "\n") {
			body := strings.Index(row, "The selected code changes behavior.")
			if body < 0 {
				continue
			}
			found = true
			border := strings.LastIndexAny(row[:body], "|│")
			if border < 0 {
				t.Fatalf("layout %v: focused agent card lacks side border", layout)
			}
			left := visibleWidth(row[:border])
			if cells.CellAt(left, y).Content != "│" || !sameCanvasColor(cells.CellAt(left, y).Style.Fg, themeBaseColor(palette, theme.FocusedBorder)) {
				t.Fatalf("layout %v: agent border is not highlighted", layout)
			}
		}
		if !found {
			t.Fatalf("layout %v: agent answer not visible", layout)
		}
		m.Close()
	}
}
