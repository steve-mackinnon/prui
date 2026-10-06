# Spec: Readable conversation Markdown and HTML

Scope authorized on 2026-10-05: specify the proposed rendering change, then implement it.

Status: implemented and verified. `./scripts/verify.sh` passed formatting, vet,
the full race-enabled test suite (including CLI PTY smoke), and build.
The runnable binary is `/Users/steve/.codex/worktrees/85e0/review/prui`.

## Objective

Make general PR comments, review bodies, and inline discussion bodies readable in
Overview and discussion detail. The Vercel and Devin bot examples should show
human prose rather than hidden metadata, raw HTML attributes, or literal newline
escapes. This extends SPEC-overview-conversation.md without changing navigation.

## Display contract

1. Render GFM paragraphs, headings, emphasis, lists, blockquotes, tables, links,
   and fenced code using the existing theme-aware Glamour adapter. Preserve real
   paragraph breaks, normalize CRLF/CR, and wrap at the current terminal width.
2. Remove HTML comments and unused Markdown reference definitions from display.
   Referenced definitions must still resolve. Do not special-case bot authors.
3. Convert HTML paragraphs, breaks, emphasis, code, anchors, images and tables
   into terminal-readable content. Strip presentation tags/attributes while
   retaining text, including relative-time labels. Images show alt text, never
   fetch assets; empty-alt avatars add no clutter. Suppress script/style content.
   Safe HTML links retain destinations; unsafe destinations leave a plain label.
4. Overview shows each details summary with a collapsed marker and an indication
   that Enter opens detail. Detail expands all loaded details content, including
   nested sections. There is no new per-section toggle or network request.
   Missing closing tags must not crash; a details block without a summary uses
   a generic Details label. Unknown HTML wrappers retain their text.
5. HTML inside inline, fenced, or indented code stays literal. Terminal controls,
   including decoded entities, cannot become terminal commands or hostile links.
6. Rendering is presentation-only. Source bodies, edit drafts, suggestions,
   submitted payloads, activity IDs, targets, freshness and permissions remain
   unchanged. Description HTML continues to use its existing literal policy;
   diff comment overlays and editors are outside this change.
7. Cache rendered bodies within a bounded tab-owned cache, keyed by raw body,
   width, expansion mode and theme. Resizing, theme changes and body edits must
   not return stale lines. Rendering and asset handling perform no network I/O.
8. On rendering failure show sanitized text with real newlines, retaining the
   comment and its navigation identity rather than dropping the activity.

Example Overview body:

```text
The latest updates on your projects. Learn more about Vercel for GitHub.

▸ 4 Skipped Deployments (Enter: detail)

Request Review
```

## Stack, structure and code style

Go 1.26.8, Bubble Tea, Glamour v2, Goldmark and golang.org/x/net/html already in
the module graph. No new module versions. Add a display adapter and tests in
internal/tui/conversation_markdown*.go; integrate Overview and legacy discussion
detail. Use Go formatting, source-offset-based code protection, and an HTML
tokenizer rather than regular expressions for nested HTML.

```go
func renderConversationMarkdown(body string, width int, palette theme.Theme, expanded bool) ([]string, error)
```

## Implementation slices

1. Reproduce screenshot failures with existing draft tests and add edge cases.
2. Normalize HTML outside code, render through the existing adapter, and test.
3. Integrate both discussion surfaces and bounded caching. Verify stable IDs,
   expansion, resize/theme behavior, raw bodies and editor payload preservation.
4. Review intentional screen changes and update README, constraints and testing
   documentation. Run full verification and build a runnable binary here.

## Testing and commands

Deterministic Go tests with synthetic comments; no live GitHub calls. Cover bot
metadata, collapsed/expanded HTML, nested/malformed details, HTML tables/images,
code preservation, referenced definitions, controls/unsafe URLs, Unicode narrow
wrapping, cached theme changes, and model/key navigation. Retain all existing
draft/write checks and review changed screen baselines.

```sh
go test ./internal/tui -run 'ConversationMarkdown|OverviewRenders' -count=1
go test ./internal/tui -run '^TestScreenSnapshots$' -args -update-golden
./scripts/verify.sh
go build -o prui ./cmd/prui
git diff --check
```

## Boundaries and success criteria

- Always: sanitize untrusted content, bound caches, preserve raw bodies and
  semantic row identity, and verify offline with synthetic fixtures.
- Ask first: add dependency versions, fetch remote images, change posting,
  persistence or navigation behavior beyond the stated detail expansion.
- Never: execute HTML, interpret remote ANSI, rewrite submitted text, suppress
  tests or reduce quality thresholds.

Success: screenshot-like comments show readable prose, concise collapsed
summaries and image labels; Enter reveals full content; code examples stay
literal; controls stay inert; width/theme refresh correctly; all verification
passes. No open questions; the existing Enter/Escape detail interaction supplies
the expansion mechanism.
