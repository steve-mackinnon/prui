# Spec: Side-by-Side Diff View

## Objective

Add an interactive side-by-side representation of textual Git hunks so a PR
reviewer can compare old and new source at the same vertical position. The
existing unified diff remains the default and remains the only representation
in `--plain` output. The feature is presentation-only: it must retain the
frozen patch bytes, inventory, guide/file navigation, read progress, and
GitHub review-comment contract exactly as they are today.

A reviewer toggles the preferred interactive layout with `S`. The preference
is tab-owned and process-local: it is neither written to a session nor applied
to another open PR. Side-by-side view is available only at a terminal width of
at least 160 columns. Below that threshold, a tab which prefers side-by-side
renders the existing unified view and states `side-by-side needs 160 columns`
in the review header. When a resize reaches 160 columns, the preferred
side-by-side representation resumes without another keypress.

## Tech Stack

- Go 1.26.8+
- Bubble Tea v2.0.9
- Lip Gloss v2.0.2 and `charmbracelet/x/ansi`
- Existing frozen `inventory` patches and `source.ReviewCommentTarget` values

## Commands

```sh
go test ./internal/tui -run 'Test(SideBySide|Comment|Guide|Raw|Color|Snapshot)' -count=1
go test -race ./internal/tui -count=1
go test ./cmd/prui -count=1
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/tui/style.go          semantic cell/row presentation roles
internal/tui/render.go         hunk-to-row projection and shared rendering helpers
internal/tui/model.go          tab-owned display preference, responsive dispatch, scroll/cursor state
internal/tui/guidedetail.go    guide detail assembly using the shared row projection
internal/tui/bindings.go       S toggle wording in persistent help
internal/tui/*_test.go         projection, state, comment, resize, and rendering regressions
internal/tui/testdata/screens/ wide split and narrow-fallback screen baselines
README.md                      reviewer-visible layout behavior and keyboard reference
```

## Interaction Contract

### Layout selection and responsive fallback

- `S` toggles the active tab's preferred layout between `unified` and
  `side-by-side`; the footer and Health & help document the binding.
- A first-opened tab prefers `unified`. Opening, switching, replacing, or
  resuming a review initializes that tab to unified. Switching tabs restores
  each tab's own preference.
- At width `>= 160`, a side-by-side preference renders a two-column detail
  pane. The existing file/guide rail, pane focus, frame treatment, and detail
  scroll behavior remain in place.
- At width `< 160`, the model renders unified detail even when the preference
  is side-by-side. It preserves the preference, vertical position, selected
  target, draft, comments, and horizontal offset. The header identifies the
  active unified fallback and its 160-column requirement.
- The unified preference always renders unified at every currently supported
  width. `--plain` never reads or exposes the preference.

### Side-by-side hunk projection

Text hunk patch bytes remain the source of truth. The renderer first derives
the same escaped source lines and immutable comment targets as unified mode,
then projects them into display rows:

| Unified patch input | Old cell | New cell |
| --- | --- | --- |
| context | old line and number | new line and number |
| deletion run followed by addition run | deletion at the matching run offset | addition at the matching run offset |
| excess deletion | deletion | blank |
| standalone addition or excess addition | blank | addition |

Runs pair only when additions immediately follow deletions in the same hunk;
the pairing is positional, not a second diff algorithm. Hunk headers and
file dividers span both cells. Metadata, binary, gitlink, unavailable, and
comment/editor rows remain full-width cards. The old and new cells each have a
fixed line-number gutter, an explicit `-`/`+` or context marker, independent
semantic styling, and a stable central separator.

Every projected row is vertically atomic: scrolling, paging, guides, file
jumps, and cursor visibility operate on rendered rows, never on one cell in
isolation. Horizontal scrolling is shared by the two code cells, applies only
to their unstyled escaped source text, and leaves number gutters, markers, and
the separator fixed. Styling occurs after clipping and cannot add, remove, or
reorder displayed text.

### Comments and cursor behavior

- The feature preserves the existing target contract without adding a new
  side-selection mode. A deletion cell carries its `LEFT` target. An addition
  or context new cell carries its `RIGHT` target. Context's old cell is
  informational and never creates a second comment target.
- The existing focused-diff `j`/`k` behavior selects commentable rendered rows.
  On a paired deletion/addition row it selects the existing `RIGHT` addition;
  on a deletion-only row it selects `LEFT`; on context it selects `RIGHT`.
  This exactly preserves which target Enter would have selected in unified
  mode.
