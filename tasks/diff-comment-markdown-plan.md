# Implementation Plan: Readable Diff Comment Markdown and HTML

Date: 2026-10-21.
Status: implemented and verified; all tasks and the full repository gate passed.
The user approved these dedicated planning files so existing unfinished task
lists stay intact.

## Overview

Make published PR comments in diff overlays readable using the existing local
HTML-to-Markdown adapter and Glamour renderer. The screenshot's bot metadata,
literal emphasis, details tags and picture attributes should become formatted
prose, expanded details content and linked image labels. This is an experiment
in readability, with no browser engine or additional dependency.

Tasks and acceptance criteria: [diff-comment-markdown-todo.md](diff-comment-markdown-todo.md).
Background: [research notes](../docs/research/terminal-html-rendering.md) and the
existing [conversation rendering contract](../docs/spec/SPEC-conversation-markdown.md).

## Architecture Decisions

- Reuse `renderConversationMarkdown` / `conversationHTML` in
  `internal/tui/conversation_markdown.go`. `reviewCommentLinesAt` in
  `internal/tui/model.go` currently escapes and wraps each raw body line.
- Render to the overlay's actual inner width, including reply indentation.
  The current `conversationLines` convenience method uses `m.Width`; do not
  call it unchanged for narrower cards. Keep Overview/detail width behavior.
- Show details bodies expanded initially. Keep their summary labels without
  introducing disclosure state or an expansion shortcut in the cards.
- Keep trusted renderer styling through card framing and final painting; never
  send formatted output through the raw-input escaping path. Keep author,
  status, source and other untrusted metadata on their existing escaping path.
- Preserve comment IDs and exact source targets on every generated row.
  Formatting cannot rewrite stored bodies, editor text, suggestions, outbound
  payloads or plain output. Existing comment actions keep their identities.
- Reuse the bounded rendering cache, extending width-aware access only as
  needed. Keys include body, effective width, theme and expansion mode; retain
  the existing 128-entry / 4 MiB limits. Avoid parsing Markdown on every scroll.
- Keep existing control/entity and URL filtering, image alt labels and sanitized
  fallback behavior. Conversion never executes HTML or fetches external assets.

## Dependency Graph and Task Index

```text
1. Readable comment body at card width
       |
2. Interaction and styling compatibility
       |
   Checkpoint: inspect representative wide/narrow cards
       |
3. Cache freshness, safety and rendering cost
       |
4. Documentation, screen evidence and repository verification
```

Tasks are sequential because they share body rendering, row assembly and cache
behavior. Task 1 must retain baseline safety and bounded caching; task 3 adds
the focused integration evidence rather than deferring those requirements.

## Scope Boundaries

Include published diff comment cards and their existing reply, file-comment and
historical projection surfaces that share the rendering path. Preserve unified
and side-by-side layouts, suggestion previews, reactions and comment selection.

Exclude new per-section controls, images, CSS, JavaScript, external executables,
new dependencies, posting behavior, persistence changes and unrelated rendering
refactors. Overview and discussion detail retain their current behavior.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Full viewport width overflows a card or indented reply | High | Pass effective inner width and check display widths with Unicode and ANSI. |
| Final painter replaces emphasis, links or code colors | Medium | Inspect final rendered cells, selected borders and colorless output. |
| More display rows change action or navigation targets | High | Preserve IDs/targets and exercise selection, replies and historical navigation. |
| Repeated overlay assembly reparses unchanged comments | Medium | Reuse bounded cache; run the existing scroll allocation budget. |
| HTML normalization consumes code or introduces controls | High | Retain existing adapter protections and test malicious/mixed fixtures through cards. |
| Broad golden updates hide regressions | Medium | Add behavioral assertions first; inspect only intentional screen changes. |

## Verification and Completion

Each task has focused verification in the task list. The completion gate is
`./scripts/verify.sh` (format checks, installer tests, vet, race-enabled tests
and build), `git diff --check`, and inspection of actual terminal output at wide
and narrow sizes. Fixtures are synthetic; no live GitHub writes are required.
Record automated and visual evidence separately, including any unavailable
manual verification. Do not claim subjective terminal QA from snapshots alone.

## Open Questions

No blocking questions for this experiment. Independent details toggles and
broader HTML fidelity remain possible follow-ups after evaluating readability.
