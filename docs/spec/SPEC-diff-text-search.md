# Spec: Diff Text Search

Status: Approved for implementation on 2026-10-04. The user confirmed current-section Guide scope and authorized proceeding without further phase gates.

## Objective

Let reviewers find literal code text in the Files and Guide diff views, inspect results grouped by file in a compact popover, and jump to highlighted matches in the diff. The supplied Linear screenshot is the visual reference: a query field, overall match/file counts, file headings with counts, code snippets, and simultaneous highlights in the underlying diff.

This is one capability: finding and navigating source occurrences. Grouped results and inline highlights share the same matches and must agree.

## Confirmed scope and defaults

- `/` opens code search from either pane on Files or Guide. Filename/path filtering moves to `F`; `2` selects Files without editing its filter.
- V1 uses case-insensitive, literal, single-line substring matching. No regex, whole-word toggle, or case toggle.
- Search covers saved patch source: additions, deletions, and included unchanged context. It does not fetch omitted context or full files.
- Files searches every changed file in the frozen comparison, independently of the filename filter.
- Guide searches only the current section, as confirmed by the user.
- Search is transient local UI state. No network access, source upload, storage migration, or new dependency is expected.

## User experience

### Open and edit

Expose a clickable `Find (/)` control in the diff header and document it in keyboard help. `/` focuses the query field in a spacious popover inset from the upper-right review area, using a rounded theme-colored border and padding. Keep the diff visible beneath it where space permits. Existing editors, confirmations, loading dialogs, and pickers retain input priority; search cannot take input from them.

The popover contains:

1. A query field with a visible caret only while editing; no Clear/Close controls.
2. A scope label and `23 matches in 2 files` summary.
3. Scrollable file groups, each with filename, directory, count, and matching source snippets.
4. A focus-specific keyboard footer.

On narrow terminals, use the available width and height rather than overflowing. At sizes too small for results, keep the query and a resize hint visible. Resizing preserves query, active occurrence, and input focus. The popover captures mouse events inside its bounds; background controls do not activate through it.

Typing updates results and visible diff highlights. Empty input shows `Type to search diff text` and no highlights. No matches shows `No matches in saved diff text`. Preserve typed spaces; do not silently trim the query. Support cursor movement, Home/End, Backspace/Delete, and single-line paste; reject line breaks and cap input at 1,024 UTF-8 bytes without splitting a rune.

### Results and navigation

- Count individual non-overlapping occurrences, including multiple occurrences on a line. Show one navigable result per occurrence, with a snippet centered on that occurrence, line number, and `+`, `-`, or context marker. Selected occurrence has a visible textual selection indicator.
- Group by stable file identity, using new path when present and old path for deletions; show old-to-new path for renames. Do not conflate identical basenames in different directories.
- Files groups follow inventory order. Guide groups follow first appearance within scope; results retain unit/patch order within each group. Within a source line, order matches left to right.
- Up/Down select the previous/next occurrence; PageUp/PageDown move through results. While editing, ordinary letters including `j`, `k`, and `q` edit the query. Escape transfers focus to results; `n/N`, `j/k`, or arrows then navigate without changing the query. `/`, Tab, or clicking the input returns to editing. Arrow navigation scrolls the results list but does not move the underlying diff until activation.
- Enter or a result click closes the popover, focuses the diff, reveals the source line and matched columns, and selects the corresponding existing source target when valid. This activation must not also open a comment composer or mark a file read.
- If the destination is hidden by the Files filename filter, activation clears that filter and shows a brief notice. Merely searching leaves the filter unchanged.
- In Guide, activation selects the correct section/file occurrence and expands only the hierarchy needed to reveal it.
- Escape while editing focuses results; Escape from results dismisses the popover, restores prior pane focus, and retains query/highlights. Reopening retains the query and active result and focuses editing. Backspace/Delete can empty the query; there are no Clear/Close buttons.
- Match-to-match navigation in V1 occurs through the popover. Existing `n/p` navigation is unchanged.

### Scope and lifecycle

