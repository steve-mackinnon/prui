package tui

import (
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"sort"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
	"prui/internal/theme"
)

func renderConversationMarkdown(body string, width int, palette theme.Theme, expanded bool) ([]string, error) {
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	normalized, err := conversationHTML(body, expanded)
	if err != nil {
		return nil, err
	}
	lines, err := renderSafeMarkdown(normalizeMarkdownControls(normalized, true), max(1, width), palette, true)
	if err != nil {
		return nil, err
	}
	var wrapped []string
	for _, line := range lines {
		// Glamour accepts arbitrary Markdown link schemes. Remove unsafe OSC 8
		// destinations from trusted output while retaining text and styling.
		if strings.Contains(line, "\x1b]8;") {
			canvas := themeCanvasBuffer(line, max(1, ansi.StringWidth(line)), 1)
			changed := false
			for x := 0; x < canvas.Width(); x++ {
				cell := canvas.CellAt(x, 0)
				if cell.Link.URL != "" && safeConversationURL(cell.Link.URL) == "" {
					cell.Link = uv.Link{}
					changed = true
				}
			}
			if changed {
				line = canvas.Render()
			}
		}
		wrapped = append(wrapped, strings.Split(ansi.Wrap(line, max(1, width), ""), "\n")...)
	}
	// Document margins separate standalone descriptions; activity cards already
	// supply their own spacing. Keep paragraph breaks inside the body.
	for len(wrapped) > 0 && strings.TrimSpace(ansi.Strip(wrapped[0])) == "" {
		wrapped = wrapped[1:]
	}
	for len(wrapped) > 0 && strings.TrimSpace(ansi.Strip(wrapped[len(wrapped)-1])) == "" {
		wrapped = wrapped[:len(wrapped)-1]
	}
	return wrapped, nil
}

// Protect code by source offsets before tokenizing HTML. Replacing '<' only in
// the parser input prevents HTML blocks from swallowing Markdown code nodes.
func protectConversationCode(body string) (string, func(string) string) {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	root := md.Parser().Parse(text.NewReader([]byte(strings.ReplaceAll(body, "<", "x"))))
	var ranges [][2]int
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindCodeBlock, ast.KindFencedCodeBlock:
			for i := 0; i < n.Lines().Len(); i++ {
				s := n.Lines().At(i)
				ranges = append(ranges, [2]int{s.Start, s.Stop})
			}
		case ast.KindCodeSpan:
			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				if t, ok := child.(*ast.Text); ok {
					ranges = append(ranges, [2]int{t.Segment.Start, t.Segment.Stop})
				}
			}
		}
		return ast.WalkContinue, nil
	})
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	prefix := "PRUICODESLOT"
	for strings.Contains(body, prefix) {
		prefix += "X"
	}
	var out strings.Builder
	var replacements []string
	last := 0
	for _, r := range ranges {
		if r[0] < last {
			continue
		}
		out.WriteString(body[last:r[0]])
		key := fmt.Sprintf("%s%dEND", prefix, len(replacements)/2)
		out.WriteString(key)
		value := body[r[0]:r[1]]
		// Keep line boundaries visible to both the HTML and Markdown parsers.
		if strings.HasSuffix(value, "\n") {
			value = strings.TrimSuffix(value, "\n")
			out.WriteByte('\n')
		}
		replacements = append(replacements, key, value)
		last = r[1]
	}
	out.WriteString(body[last:])
	return out.String(), strings.NewReplacer(replacements...).Replace
}

type conversationDetails struct{ summary, hasSummary bool }

