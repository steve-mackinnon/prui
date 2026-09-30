# Spec: Shared Diff Rendering

## Objective and scope

Consolidate the commit and main-review diff presentation without coupling their
navigation or write policies. Preserve existing screens and controls, reduce
repeated patch parsing during commit comment editing, and keep the active editor
visible after a terminal resize. The user authorized specification, task planning,
and delegated implementation on 2026-09-30.

This is one TUI refactor with a resize correction. Existing commit-discussion and
split-diff specifications remain authoritative for comment eligibility and input.
Commit side-by-side mode, new controls, delivery changes, capture, and persistence
are outside scope. No new dependencies are required.

## Rendering contract

1. A pure shared text-hunk renderer accepts a file, unit, patch bytes, and frozen
   source identity/SHA. It never constructs a review Session, mutates an inventory,
   or decides whether a historical comment may be posted.
2. Parse hunk starts, walk line counters, escape text, classify rows, and construct
   raw-coordinate anchors once. Preserve preamble suppression, trailing empty
   rows, multiple hunk headers, no-newline markers, and RIGHT context targets.
3. Keep old/new line numbers separate from escaped source text. Main unified
   presentation remains unchanged; commit presentation formats its existing
   numbered columns at a presentation boundary. Existing split projection must
   continue to receive source text with its patch marker intact.
4. Retain caller-owned file headings, commit metadata, and informational card
   wording. Share non-text presentation only where it actually removes duplicate
   logic without introducing a configurable renderer framework.
5. Presentation anchors never replace raw provenance checks in internal/commits.
   Preserve invalid-path rejection and all head/historical delivery gates.

## Cache and overlay contract

Cache immutable source rows for only the selected session/commit snapshot. Source
rows do not depend on editor state, discussion generation, width, or theme. These
events may rebuild overlays or visible presentation, but must not reparse patches.
Switching sessions or selected commits replaces the bounded source cache.

Keep the existing overlay cache where useful, with correct invalidation for
discussion refresh, comment creation/deletion, width/theme, and composer state.
Editor draft, cursor, blink, error, and cancellation must appear immediately;
cached source rows must never acquire editor or discussion content. Avoid extra
copies when no overlays are present. Retain the existing ordering of discussions,
comment labels, and editor rows.

## Viewport and frame contract

After shrinking or growing a terminal with a commit composer open, show the whole
editor when it fits. For an editor taller than the viewport, show its active text
cursor. Reuse the existing commit visibility helper at resize; keep main-review
scroll and commit per-SHA scroll independent.

Extract a small shared pane-body assembly helper for the main and commit views.
Callers provide already-presented rows, pane widths, focus, and existing border
classes. Preserve their labels, borders, selected-row styling, wide layout at
100 columns, narrow focused-pane layout, and viewport height. Do not introduce a
general layout system or merge view state merely to share chrome. Header/geometry
extraction is optional only if it makes the implementation smaller and clearer.

## Structure and code style

Production and tests live under internal/tui; use Go's testing package and gofmt.
Use narrow typed functions, immutable cached rows, and existing diffLine/styledLine
types. For example, a caller formats a source row by value rather than editing the
cached row's Text. Source identity comes from the owning view's frozen data.

Plans use the repository's feature-specific tasks/<feature>-plan.md and
tasks/<feature>-todo.md convention, preserving unrelated open task lists.

## Acceptance and verification

- Shared-renderer tests cover added, removed, context, multiple headers, preamble,
  no-newline, escaped content, and exact SHA/path/side/line targets. Existing main
  split behavior and commit snapshots remain unchanged.
- An allocation regression or benchmark demonstrates that editing/blinking a
  warmed large commit no longer allocates a new escaped/parsed row per patch line.
  Test visible editor changes, discussion refresh, cancellation, and source-cache
  replacement, not just cache implementation details.
- A regression test first reproduces resize hiding the commit editor, then passes
  after the fix; cover a short and a multiline editor and main scroll isolation.
- Existing narrow/wide, mouse, theme, program, and screenshot tests verify shared
  framing without golden churn or changes to review progress and comment rules.
- Focused tests and the complete verification gate pass:

```sh
go test ./internal/tui ./internal/commits -count=1
./scripts/verify.sh
git diff --check
```

Always preserve source provenance and isolated view state. Ask before adding
dependencies or expanding product behavior. Never weaken tests, change storage
schemas, post remote comments, or enable unsupported historical targets here.
