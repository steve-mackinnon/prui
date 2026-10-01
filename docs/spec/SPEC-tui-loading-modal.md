# Spec: TUI blocking-loading modal

## Objective

Replace each blocking loading presentation in the interactive TUI with one shared,
centered loading modal. The modal makes work feel active rather than stalled,
keeps the current screen recognizable behind it, and makes cancellation discoverable
without claiming an operation can be cancelled when it cannot.

The user sees a compact, single-cell breathing-dot loader and the operation's current
loading text directly below the loader. A cancellable operation also shows `esc: cancel`.
This applies to the initial review open and every foreground operation that blocks
normal key handling; it does not change plain output or background work that leaves
the TUI interactive.

## Tech stack

- Go 1.26.8
- `charm.land/bubbletea/v2` for the event loop and animation ticks
- `charm.land/lipgloss/v2` and `github.com/charmbracelet/x/ansi` for existing
  terminal styling, width measurement, and escape-safe clipping

## Commands

```sh
go test ./internal/tui -count=1
go test -race ./internal/tui -count=1
go vet ./... && go test -race -count=1 ./... && go build ./...
```

## Project structure

```text
internal/tui/model.go          Model state, update loop, and top-level rendering
internal/tui/lifecycle.go      Foreground operation lifecycle and cancellation
internal/tui/picker.go         Picker loading operations
internal/tui/render.go         Escape-safe rendering primitives
internal/tui/chrome.go         Shared TUI chrome and status presentation
internal/tui/*_test.go         Behavior and snapshot coverage
internal/tui/testdata/screens/ Golden terminal screen baselines
SPEC-tui-loading-modal.md      This specification
```

## Design and interaction

### Modal composition

The overlay is centered within the current terminal viewport and has a simple box
border. The glyph and loading label share a row; the status notice follows beneath them.
Its content is, in order:

```text
┌────────────────────────────────────┐
│  ⠋ Working                         │
│                                    │
│  ↳ Checking metadata freshness…    │
│                                    │
│  esc: cancel                       │
└────────────────────────────────────┘
```

- The title is `Loading` for initial open and `Working` for an in-review action;
  implementation may use the current operation name if it is already available
  without adding new strings.
- The loader occupies exactly one terminal cell beside the loading label.
  Twelve Braille-dot frames grow, rotate, and shrink over a 1.2-second loop.
  It uses the same glyph in tiny viewports and never relies on color.
- The current, escaped notification text is rendered immediately beneath the loader.
  It wraps within the modal's interior width and available viewport height, with
  aligned continuation lines. If the viewport cannot show the full notice, the
  final visible line ends in an ellipsis. It never grows beyond the viewport or
  leaks terminal control bytes.
- `esc: cancel` is a separate final line only when the action has an active
  cancellation callback. For non-cancellable work, the line is omitted rather
  than replaced by a disabled or misleading instruction.
- The modal is at least 36 columns wide where the terminal permits it, is capped
  at 60 columns, and uses the available width on narrower terminals. At widths or
  heights too small to draw a complete box, render a clipped one- or two-line
  fallback containing the glyph and notice; it must never exceed the viewport.
- The background is not interactive while the modal is present. Where terminal
  color support exists it may be visually muted, but the modal border, title,
  animation, message, and cancel hint must convey the complete state without
  relying on color or dimming.

### Loading-state coverage

| Operation | Existing state/source | Overlay behavior | Cancel hint |
|---|---|---|---|
| Initial review open | `Loading` from `New` / `Init` | Full-screen underlying loading view with centered modal | Yes, after the loader owns a cancellable action context |
| Save reading progress or plan edit | `Busy` from `start` | Overlay over the active review | Only if the write is wired to a cancellable context |
| Metadata refresh | `Busy` | Overlay over the active review | Yes |
| New comparison | `Busy` | Overlay over the active review | Yes |
| Guide generation | `Busy` | Overlay over the active review after consent | Yes |
| Session/repository/PR listing | `Busy` | Overlay over the relevant picker or PR switcher | Yes |
| Open selected PR | `Busy` | Overlay over the picker/switcher or active review | Yes |

