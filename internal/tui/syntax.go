package tui

import (
	"context"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"prui/internal/inventory"
	"prui/internal/syntax"
	"prui/internal/theme"
)

// escapedSpans translates trusted numeric offsets through the same escaping as
// source text. Malformed optional metadata is ignored, never used as ANSI.
func escapedSpans(raw []byte, spans []syntax.Span) []syntax.Span {
	var out []syntax.Span
	if !utf8.Valid(raw) {
		return nil
	}
	end, display := 0, 1
	for _, s := range spans {
		if s.Start < end || s.End <= s.Start || s.End > len(raw) || s.Kind < syntax.Keyword || s.Kind > syntax.Operator || (s.Start < len(raw) && !utf8.RuneStart(raw[s.Start])) || (s.End < len(raw) && !utf8.RuneStart(raw[s.End])) {
			continue
		}
		display += len(Escape(string(raw[end:s.Start])))
		start := display
		display += len(Escape(string(raw[s.Start:s.End])))
		out = append(out, syntax.Span{Start: start, End: display, Kind: s.Kind})
		end = s.End
	}
	return out
}

// fragmentSyntax is a compatibility fallback. Each hunk and side is lexed in
// isolation so missing regions cannot accidentally join lexical states.
func fragmentSyntax(f inventory.FileChange, patch []byte) syntax.Patch {
	result := syntax.Patch{}
	raw := strings.Split(string(patch), "\n")
	for start := 0; start < len(raw); {
		if !strings.HasPrefix(raw[start], "@@ ") {
			start++
			continue
		}
		end := start + 1
		for end < len(raw) && !strings.HasPrefix(raw[end], "@@ ") {
			end++
		}
		for _, old := range []bool{true, false} {
			var text strings.Builder
			var indexes []int
			for i := start + 1; i < end; i++ {
				if len(raw[i]) == 0 {
					continue
				}
				marker := raw[i][0]
				if marker == ' ' || (old && marker == '-') || (!old && marker == '+') {
					text.WriteString(raw[i][1:])
					text.WriteByte('\n')
					indexes = append(indexes, i)
				}
			}
			path := f.NewPath
			if old {
				path = f.OldPath
			}
			tokens := syntax.Tokenize(context.Background(), string(path), []byte(text.String()))
			for n, i := range indexes {
				row := result[i]
				if old {
					row.Old = tokens[n+1]
				} else {
					row.New = tokens[n+1]
				}
				result[i] = row
			}
		}
		start = end
	}
	return result
}

func cropSpans(spans []syntax.Span, start, end, shift int) []syntax.Span {
	var result []syntax.Span
	for _, s := range spans {
		a, b := max(start, s.Start), min(end, s.End)
		if a < b {
			result = append(result, syntax.Span{Start: a - start + shift, End: b - start + shift, Kind: s.Kind})
		}
	}
	return result
}

func wrappedSyntax(line diffLine, part string, offset *int) diffLine {
	at := strings.Index(line.Text[*offset:], part)
	if at < 0 {
		line.syntax = nil
		line.oldSyntax = nil
		line.wordChanges = nil
		return line
	}
	start := *offset + at
	line.sourceOffset += start - 1
	line.syntax = cropSpans(line.syntax, start, start+len(part), 1)
	line.oldSyntax = cropSpans(line.oldSyntax, start, start+len(part), 1)
	line.wordChanges = cropSpans(line.wordChanges, start, start+len(part), 1)
	*offset = start + len(part)
	return line
}

