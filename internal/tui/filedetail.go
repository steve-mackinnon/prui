package tui

import (
	"prui/internal/inventory"
	"prui/internal/review"
)

// A single snapshot bounds retained source rows, including the optional split
// projection. Comment and editor overlays are deliberately excluded.
type fileDetailCache struct {
	navigation codeNavigation
	session    *review.Session
	lines      []diffLine
	splitLines []diffLine
}

func (m *Model) cachedFileDetail(split bool) []diffLine {
	if m.Session == nil {
		m.fileCache = fileDetailCache{}
		return nil
	}
	navigation := m.navigation
	if m.selectedReviewView() != viewFiles {
		navigation = codeNavigation{}
	}
	c := &m.fileCache
	if c.session != m.Session || c.navigation != navigation {
		*c = fileDetailCache{session: m.Session, navigation: navigation}
		for f, slice := range m.Session.Slices {
			c.lines = append(c.lines, diffLine{styledLine: styledLine{Class: classFileHeader, Text: fileDivider(m.Session.Inventory.Files[f])}})
			if navigation.mode != "" || navigation.whitespace {
				c.lines = append(c.lines, m.navigationDetail(f)...)
				continue
			}
			for _, unit := range slice.Units {
				if m.Session.Inventory.Units[unit].Kind != inventory.FileMetadata {
					c.lines = append(c.lines, unitLines(m.Session, unit)...)
				}
			}
		}
	}
	if split {
		if c.splitLines == nil {
			c.splitLines = projectSideBySideDetail(c.lines)
		}
		return c.splitLines
	}
	return c.lines
}
