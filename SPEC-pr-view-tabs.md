# Spec: PR Context View Tabs

## Objective

Give every open interactive PR review a stable, keyboard-accessible view
switcher: **Changes**, **Description**, and **Commits**. Changes remains the
default and retains today's guide/file/inventory workspace. Description and
Commits are PR-level views rather than entries in the file rail, so long prose
does not compete with source navigation and a future commit list has a natural
home.

Reviewers need to move between the PR's intent, its history, and its actual
diff without losing reading place, progress, comments, or another open PR's
state.

## Tech Stack

- Go 1.26.8+
- Bubble Tea v2.0.9 and Lip Gloss v2.0.2
- Existing immutable `session.Snapshot` records and `internal/tui` screen tests

## Commands

```sh
go test ./internal/tui -run 'Test(PRContextView|ReviewTab|Snapshot)' -count=1
go test ./internal/session ./internal/review ./internal/source -count=1
go test -race ./internal/tui ./internal/session ./internal/review ./internal/source
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/tui/model.go          per-open-review selected context view and key routing
internal/tui/render.go         header/tab strip and view-specific layout dispatch
internal/tui/bindings.go       discoverable view bindings and Health & help copy
internal/tui/*_test.go         model, keyboard, isolation, and render regressions
internal/tui/testdata/screens/ deterministic wide and narrow screen baselines
README.md                      reviewer-visible view and keyboard documentation
```

## Interaction Contract

- The interactive review header renders `Changes | Description | Commits` in
  that order at widths where the existing identity header is usable. The active
  view has textual, non-color identification; narrow terminals may abbreviate
  labels only if the active view remains explicit.
- `Changes` is selected whenever a review is first opened. It preserves all
  current guide/file/inventory keys and renders the existing two-pane/one-pane
  responsive workspace unchanged.
- `Description` and `Commits` replace the review body with one scrollable,
  read-only detail surface. They do not render a file rail, diff cursor,
  comment overlay, or source-progress control.
- `v` cycles Changes → Description → Commits; `V` cycles in reverse. The
  existing PR workspace owns numeric keys `1`–`9`, so context views must not
  overload them. The bindings apply only while the top-level review is active
  and are documented in Health & help.
- Switching views never performs I/O, changes the frozen snapshot, marks a
  slice, changes the current diff selection, dismisses a local comment draft,
  or changes freshness. Returning to Changes restores its exact tab-owned
  state.
- Context-view selection belongs to `reviewTabState`; opening/switching among
  PR workspace tabs restores each PR's own view. It is not stored in
  `snapshot.json` or `state.json` and resets to Changes on process restart.
- `--plain` remains a source-review output and does not expose context tabs.
  Picker, help, evidence, URL, loading, and error pages retain their present
  behavior; `esc` continues to leave overlays before changing a view.

## Code Style

Use a small typed enum at the UI boundary. Keep view dispatch pure and make
the safe default explicit; do not overload `Files` or `Inventory` with a new
meaning.

```go
type reviewView uint8

const (
    viewChanges reviewView = iota
    viewDescription
    viewCommits
)

func (m *Model) reviewView() reviewView {
    if m.activeTab < 0 { return viewChanges }
    return m.tabs[m.activeTab].review.View
}
```

All labels and content flowing from GitHub remain escaped before styling,
clipping, or wrapping. Color supplements the active-view label; it never
defines which view is selected.

## Testing Strategy

- Start with focused failing model tests for default selection, direct key
  routing, focus restoration, and PR-tab isolation.
- Add render tests at wide and narrow widths proving active-view wording,
  Changes parity, and no diff/file rail in context views.
- Update deterministic screen baselines only after behavioral assertions pass.
- Run race-enabled TUI tests because view selection travels through tab save and
  restore paths.

## Boundaries

- **Always:** preserve Changes behavior and all existing source/progress/comment
  invariants; keep selected state textual; maintain wide and narrow support.
- **Ask first:** changing an existing key binding, persisting UI view state,
  changing plain output, or adding a dependency.
- **Never:** put Description or Commits in the file rail; trigger network work
  on a view switch; make a context view imply source review or approval.

## Success Criteria

1. Every interactive PR review defaults to Changes and exposes the three views
   in stable order.
2. A reviewer can reach each view from the keyboard and identify the active one
   with color disabled.
3. Changes preserves its selection, scroll, progress, comment state, and
   existing navigation after any context-view round trip.
4. Two open PR reviews can select different context views without affecting one
   another; restarting the app returns each to Changes.
5. Existing picker, plain-output, guide, and comment regressions pass along
   with focused wide/narrow TUI tests.

## Open Questions

None. `v`/`V` are selected because the PR workspace already uses `1`–`9` for
open-review tab activation.
