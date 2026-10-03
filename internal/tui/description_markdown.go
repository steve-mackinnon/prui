package tui

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"prui/internal/theme"
)

// Glamour unescapes character references in text and code spans after parsing.
// Neutralize references that would become controls before they reach its ANSI
// renderer, which also emits trusted styling and OSC 8 link sequences.
var descriptionEntity = regexp.MustCompile(`&(?:#[xX][0-9A-Fa-f]+|#[0-9]+|[A-Za-z][A-Za-z0-9]+);?`)

// renderDescriptionMarkdown converts a frozen PR description into safe,
// width-aware terminal lines. It intentionally accepts no Model state so its
// output can be cached by the Description view.
func renderDescriptionMarkdown(body string, width int) ([]string, error) {
	return renderDescriptionMarkdownForTheme(body, width, theme.Dark)
}

// renderDescriptionMarkdownForTheme uses the catalog's explicit appearance
// classification to choose the existing Glamour light or dark baseline.
func renderDescriptionMarkdownForTheme(body string, width int, themeName string) ([]string, error) {
	resolved, err := theme.Resolve(themeName, nil)
	if err != nil {
		return nil, err
	}
	return renderDescriptionMarkdownWithTheme(body, width, resolved)
}

// renderDescriptionMarkdownWithTheme retains the existing syntax foregrounds
// while letting explicit base colors own ordinary text and block surfaces.
func renderDescriptionMarkdownWithTheme(body string, width int, palette theme.Theme) ([]string, error) {
	if width < 1 {
		width = 1
	}

	style := styles.DarkStyle
	if palette.IsLight() {
		style = styles.LightStyle
	}
	hasBase := palette.Syntax(theme.Foreground) != "default" || palette.Syntax(theme.Background) != "default"
	option := glamour.WithStandardStyle(style)
	if hasBase {
		local := *styles.DefaultStyles[style]
		local.Document.Color = nil
		local.H1.BackgroundColor = nil
		local.Code.BackgroundColor = nil
		local.CodeBlock.BackgroundColor = nil
		option = glamour.WithStyles(local)
	}
	renderer, err := glamour.NewTermRenderer(
		option,
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(true),
	)
	if err != nil {
		return nil, fmt.Errorf("create markdown renderer: %w", err)
	}

	rendered, err := renderer.Render(normalizeDescriptionControls(body))
	if err != nil {
		return nil, fmt.Errorf("render markdown description: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
	if hasBase {
		lines = descriptionWithoutBackgrounds(lines)
	}
	return lines, nil
}

// Glamour's shared Chroma registry can retain an earlier style. Remove only
// background channels from trusted renderer output as a final safeguard; keep
// foregrounds, attributes, hyperlinks, and exact logical text/whitespace.
func descriptionWithoutBackgrounds(lines []string) []string {
	for i, line := range lines {
		width := ansi.StringWidth(line)
		if width == 0 {
			continue
		}
		buf := themeCanvasBuffer(line, width, 1)
		for x := 0; x < width; x++ {
			buf.CellAt(x, 0).Style.Bg = nil
		}
		rendered := buf.Render()
		lines[i] = rendered + strings.Repeat(" ", max(0, width-ansi.StringWidth(rendered)))
	}
	return lines
}

func normalizeDescriptionControls(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	codeLines := descriptionCodeLines(body)
	entityMatches := descriptionEntity.FindAllStringIndex(body, -1)
	var decodedBody strings.Builder
	decodedBody.Grow(len(body))
	last := 0
	for _, match := range entityMatches {
		decodedBody.WriteString(body[last:match[0]])
		entity := body[match[0]:match[1]]
		if inDescriptionCodeLine(match[0], codeLines) {
			decodedBody.WriteString(entity)
		} else {
			decodedBody.WriteString(sanitizeDescriptionEntity(entity))
		}
		last = match[1]
	}
	decodedBody.WriteString(body[last:])
	body = decodedBody.String()

	var safe strings.Builder
	safe.Grow(len(body))
	for _, r := range body {
		switch {
		case r == '\n' || r == '\t':
			safe.WriteRune(r)
		case r == '<':
			// Glamour sanitizes raw HTML away. Encode it before parsing so it
			// remains visible, inert text as approved for PR descriptions.
			safe.WriteString("&lt;")
		case unsafeDescriptionRune(r):
			writeDescriptionControl(&safe, r)
		default:
			safe.WriteRune(r)
		}
	}
	return safe.String()
}

func sanitizeDescriptionEntity(entity string) string {
	decoded := html.UnescapeString(entity)
	if decoded == entity {
		return entity
	}
	var safe strings.Builder
	for _, r := range decoded {
		if unsafeDescriptionRune(r) {
			writeDescriptionControl(&safe, r)
		} else {
			safe.WriteRune(r)
		}
	}
	// Keep harmless references intact so Markdown retains its normal
	// entity and link handling.
	if safe.String() == decoded {
		return entity
	}
	return safe.String()
}

// Code blocks display entity references literally. Use Glamour's Goldmark
// extensions to identify their source lines before rewriting other references.
func descriptionCodeLines(body string) [][2]int {
	if !strings.Contains(body, "&") {
		return nil
	}
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	// The renderer sees raw '<' as &lt;. Replace it with a single plain
	// character here so HTML blocks cannot mask code fences, while source
	// offsets still match the original body.
	parseBody := strings.ReplaceAll(body, "<", "x")
	root := md.Parser().Parse(text.NewReader([]byte(parseBody)))
	var lines [][2]int
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && (node.Kind() == ast.KindCodeBlock || node.Kind() == ast.KindFencedCodeBlock) {
			for i := 0; i < node.Lines().Len(); i++ {
				segment := node.Lines().At(i)
				lines = append(lines, [2]int{segment.Start, segment.Stop})
			}
		}
		return ast.WalkContinue, nil
	})
	sort.Slice(lines, func(i, j int) bool { return lines[i][1] < lines[j][1] })
	return lines
}

func inDescriptionCodeLine(pos int, lines [][2]int) bool {
	i := sort.Search(len(lines), func(i int) bool { return lines[i][1] > pos })
	return i < len(lines) && pos >= lines[i][0]
}

func unsafeDescriptionRune(r rune) bool {
	return r < 0x20 || r >= 0x7f && r <= 0x9f ||
		r == 0x061c || r == 0x200e || r == 0x200f ||
		r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x206f
}

func writeDescriptionControl(safe *strings.Builder, r rune) {
	if r <= 0xff {
		fmt.Fprintf(safe, `\x%02x`, r)
	} else {
		fmt.Fprintf(safe, `\u%04x`, r)
	}
}
