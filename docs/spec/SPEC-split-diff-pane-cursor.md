# Spec: Split-Diff Pane Cursor and Comment Targeting

## Objective

Restore a visible, keyboard-operable comment cursor in the interactive
side-by-side diff and let a reviewer choose the commentable side of an aligned
row. The selected cell, rather than a whole rendered row, supplies the exact
GitHub target used by `enter`.

This addresses two effects of the initial split-diff implementation:

1. the split renderer does not render the unified view's cursor gutter, so a
   reviewer cannot see the selected source location; and
2. a paired deletion/addition row is collapsed to one `RIGHT` target before
   navigation. A reviewer therefore cannot deliberately choose its `LEFT`
   deletion.

The feature remains presentation and interaction state only. It does not
change frozen patches, session persistence, offline/plain behavior, comment
preflight, or the GitHub request shape.

## Assumptions

1. “Left/right” means the GitHub diff sides, not the app's file rail and diff
   pane. The new control applies only while the split diff itself is focused.
2. The intended scope is GitHub-commentable source cells. Blank cells,
   headers, hunk labels, and comment/editor cards are not selectable panes.
3. The existing `S` binding continues to toggle unified versus split layout.
   `P` (currently unbound) toggles the selected split cell. This keeps the
   existing `h`/`l`, arrow-key, and `tab` meanings intact.
4. The default on a paired row remains the current behavior: select `RIGHT`.

## GitHub Target Contract

The source target remains derived solely from frozen raw patch bytes:

| Visible cell | GitHub side | Commentable |
| --- | --- | --- |
| Deleted old-file line | `LEFT` | Yes |
| Added new-file line | `RIGHT` | Yes |
| Unchanged context, new/right cell | `RIGHT` | Yes |
| Unchanged context, old/left cell | — | No |
| Blank or structural/card cell | — | No |

The old context cell deliberately has no `LEFT` target. GitHub documents
`LEFT` for deletions and `RIGHT` for additions and unchanged context; offering
the old context cell as a target would create a request GitHub can reject.

## Interaction Contract

### Visible cursor

- Every split source cell reserves a two-character cursor gutter between its
  line marker and source text. The active cell renders the existing textual
  `› ` chevron; all other cells render two spaces. This is visible without
  color and does not move line-number gutters or the central separator.
- The split-detail header/footer identifies the active target side as
  `comment side: LEFT` or `comment side: RIGHT`. The chevron is the primary
  indication; the label makes the state discoverable and unambiguous in
  screenshots and colorless terminals.
- A row that has no commentable source cell has no cell cursor. Comment cards
  retain their existing full-width selected styling and action-menu behavior.

### Selecting a side

- With the split diff focused, `P` switches the cursor to the other
  commentable cell on the current aligned source row. On a paired replacement,
  it alternates `RIGHT` addition and `LEFT` deletion.
- For a right-only row (addition or context) and a left-only row (deletion),
  `P` leaves the cursor on its sole target and shows a concise non-error notice
  such as `no LEFT comment target on this row`. It must not move vertical
  position or open a composer.
- `enter` opens the composer for precisely the selected cell's frozen target.
  The composer labels the target path, line, and side as it does today; after
  submit/failure/cancel, the same cell remains selected.
- `P` is ignored while a composer, comment menu, picker, or background action
  owns input. It never changes an in-flight or drafted target.

### Navigation and layout changes

- `j`/`k` still move only among commentable source targets and selected comment
  cards. For source-to-source movement, keep the current side when the next
  selected row supports it; otherwise fall back to that row's only target.
  Thus a cursor moving from a deletion to context bounces from `LEFT` to
  `RIGHT`, while a subsequent paired replacement stays on `RIGHT` until the
  reviewer presses `P`.
- The selected item is semantic: a source selection is the full immutable
  `ReviewCommentTarget` (including path, side, and line), and a comment-card
  selection is its stable comment ID. Display-row indexes are cacheable
  projections only. This is necessary because unified/split layout, guides,
  overlays, and resize alter row indexes.
