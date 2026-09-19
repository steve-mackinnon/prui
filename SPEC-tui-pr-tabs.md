# Spec: Persistent Pull-Request List and Review Tabs

## Objective

Make it possible to review several pull requests in one `pr-review` process.
The interactive TUI will always expose a fixed **PRs** tab containing the
existing repository/PR browser. Every PR selected from that browser opens in
its own review tab. A reviewer can return to the PR list or switch among the
already opened reviews without quitting or re-opening the application.

The user is a developer reviewing multiple open PRs for one or more remembered
repositories. Success means the reviewer can open PR A, return to the fixed PR
list, open PR B, and switch between the two while retaining the exact
in-process reading position of each review.

This is one capability: a TUI workspace shell. It composes the existing PR
browser and frozen review screen; it does not alter PR source pinning, GitHub
listing, session storage, or review-guide analysis.

### User journey

```text
PRs (fixed) --select PR A--> Review: owner/repo#A
    ^                              |
    |----- b / PRs tab ------------|
    |
    `--select PR B--> Review: owner/repo#B
                         ^       |
                         `-- t/T-'
```

- Start in the existing initial destination: a review tab for `open`/`resume`,
  the PR browser for no-argument/current-checkout launch and `prs` browser.
- The tab strip is always visible in an interactive TUI. Its fixed first tab
  is labelled `1 PRs`; opened review tabs receive the next available number
  through `9`, followed by escaped, disambiguating `owner/repo#number` text.
- Pressing `1` through `9` activates the correspondingly numbered tab. `b`
  remains a discoverable alias for tab `1`; `t` and `T` select the next and
  previous tabs, wrapping at either end. The header and help view list these
  keys.
- Selecting a PR opens a new review tab and activates it. If a tab for the
  same immutable review identity is already open, activate that tab instead;
  do not fetch/re-pin or create a duplicate tab.
- The workspace holds at most nine tabs: the fixed PRs tab plus eight review
  tabs. Selecting another PR when all review slots are occupied leaves the PR
  browser active and shows a stated error. A review tab has no close command
  in this first version. Tabs exist only for the process lifetime and all
  disappear when the program exits.

## Tech Stack

Go 1.26.8, Bubble Tea v2.0.9, and Lip Gloss v2.0.2. Reuse the existing
`internal/tui`, `internal/review`, `internal/source`, and `internal/session`
packages. No new dependency and no session-storage schema change are needed.

## Commands

```sh
go test ./internal/tui ./cmd/pr-review -count=1
go test -race ./internal/tui ./cmd/pr-review -count=1
go vet ./...
go test -race -count=1 ./...
go build ./...
./scripts/verify.sh
git diff --check
```

Manual interactive smoke test, only in a checkout and with already-authorized
GitHub access:

```sh
cd /path/to/github-checkout
pr-review
```

## Project Structure

```text
internal/tui/model.go           workspace, tabs, active-tab routing
internal/tui/lifecycle.go       PR-browser actions and review-tab creation
internal/tui/render.go          tab strip, active tab and narrow-layout render
internal/tui/bindings.go        tab navigation help/footer bindings
internal/tui/*_test.go          model, lifecycle, snapshot and program journeys
cmd/pr-review/wiring.go         initial workspace construction/wiring only if needed
cmd/pr-review/*_test.go         entry-point regression coverage only if needed
README.md                       interactive keyboard reference and behavior
```

## Code Style

Keep one explicit workspace state owned by `tui.Model`; review state is owned
by a tab value rather than copied into unrelated picker pages. Preserve the
existing typed asynchronous Bubble Tea messages and cancellation behavior.

```go
type reviewTab struct {
    identity source.Identity
    model    reviewViewState
}

func (m *Model) activateTab(index int) {
    m.activeTab = max(0, min(len(m.tabs)-1, index))
}
```

Use package-private tab types unless another package genuinely needs the
contract. Escape all tab labels and retain the established invariant that
terminal text and colors are presentation only. Do not encode workspace state
in persisted session records.

## Functional Requirements

### Workspace and tab state

1. The interactive TUI owns exactly one permanent PR-browser tab plus zero or
   more review tabs, capped at nine total workspace tabs.
2. The initial review session for `open` or `resume` becomes the first review
   tab. Initial PR-browser flows start with the fixed PRs tab active.
3. Each review tab retains its own session, selected unit/guide row, guide
   expansion, inventory/file-plan mode, focus pane, vertical and horizontal
   offsets, transient action error, and notice while another tab is active.
4. Resize state remains global, and every tab renders correctly after resizing.
5. Existing asynchronous work is associated with its initiating tab. A result
   must not overwrite whichever tab happens to be active when it arrives.

### PR browser tab

