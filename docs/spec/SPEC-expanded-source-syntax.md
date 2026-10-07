# Spec: Expanded source syntax highlighting

Status: Proposed; implementation has not started.

## Objective and scope

Reviewers should see the existing syntax palette throughout loaded pinned source,
including unchanged context outside patch hunks. Highlighting must preserve the
source navigation, rendering, and style-composition contracts.

This covers E-expanded context in Files and Guide and complete Alt+O/Alt+N source
views in Files. Unified, split, and narrow fallback layouts are included. No new
shortcut, setting, theme key, dependency, or language detector is required.

Assumptions: use the existing Chroma lexer selection and theme categories; retain
current best-effort limits; include full OLD/NEW views because they share the same
source-row path. These are proposed scope decisions for review.

## Current behavior and project structure

- `internal/syntax/syntax.go` tokenizes complete source into one-based line maps
  of raw byte spans, with bounded work and plain-text fallback.
- `internal/inventory/syntax.go` retains tokens only for canonical patch rows.
- `internal/tui/code_navigation.go` builds expanded and complete-source rows
  without syntax metadata; pinned bytes already exist in `inventory.FullSource`.
- `internal/tui/syntax.go` translates raw spans through escaping and renders
  theme colors, wrapping, clipping, search, and changed-word emphasis.
- `internal/tui/render.go` projects context rows into split panes and uses
  `oldSyntax` for the OLD pane and `syntax` for the NEW pane.
- `internal/tui/filedetail.go` and `guidedetail.go` cache source projections.
- New regression tests belong alongside existing TUI navigation and syntax tests;
  this spec lives in `docs/spec/`.

Related contracts: [code navigation](SPEC-code-navigation.md),
[diff syntax](SPEC-diff-syntax.md), and
[style composition](SPEC-diff-style-composition.md).

## Required behavior

1. Lex complete pinned blobs, rather than individual gaps or lines, so multiline
   comments and strings retain state across hunk boundaries.
2. Use each side's own path for lexer selection, including renames. Identical
   unchanged text may have different token categories on OLD and NEW because of
   preceding changed text or a different filename extension.
3. Expanded unchanged rows carry both OLD and NEW spans. Unified rows use NEW
   spans; split panes use their respective side's spans. Full-source views use
   the selected side's spans. Canonical patch rows keep their existing metadata.
4. Convert spans using `escapedSpans` before attaching them to display rows.
   Cached metadata contains numeric offsets/categories, never terminal ANSI or
   theme-dependent colors. Source text, line numbers, and search identities stay
   identical with or without highlighting.
5. Use the existing palette and composition behavior, including active search,
   row selection, diff markers/backgrounds, wrapping, horizontal scrolling,
   Unicode, tabs, control-character escaping, and monochrome fallback.
6. Missing source, unsupported languages, invalid input, cancellation, panic,
   and exhausted budgets produce normal source foregrounds. A highlighting
   failure must not produce a source-unavailable marker or prevent navigation.
7. Additional context remains read-only. Highlighting never creates comment
   targets or changes canonical patch identity, review progress, or search scope.

## Cache and resource contract

Maintain a transient token cache owned by the open review tab/snapshot, shared by
Files and Guide projections. Keys include immutable blob OID and exact lexer path;
path alone and OID alone are insufficient. Cache both successful and empty/failure
results. Theme, width, selection, search, mode toggles, and scrolling must not
trigger another lex of the same key. Temporary projection Models must share this
cache rather than create fresh tokenization work.

Populate entries on first use of already available complete source, including
offline captured source and on-demand source completion. Replacing a snapshot or
closing its tab releases its cache. Source completion invalidates rendered row
projections so newly loaded spans appear; it must not discard valid token entries
for previously loaded blobs. Keep cache ownership isolated across review tabs.

