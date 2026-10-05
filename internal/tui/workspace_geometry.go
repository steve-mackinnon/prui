package tui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"image"
	"prui/internal/inventory"
)

type workspaceGeometry struct{ Rail, Detail, Divider image.Rectangle }

func (m *Model) workspaceGeometry() workspaceGeometry {
	if m.Session == nil || m.top() != pageReview || !m.diffReviewView() || len(m.Session.Inventory.Units) == 0 {
		return workspaceGeometry{}
	}
	screen := image.Rect(0, 0, max(0, m.Width), max(0, m.Height))
	body := image.Rect(0, 3, m.Width, 3+m.bodyHeight()).Intersect(screen)
	if m.Width < 100 {
		if m.Focus == paneDiff {
			return workspaceGeometry{Detail: image.Rect(1, body.Min.Y, m.Width-1, body.Max.Y).Intersect(screen)}
		}
		return workspaceGeometry{Rail: image.Rect(1, body.Min.Y, m.Width-1, body.Max.Y).Intersect(screen)}
	}
	left := m.listWidth()
	return workspaceGeometry{
		Rail:    image.Rect(1, 3, left+1, 3+m.bodyHeight()).Intersect(screen),
		Divider: image.Rect(left+1, 3, left+2, 3+m.bodyHeight()).Intersect(screen),
		Detail:  image.Rect(left+2, 3, m.Width-1, 3+m.bodyHeight()).Intersect(screen),
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
		var lastCategory inventory.Category
		counts := m.categoryCounts()
		for _, i := range m.filteredFiles() {
			category := m.fileCategory(i)
			if m.groupFiles && category != lastCategory {
				list = append(list, listLine{row: -1, text: categoryLabel(category) + fmt.Sprintf(" · %d", counts[category])})
				lastCategory = category
			}
			f := s.Inventory.Files[i]
			marker := selectionMarker(i == selectedRow)
			prefix := marker + readMarker(s, f.ID)
			name, width := fileRailName(f, leftWidth-visibleWidth(prefix))
			text := prefix + name
			if c, ok := s.Inventory.Classifications[f.ID]; ok && c.Partial {
				text += " [category unavailable]"
			}
			mutedFrom := 0
			path := fileDirectory(f)
			pathWidth := width - visibleWidth(name) - 2
			if path != "" && pathWidth > 0 {
				text += "  "
				mutedFrom = visibleWidth(text)
				if i == selectedRow && m.Focus == paneList && visibleWidth(path) > pathWidth {
					path = ansi.Cut(path, m.guidePathOffset, m.guidePathOffset+pathWidth)
				} else {
					path = middleTruncate(path, pathWidth)
				}
				text += path
			}
			list = append(list, listLine{row: i, text: text, mutedFrom: mutedFrom})
		}
	}

	var pinned []listLine
	if m.fileView() && m.collapseGenerated && m.categoryCounts()[inventory.Generated] > 0 {
		pinned = append(pinned, listLine{row: -1, text: fmt.Sprintf("Generated · %d · Alt+C: reveal all", m.categoryCounts()[inventory.Generated])})
		bodyHeight = max(0, bodyHeight-1)
	}
	if m.fileView() && (m.fileFilterEditing || m.fileFilter != "") {
		label := "Filter files: " + Escape(m.fileFilter)
		if m.fileFilterEditing {
			label += "▏"
		}
		pinned = append(pinned, listLine{row: -1, text: middleTruncate(label, leftWidth)})
		bodyHeight = max(0, bodyHeight-1)
		if len(list) == 0 {
			list = append(list, listLine{row: -1, text: "No matching files"})
		}
	}
	if rows != nil && !m.Inventory && bodyHeight >= 3 && len(list) > 0 {
		guide := rows[selectedRow].guide
		pinned = []listLine{list[0], {row: -1, text: fmt.Sprintf("  %02d / %02d · { previous · } next", guide+1, len(s.Guides.Items))}}
		list = list[1:]
		bodyHeight -= len(pinned)
	}
	selectedLine := firstDisplayLine(list, selectedRow)
	contextEnd := selectedLine + 1
	for contextEnd < len(list) && (list[contextEnd].row < 0 || list[contextEnd].row == selectedRow) {
		contextEnd++
	}
	// Keep the selection and its explanation together when navigating down.
	start := max(0, min(selectedLine, contextEnd-bodyHeight))
	list = list[start:min(len(list), start+bodyHeight)]
	return append(pinned, list...), selectedRow
}
