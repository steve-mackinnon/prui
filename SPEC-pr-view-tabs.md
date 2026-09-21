# Spec: PR Context View Tabs

## Objective

Give the single active interactive PR review a stable, keyboard-accessible view
switcher: **Diff**, **Description**, and **Commits**. Diff remains the default
and retains today's guide/file/inventory workspace. Description and Commits are
PR-level views rather than entries in the file rail, so long prose does not
compete with source navigation and a future commit list has a natural home.

Reviewers need to move between the PR's intent, its history, and its actual
diff without losing their place. Switching to another pull request happens
through the PR selector, not through an in-memory multi-PR workspace; durable
review material remains in the existing disk-backed sessions.

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
internal/tui/model.go          one active review, selected context view, and key routing
internal/tui/render.go         header/tab strip and view-specific layout dispatch
internal/tui/bindings.go       discoverable view bindings and Health & help copy
internal/tui/*_test.go         model, keyboard, isolation, and render regressions
internal/tui/testdata/screens/ deterministic wide and narrow screen baselines
README.md                      reviewer-visible view and keyboard documentation
```

## Interaction Contract

- The first line of every interactive review is a dedicated, low-noise tab
  strip: `› Diff [F1] · Description [F2] · Commits [F3]`. The `›` marks the
  selected tab and must remain visible without color. It replaces the prior PR
  identity, guide count, and persistent `ctrl+p` text; those details remain
  available from the switcher and Health & help.
- `Diff` is selected whenever a review is first opened. It preserves all
  current guide/file/inventory keys and renders the existing two-pane/one-pane
  responsive workspace unchanged.
- `Description` and `Commits` replace the review body with one scrollable,
  read-only detail surface. They do not render a file rail, diff cursor,
  comment overlay, or source-progress control.
- `F1`, `F2`, and `F3` directly select Diff, Description, and Commits. `v` and
  `V` retain their existing next/previous cycling behavior. The function keys
  avoid the PR workspace's numeric-tab bindings; all context bindings apply
  only while the top-level review is active and are documented in Health & help.
- Switching views never performs I/O, changes the frozen snapshot, marks a
  slice, changes the current diff selection, dismisses a local comment draft,
  or changes freshness. Returning to Changes restores its exact tab-owned
  state.
- The model holds exactly one active review. Context-view selection and context
  scroll are process-local and reset to Diff whenever the selector successfully
  replaces that review; they are not stored in `snapshot.json` or `state.json`.
- `ctrl+p` opens the active repository's PR selector over the current review.
  It lists remote open PRs only—there is no opened-review section, numeric
  review-tab navigation, capacity limit, or duplicate activation path. Enter
  opens the selected PR; its completed session atomically replaces the current
  review and resets transient review state. Cancellation, failure, and Escape
  retain the current review unchanged. The existing `s` session picker remains
  the explicit way to resume disk-backed historical snapshots.
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

func (m *Model) selectReviewView(view reviewView) {
    m.ContextView = view
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
3. Diff preserves its selection, scroll, progress, comment state, and
   existing navigation after any context-view round trip.
4. The PR selector replaces one active review only after the selected PR opens;
   cancel/error keeps the old review, and neither the multi-review capacity nor
   numeric review-tab behavior exists.
5. Existing picker, plain-output, guide, and comment regressions pass along
   with focused wide/narrow TUI tests.

## Open Questions

None. `F1`–`F3` provide direct tab selection; no numeric review-tab bindings
remain.
