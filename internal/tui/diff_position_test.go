package tui

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/source"
)

// Exercise projection changes at the preservation boundary; input routing is
// covered by the existing window, keyboard, mouse and line-number regressions.
func TestDiffPositionTransitionMatrix(t *testing.T) {
	policies := []struct {
		name   string
		policy diffPositionPolicy
	}{
		{"window", positionWindowResize}, {"keyboard", positionKeyboardResize},
		{"mouse", positionMouseResize}, {"gutter", positionGutterToggle}, {"display", positionDisplayToggle},
	}
	for _, view := range []reviewView{viewFiles, viewGuide} {
		for _, event := range policies {
			for _, selection := range []string{"LEFT", "RIGHT", "comment", "source", "search", "source search", "reply"} {
				t.Run(fmt.Sprintf("view=%d/%s/%s", view, event.name, selection), func(t *testing.T) {
					long := strings.Repeat("wrapping words ", 30)
					patch := "@@ -1,2 +1,2 @@\n-" + long + "old\n+" + long + "new\n tail\n"
					m := navigationModel(patch, long+"old\ntail\nreadonly needle\n", long+"new\ntail\nreadonly needle\n", inventory.Range{Start: 1, Count: 2}, inventory.Range{Start: 1, Count: 2})
					defer m.Close()
					m.Session.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Guide", Sections: []guide.Section{{Title: "Section", UnitIDs: []string{m.Session.Inventory.Units[0].ID}}}}}}
					m.selectReviewView(view)
					m.layout = diffLayoutSideBySide
					m.Focus, m.cursorActive = paneDiff, true
					readonly := strings.HasPrefix(selection, "source")
					if readonly {
						if view == viewFiles {
							m.navigation.mode = "NEW"
						} else {
							m.guideExpanded = true
						}
					}
					if strings.Contains(selection, "search") {
						query := "new"
						if readonly {
							query = "needle"
						}
						// Seed a selected occurrence at the preservation seam. Input and
						// scope routing remain covered by the existing search regressions.
						state := m.searchState()
						state.query = query
						for _, row := range m.displayDetail() {
							for _, cell := range positionTestCells(row) {
								if cell.searchID.Unit != "" && strings.Contains(cell.rawSource, query) {
									state.documents = []searchDocument{{ID: cell.searchID, Text: cell.rawSource}}
									start := strings.Index(cell.rawSource, query)
									state.matches = []searchMatch{{Range: searchRange{Start: start, End: start + len(query)}}}
								}
							}
						}
						m.revealSearchMatch()
						if !m.searchMatchAtCursor() {
							t.Fatal("fixture search missing")
						}
					} else if readonly {
						found := false
						for i, row := range m.displayDetail() {
							cells := positionTestCells(row)
							for _, cell := range cells {
								if cell.rawSource == "readonly needle" {
									m.setCursor(i)
									found = true
									break
								}
							}
						}
						if !found {
							t.Fatal("fixture source missing")
						}
					} else {
						side := selection
						if side != "LEFT" {
							side = "RIGHT"
						}
						var target *source.ReviewCommentTarget
						for _, row := range m.displayDetail() {
							for _, cell := range positionTestCells(row) {
								if cell.target != nil && cell.target.Side == side && cell.target.Line == 1 {
									copy := *cell.target
									target = &copy
									break
								}
							}
							if target != nil {
								break
							}
						}
						if target == nil {
							t.Fatal("fixture target missing")
						}
						m.restoreCursorAnchor(target, 0)
						if selection == "comment" || selection == "reply" {
							m.Comments = []source.ReviewComment{{ID: 42, Target: *target, Body: "comment", Author: "reviewer"}}
							m.restoreCursorAnchor(nil, 42)
							if selection == "reply" {
								m.CommentMenu = &commentActionMenu{CommentID: 42, Target: *target, mode: commentActionReply, Draft: "private reply", Cursor: 7}
							}
						}
					}
					target, commentID := m.cursorAnchor()
					var sourceID searchSourceID
					if readonly {
						for _, cell := range positionTestCells(m.displayDetail()[m.cursor()]) {
							if cell.searchID.Unit != "" {
								sourceID = cell.searchID
								break
							}
						}
						if sourceID.Unit == "" || target != nil {
							t.Fatal("fixture read-only identity missing")
						}
					}
					raw := bytes.Clone(m.Session.Inventory.Patches["p"])
					blobs := map[string][]byte{}
					for id, blob := range m.Session.Inventory.FullSource.Blobs {
						blobs[id] = bytes.Clone(blob)
					}
					for _, width := range []int{70, 210, 100, 180} {
						m.preserveDiffPosition(event.policy, func() { m.Width = width; m.hideLineNumbers = !m.hideLineNumbers; m.diffWrapCache = diffWrapCache{} })
						got, id := m.cursorAnchor()
						if !reflect.DeepEqual(got, target) || id != commentID {
							t.Fatalf("width %d: anchor %v/%d -> %v/%d", width, target, commentID, got, id)
						}
						if readonly {
							found := false
							for _, cell := range positionTestCells(m.displayDetail()[m.cursor()]) {
								found = found || cell.searchID == sourceID
							}
							if !found {
								t.Fatalf("width %d: lost read-only identity %v", width, sourceID)
							}
						}
						if strings.Contains(selection, "search") && (!m.searchMatchAtCursor() || !strings.Contains(m.View().Content, map[bool]string{true: "needle", false: "new"}[readonly])) {
							t.Fatal("search no longer selected and visible")
						}
						if selection == "reply" {
							if m.CommentMenu.Draft != "private reply" || m.CommentMenu.Cursor != 7 {
								t.Fatal("reply draft changed")
							}
							if event.policy != positionDisplayToggle {
								end := -1
								for i, row := range m.displayDetail() {
									if row.editor {
										end = i
									}
								}
								if end < m.offset() || end >= m.offset()+m.bodyHeight() {
									t.Fatalf("reply end %d outside viewport %d", end, m.offset())
								}
							}
						}
					}
					if !bytes.Equal(raw, m.Session.Inventory.Patches["p"]) || !reflect.DeepEqual(blobs, m.Session.Inventory.FullSource.Blobs) {
						t.Fatal("projection changed pinned source")
					}
				})
			}
		}
	}
}

