package tui

import (
	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/review"
)

// guideDetail is the selected guide's diff assembled from its ordered section
// unit IDs. Lines carry the same optional review targets as unitLines emits.
type guideDetail struct {
	lines      []diffLine
	files      []fileAnchor
	splitLines []diffLine
	splitFiles []fileAnchor
}

type fileAnchor struct {
	section int
	file    int
	offset  int
}

// anchorFor resolves a selectable hierarchy row to its guide-detail offset.
// Guide rows deliberately have no anchor so entering one preserves its scroll.
func anchorFor(detail guideDetail, r row) (int, bool) {
	return anchorForLayout(detail, r, false)
}

func anchorForLayout(detail guideDetail, r row, sideBySide bool) (int, bool) {
	anchors := detail.files
	if sideBySide {
		anchors = detail.splitFiles
	}
	for _, anchor := range anchors {
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
	appendLines := func(lines []diffLine) {
		detail.lines = append(detail.lines, lines...)
		detail.splitLines = append(detail.splitLines, projectSideBySideDetail(lines)...)
	}
	previous := -1
	previousSection := -1
	for si, section := range s.Guides.Items[guide].Sections {
		for _, id := range section.UnitIDs {
			unit, ok := index[id]
			if !ok {
				continue
			}
			file := s.UnitFiles[unit]
			if file == previous && si != previousSection {
				detail.files = append(detail.files, fileAnchor{section: si, file: file, offset: len(detail.lines)})
				detail.splitFiles = append(detail.splitFiles, fileAnchor{section: si, file: file, offset: len(detail.splitLines)})
			}
			if file != previous && s.Inventory.Units[unit].Kind != inventory.FileMetadata {
				detail.files = append(detail.files, fileAnchor{section: si, file: file, offset: len(detail.lines)})
				detail.splitFiles = append(detail.splitFiles, fileAnchor{section: si, file: file, offset: len(detail.splitLines)})
				appendLines([]diffLine{{styledLine: styledLine{Class: classFileHeader, Text: fileDivider(s.Inventory.Files[file])}}})
			}
			if file != previous && s.Inventory.Units[unit].Kind == inventory.FileMetadata {
				detail.files = append(detail.files, fileAnchor{section: si, file: file, offset: len(detail.lines)})
				detail.splitFiles = append(detail.splitFiles, fileAnchor{section: si, file: file, offset: len(detail.splitLines)})
			}
			previous = file
			previousSection = si
			appendLines(unitLines(s, unit))
		}
	}
	// Attach occurrence boundaries after projection so wrapping and overlays
	// preserve their positions in both rendered layouts.
	for i := range detail.files {
		anchor := &detail.files[i]
		detail.lines[anchor.offset].guideAnchor = anchor
	}
	for i := range detail.splitFiles {
		anchor := &detail.splitFiles[i]
		detail.splitLines[anchor.offset].guideAnchor = anchor
	}
	return detail
}

// syncGuideToLine follows the rendered occurrence, including repeated files
// and comment overlays. Collapsed hierarchy rows remain collapsed.
func (m *Model) syncGuideToLine(index int) {
	guide, ok := m.activeGuide()
	if !ok {
		return
	}
	var occurrence *fileAnchor
	for i, line := range m.displayDetail() {
		if i > index {
			break
		}
		if line.guideAnchor != nil {
			occurrence = line.guideAnchor
		}
	}
	if occurrence == nil {
		return
	}
	for i, r := range m.rows() {
		if r.guide != guide {
			continue
		}
		if r.kind == guideRow || r.section == occurrence.section && (r.kind == sectionRow || r.file == occurrence.file) {
			m.Row = i
			if len(r.units) > 0 {
				m.Selected = r.units[0]
			}
		}
	}
}

// Keep only the most recently used guide. Frozen source and guide bundles are
// immutable; selecting another snapshot or replacing its bundle misses the cache.
// Overlay/editor rows and styled text are built separately, never cached here.
type guideDetailCache struct {
	session *review.Session
	bundle  *guide.Bundle
	index   int
	detail  guideDetail
}

func (m *Model) cachedGuideDetail(index int) guideDetail {
	if m.Session == nil {
		return guideDetail{}
	}
	c := &m.guideCache
	if c.session != m.Session || c.bundle != m.Session.Guides || c.index != index {
		*c = guideDetailCache{session: m.Session, bundle: m.Session.Guides, index: index, detail: detailFor(m.Session, index)}
	}
	return c.detail
}