1. The PRs tab reuses the existing remembered-repository and open-PR picker
   flows, including loading, empty, error, retry, cancellation, escaped text,
   and viewport behavior.
2. `b` activates that existing fixed tab rather than constructing a second
   picker or discarding an open review.
3. Opening a PR from the browser creates/activates a review tab only after the
   existing pinned, read-only opening operation succeeds. A failure leaves the
   PRs tab usable and every open review unchanged.
4. The no-argument current-checkout entry continues to list that checkout's
   repository first. Its fixed PRs tab retains the current validated checkout
   needed to open selected PRs.

### Review-tab compatibility

1. Within the active review tab, every existing review key keeps its current
   behavior, including guide navigation, marking, evidence, analysis consent,
   refresh, new comparison, saved-session picker, and quit.
2. `N` replaces the active tab's comparison as it does today; it does not
   create a different tab. Opening a saved session from the existing session
   picker likewise replaces the active review tab, preserving the current
   command's single-review semantics.
3. Persisted marking continues to save immediately to the immutable review's
   existing local session record. Switching tabs neither saves nor resets any
   unrelated tab.
4. The existing `tab` key continues to expand/collapse guides or sections. It
   is not repurposed for workspace navigation.
5. `q` and Ctrl+C continue to quit the entire application, canceling active
   operations through the established lifecycle. Escape keeps its existing
   page/back behavior; at a top-level review it does not implicitly close a
   tab.

### Rendering and accessibility

1. The tab strip clearly marks the active tab using text that survives color
   removal; color can reinforce, never convey, selection.
2. Every visible tab includes its assigned number. Number keys `1` through
   `9` activate the matching tab; unavailable tab numbers have no effect.
3. Review content still receives the existing width-aware layout. In narrow
   terminals the tab strip is safely clipped/scrolls its labels without
   emitting over-wide lines or control sequences.
4. Full keyboard help, the concise footer, README, and plain-output behavior
   accurately distinguish interactive tabs from noninteractive output.
5. `--plain`, redirected output, and `TERM=dumb` retain their current
   one-review, noninteractive behavior; tabs are not simulated in text output.

## Testing Strategy

- Add focused model tests for fixed-PR-tab creation, numeric labels and
  hotkeys, next/previous wrapping, direct PR-list activation, deduplication,
  the nine-tab capacity message, and no state loss while switching.
- Add lifecycle tests for opening two distinct PRs, a duplicate selection,
  an opening failure, cancellation, and an async result arriving after the
  user selects another tab.
- Add scenario/program tests that open PR A, return to PRs, open PR B, switch
  both directions, and assert that independent list/diff positions and marks
  remain correct.
- Extend deterministic snapshots for a wide and narrow tab strip, including
  long or hostile PR titles/repository labels. Keep behavior assertions
  independent from golden updates.
- Retain regression coverage for all existing browser and review keys,
  startup modes, terminal restoration, persistence, and color/plain output.
- Use only synthetic sessions, fake GitHub clients, and local fixture
  endpoints. Never contact GitHub, read real credentials, or execute reviewed
  repository code in automated tests.

## Boundaries

- Always: preserve immutable source snapshots and read-only opening; keep a
  single fixed PRs tab and nine total workspace slots; retain existing review
  behavior within a tab; escape output; associate async results with their
  initiating tab; run focused and full synthetic verification before
  completion.
- Ask first: tab-closing/reordering/pinning, restoring tabs after restart,
  background preloading/polling, changing the current GitHub API/list limits,
  supporting multiple worktrees in a current-checkout browser, adding a
  dependency, changing storage schema, or changing CI.
- Never: refetch/re-pin merely to switch tabs; lose a review's in-memory state
  while another tab opens; persist workspace/tab state accidentally; repurpose
  the existing guide-expansion `tab` key; weaken cancellation, source safety,
  synthetic test coverage, or plain-output guarantees.

## Success Criteria

- The interactive TUI always exposes a discoverable, fixed PRs tab.
- Tabs are visibly labelled `1` through `9`, and pressing a displayed number
  switches directly to its tab.
- A reviewer can open at least two distinct PRs in one process and switch
  among their review tabs and PRs without quitting or reopening the app.
- Returning to a review restores its prior selection, view mode, pane focus,
  expansion, and scroll position; its marking state remains correct.
- Opening an already-open immutable PR activates its existing tab and does not
  create a duplicate session or network operation.
- Failed/canceled opening and late asynchronous responses do not corrupt or
  replace another tab's state.
- Existing commands, review keys, safety boundaries, persistence behavior,
  terminal behavior, plain output, and the full synthetic verification gate
  remain passing.

## Open Questions

None for the initial scope. Tab close/reorder/pinning, cross-process tab
restore, and background PR updates are intentionally deferred.
