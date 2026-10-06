package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
	"prui/internal/theme"
)

func themeForName(t *testing.T, name string) theme.Theme {
	t.Helper()
	palette, err := theme.Resolve(name, nil)
	if err != nil {
		t.Fatal(err)
	}
	return palette
}

func TestConversationMarkdownHidesBotMetadataAndRendersProse(t *testing.T) {
	body := `<!-- bot metadata {"id":"opaque-machine-id"} -->
[vc]: #opaque-deployment-data

### Same-page hash redirect

With **a same-page hash redirect**, ` + "`onTarget`" + ` is false.

<details><summary>Learn more</summary>

A link through [authRedirectField](https://github.com/example/repo).

</details>`
	lines, err := renderConversationMarkdown(body, 72, themeForName(t, theme.Dark), false)
	if err != nil {
		t.Fatal(err)
	}
	out := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Same-page hash redirect", "onTarget", "Learn more"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	for _, bad := range []string{"opaque-machine-id", "opaque-deployment-data", "<details>", "<summary>", `\n`, "authRedirectField"} {
		if strings.Contains(out, bad) {
			t.Fatalf("leaked %q: %s", bad, out)
		}
	}
	expanded, err := renderConversationMarkdown(body, 72, themeForName(t, theme.Dark), true)
	if err != nil || !strings.Contains(ansi.Strip(strings.Join(expanded, "\n")), "authRedirectField") {
		t.Fatal("detail lost expandable content", err)
	}
}

func TestConversationMarkdownHTMLLinksTablesAndImages(t *testing.T) {
	body := `<p>The latest updates on your projects. <a href="https://vercel.com/docs">Vercel for GitHub</a>.</p>
<details><summary>4 Skipped Deployments</summary>

| Project | Status | Updated |
| --- | --- | --- |
| <a href="https://vercel.com/cms"><sup><img src="https://vercel.com/avatar" alt="" /></sup>cms</a> | ![Ignored](https://vercel.com/ignored.svg) | <relative-time datetime="2026-10-05T15:58:17Z">Oct 5, 3:58pm UTC</relative-time> |

</details>
<a href="https://vercel.com/request-review"><picture><source srcset="https://vercel.com/dark.svg"><img src="https://vercel.com/light.svg" alt="Request Review"></picture></a>`
	lines, err := renderConversationMarkdown(body, 80, themeForName(t, theme.Dark), false)
	if err != nil {
		t.Fatal(err)
	}
	out := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"The latest updates", "Vercel for GitHub", "4 Skipped Deployments", "Request Review"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s: %s", want, out)
		}
	}
	for _, bad := range []string{"srcset", "<img", "<sup", "<relative-time", "avatar", "cms", "Oct 5"} {
		if strings.Contains(out, bad) {
			t.Fatalf("collapsed HTML leaked %s: %s", bad, out)
		}
	}
	lines, err = renderConversationMarkdown(body, 80, themeForName(t, theme.Dark), true)
	if err != nil {
		t.Fatal(err)
	}
	out = ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Project", "Status", "cms", "Ignored", "Oct 5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expanded table lost %s: %s", want, out)
		}
	}
}

func TestConversationMarkdownPreservesCodeAndNeutralizesControls(t *testing.T) {
	body := "Example `<!-- visible in code -->` and `<details>`\n\n```html\n<!-- visible fenced -->\n<img src=\"example\">\n```\n\n<script>hiddenScript()</script><a href=\"javascript:bad()\">Safe label</a>\nunsafe\x1b]52;c;bad\a &#7;"
	lines, err := renderConversationMarkdown(body, 80, themeForName(t, theme.Dark), true)
	if err != nil {
		t.Fatal(err)
	}
	out := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"visible in code", "visible fenced", "<img", "Safe label", `\x1b`, `\x07`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %q", want, out)
		}
	}
	for _, bad := range []string{"hiddenScript()", "javascript:bad()", "\x1b]52", "\a"} {
		if strings.Contains(out, bad) {
			t.Fatalf("unsafe content survived: %q", out)
		}
	}
}

func TestOverviewRendersCommentMarkdownWithoutMutatingRawBody(t *testing.T) {
	m := commitModel(t)
	body := "<!-- hidden metadata -->\n## Readable heading\n\nFirst paragraph.\n\nSecond paragraph with **bold**."
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Author: "alice", Body: body}}}}
	key(m, 'D')
	out := ansi.Strip(m.View().Content)
	if strings.Contains(out, "hidden metadata") || strings.Contains(out, `\n`) || !strings.Contains(out, "Readable heading") {
		t.Fatal("Overview bypassed Markdown rendering", out)
	}
	if m.discussions.snapshot.Snapshot.Events[0].Body != body {
		t.Fatal("rendering changed raw comment body")
	}
}

