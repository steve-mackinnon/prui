package tui

import (
	"prui/internal/inventory"
	"prui/internal/review"
	"strings"
)

// A single snapshot bounds retained source rows, including the optional split
// projection. Comment and editor overlays are deliberately excluded.
type fileDetailCache struct {
	navigation                          codeNavigation
	session                             *review.Session
	lines                               []diffLine
	splitLines                          []diffLine
	collapsedLines, collapsedSplitLines []diffLine
	collapsedFile                       int
	collapsedValid                      bool
}

func (m *Model) cachedFileDetail(split bool) []diffLine {
	if m.Session == nil {
		m.fileCache = fileDetailCache{}
		return nil
	}
	navigation := m.navigation
	if m.selectedReviewView() != viewFiles {
		navigation = codeNavigation{}
		if m.guideExpanded {
			navigation.mode = "expanded"
		}
	}
	c := &m.fileCache
	if c.session != m.Session || c.navigation != navigation {
		*c = fileDetailCache{session: m.Session, navigation: navigation}
		for f, slice := range m.Session.Slices {
			c.lines = append(c.lines, diffLine{styledLine: styledLine{Class: classFileHeader, Text: fileDivider(m.Session.Inventory.Files[f])}})
			if navigation.mode != "" || navigation.whitespace {
				c.lines = append(c.lines, (&Model{reviewTabState: &reviewTabState{Session: m.Session, sourceTokens: m.sourceSyntaxCache(), navigation: navigation}}).navigationDetail(f)...)
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

// Keep the raw cache intact. Only the rendered projection replaces generated
// bodies with a discoverable placeholder; every file boundary stays in order.
func (m *Model) presentedFileDetail(split bool) []diffLine {
	raw := m.cachedFileDetail(false)
	if !m.collapseGenerated || m.fileFilter != "" {
		return m.cachedFileDetail(split)
	}
	selected := -1
	if m.Selected >= 0 && m.Selected < len(m.Session.UnitFiles) {
		selected = m.Session.UnitFiles[m.Selected]
	}
	c := &m.fileCache
	if !c.collapsedValid || c.collapsedFile != selected {
		c.collapsedLines = nil
		c.collapsedSplitLines = nil
		c.collapsedFile = selected
		c.collapsedValid = true
		file := -1
		hide := false
		for _, line := range raw {
			if line.Class == classFileHeader && strings.HasPrefix(line.Text, "── ") {
				file++
				hide = file != selected && m.fileCategory(file) == inventory.Generated
				c.collapsedLines = append(c.collapsedLines, line)
				if hide {
					c.collapsedLines = append(c.collapsedLines, diffLine{styledLine: styledLine{Class: classPlain, Text: "Generated content collapsed · Alt+C: reveal all · F: find file"}})
				}
			} else if !hide {
				c.collapsedLines = append(c.collapsedLines, line)
			}
		}
	}
	if split {
		if c.collapsedSplitLines == nil {
			c.collapsedSplitLines = projectSideBySideDetail(c.collapsedLines)
		}
		return c.collapsedSplitLines
	}
	return c.collapsedLines
}
