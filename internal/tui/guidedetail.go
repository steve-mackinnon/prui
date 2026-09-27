package tui

import (
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/review"
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
	for si, section := range s.Guides.Items[guide].Sections {
		for _, id := range section.UnitIDs {
			unit, ok := index[id]
			if !ok {
				continue
			}
			file := s.UnitFiles[unit]
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
			appendLines(unitLines(s, unit))
		}
	}
	return detail
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