func TestConversationMarkdownNestedAndMalformedDetails(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"nested", "Before\n<details><summary>Outer</summary>Hidden outer<details><summary>Inner</summary>Hidden inner</details></details>\nAfter"},
		{"unclosed", "Before\n<details><summary>Outer</summary>Hidden outer"},
		{"no summary", "Before\n<details>Hidden outer</details>\nAfter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, expanded := range []bool{false, true} {
				lines, err := renderConversationMarkdown(tc.body, 72, themeForName(t, theme.Dark), expanded)
				if err != nil {
					t.Fatal(err)
				}
				out := ansi.Strip(strings.Join(lines, "\n"))
				if strings.Contains(out, "Hidden outer") != expanded || !strings.Contains(out, "Before") {
					t.Fatal(out)
				}
				if !expanded && (!strings.Contains(out, "Enter: expand") || strings.Contains(out, "Hidden inner")) {
					t.Fatal(out)
				}
				if tc.name == "nested" && expanded && !strings.Contains(out, "Hidden inner") {
					t.Fatal(out)
				}
				if tc.name != "unclosed" && !strings.Contains(out, "After") {
					t.Fatal("lost following prose", out)
				}
			}
		})
	}
}

func TestConversationMarkdownCodeReferencesAndAutolinks(t *testing.T) {
	body := "[Docs][docs]\n\n[docs]: <https://example.com/docs>\n[vc]: #opaque\n\n<https://example.com/autolink>\n\n    <!-- indented literal -->\n    <details>\n\n~~~html\n<img src=\"literal\">\n&#7; &lrm;\n~~~\n\n`&#7;` and ``<summary>literal</summary>``\n\n<pre><code>```html\n&lt;img src=\"pre-literal\"&gt;\n```</code></pre>"
	lines, err := renderConversationMarkdown(body, 100, themeForName(t, theme.Dark), true)
	if err != nil {
		t.Fatal(err)
	}
	out := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Docs", "https://example.com/autolink", "indented literal", "<details>", "<img", "&#7; &lrm;", `\x07`, "<summary>literal</summary>", "pre-literal"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lost %q: %s", want, out)
		}
	}
	if strings.Contains(out, "opaque") || strings.ContainsRune(out, '\a') || strings.ContainsRune(out, '\u200e') {
		t.Fatal("metadata/control leak", out)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "https://example.com/docs") {
		t.Fatal("lost reference link destination")
	}
}

func TestConversationMarkdownUnsafeLinksAndConciseImages(t *testing.T) {
	body := `[click](javascript:alert%281%29) [data](data:text/plain,abc)
<a href="java&#115;cript:bad()">HTML label</a>
<a href="https://example.com/?x=&#7;">Control label</a>
<a href="https://example.com/review">Request Review</a>
![Ignored](https://example.com/long-status-image.svg)
<img src="https://example.com/hidden-avatar.svg" alt="">
<img src="https://example.com/status.svg" alt="**Plain label**">`
	lines, err := renderConversationMarkdown(body, 100, themeForName(t, theme.Dark), false)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Join(lines, "\n")
	out := ansi.Strip(raw)
	for _, want := range []string{"click", "data", "HTML label", "Control label", "Request Review", "Ignored", "**Plain label**"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lost %q: %s", want, out)
		}
	}
	for _, bad := range []string{"javascript:", "data:text", "hidden-avatar", "long-status-image.svg", "https://example.com/review"} {
		if strings.Contains(out, bad) {
			t.Fatalf("visible clutter %q: %s", bad, out)
		}
	}
	if strings.Contains(raw, "javascript:") || strings.Contains(raw, "data:text") || !strings.Contains(raw, "https://example.com/review") {
		t.Fatalf("unsafe or lost hyperlink: %q", raw)
	}
}

func TestConversationMarkdownHTMLTable(t *testing.T) {
	body := `<table><thead><tr><th>Project</th><th>Status</th></tr></thead><tbody><tr><td>cms</td><td><strong>Ignored</strong></td></tr></tbody></table><custom-wrapper>Following text</custom-wrapper>`
	lines, err := renderConversationMarkdown(body, 72, themeForName(t, theme.Dark), true)
	if err != nil {
		t.Fatal(err)
	}
	out := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Project", "Status", "cms", "Ignored", "Following text"} {
		if !strings.Contains(out, want) {
			t.Fatalf("lost %s: %s", want, out)
		}
	}
	if strings.Contains(out, " --- ") || strings.Contains(out, "<table>") {
		t.Fatal("unrendered HTML table", out)
	}
}

