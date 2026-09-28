# Spec: Guide Path Overflow

## Objective

Make long file paths in the interactive guides list identifiable without
requiring a wider terminal. Every overflowing path uses stable middle
truncation when it is not selected. When the selected row is a file portion
whose path does not fit, its path automatically pans horizontally so the
reviewer can read the complete rendered path.

The feature is presentation-only. It does not change guide navigation, file
ownership, reading progress, stored sessions, plain output, or the detail-pane
horizontal-scroll controls.

## Capability Map

| Module id | Responsibility | Depends on |
| --- | --- | --- |
| `path-truncation` | Render long guide-list paths with stable middle truncation. | — |
| `selected-path-scroll` | Animate an overflowing selected guide-file path by terminal display cells. | `path-truncation` |

Build order: `path-truncation` → `selected-path-scroll`.

## Tech Stack

- Go 1.26.8
- Bubble Tea v2.0.9 for messages and scheduled UI updates
- Lip Gloss v2.0.2 and Charm ANSI helpers for terminal display width, clipping,
  and ANSI-safe rendering
- Existing TUI tests in `internal/tui`

## Commands

```sh
go test -race ./internal/tui -count=1
go test -race ./internal/tui -run 'TestGuide|Test.*Path|Test.*Scroll' -count=1
go test ./internal/tui -run '^TestScreenSnapshots$' -count=1
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/tui/guides.go        guide row labels and guide-list construction
internal/tui/model.go         selection, view rendering, and timed UI updates
internal/tui/render.go        display-width-aware clipping helpers
internal/tui/guides_test.go   guide-list behavior and rendering tests
internal/tui/model_test.go    model state, key/message, and width tests
internal/tui/testdata/screens deterministic screen baselines, if wording/layout changes
```

## Behavior

### Path truncation

- Apply only to `portionRow` file-path labels in the interactive guides list.
  Guide and section titles retain their existing clipping behavior.
- Reserve the row's existing selection marker, indentation, read marker, and
  unit suffix (for example, ` [text_hunk]`) before calculating path space.
- When the full escaped path label fits, render it unchanged.
- When it does not fit, render a middle-truncated label containing a single
  ellipsis (`…`), a non-empty prefix, and a non-empty filename-side suffix
  whenever the available path width permits. Preserve the unit suffix.
- A rename label (`old/path -> new/path`) is one path label. Prefer retaining
  the destination filename-side suffix; do not let truncation expose an
  unescaped or partially escaped terminal sequence.
- The final rendered line must never exceed the left-pane width, including
  wide Unicode display cells and all markers/suffixes.

### Selected-path auto-scroll

- Run only when all of these are true: guides are displayed, the current
  selection is a `portionRow`, the list pane has focus, and the full rendered
  path label exceeds the available path space.
- The marker, indentation, read marker, ellipsis-free unit suffix, and row
  selection treatment stay stationary. Only the path segment moves.
- Start at the beginning of the path, pause briefly, advance one terminal
  display cell per tick, pause briefly at the end, then return to the start and
  repeat. The exact tick and pause duration are implementation details, but
  must feel readable rather than jittery.
- Reset the auto-scroll position to the beginning when selection, list focus,
  active view, terminal width, or the selected row's full path changes. Stop
  scheduling ticks whenever its start conditions are false.
- Existing `left`/`right` keys retain their current detail-pane horizontal
  scrolling behavior. The guide-list auto-scroll has no new key binding and
  must not modify `Model.Horizontal`.
- On narrow terminals, the behavior applies when the list pane is active; it
  stops while the diff pane is active.

## Code Style

Keep text transformation pure and separate from state transitions. Measure and
slice in terminal display cells, not byte offsets or rune counts. Preserve the
project's escape-before-style rule.

```go
// The label keeps its suffix; only the available path region is transformed.
func guidePathText(full string, pathWidth int, offset int) string {
	return scrollWindow(full, pathWidth, offset)
}
```

Do not add a general animation abstraction or a dependency solely for this
feature. A narrowly scoped Bubble Tea message and small model state are
sufficient.

## Testing Strategy

- Begin each capability with focused failing tests in `internal/tui`.
- Test unchanged short paths, static middle truncation, narrow widths, renamed
  paths, escaped/control-like path content, and wide Unicode display-width
  boundaries.
- Test that the selected overflowing portion advances on a scheduled tick,
  pauses/restarts at its bounds, and resets/stops on every listed state change.
- Test that guide/section rows and the detail pane do not acquire list-scroll
  behavior, and that `Model.Horizontal` remains dedicated to the detail pane.
- Preserve existing no-overflow and ANSI/colorless rendering invariants.
- Run the full repository verification gate after implementation.

## Boundaries

- **Always:** preserve text escaping, display-width limits, existing key
  meanings, and non-color selection cues; make animation conditional and
  cancellable through normal model state.
- **Ask first:** adding a dependency, changing a persistent model/session
  format, changing shortcut bindings, applying this behavior outside guides, or
  changing plain output.
- **Never:** continuously animate every row, alter guide/file navigation or
  completion semantics, use byte/rune slicing that corrupts display width, or
  allow a rendered line to exceed its pane.

## Success Criteria

1. In the guides list, every unselected overflowing file path remains
   distinguishable through a width-safe middle-truncated representation and
   retains its unit suffix.
2. A selected overflowing guide file path automatically reveals its complete
   rendered path through a readable repeating horizontal pan, with fixed row
   chrome and suffix.
3. Auto-scroll is inactive for short paths, guide/section rows, an unfocused
   list, non-guide views, and inactive narrow-screen list panes; it resets
   predictably when context changes.
4. Detail-pane `left`/`right` behavior and `Model.Horizontal` remain unchanged.
5. Focused TUI tests, screen checks if changed, `./scripts/verify.sh`, and
   `git diff --check` pass.

## Open Questions

None. The tick cadence and endpoint pause are intentionally left to the
implementation plan, provided tests verify the state-machine behavior rather
than wall-clock timing.
