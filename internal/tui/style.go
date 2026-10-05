package tui

import (
	"bytes"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/syntax"
	"prui/internal/theme"
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
	classPaneHeaderFocused
	classAddedSource
	classRemovedSource
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
	searchID     searchSourceID
	rawSource    string
	sourceOffset int // escaped-source bytes removed by wrapping or marker stripping
	styledLine
	syntax, oldSyntax []syntax.Span
	wordChanges       []syntax.Span
	oldLine, newLine  int                         // raw source coordinates; zero means absent
	oldTarget         *source.ReviewCommentTarget // alternate raw context coordinate, used only for ranges
	target            *source.ReviewCommentTarget
	commentID         int64       // nonzero only for an already anchored overlay comment
	editor            bool        // true for rows in the active inline editor
	sideBySide        *diffRow    // non-nil for one logical, aligned source row
	guideAnchor       *fileAnchor // guide occurrence beginning at this source row
}

// palette is the semantic style table, named by meaning rather than color.
// Tab conversion is disabled so a style only wraps its line and never edits it.
func semantic(c color.Color, bold bool) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Bold(bold).TabWidth(lipgloss.NoTabConversion)
}

func stylesFor(t theme.Theme) map[lineClass]lipgloss.Style {
	colorFor := func(token theme.Token) color.Color {
		c, ok := t.Color(token)
		if !ok {
			panic("resolved theme missing semantic token")
		}
		return c
	}
	return map[lineClass]lipgloss.Style{
		classAddedSource:   lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Background(diffBackground(t, theme.Added)),
		classRemovedSource: lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Background(diffBackground(t, theme.Removed)),
		classTitle:         semantic(colorFor(theme.Title), true),
		classFileHeader:    semantic(colorFor(theme.FileHeader), true),
		classHunk:          semantic(colorFor(theme.Hunk), false),
		classAdded:         semantic(colorFor(theme.Added), false).Background(diffBackground(t, theme.Added)),
		classRemoved:       semantic(colorFor(theme.Removed), false).Background(diffBackground(t, theme.Removed)),
		classMetadata:      semantic(colorFor(theme.Metadata), false),
		classWarning:       semantic(colorFor(theme.Warning), true),
		classUnavailable:   semantic(colorFor(theme.Unavailable), true),
		// A muted background makes the selected row discoverable even while focus is
		// in the detail pane. The textual chevron remains the primary cue when a
		// terminal removes colors.
		classSelection: lipgloss.NewStyle().Bold(true).Background(colorFor(theme.Selection)).TabWidth(lipgloss.NoTabConversion),
		// The selection in the focused pane adds reverse video, also colorless, so
		// the two selection states differ without either depending on color.
		classSelectionFocused: lipgloss.NewStyle().Bold(true).Reverse(true).TabWidth(lipgloss.NoTabConversion),
		// Pane boundaries stay neutral until focused. The active boundary carries
		// the workspace accent, with bold retained as a color-independent cue.
		classPaneBorder:        semantic(colorFor(theme.Border), false),
		classPaneBorderFocused: semantic(colorFor(theme.FocusedBorder), true),
		classPaneHeaderFocused: lipgloss.NewStyle().Foreground(colorFor(theme.FocusedBorder)).Background(colorFor(theme.Selection)).Bold(true).TabWidth(lipgloss.NoTabConversion),
	}
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
func (m *Model) styleLine(c lineClass, s string) string {
	style, ok := m.styles[c]
	if !ok || s == "" {
		return s
	}
	rendered := style.Render(s)
	if (c == classAdded || c == classRemoved || c == classAddedSource || c == classRemovedSource) && m.colorProfile > colorprofile.Ascii {
		role := theme.Added
		if c == classRemoved || c == classRemovedSource {
			role = theme.Removed
		}
		// Nested syntax SGR resets can clear an outer Lip Gloss background.
		// Resolve the final cells, then restore only missing background channels.
		rendered = paintThemeCanvas(rendered, visibleWidth(rendered), strings.Count(rendered, "\n")+1, nil, diffBackground(m.theme, role))
	}
	return rendered
}

// Diff backgrounds belong to rows, independently of language support or visible
// tokens. A quarter accent gives changed blocks a clear surface while retaining
// readable syntax colors. Inherited themes use their light/dark family baseline.
func diffBackground(t theme.Theme, role theme.Token) color.Color {
	background, _ := t.Color(theme.Background)
	if background == nil || t.Syntax(theme.Background) == "default" {
		background = color.RGBA{R: 24, G: 24, B: 24, A: 255}
		if t.IsLight() {
			background = color.RGBA{R: 255, G: 255, B: 255, A: 255}
		}
	}
	accent, _ := t.Color(role)
	if accent == nil {
		return background
	}
	ar, ag, ab, _ := accent.RGBA()
	br, bg, bb, _ := background.RGBA()
	channel := func(accent, base uint32) uint8 {
		// RGBA channels are 16-bit values; widen the blend and explicitly
		// bound its result before narrowing to an 8-bit output channel.
		value := (uint64(accent) + 3*uint64(base)) / 4 >> 8
		if value > 255 {
			return 255
		}
		return uint8(value)
	}
	return color.RGBA{R: channel(ar, br), G: channel(ag, bg), B: channel(ab, bb), A: 255}
}

// Source rows carry backgrounds only; semantic status indicators still use the
// existing added/removed foreground styles.
func sourceLineClass(class lineClass) lineClass {
	switch class {
	case classAdded:
		return classAddedSource
	case classRemoved:
		return classRemovedSource
	default:
		return class
	}
}
