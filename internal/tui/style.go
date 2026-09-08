package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"pr-review/internal/inventory"
)

// lineClass names the semantic role of a complete display line. Classes are
// presentation-only; they never change the escaped text they describe.
type lineClass int

const (
	classPlain lineClass = iota
	classTitle
	classFileHeader
	classHunk
	classAdded
	classRemoved
	classContext
	classMetadata
	classWarning
	classUnavailable
	classSelection
)

// styledLine is one already-escaped display line plus its semantic class.
// Text never contains ANSI: styling happens after scrolling and clipping.
type styledLine struct {
	Class lineClass
	Text  string
}

// palette is the semantic style table, named by meaning rather than color.
// Tab conversion is disabled so a style only wraps its line and never edits it.
func semantic(c color.Color, bold bool) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Bold(bold).TabWidth(lipgloss.NoTabConversion)
}

var palette = map[lineClass]lipgloss.Style{
	classTitle:       semantic(lipgloss.Cyan, true),
	classFileHeader:  semantic(lipgloss.Yellow, true),
	classHunk:        semantic(lipgloss.Magenta, false),
	classAdded:       semantic(lipgloss.Green, false),
	classRemoved:     semantic(lipgloss.Red, false),
	classMetadata:    semantic(lipgloss.Blue, false),
	classWarning:     semantic(lipgloss.BrightYellow, true),
	classUnavailable: semantic(lipgloss.BrightRed, true),
	// Selection carries no color so it survives downsampling and any background.
	classSelection: lipgloss.NewStyle().Bold(true).TabWidth(lipgloss.NoTabConversion),
}

// cardClass is the state style for a non-text unit body. Text hunks classify
// per line instead, because their grammar lives in the patch bytes.
func cardClass(k inventory.Kind) lineClass {
	if k == inventory.Unavailable {
		return classUnavailable
	}
	return classMetadata
}

// fileHeaders are the Git extended-header prefixes that describe a file rather
// than its content. They are checked before "+"/"-" so "+++"/"---" never read
// as added or removed content lines.
var fileHeaders = []string{"diff --git ", "index ", "--- ", "+++ ", "old mode ", "new mode ", "new file mode ", "deleted file mode ", "similarity index ", "dissimilarity index ", "rename from ", "rename to ", "copy from ", "copy to "}

// classifyPatch reads Git diff grammar from an already-escaped line. Escaping
// turns control bytes into visible backslash sequences, so hostile content can
// only ever fall through to context.
func classifyPatch(escaped string) lineClass {
	for _, p := range fileHeaders {
		if strings.HasPrefix(escaped, p) {
			return classFileHeader
		}
	}
	switch {
	case strings.HasPrefix(escaped, "@@"):
		return classHunk
	case strings.HasPrefix(escaped, "+"):
		return classAdded
	case strings.HasPrefix(escaped, "-"):
		return classRemoved
	}
	return classContext
}

// styleLine wraps one complete display line in its semantic style. Empty and
// unstyled lines are returned untouched so colorless output stays byte-identical.
func styleLine(c lineClass, s string) string {
	style, ok := palette[c]
	if !ok || s == "" {
		return s
	}
	return style.Render(s)
}