Files scope includes available textual hunks across the comparison even if the file rail is filtered. Guide scope includes all unit IDs in the active section, including collapsed children. If the selected Guide row has no section, prompt the reviewer to select a section; do not silently search a different scope. The raw-inventory mode uses all available comparison text and labels that scope explicitly.

Store separate query/selection state for Files and Guide in each open review. Changing the active guide section recomputes its matches, preserving query and retaining an active match only if still present. Switching tabs closes the popover and restores that view's query/highlights when revisited. Switching review sessions or replacing the comparison drops old match indexes; no asynchronous response may populate a different session or scope. Theme/layout/width changes preserve semantic selection.

### Matching boundaries

- Match raw source text after the patch marker, excluding filenames, file/hunk headings, line numbers, no-newline markers, guide prose, comments, and editor text.
- Match Unicode runes using simple case folding; do not normalize accents or expand characters into multiple characters. Match offsets always refer to the original source bytes. Define malformed UTF-8 as non-searchable text and report skipped content rather than silently producing unreliable offsets.
- A context row counts once even when split view displays it on both sides. Highlight both copies; activation uses its valid RIGHT target. Deletions use LEFT and additions use RIGHT.
- Repeated projections of the same unit/source occurrence do not inflate counts. Retain scope/occurrence information needed to navigate to the first applicable appearance deterministically.
- Binary, unavailable, oversized, or otherwise absent source is not searchable. Show an explicit `Saved diff text only` scope hint and the number of files with skipped/unavailable text when applicable. Zero matches must not imply the full source files were searched.

## Highlighting contract

Highlight every visible occurrence of the query in source cells, with a distinct active occurrence. Result snippets use the same match ranges. Search is additive highlighting; it does not remove nonmatching rows from the diff.

Preserve syntax foregrounds where legible, diff markers and gutters, and addition/deletion row semantics. Use theme-aware search styling with an active-match distinction. In colorless output, retain source bytes and rely on result selection markers, line/side labels, and the existing diff cursor; do not insert characters into source text to simulate highlights. A human checks legibility in dark/light themes and a real terminal.

Match identity and byte offsets must survive terminal escaping, Unicode display widths, tabs, word wrapping, horizontal panning, unified/split projection, and comment overlays. Clip highlight spans with the same source-to-display mapping as the text. Terminal control sequences in source or query are escaped, never executed. Search must not change frozen comment targets or create a comment target for an otherwise invalid line.

## Existing architecture and feasibility

The repository uses Go 1.26.8, Bubble Tea v2.0.9, Lip Gloss v2.0.4, and charmbracelet/x/ansi v0.11.7, as pinned in `go.mod`.

Inspection found these existing integration points:

| Location | Existing responsibility / search implication |
| --- | --- |
| `internal/tui/file_filter.go` | Separate filename/path filtering; move filtering to `F` and explicitly handle navigation outside its selection. |
| `internal/tui/filedetail.go` | Caches a continuous all-file diff for the selected session. |
| `internal/tui/guidedetail.go` | Builds a continuous selected-guide diff and preserves section/file occurrence anchors. Search scope needs explicit unit membership. |
| `internal/tui/render.go` | `textHunkLines` parses frozen patch bytes and stores old/new line coordinates separately from escaped text; split projection is shared. |
| `internal/tui/syntax.go`, `diff_wrap.go` | Translate source spans through escaping, wrapping, panning, and clipping. Search spans need equivalent transformations and independent styling. |
| `internal/tui/model.go`, `mouse.go`, `workspace_geometry.go` | Own input routing, focus, hit testing, and workspace geometry. |
| `internal/tui/style.go`, `internal/theme/` | Semantic row styling and theme presentation. |
| `internal/tui/*_test.go`, `testdata/screens/` | Model, renderer, snapshot, and terminal regression coverage. |
| `docs/spec/`, `tasks/` | Feature specs and feature-specific implementation plans/checklists. |

Use a dedicated, typed match identity containing snapshot ownership, file/unit identity, patch source row, and original byte range. Guide display occurrence and optional valid comment target are separate metadata. Rendered row indexes are derived coordinates, never identity. Keep the matcher independent of ANSI-formatted screen strings.

