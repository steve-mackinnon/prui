package tui

import "pr-review/internal/review"

// guideDetail is the selected guide's diff assembled from its ordered section
// unit IDs. Lines are unstyled styledLine values, exactly as unitLines emits.
type guideDetail struct {
	lines []styledLine
	files []fileAnchor
}

type fileAnchor struct {
	section int
	file    int
	offset  int
}

// anchorFor resolves a selectable hierarchy row to its guide-detail offset.
// Guide rows deliberately have no anchor so entering one preserves its scroll.
func anchorFor(detail guideDetail, r row) (int, bool) {
	for _, anchor := range detail.files {
		if anchor.section != r.section {
			continue
		}
		if r.kind == sectionRow || anchor.file == r.file {
			return anchor.offset, true
		}
	}
	return 0, false
}

// detailFor preserves guide and section order, including repeated file
// occurrences, while inserting a visible boundary at each file transition.
func detailFor(s *review.Session, guide int) guideDetail {
	if s == nil || s.Guides == nil || guide < 0 || guide >= len(s.Guides.Items) {
		return guideDetail{}
	}
	index := make(map[string]int, len(s.Inventory.Units))
	for i, u := range s.Inventory.Units {
		index[u.ID] = i
	}
	var detail guideDetail
	previous := -1
	for si, section := range s.Guides.Items[guide].Sections {
		for _, id := range section.UnitIDs {
			unit, ok := index[id]
			if !ok {
				continue
			}
			file := s.UnitFiles[unit]
			if file != previous {
				detail.files = append(detail.files, fileAnchor{section: si, file: file, offset: len(detail.lines)})
				detail.lines = append(detail.lines, styledLine{Class: classFileHeader, Text: pathLabel(s.Inventory.Files[file])})
				previous = file
			}
			detail.lines = append(detail.lines, unitLines(s, unit)...)
		}
	}
	return detail
}