// HTML is converted locally to Markdown, never executed or fetched. Text tokens
// retain their raw bytes so Markdown syntax and harmless entities still parse.
func conversationHTML(body string, expanded bool) (string, error) {
	// HTML preformatted content may itself contain Markdown fences.
	longest, run := 2, 0
	for _, r := range body {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	preFence := strings.Repeat("`", longest+1)
	body, restore := protectConversationCode(body)
	z := html.NewTokenizer(strings.NewReader(body))
	var out strings.Builder
	var details []conversationDetails
	var links []string
	suppressed := ""
	columns := 0
	headerRow := false
	inPre := false
	offset := 0
	hidden := 0
	visible := func() bool {
		return (expanded || hidden == 0) && suppressed == ""
	}
	write := func(s string) {
		if visible() {
			out.WriteString(s)
		}
	}
	closeDetails := func() {
		if len(details) == 0 {
			return
		}
		d := details[len(details)-1]
		details = details[:len(details)-1]
		if !d.summary {
			hidden--
		}
		if !d.hasSummary {
			if expanded {
				write("\n\n▾ Details\n\n")
			} else {
				write("\n\n▸ Details (Enter: expand)\n\n")
			}
		} else {
			write("\n\n")
		}
	}
	for {
		kind := z.Next()
		raw := string(z.Raw())
		start := offset
		offset += len(raw)
		if kind == html.ErrorToken {
			if z.Err() != io.EOF {
				return "", z.Err()
			}
			for len(details) > 0 {
				closeDetails()
			}
			return restore(out.String()), nil
		}
		if kind == html.TextToken {
			if inPre {
				write(html.UnescapeString(raw))
			} else {
				write(raw)
			}
			continue
		}
		if kind != html.StartTagToken && kind != html.EndTagToken && kind != html.SelfClosingTagToken {
			continue // comments, doctypes and processing instructions
		}
		t := z.Token()
		end := kind == html.EndTagToken
		// HTML tokenization also recognizes Markdown autolinks as tag tokens.
		if !end && strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
			value := raw[1 : len(raw)-1]
			address, err := mail.ParseAddress(value)
			if safeConversationURL(value) != "" {
				prefix := strings.TrimSpace(body[strings.LastIndex(body[:start], "\n")+1 : start])
				if strings.HasPrefix(prefix, "[") && strings.HasSuffix(prefix, "]:") {
					// Angle-bracket destinations in reference definitions are not
					// autolink labels; leave the definition available to Goldmark.
					write(conversationDestination(value))
				} else {
					write("[" + conversationLabel(value) + "](" + conversationDestination(value) + ")")
				}
				continue
			}
			if err == nil && address.Address == value {
				write("[" + conversationLabel(value) + "](mailto:" + conversationDestination(value) + ")")
				continue
			}
		}
		if suppressed != "" {
			if end && t.Data == suppressed {
				suppressed = ""
			}
			continue
		}
		if t.Data == "script" || t.Data == "style" || t.Data == "iframe" {
			if !end {
				suppressed = t.Data
			}
			continue
		}
		switch t.Data {
		case "details":
			if end {
				closeDetails()
			} else {
				write("\n\n")
				details = append(details, conversationDetails{})
				hidden++
			}
		case "summary":
			if len(details) == 0 {
				continue
			}
			d := &details[len(details)-1]
			if end {
				if !expanded {
					write(" (Enter: expand)")
				}
				write("\n\n")
				if d.summary {
					hidden++
				}
				d.summary = false
			} else {
				if !d.summary {
					hidden--
				}
				d.summary, d.hasSummary = true, true
				if expanded {
					write("▾ ")
				} else {
					write("▸ ")
				}
			}
		case "a":
			if end {
				if len(links) > 0 {
					href := links[len(links)-1]
					links = links[:len(links)-1]
					if href != "" {
						write("](" + conversationDestination(href) + ")")
					}
				}
			} else {
				href := safeConversationURL(htmlAttribute(t, "href"))
				links = append(links, href)
				if href != "" {
					write("[")
				}
			}
		case "img":
			if !end {
				write(conversationLabel(htmlAttribute(t, "alt")))
			}
		case "pre":
			if end {
				write("\n" + preFence + "\n\n")
				inPre = false
			} else {
				write("\n\n" + preFence + "text\n")
				inPre = true
			}
		case "p", "div", "section", "blockquote", "table":
			write("\n\n")
		case "br":
			write("  \n")
		case "hr":
			write("\n\n---\n\n")
		case "h1", "h2", "h3", "h4", "h5", "h6":
			if end {
				write("\n\n")
			} else {
				write("\n\n" + strings.Repeat("#", int(t.Data[1]-'0')) + " ")
			}
		case "strong", "b":
			write("**")
		case "em", "i":
			write("*")
		case "code":
			if !inPre {
				write("`")
			}
		case "li":
			if end {
				write("\n")
			} else {
				write("\n- ")
			}
		case "tr":
			if end {
				write("\n")
				if headerRow {
					write("|" + strings.Repeat(" --- |", columns) + "\n")
				}
			} else {
				columns = 0
				headerRow = false
				write("| ")
			}
		case "td", "th":
			if end {
				write(" | ")
			} else {
				columns++
				headerRow = headerRow || t.Data == "th"
			}
		}
	}
}

func htmlAttribute(t html.Token, key string) string {
	for _, a := range t.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func conversationLabel(s string) string {
	s = html.EscapeString(s)
	return strings.NewReplacer("[", "\\[", "]", "\\]", "*", "\\*", "`", "&#96;", "_", "\\_").Replace(s)
}

func conversationDestination(s string) string {
	return strings.NewReplacer("(", "%28", ")", "%29").Replace(s)
}

func safeConversationURL(s string) string {
	for _, r := range s {
		if unsafeDescriptionRune(r) || r == '<' || r == '>' || r == ' ' || r == '\\' {
			return ""
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host != "" {
			return s
		}
	case "mailto":
		return s
	}
	return ""
}

type conversationRenderKey struct {
	body, theme string
	width       int
	expanded    bool
}
type conversationRenderCache struct {
	lines map[conversationRenderKey][]string
	bytes int
}

func (m *Model) conversationLines(body string, expanded bool) []string {
	key := conversationRenderKey{body: body, theme: m.theme.Name + "/" + m.theme.Syntax(theme.Foreground) + "/" + m.theme.Syntax(theme.Background), width: max(1, m.Width), expanded: expanded}
	c := &m.discussions.markdown
	if lines, ok := c.lines[key]; ok {
		return lines
	}
	lines, err := renderConversationMarkdown(body, key.width, m.theme, expanded)
	if err != nil {
		// Retain line breaks while neutralizing every untrusted control.
		body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
		for _, line := range strings.Split(body, "\n") {
			lines = append(lines, strings.Split(ansi.Wrap(Escape(line), key.width, ""), "\n")...)
		}
	}
	size := len(body)
	for _, line := range lines {
		size += len(line)
	}
	const maxBytes = 4 << 20
	if size <= maxBytes {
		if len(c.lines) >= 128 || c.bytes+size > maxBytes || c.lines == nil {
			*c = conversationRenderCache{lines: make(map[conversationRenderKey][]string)}
		}
		c.lines[key] = lines
		c.bytes += size
	}
	return lines
}