// syntaxText separates the structural patch marker from source before styling.
// Fragment renderers use sourceText directly, so source '+'/'-' is never a marker.
func (m *Model) syntaxText(line diffLine, horizontal, width int, prefix string) string {
	if line.Class != classAdded && line.Class != classRemoved && line.Class != classContext {
		prefix = clip(prefix, width)
		runes := []rune(line.Text)
		rendered := prefix + clip(string(runes[min(max(horizontal, 0), len(runes)):]), max(0, width-visibleWidth(prefix)))
		if m.linksAvailable() {
			rendered = linkSourceURLs(rendered, line.Text, horizontal, visibleWidth(prefix))
		}
		return rendered
	}
	marker, text := splitPatchMarker(line.Text)
	if len(line.Text) > 0 && (line.Text[0] == '+' || line.Text[0] == '-' || line.Text[0] == ' ') {
		line.Text = text
		line.sourceOffset += len(marker)
		line.syntax = cropSpans(line.syntax, len(marker), len(text)+len(marker), 0)
		line.wordChanges = cropSpans(line.wordChanges, len(marker), len(text)+len(marker), 0)
		if horizontal <= 0 {
			prefix += m.patchMarker(marker, line.Class)
		} else {
			horizontal--
		}
	}
	return m.sourceText(line, horizontal, width, prefix, theme.Foreground)
}

func (m *Model) patchMarker(marker string, class lineClass) string {
	if m.colorProfile <= colorprofile.Ascii {
		return marker
	}
	role := theme.Foreground
	if class == classAdded {
		role = theme.Added
	} else if class == classRemoved {
		role = theme.Removed
	}
	c, _ := m.theme.Color(role)
	return lipgloss.NewStyle().Foreground(c).TabWidth(lipgloss.NoTabConversion).Render(marker)
}

// sourceText resolves foreground resets in each visible source fragment before
// row styling. The base role is normally Foreground; a search match uses its
// contrasting foreground while explicit syntax colors remain authoritative.
func (m *Model) sourceText(line diffLine, horizontal, width int, prefix string, baseRole theme.Token) (rendered string) {
	defer func() {
		if m.colorProfile > colorprofile.Ascii {
			foreground, _ := m.theme.Color(baseRole)
			if foreground == nil {
				foreground, _ = m.theme.Color(theme.Foreground)
			}
			rendered = paintThemeCanvas(rendered, visibleWidth(rendered), 1, foreground, nil)
		}
		if m.linksAvailable() {
			rendered = linkSourceURLs(rendered, line.Text, horizontal, visibleWidth(clip(prefix, width)))
		}
	}()
	if highlighted, ok := m.searchHighlightedText(line, horizontal, width, prefix, baseRole); ok {
		return highlighted
	}
	if len(line.wordChanges) > 0 && m.colorProfile > colorprofile.Ascii {
		return m.wordHighlightedText(line, horizontal, width, prefix, baseRole)
	}
	prefix = clip(prefix, width)
	runes := []rune(line.Text)
	start := len(string(runes[:min(max(horizontal, 0), len(runes))]))
	text := clip(line.Text[start:], max(0, width-visibleWidth(prefix)))
	spans := cropSpans(line.syntax, start, start+len(text), 0)
	if len(spans) == 0 || m.colorProfile <= colorprofile.Ascii {
		return prefix + text
	}
	var out strings.Builder
	out.WriteString(prefix)
	last := 0
	for _, s := range spans {
		if s.Start < last || s.End > len(text) {
			continue
		}
		out.WriteString(text[last:s.Start])
		token := theme.Foreground
		switch s.Kind {
		case syntax.Keyword:
			token = theme.FocusedBorder
		case syntax.String:
			token = theme.Added
		case syntax.Number:
			token = theme.Warning
		case syntax.Comment:
			token = theme.Metadata
		case syntax.Name:
			token = theme.Title
		case syntax.Operator:
			token = theme.FileHeader
		}
		c, ok := m.theme.Color(token)
		if ok {
			out.WriteString(lipgloss.NewStyle().Foreground(c).TabWidth(lipgloss.NoTabConversion).Render(text[s.Start:s.End]))
		} else {
			out.WriteString(text[s.Start:s.End])
		}
		last = s.End
	}
	out.WriteString(text[last:])
	return out.String()
}