Search indexes/results are bounded by the current snapshot and scope. Do not reparse all patches or rebuild syntax tokens on every keystroke or repaint. Do not mutate cached source rows to apply a query. Keep only up to 10,000 navigable occurrences; when exceeded, label results `First 10,000 matches; refine your query` and do not present a partial file count as the total. Inline highlighting still covers visible source rows. Long scans must be cancellable or broken into bounded work, and stale query/scope generations discarded.

Implementation follows [the feature plan](../../tasks/diff-text-search-plan.md), authorized by the user on 2026-10-04.

## Code style

Follow existing `gofmt`, small typed helpers, unexported package-local state, table-driven tests, and immutable presentation caches. Existing source-coordinate logic is the model:

```go
case '+':
    row.newLine = new
    row.target = target(f.NewPath, "RIGHT", new)
    new++
case '-':
    row.oldLine = old
    row.target = target(f.OldPath, "LEFT", old)
    old++
```

Search positions supplement these raw coordinates; never derive posting coordinates from highlighted strings or screen positions.

## Commands

Run from the repository root. Commands below are implementation verification requirements, not claims of checks already performed for this documentation change.

```sh
# Build and development entry point
go build ./...
go run ./cmd/prui --help
# Focused behavior and integration tests
go test ./internal/tui -count=1
# Existing complete gate: formatting, vet, race tests including PTY tests, build
./scripts/verify.sh
# Additional static analysis and documentation whitespace
golangci-lint run ./...
git diff --check
```

## Testing strategy and acceptance criteria

1. Pure matcher tests cover literal metacharacters, case folding, multiple occurrences, whitespace, Unicode offsets, malformed text, empty queries, query limits, and exclusions. Context is counted once in split view.
2. Model tests open/edit/clear/close/reopen search on Files and Guide; verify scope, filename-filter interaction, grouping/order/counts, no matches, repeated guide occurrences, rename/deletion identity, unavailable-content labels, and result cap wording.
3. Navigation tests activate an exact old/new source occurrence through wrapping, clipping, split layout and its narrow fallback, resize, and comment overlays. Verify horizontal reveal, focus, hierarchy selection, and unchanged read progress. Enter activation cannot open or submit a composer.
4. Rendering tests assert query spans across syntax-token boundaries, escaped source, tabs, wide characters, multiple matches, active styling, theme changes, and ANSI-free colorless output. Cached source must remain unchanged.
5. Lifecycle tests prove stale query/session/scope work is ignored, indexes are replaced, and existing modal/editor input wins. Mouse tests ensure the popover prevents background click-through.
6. Add wide, narrow, and split snapshots of grouped results and highlighted diffs; preserve unrelated snapshots. Include a PTY flow that opens search, edits, selects a result, and returns to review without input leakage.
7. Benchmark query edits and result navigation against a synthetic 100,000-line snapshot. Proposed interactive target: warm query-to-results p95 under 100 ms on a recorded development machine; document hardware, allocations, cap behavior, and cancellation latency. CI should use deterministic bounded-work assertions rather than a fragile universal wall-clock threshold.
8. Manually verify real-terminal keyboard/mouse behavior, small-window resize, light/dark themes, and colorless readability. Automated snapshots alone do not establish usability.

## Boundaries

- **Always:** Preserve `CONSTRAINTS.md`, frozen source provenance, escaping, offline operation, comment-side validity, existing filename filtering and navigation, and bounded session-owned caches. Update user help/reference when implemented. Keep tests synthetic.
- **Ask first:** Expand to regex/full-file search or Commits/Description, add dependencies, change storage or source-capture limits, or alter established bindings. Guide scope is fixed to the current section.
- **Never:** Fetch source as a search side effect, execute reviewed content, transmit queries/source, persist search state, log source snippets, mutate review data to highlight text, weaken tests, or make a GitHub write through search.

## Review gate

The earlier review gate is closed. The user approved current-section scope and explicitly authorized proceeding on 2026-10-04.

## Interaction revision — 2026-10-04

User requested `/` for code search, `F` for filename filtering, removal of Clear/Close, and Escape-to-results with vim-style `j/k` navigation. These requirements supersede the initial key bindings and dismissal behavior.