- `S` preserves the selected target while changing layouts. In unified layout
  the usual one-line chevron appears on that target. Returning to split
  restores the corresponding cell chevron. A `LEFT` selection maps to its
  deleted unified line; a `RIGHT` selection maps to its added/context unified
  line.
- Below 160 columns, the existing unified fallback retains the selected target
  and returns the chevron to the same split cell after a wide resize. `P` is
  unavailable in fallback/unified layout and advertises no misleading split
  interaction.
- Switching tabs, file/guide sections, raw inventory, or scrolling preserves
  the existing per-unit reading behavior. A stored selection that cannot be
  found after a projection change falls back to the first visible valid target
  using the existing direction-aware rule; it never fabricates a target.

### Comments and overlays

- Existing remote-comment overlays continue to anchor by their complete
  frozen target. A paired row may render `LEFT` thread blocks first and
  `RIGHT` blocks second; their placement does not change when the active cell
  changes.
- Navigating onto a comment card selects the card, not either source cell;
  `enter` opens its existing action menu. Leaving the card restores normal
  source-side selection rules.
- Drafts, comments, replies, reactions, and delete confirmation retain their
  current memory-only and preflight guarantees.

## Design and Implementation Boundaries

- Preserve separate concepts for: aligned display row, old/new source cell,
  immutable source target, selected target/comment ID, and terminal cursor
  glyph. Do not infer a target from a cell's rendered coordinates or text.
- Replace the split row's single `rowTarget` navigation authority with a
  target-aware cursor selection. `rowTarget` may remain only as the explicit
  initial/fallback preference (`RIGHT`, then `LEFT`).
- Keep target creation in raw patch parsing. Context must retain distinct old
  and new line counters even though only its new/right cell receives a target.
- Apply the two-space cursor gutter before calculating/clipping the cell's
  source-text width, and use display-width helpers for wide runes and escaped
  hostile text. Styling must remain after clipping.
- Do not change `source.ReviewComment`, GitHub request validation, GitHub API
  calls, snapshot schema, `--plain`, or the 160-column threshold.

## Project Structure

```text
internal/tui/model.go          semantic source/comment cursor state and key routing
internal/tui/render.go         per-cell cursor gutter and split-row projection
internal/tui/style.go          cell-safe marker/style helpers if needed
internal/tui/model_test.go     keyboard, target, resize, tab, draft regressions
internal/tui/render_test.go    gutter alignment, clipping, colorless rendering
internal/tui/*screens*         wide split snapshots for both selected sides
internal/tui/bindings.go       `P` help wording
README.md                      reviewer-visible split target behavior
```

## Testing Strategy

- Table-test target selection for paired replacement, deletion-only,
  addition-only, context, blank, hunk/header, and malformed/no-newline input.
  Assert exact path, line, and `LEFT`/`RIGHT` side.
- Model-test `P`, `j`/`k`, `enter`, and `S` from both sides of a paired row;
  prove a composer receives the intended frozen target and a one-sided row
  cannot switch to an invented side.
- Test resize fallback/restoration, tab isolation, guide/raw-detail movement,
  scrolling, composer cancellation/failure, and comment-card navigation.
- Render-test stable per-cell gutters, central-separator alignment, shared
  horizontal clipping, ANSI-free/colorless output, wide Unicode, and escaped
  terminal-control input.
- Run:

```sh
go test ./internal/tui -run 'Test(SideBySide|Comment|Guide|Raw|Color|Snapshot)' -count=1
go test -race ./internal/tui -count=1
go test ./cmd/prui -count=1
./scripts/verify.sh
git diff --check
```

## Success Criteria

1. A focused split diff always visibly identifies its selected commentable
   cell without relying on color.
2. `P` alternates between both valid sides of a paired replacement, and
   `enter` posts through the existing flow to exactly that cell's GitHub
   target.
3. One-sided rows and context never expose an invalid `LEFT` target.
4. Navigation, unified fallback, resize, layout changes, guides, and tab
   switching preserve the semantic selected target rather than a stale row
   index.
5. Existing comment overlays/actions and all source/privacy constraints remain
   unchanged.

## Open Questions

None for the first slice. This spec deliberately does not add multi-line
comments, comments on unchanged old-side context, mouse interaction, or a
new GitHub API contract.
