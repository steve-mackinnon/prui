package tui

import (
	"pr-review/internal/inventory"
	"pr-review/internal/review"
)

// A single snapshot bounds retained source rows, including the optional split
// projection. Comment and editor overlays are deliberately excluded.
type fileDetailCache struct {
	session    *review.Session
	lines      []diffLine
	splitLines []diffLine
}

func (m *Model) cachedFileDetail(split bool) []diffLine {
	if m.Session == nil {
		m.fileCache = fileDetailCache{}
		return nil
	}
	c := &m.fileCache
	if c.session != m.Session {
		*c = fileDetailCache{session: m.Session}
		for f, slice := range m.Session.Slices {
			c.lines = append(c.lines, diffLine{styledLine: styledLine{Class: classFileHeader, Text: fileDivider(m.Session.Inventory.Files[f])}})
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