func positionTestCells(row diffLine) []diffLine {
	if row.sideBySide == nil {
		return []diffLine{row}
	}
	var cells []diffLine
	for _, cell := range []*diffCell{row.sideBySide.new, row.sideBySide.old} {
		if cell != nil {
			cells = append(cells, *cell.line)
		}
	}
	return cells
}

func TestDiffPositionViewportPolicies(t *testing.T) {
	for _, policy := range []diffPositionPolicy{positionWindowResize, positionKeyboardResize, positionMouseResize, positionGutterToggle} {
		for _, active := range []bool{false, true} {
			t.Run(fmt.Sprintf("policy=%d/active=%t", policy, active), func(t *testing.T) {
				m := largeModel(largeTextSession(1, 1), 120, 15)
				defer m.Close()
				m.selectReviewView(viewFiles)
				// A structural row has no semantic anchor; viewport fallback remains
				// event-specific even when the numeric cursor is outside the reading area.
				m.setCursor(0)
				m.setOffset(20)
				m.cursorActive = active
				m.preserveDiffPosition(policy, func() { m.Width = 100 })
				switch policy {
				case positionWindowResize, positionKeyboardResize:
					if m.cursor() < m.offset() || m.cursor() >= m.offset()+m.bodyHeight() {
						t.Fatal("fallback cursor outside viewport")
					}
				case positionMouseResize:
					if active && m.offset() != 0 {
						t.Fatal("mouse failed to reveal active cursor")
					}
					if !active && m.offset() != 20 {
						t.Fatal("mouse moved inactive reading offset")
					}
				case positionGutterToggle:
					if m.offset() != 20 || m.cursor() != 0 {
						t.Fatal("gutter applied resize fallback")
					}
				}
			})
		}
	}
}
