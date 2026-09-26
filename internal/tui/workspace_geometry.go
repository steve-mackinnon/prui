package tui

import (
	"github.com/charmbracelet/x/ansi"
	"image"
)

type workspaceGeometry struct{ Rail, Detail, Divider image.Rectangle }

func (m *Model) workspaceGeometry() workspaceGeometry {
	if m.Session == nil || m.top() != pageReview || m.selectedReviewView() != viewChanges || len(m.Session.Inventory.Units) == 0 {
		return workspaceGeometry{}
	}
	screen := image.Rect(0, 0, max(0, m.Width), max(0, m.Height))
	body := image.Rect(0, 3, m.Width, 3+m.bodyHeight()).Intersect(screen)
	if m.Width < 100 {
		if m.Focus == paneDiff {
			return workspaceGeometry{Detail: body}
		}
		return workspaceGeometry{Rail: body}
	}
	left := m.listWidth()
	return workspaceGeometry{
		Rail:    image.Rect(0, 3, left, 3+m.bodyHeight()).Intersect(screen),
		Divider: image.Rect(left, 3, left+3, 3+m.bodyHeight()).Intersect(screen),
		Detail:  image.Rect(left+3, 3, m.Width, 3+m.bodyHeight()).Intersect(screen),
	}
}

func (m *Model) reviewListPresentation() ([]listLine, int) {
	s := m.Session
	if s == nil || len(s.Inventory.Units) == 0 {
		return nil, 0
	}
	rows := m.navigable()
	bodyHeight := m.bodyHeight()
	leftWidth := m.listWidth()
	list := []listLine{}
	var selectedRow int
	switch {
	case m.Inventory:
		selectedRow = m.Selected
		for i, u := range s.Inventory.Units {
			marker := selectionMarker(i == m.Selected)
			list = append(list, listLine{row: i, text: marker + pathLabel(s.Inventory.Files[s.UnitFiles[i]]) + " [" + string(u.Kind) + "]"})
		}
	case rows != nil:
		selectedRow = max(0, min(len(rows)-1, m.Row))
		list = guideList(s, rows, selectedRow, leftWidth, m.Focus == paneList, m.guidePathOffset)
	case !m.Files && !m.Inventory:
		list = append(list, listLine{row: -1, text: "No guide yet. Press g to generate."})
	default:
		selectedRow = s.UnitFiles[m.Selected]
		for i, f := range s.Inventory.Files {
			marker := selectionMarker(i == selectedRow)
			prefix := marker + readMarker(s, f.ID)
			path := pathLabel(f)
			width := leftWidth - visibleWidth(prefix)
			if i == selectedRow && m.Focus == paneList && visibleWidth(path) > width {
				path = ansi.Cut(path, m.guidePathOffset, m.guidePathOffset+max(0, width))
			} else {
				path = middleTruncate(path, width)
			}
			list = append(list, listLine{row: i, text: prefix + path})
		}
	}
	selectedLine := firstDisplayLine(list, selectedRow)
	contextEnd := selectedLine + 1
	for contextEnd < len(list) && list[contextEnd].row < 0 {
		contextEnd++
	}
	// Keep the selection and its explanation together when navigating down.
	start := max(0, min(selectedLine, contextEnd-bodyHeight))
	list = list[start:min(len(list), start+bodyHeight)]
	return list, selectedRow
}
