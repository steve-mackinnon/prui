package tui

import (
	"bytes"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"pr-review/internal/inventory"
	"pr-review/internal/source"
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
	classSelectionFocused
	classPaneBorder
	classPaneBorderFocused
)

// styledLine is one already-escaped display line plus its semantic class.
// Text never contains ANSI: styling happens after scrolling and clipping.
type styledLine struct {
	Class lineClass
	Text  string
}

// diffLine keeps the immutable GitHub review target next to the already-safe
// display data. The target is derived from patch bytes, never terminal text.
type diffLine struct {
	styledLine
	target     *source.ReviewCommentTarget
	commentID  int64    // nonzero only for an already anchored overlay comment
	sideBySide *diffRow // non-nil for one logical, aligned source row
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
	// A muted background makes the selected row discoverable even while focus is
	// in the detail pane. The textual chevron remains the primary cue when a
	// terminal removes colors.
	classSelection: lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("236")).TabWidth(lipgloss.NoTabConversion),
	// The selection in the focused pane adds reverse video, also colorless, so
	// the two selection states differ without either depending on color.
	classSelectionFocused: lipgloss.NewStyle().Bold(true).Reverse(true).TabWidth(lipgloss.NoTabConversion),
	// Wide review panes keep a quiet outline until selected. The active pane
	// uses the same warm accent as the review chrome, with bold retained as a
	// color-independent focus cue.
	classPaneBorder:        semantic(lipgloss.Color("240"), false),
	classPaneBorderFocused: semantic(lipgloss.Color("208"), true),
}

// selectionMarker makes selection readable even when styles are unavailable.
// Focus is expressed by the style, not another glyph, so every selected row
// scans consistently across review and picker surfaces.
func selectionMarker(selected bool) string {
	if !selected {
		return "  "
	}
	return "› "
}

// cursorMarker reserves a stable detail gutter. The selected line uses the
// same color-independent chevron as list selection.
func cursorMarker(selected bool) string {
	if selected {
		return "› "
	}
	return "  "
}

func selectedClass(focused bool) lineClass {
	if focused {
		return classSelectionFocused
	}
	return classSelection
}

func paneBorderClass(focused bool) lineClass {
	if focused {
		return classPaneBorderFocused
	}
	return classPaneBorder
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

// isGitFilePreamble recognizes structural metadata that identifies Git blobs
// rather than a reviewable source line. The renderer uses the inventory's
// authoritative file label in its place.
func isGitFilePreamble(raw []byte) bool {
	for _, p := range fileHeaders {
		if bytes.HasPrefix(raw, []byte(p)) {
			return true
		}
	}
	return false
}

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
