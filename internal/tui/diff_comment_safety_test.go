package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
	"prui/internal/theme"
)

func diffCommentCardText(lines []diffLine) string {
	var text strings.Builder
	for _, line := range lines {
		text.WriteString(line.Text)
		text.WriteByte('\n')
	}
	return text.String()
}

func TestDiffCommentCardCacheFreshnessAndBounds(t *testing.T) {
	m := fileScrollModel(120)
	defer m.Close()
	comment := source.ReviewComment{ID: 1, Body: "# Heading\n\n" + strings.Repeat("中文 café words ", 16)}
	card := func(indent int) string {
		t.Helper()
		rows := m.reviewCommentLinesAt(comment, indent)
		for _, row := range rows {
			if ansi.StringWidth(row.Text) > m.overlayInnerWidth(indent)+indent+6 {
				t.Fatalf("card overflows its inner width: %q", row.Text)
			}
		}
		return diffCommentCardText(rows)
	}
	root := card(0)
	cacheBytes, cacheEntries := m.discussions.markdown.bytes, len(m.discussions.markdown.lines)
	if cacheEntries == 0 {
		t.Fatal("published card did not reuse the bounded conversation cache")
	}
	if again := card(0); again != root || m.discussions.markdown.bytes != cacheBytes || len(m.discussions.markdown.lines) != cacheEntries {
		t.Fatal("unchanged card was not served consistently from cache")
	}
	reply := card(8)
	if len(strings.Split(reply, "\n")) <= len(strings.Split(root, "\n")) {
		t.Fatal("indented reply reused root width")
	}
	m.Width = 64
	narrow := card(0)
	if len(strings.Split(narrow, "\n")) <= len(strings.Split(root, "\n")) {
		t.Fatal("resize reused stale card width")
	}
	m.SetTheme(themeForName(t, theme.Light))
	light := card(0)
	m.SetTheme(themeForName(t, theme.Dark))
	if dark := card(0); dark == light {
		t.Fatal("theme change reused stale card styling")
	}
	comment.Body = "Edited **replacement**"
	if edited := ansi.Strip(card(0)); !strings.Contains(edited, "replacement") || strings.Contains(edited, "Heading") {
		t.Fatal("edited body reused stale card", edited)
	}
	for i := 0; i < 140; i++ {
		comment.Body = fmt.Sprintf("Published card %d", i)
		card(0)
	}
	if len(m.discussions.markdown.lines) > 128 || m.discussions.markdown.bytes > 4<<20 {
		t.Fatal("published cards bypassed conversation cache bounds")
	}
}

func TestDiffCommentCardSanitizesControlsAndUnsafeLinks(t *testing.T) {
	m := fileScrollModel(120)
	defer m.Close()
	comment := source.ReviewComment{ID: 1, Author: "bot\x1b[2J", Body: "Visible\x1b[2J\a\n&#27;[2J &#7;\n" +
		`<a href="java&#115;cript:alert(1)">unsafe label</a> [data label](data:text/plain,abc) ` +
		`<a href="https://example.com/review">safe label</a>`}
	raw := diffCommentCardText(m.reviewCommentLines(comment))
	out := ansi.Strip(raw)
	// OSC 8 hyperlinks legitimately use BEL as a terminator. Inspect controls
	// after removing only those renderer-produced link sequences.
	withoutLinks := regexp.MustCompile("\x1b]8;[^\x07\x1b]*(?:\x07|\x1b\\\\)").ReplaceAllString(raw, "")
	for _, want := range []string{"Visible", "unsafe label", "data label", "safe label"} {
		if !strings.Contains(out, want) {
			t.Fatalf("card lost %q: %s", want, out)
		}
	}
	for _, bad := range []string{"\x1b[2J", "\a", "javascript:", "data:text", "&#27;", "&#7;"} {
		if strings.Contains(withoutLinks, bad) {
			t.Fatalf("card leaked control or unsafe destination %q: %q", bad, raw)
		}
	}
	if !strings.Contains(raw, "https://example.com/review") {
		t.Fatal("safe card hyperlink was lost")
	}
}

func TestDiffCommentCardKeepsCodeAndUnclosedDetailsContent(t *testing.T) {
	m := fileScrollModel(120)
	defer m.Close()
	body := "<!-- hidden metadata -->\n```html\n<summary>literal code</summary>\n```\n\n" +
		"Inline `<img src=\"literal\">`.\n\n<details><summary>Unclosed details</summary>\n\nRetained prose"
	comment := source.ReviewComment{ID: 1, Body: body}
	out := ansi.Strip(diffCommentCardText(m.reviewCommentLines(comment)))
	for _, want := range []string{"<summary>literal code</summary>", `<img src="literal">`, "Unclosed details", "Retained prose"} {
		if !strings.Contains(out, want) {
			t.Fatalf("card lost %q: %s", want, out)
		}
	}
	for _, bad := range []string{"hidden metadata", "<details>", "Enter: expand"} {
		if strings.Contains(out, bad) {
			t.Fatalf("card retained HTML clutter %q: %s", bad, out)
		}
	}
}