The shared overlay must be driven by explicit loading state, current notice, and
whether a cancellation callback is available. It must not infer cancellability
from `Busy` alone: some existing writes set `Busy` but do not create `actionCtx`.
This distinction prevents an Esc instruction that cannot work.

### Animation lifecycle

- Starting an overlay resets its frame to zero and schedules a 100 ms tick.
- A tick changes only the visual frame and schedules the next tick while the same
  loading operation remains active.
- Completion, failure, cancellation, resize, tab change, and modal replacement
  stop the animation naturally by making pending ticks no-ops. Stale ticks must
  not restart an overlay or mutate another tab's state.
- The animation must not launch workers, create network calls, or change the
  operation result/cancellation lifecycle.

### Keyboard semantics

- During a cancellable modal, `esc` invokes the operation's existing cancellation
  callback exactly once. `q` and Ctrl+C retain their current application-quit
  behavior.
- During a non-cancellable modal, `esc` does nothing and no Esc hint is shown.
- Other normal navigation and mutation keys remain blocked exactly as they are
  today. The PR-switcher exception stays intentional: users may choose an
  already-open review while another PR opens; its loading overlay must follow the
  operation without allowing a stale completion to replace the selected tab.
- Results preserve today's semantics: cancellation retains the current review or
  picker, failure leaves its existing error state, and late results cannot modify
  an unrelated active tab.

## Code style

Use a small, pure renderer and explicit state rather than embedding animation
logic in individual view branches:

```go
type loadingModal struct {
    active     bool
    cancelable bool
    frame      int
    notice     string
}

func renderLoadingModal(width, height int, modal loadingModal) string {
    // Escape and clip notice before assembling already-safe display lines.
    // The caller overlays it without changing operation state.
}
```

- Use Go naming and existing `gofmt` conventions.
- Escape external/notification text before styling or clipping, consistent with
  `Escape`, `clip`, and the terminal-control-byte tests.
- Keep operation ownership in `Model` and lifecycle functions; presentation code
  must not cancel contexts or mutate tabs.
- Prefer fixed, deterministic frame tests over timer sleeps.

## Testing strategy

Add focused unit and snapshot coverage in `internal/tui`.

1. Renderer tests cover wide, narrow, and tiny terminal dimensions; output fits
   width/height, uses no unescaped control bytes, and remains intelligible after
   ANSI stripping.
2. Snapshot tests cover initial loading, cancellable in-review loading, and a
   non-cancellable write. The snapshots assert glyph above notice and the presence
   or absence of `esc: cancel`.
3. Update-loop tests inject animation ticks and prove the frame changes only while
   active, stale ticks do not revive a completed modal, and no timer-dependent
   sleep is required.
4. Lifecycle tests cover Esc cancellation for each context-backed path and prove
   that Esc on non-cancellable work is ignored. Preserve current regression tests
   for snapshot retention, picker retry, and late PR-open result isolation.
5. Run the commands above, including the full race suite before merge.

## Boundaries

- Always: preserve the existing action/result and tab-isolation semantics; keep
  `--plain` output free of loading chrome and terminal sequences; escape and clip
  all dynamic messages; update golden screens intentionally.
- Ask first: add a dependency, alter persistence/network behavior, change
  cancellation guarantees of a storage write, or change the existing `q`/Ctrl+C
  quit contract.
- Never: send new telemetry, store loading data, expose source or credentials in
  the modal, make a non-cancellable action appear cancellable, or remove existing
  cancellation/error regression coverage.

## Success criteria

- Every foreground, input-blocking TUI operation presents the shared centered
  loading modal while it is active.
- The loading text wraps within the modal when space permits. The animated loader
  remains visibly above it and advances on deterministic ticks.
- A real cancel callback always yields a visible `esc: cancel` hint, and no hint
  appears when Esc cannot cancel the operation.
- Esc cancellation, failures, retries, current-review retention, and PR tab
  isolation retain their current behavior.
- Colorless, narrow, and tiny terminals remain readable, escape-safe, and within
  their dimensions.
- Plain mode remains unchanged and all existing plus new TUI tests pass.

## Open questions

None. The visual default is the single-cell loader above;
after the first rendered snapshot, a future styling-only pass can tune glyphs or
colors without changing the behavioral contract.