- Enter opens the existing inline editor beneath the projected row. Existing
  remote comment threads are inserted beneath their anchored row, after the
  row's source cells; a paired row can therefore contain both old- and
  new-side thread blocks in deterministic old-then-new order.
- Composer drafts, action menus, remote overlays, preflight, escaping,
  invalid-path refusal, and offline/plain restrictions retain their existing
  semantics. No comment data or display preference is persisted.

### Guide, file, and non-text behavior

Guides continue to assemble ordered immutable units. A file divider is still
inserted at each guide file transition; text units beneath it use the active
layout. File-plan and full-inventory selection continue to identify the same
unit IDs, and read progress remains file-slice based. A layout toggle does not
change selection, guide expansion, scroll meaning, cursor target, or marking.

## Code Style

Represent the rendering unit explicitly rather than teaching `diffLine` to
mean two incompatible layouts. Cells retain source provenance separate from
already-escaped terminal text; full-width rows do not fabricate empty cells.

```go
type diffCell struct {
	line   *diffLine // nil means no source line on this side
	number int
}

type diffRow struct {
	old, new *diffCell
	full     *diffLine
}
```

The final names may vary, but the implementation must keep these separations:
immutable source target, escaped unstyled text, semantic style, and rendered
row position. Do not re-parse terminal text, construct a second Git diff, add
a dependency, or mutate the frozen inventory to support presentation.

## Testing Strategy

- Start with pure, table-driven projection tests covering context, replacement
  runs of equal and unequal lengths, pure insertions/deletions, multiple hunks,
  no-newline markers, and escaped hostile patch bytes. Assert cell text, line
  numbers, semantic classes, and `LEFT`/`RIGHT` targets.
- Add model tests for the `S` toggle, tab isolation, resize fallback and
  automatic restoration, and preservation of selection/scroll/cursor/draft.
- Add focused comment tests proving the side-by-side cursor selects the same
  immutable target unified mode selected, and that old/new remote overlays
  remain anchored and ordered.
- Add wide screen tests for headers, gutters, separator alignment, clipping,
  shared horizontal scrolling, guides, raw inventory, and non-text cards. Add
  a 159-column snapshot/assertion proving the explicit unified fallback.
- Preserve the color-removal invariant: stripping styles yields identical
  wording, labels, ordering, widths, and target-independent text. Run race
  tests because the tab state is saved/restored around asynchronous results.
- Run the complete verifier and inspect the wide split and narrow fallback in
  a real terminal; automated render tests do not establish terminal usability.

## Boundaries

- **Always:** derive rows only from frozen stored patch bytes; escape before
  layout/styling; preserve GitHub comment target derivation; keep old/new cells
  vertically aligned; retain an explicit, readable narrow fallback; test both
  colorless and framed/unframed layouts.
- **Ask first:** changing existing key bindings, the 160-column threshold,
  plain output, persisted state/schema, GitHub comment-side behavior, source
  inventory semantics, or adding a dependency.
- **Never:** execute a second diff against a checkout; mutate snapshot data;
  make side-by-side the default without approval; silently squeeze two code
  columns below the threshold; use color, mouse input, or terminal glyphs as
  the only indication of target/focus; create comment targets from display
  coordinates or text.

## Success Criteria

1. On a 160-column-or-wider terminal, `S` renders each textual hunk as aligned
   old/new columns with stable line-number gutters, central separator, correct
   blank cells, and full-width structural/card rows.
2. On widths below 160, a tab preferring side-by-side renders the existing
   unified diff and visibly states the fallback condition; crossing the
   threshold restores split rendering with the same review state.
3. Unified remains the initial mode, per-tab layout preferences do not leak
   across review tabs, and no layout state is stored in a session or plain
   output.
4. Every displayed comment action resolves to the exact same frozen target as
   before: deletion `LEFT`; addition/context `RIGHT`; non-commentable material
   remains non-commentable.
5. Guide/file/inventory navigation, marks, headers, overlays, drafts, scrolling,
   colorless rendering, and all existing source/privacy guarantees remain
   behaviorally intact.
6. Focused projection/model/render/comment tests, the race-enabled TUI suite,
   `./scripts/verify.sh`, and `git diff --check` pass; wide split and narrow
   fallback behavior are manually inspected.

## Open Questions

None. The approved narrow-terminal behavior is automatic unified fallback.
