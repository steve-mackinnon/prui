# Spec: Calm Review Workspace Layout

## Objective

Redesign the interactive review TUI so a reviewer can orient themselves and
read a change without competing diagnostic text or a persistent shortcut dump.
The review screen will use a compact identity header, a clearly selected left
rail, an uncluttered detail pane, and a persistent status bar. The existing
`?` keyboard reference becomes **Health & help**, the single destination for
secondary shortcuts and state explanations.

The redesign applies the same chrome, selection, status, and footer language
to review, PR switcher, repository/session pickers, and help. It must not
change source pinning, persisted session data, progress semantics, guide
meaning, or the set of supported keyboard operations.

## Tech Stack

- Go 1.26.8+
- Bubble Tea v2.0.9
- Lip Gloss v2.0.2
- Existing snapshot and program tests in `internal/tui`

## Commands

```sh
go test ./internal/tui -count=1
go test ./internal/tui -run 'Test(Review|Guide|Raw|Color|Snapshot)' -count=1
go test ./cmd/pr-review -count=1
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/tui/style.go       semantic theme tokens and shared line styles
internal/tui/render.go      shared rendering primitives and secondary views
internal/tui/model.go       review layout assembly and responsive behavior
internal/tui/lifecycle.go   switcher/picker views and shared footer use
internal/tui/bindings.go    concise persistent affordances and Health & help groups
internal/tui/*_test.go      state, rendering, colorless, and snapshot regressions
internal/tui/testdata/      deterministic wide and narrow screen baselines
README.md                   reviewer-visible keyboard and layout documentation
```

## Layout Contract

### Shared chrome

Every interactive surface uses these reusable primitives rather than bespoke
headers and footers:

- **App header:** one clipped line identifying the current destination. The
  review form is `review · owner/repo#123 · guide 2/4`, followed by the
  discoverable `ctrl+p: switch PR` affordance where width permits.
- **Section header:** one clipped line naming the active mode and the detail
  context. Review uses `FILES 13` / `GUIDES 4` / `INVENTORY 26`; the right
  side names the selected path and unit kind when width permits.
- **Pane selection:** selected rows always carry a textual `›` marker and a
  non-color cue. An unfocused selected row uses a muted background/contrast;
  its focused counterpart adds reverse video. Read markers remain intact.
- **Status bar:** one bottom line for durable review state. It reports reading
  progress, inventory completeness, guide/analysis state, freshness, and
  unavailable-content count in priority order. Attention states use words and
  markers (`!`, `?`, `✓`), not color alone.
- **Health & help:** `?` opens grouped full-screen help: Navigate, Review,
  Views, Diagnostics, and App. It includes the complete shortcut reference,
  scope explanations, and any lower-priority actions that do not fit in
  persistent chrome.

### Review surface

At width `>= 100`, render a two-pane workspace. The left rail is at most one
third of the width and no more than 36 columns; the remainder belongs to the
detail pane. The separator is visually quiet. The selected item and detail
title agree on the active file/guide.

At width `< 100`, render one active pane at a time. The section header names
the current pane and selection; the bottom status bar remains visible. No
layout may rely on color, mouse capture, or hidden horizontal text.

The active mode controls only the left-rail hierarchy:

- **Guides:** selected guide/section/file portion and its existing detail.
- **Files:** deterministic file plan.
- **Inventory:** every raw review unit.

No mode changes its existing keyboard behavior or progress model.

### Status priority and compactness

The status bar must fit exactly one line and degrade from right to left. On a
wide terminal it shows, in order: progress, inventory, guides/analysis,
freshness, unavailable count, then `?: Health & help`. On a narrow terminal it
keeps progress, the highest-severity state, and the help affordance; omitted
healthy states are recoverable from Health & help.

Warnings take precedence over healthy indicators. For example, an incomplete
inventory or stale freshness must remain visible before `guides unavailable`.

### Pickers and overlays

The PR switcher remains an overlay that does not consume a review column. It,
the repository picker, session picker, and error/empty states use the shared
app header, selected-row treatment, and one-line status/footer primitive.
Picker-specific instructions remain local to the picker because typing/filter
behavior is only meaningful there.

## Code Style

Keep presentation helpers pure, compact, and named by meaning. Use semantic
line classes rather than raw terminal colors; all visible text must work after
styles are removed.

```go
func selectedStyle(focused bool) lineClass {
	if focused {
		return classSelectionFocused
	}
	return classSelection
}
```

Do not introduce a general component framework. Extract a helper only where
at least two interactive surfaces share the same chrome or status behavior.

## Testing Strategy

- Start each behavior slice with a focused failing Go test.
- Use state-based rendering assertions for header, selection, status priority,
  help grouping, and narrow-screen fallback.
- Update small deterministic wide/narrow golden screens only after reviewing
  the textual change; snapshots complement, not replace, behavioral tests.
- Keep the existing color-removal invariant: unstyled output contains the same
  wording, markers, and ordering as styled output.
- Run focused `internal/tui` tests for each slice; run `./scripts/verify.sh`
  and `git diff --check` after the completed redesign.

## Boundaries

- **Always:** preserve existing keys and state transitions; escape untrusted
  text before display; keep color supplemental; verify wide and narrow output;
  use shared chrome helpers for interactive TUI surfaces.
- **Ask first:** new dependencies, persisted-schema changes, changed key
  bindings, changing CLI/plain output behavior, or altering source/analysis
  lifecycle semantics.
- **Never:** hide an incomplete/stale state solely because the terminal is
  narrow; make color the only selection/status cue; remove raw inventory or
  deterministic file-plan access; add mouse capture or telemetry.

## Success Criteria

1. Normal wide review uses a one-line app header, one-line section header,
   two panes, and a one-line status bar; it no longer renders the long shortcut
   list in its footer.
2. The selected left-rail item is identifiable when its pane is both focused
   and unfocused, with and without terminal color.
3. `?` presents grouped Health & help containing every current key binding and
   the lower-priority review/status explanations.
4. Status text presents progress and the highest-severity health signal at all
   supported widths; wide status also includes normal inventory, guide, and
   freshness state.
5. Switcher and picker surfaces reuse the shared header, selection, and
   footer/status primitives without changing filter, cancel, empty, loading,
   or error behavior.
6. Wide and narrow snapshot tests, semantic-color tests, existing TUI tests,
   `./scripts/verify.sh`, and `git diff --check` pass.

## Open Questions

None. This specification deliberately keeps `ctrl+p` as the single persistent
shortcut because PR switching is a primary workspace action; all other
shortcuts move into Health & help.