func TestConversationMarkdownCacheResizeThemeAndBounds(t *testing.T) {
	m := commitModel(t)
	body := "# Heading\n\n" + strings.Repeat("中文 café words ", 15)
	m.Width = 80
	wide := strings.Join(m.conversationLines(body, false), "\n")
	m.Width = 24
	narrow := strings.Join(m.conversationLines(body, false), "\n")
	if wide == narrow {
		t.Fatal("resize reused stale lines")
	}
	for _, line := range strings.Split(narrow, "\n") {
		if ansi.StringWidth(line) > 24 {
			t.Fatal("line overflows", line)
		}
	}
	m.SetTheme(themeForName(t, theme.Light))
	light := strings.Join(m.conversationLines(body, false), "\n")
	m.SetTheme(themeForName(t, theme.Dark))
	dark := strings.Join(m.conversationLines(body, false), "\n")
	if light == dark {
		t.Fatal("theme reused stale render")
	}
	for i := 0; i < 140; i++ {
		m.conversationLines(fmt.Sprintf("comment %d", i), false)
	}
	if len(m.discussions.markdown.lines) > 128 || m.discussions.markdown.bytes > 4<<20 {
		t.Fatal("unbounded cache")
	}
}

func TestOverviewConversationMarkdownDetailIdentity(t *testing.T) {
	m := commitModel(t)
	m.Height = 80
	body := "<!-- metadata -->\nVisible prose\n<details><summary>More information</summary>Expanded content</details>"
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Author: "alice", Body: body}}}}
	key(m, 'D')
	out := ansi.Strip(m.View().Content)
	if strings.Contains(out, "Expanded content") || !strings.Contains(out, "More information") {
		t.Fatal(out)
	}
	namedKey(m, tea.KeyEnter)
	out = ansi.Strip(m.View().Content)
	if !strings.Contains(out, "Expanded content") {
		t.Fatal("detail did not expand", out)
	}
	for _, row := range m.overviewRows() {
		if strings.Contains(ansi.Strip(row.text), "Expanded content") && row.id != "PR comment:1" {
			t.Fatal("lost semantic row identity", row)
		}
	}
	namedKey(m, tea.KeyEscape)
	if m.discussions.overviewExpanded["PR comment:1"] || m.discussions.selectedID != "PR comment:1" {
		t.Fatal("detail return lost selection")
	}
	if m.discussions.snapshot.Snapshot.Events[0].Body != body {
		t.Fatal("render rewrote raw body")
	}
}

func TestConversationMarkdownScreenSnapshots(t *testing.T) {
	body := `[vc]:
#opaque-deployment-data
<!-- opaque bot metadata -->
The latest updates on your projects. Learn more about [Vercel for GitHub](https://vercel.link/github-learn-more).

<details><summary>4 Skipped Deployments</summary>

| Project | Status | Updated |
| --- | --- | --- |
| <a href="https://vercel.com/cms"><img src="https://vercel.com/avatar" alt="" />cms</a> | ![Ignored](https://vercel.com/canceled.svg) | <relative-time>Oct 5, 3:58pm UTC</relative-time> |

</details>
<a href="https://vercel.com/request-review"><picture><img src="https://vercel.com/review.svg" alt="Request Review"></picture></a>`
	for _, tc := range []struct {
		name   string
		width  int
		detail bool
	}{
		{"conversation_markdown_wide", 100, false},
		{"conversation_markdown_narrow", 48, false},
		{"conversation_markdown_detail", 100, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := commitModel(t)
			m.Width, m.Height = tc.width, 24
			m.discussions.loaded = true
			m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:1", Kind: "PR comment", Author: "vercel[bot]", Body: body}}}}
			key(m, 'D')
			if tc.detail {
				namedKey(m, tea.KeyEnter)
			}
			out := ansi.Strip(m.View().Content)
			lines := strings.Split(out, "\n")
			if len(lines) > m.Height {
				t.Fatal("height overflow")
			}
			for i, line := range lines {
				if ansi.StringWidth(line) > tc.width {
					t.Fatal("width overflow", line)
				}
				lines[i] = strings.TrimRight(line, " ")
			}
			if strings.Contains(out, "opaque") || strings.Contains(out, "<picture>") || !strings.Contains(out, "Request Review") {
				t.Fatal("unreadable bot screen", out)
			}
			checkScreen(t, tc.name, strings.Join(lines, "\n")+"\n")
		})
	}
}