Retain the existing per-blob tokenizer limits: 256 KiB input, 16,000 spans, and
100 ms cooperative deadline with cancellation. The deadline is not hard
preemption. Full-source navigation can display blobs larger than the highlighting
limit; those blobs remain readable without syntax colors.

Bound transient work per review tab to 4 MiB of lexing input, 32,000 retained spans,
and 500 ms cumulative lexing time, matching the existing inventory budgets.
Charge attempts once per distinct key, including unsuccessful attempts; cache
budget failures so redraws do not retry them. Admit each result atomically rather
than retaining a truncated token map. Cache size is also bounded by available
full-source blob/path keys. Do not increase existing limits to make tests pass.

Do not serialize this cache or change session payloads. No additional blob reads,
fetches, working-tree access, AI uploads, or network requests are needed.

## Code style

Follow existing Go formatting, unexported helper names, and `syntax.Lines` /
`syntax.Span` types. Keep lexing/cache lookup separate from per-row rendering.
For example, row attachment follows the existing conversion style:

```go
line.oldSyntax = escapedSpans([]byte(oldText), oldTokens[oldNumber])
line.syntax = escapedSpans([]byte(newText), newTokens[newNumber])
```

The names above illustrate the contract, not a required helper signature. Avoid
duplicating the renderer or introducing a second syntax palette.

## Testing strategy and acceptance criteria

Use Go's existing testing framework and deterministic synthetic pinned blobs.
Tests must verify token categories and rendered composition, not merely that
some ANSI escape exists.

- Expanded Files and Guide rows outside hunks receive expected keyword, string,
  and comment spans; full OLD/NEW views receive their selected side's spans.
- A fixture changes an opening multiline delimiter inside a hunk, leaving
  identical later text with different OLD/NEW lexical state. Split panes must
  use the correct categories independently; unified context uses NEW categories.
- Rename fixtures verify path-dependent lexer selection for the same blob OID.
- Escaped controls, Unicode, tabs, wrapping, and horizontal clipping preserve
  visible text and align token spans. Search/selection and theme switching obey
  the existing composition contract in unified, split, and narrow layouts.
- Repeated rendering, resize, theme changes, mode toggles, and Files/Guide
  switching reuse cached attempts, including unsupported and budget failures.
  Assert tokenizer invocation counts through a narrow test seam if necessary.
- Cached/offline source and newly loaded source behave identically; source
  completion refreshes projections. Snapshot/tab replacement cannot leak spans.
- Per-blob and cumulative limit fixtures fall back without partial maps or
  repeated attempts. Avoid wall-clock-sensitive tests for cumulative budgets;
  use a controlled clock/tokenizer seam where necessary.
- Stripped rendered text, source coordinates, canonical targets, read-only
  context, review progress, and existing search coverage remain unchanged.

## Commands

Run from the repository root after implementation:

```sh
go test ./internal/syntax ./internal/inventory ./internal/tui
sh scripts/verify.sh
git diff --check
```

`scripts/verify.sh` checks Go formatting, vet, the complete race-enabled test
suite, and builds. Inspect a real terminal in unified/split modes with both theme
families, expanded context, full source, search, and horizontal scrolling; record
whether human terminal QA was performed instead of equating tests with human QA.

## Boundaries

- Always: reuse verified pinned bytes, existing escaping/style composition,
  bounded best-effort lexing, and snapshot/tab isolation; run required checks.
- Ask first: expanding scope to new languages/settings, adding dependencies,
  persisting full-source tokens, or raising resource limits.
- Never: replace pinned bytes with working files, invent comment anchors, retry
  failed lexing on every draw, weaken tests, or change source capture consent.

## Estimate and open questions

Expected size: a small TUI integration plus cache lifecycle/budget handling and
focused regressions. The earlier 2–4 hour estimate remains a rough estimate;
cache sharing across projection Models and deterministic budget tests are its
main sources of uncertainty.

No blocking product questions. Proposed scope includes full OLD/NEW views and
Guide expansion; review can narrow that scope before implementation.
