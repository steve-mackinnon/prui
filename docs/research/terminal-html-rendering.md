# Terminal HTML rendering options

Research date: 2026-10-21. Scope: improve readability of GitHub PR comments in
the existing Go terminal UI; no implementation change is proposed by this note.

## Recommendation

Reuse the repository's existing HTML-to-Markdown adapter before considering a
new dependency. [`internal/tui/conversation_markdown.go`](../../internal/tui/conversation_markdown.go)
already tokenizes HTML using `golang.org/x/net/html`, protects Markdown code,
supports details/summary, links, image placeholders and tables, and sends the
result through the existing Markdown renderer. Its comment states that HTML is
converted locally and never executed or fetched. The
[`conversation Markdown spec`](../spec/SPEC-conversation-markdown.md) explicitly
excluded diff comment overlays, which explains why a similar comment can look
different in the screenshot. This is a coverage gap with an existing approach
available, rather than evidence that a browser engine is needed.

For the screenshot, the useful improvements are semantic: hide HTML comment
metadata, present the summary as a heading or disclosure, preserve its body and
Markdown formatting, and turn the linked picture badge into a readable label.
Displaying the actual badge image or reproducing its CSS would add little to
the review task. This is a product judgment, not a capability claim about the
libraries below.

## Existing tools and libraries

| Option | What it does | Fit here |
| --- | --- | --- |
| [JohannesKaufmann/html-to-markdown v2](https://github.com/JohannesKaufmann/html-to-markdown) | Go HTML-to-Markdown library and CLI; extensible rendering rules, links, images, lists and code; a separate table plugin supports alignment and spanning cells. | Best external candidate if the existing adapter becomes difficult to maintain. It produces Markdown for the current renderer rather than introducing layout infrastructure. |
| [jaytaylor/html2text](https://github.com/jaytaylor/html2text) | Go HTML-to-plaintext conversion with Markdown-like output and ASCII tables. | Simple fallback for HTML documents, but loses interactive details semantics and would need care with mixed Markdown/HTML comments. |
| [k3a/html2text](https://github.com/k3a/html2text) | Minimal Go text extraction; strips most tags and retains link destinations. Its author calls it feature-complete and recommends alternatives for richer conversion. | Smallest external conversion option; gives up much of the formatting the existing adapter preserves. |
| [w3m](https://github.com/tats/w3m/blob/master/doc/w3m.1) | Text browser/pager supporting HTML tables and frames; ignores CSS and JavaScript. `-dump`, `-T text/html` and `-cols` allow formatted text output from HTML. | Useful reference or optional external viewer; embedding a subprocess and shipping another program would be additional integration work. |
| [Lynx](https://lynx.invisible-island.net/manpage/) | Text browser with `-dump` formatted output, `-stdin` input and configurable width. | Another established external converter/viewer, with the same executable dependency tradeoff. |
| [Chawan](https://chawan.net/) | Text browser with HTML/CSS layout, including tables and flex; optional JavaScript and terminal images. Its [manual](https://chawan.net/doc/cha/cha.html) documents `-d` dump mode and piped HTML via `-T`. | Demonstrates that sophisticated HTML rendering in a terminal exists. Its browser engine and process model are substantially broader than this comment-readability requirement. |

The conversion libraries should be evaluated on real mixed Markdown/HTML
fixtures rather than assuming an HTML document converter preserves Markdown
source. In particular, code containing `<tag>` must stay literal, whitespace
must remain useful, and replacing HTML must not discard the prose inside
unknown wrappers. The existing adapter already treats code protection as an
explicit requirement. These are integration concerns, not verified failures of
any listed library.

## Glamour and Goldmark are not an HTML engine

[Glamour](https://github.com/charmbracelet/glamour) is a Markdown-to-ANSI renderer.
Its current [element implementation](https://github.com/charmbracelet/glamour/blob/main/ansi/elements.go)
passes HTML blocks and raw HTML through `SanitizeHTML` into styled text elements;
it does not create HTML layout or disclosure widgets. Enabling raw HTML is
therefore not equivalent to teaching it HTML semantics.

Similarly, Goldmark's [`html.WithUnsafe`](https://github.com/yuin/goldmark#html-renderer-options)
option lets its **HTML output renderer** emit raw HTML and potentially dangerous
links. It does not render HTML to ANSI and would not solve this terminal UI
problem.

## Boundaries for a small experiment

An initial experiment can share the existing conversion path with diff comment
display and show details bodies expanded. Collapsing each individual disclosure
would require UI state and keyboard behavior, which can be assessed separately.
Keep stored/submitted comment text intact and preserve a raw view or GitHub link
for inspecting fidelity. Continue to use the existing terminal control and URL
filtering when converting untrusted comments.

If a general converter is evaluated later, html-to-markdown explicitly warns
that it is not a sanitizer. Its generated Markdown must not be mistaken for a
security boundary when converted back to browser HTML. Its
[current module file](https://github.com/JohannesKaufmann/html-to-markdown/blob/main/go.mod)
also declares Go 1.25; assess the selected release and its actual dependency
graph before adoption rather than assuming a converter has zero cost.
