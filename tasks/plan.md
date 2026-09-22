# TUI reliability and automated verification

# Implementation Plan: Split-Diff Pane Cursor and Comment Targeting

## Overview

Make split-diff selection cell-aware so the visible cursor identifies one
commentable old/new source cell and `enter` submits that cell's immutable
GitHub target. The work repairs the missing split cursor and adds an explicit
`P` side toggle without changing the GitHub client contract or the existing
unified fallback.

## Architecture Decisions

- Store source selection as a complete `ReviewCommentTarget`; keep comment-card
  selection as its stable comment ID. Rendered row indexes are derived state.
- A split row exposes all valid source targets, with `RIGHT` then `LEFT` only
  as the initial/fallback priority. Context remains right-only.
- Render the existing text chevron inside a reserved gutter for each source
  cell. `P` changes selection only if the same row has another valid target.
- Retain semantic selection through `S`, resize, guide/raw-detail projection,
  scrolling, and per-tab save/restore. No new persisted field is introduced.

## Dependency Graph

```text
semantic target/comment selection
        |
        +-- split cell cursor gutter and target-aware rendering
        |       |
        |       +-- P key routing, help, composer integration
        |               |
        |               +-- guides/resize/overlay regressions and docs
```

## Task List

See the `Split-Diff Pane Cursor and Comment Targeting tasks` section in
`tasks/todo.md`. Tasks 1–3 are sequential because they share the cursor-state
contract; task 4 verifies integration after that contract is stable.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Row indexes change between layouts/overlays | High | Select full target/comment identity and resolve its current display row only when rendering or moving. |
| Left context is offered to GitHub | High | Preserve raw-patch target rules: only deleted cells are `LEFT`; context stays `RIGHT`. |
| Cursor gutters misalign colored or wide text | High | Reserve/gauge gutter before clipping and assert ANSI-free display widths. |
| Existing comments become confused with source selection | Medium | Keep comment-ID selection distinct; P is a no-op on comment cards. |

## Open Questions

None. `P` is the approved unbound side-toggle key; `S` continues to select the
layout.

# Implementation Plan: Side-by-Side Diff View

## Overview

Implement the approved interactive side-by-side diff view as four sequential,
verifiable slices. The feature projects the existing frozen unified patch into
aligned presentation rows; it does not obtain a second diff, alter inventory
data, or change the GitHub comment contract. Unified remains the initial
layout and is automatically used below 160 columns even when a tab prefers the
split view.

## Architecture Decisions

- Introduce a typed, tab-owned diff-layout preference (`unified` or
  `side-by-side`) in `internal/tui.Model` and its saved review-tab state. It is
  process-local and never enters `snapshot.json`, `state.json`, or `--plain`.
- Make hunk projection a pure transformation from existing escaped `diffLine`
  source rows into aligned render rows. A deletion run is paired only with the
  immediately following addition run, position by position; surplus cells are
  blank.
- Keep source provenance at the cell boundary. Old cells retain `LEFT`
  deletion targets; new cells retain `RIGHT` addition/context targets; context
  old cells intentionally have no second target. Cursor and Enter therefore
  retain today's observable commenting behavior.
- Make full-width structural rows explicit, rather than pretending headers,
  hunk labels, cards, comments, and editors are two empty source cells.
- Use a 160-column eligibility check at the layout dispatch boundary. A tab
  can prefer side-by-side while rendering unified at 159 or fewer columns;
  resize back to eligibility restores split without changing tab state.
- Keep one shared horizontal code offset. It clips unstyled source content in
  both code cells after fixed gutters and before styling, so gutters and the
  separator never drift.

## Dependency Graph

```text
pure patch-line provenance
        |
        +-- aligned hunk-row projection + row tests
        |       |
        |       +-- split renderer + width eligibility
        |       |       |
        |       |       +-- tab preference/key + cursor/overlay adaptation
        |       |               |
        |       |               +-- guides, snapshots, docs, complete verification
        |
        +-- target-equivalence regressions -------------------^
```

## Task List

The detailed, approved checklist will be appended to `tasks/todo.md` after
this plan's review gate. The intended order is:

1. Create the pure aligned-row projection and prove source/target equivalence.
2. Render split rows behind the 160-column eligibility boundary, preserving
   unified rendering exactly outside split eligibility.
3. Add tab-owned `S` preference, resize behavior, cursor selection, and
   comment/editor/overlay placement.
4. Route guides and raw detail through the projection, update bindings and
   README, then complete snapshots and full verification.

## Verification Checkpoints

- After task 1: focused pure projection and immutable target tests pass.
- After task 2: focused renderer tests prove widths 159 and 160, alignment,
  clipping, colorless output, and unified parity.
- After task 3: focused model/comment tests prove tab isolation, resize
  restoration, cursor target equivalence, and overlay ordering.
- After task 4: `go test -race -count=1 ./...`, `go build ./...`,
  `./scripts/verify.sh`, and `git diff --check` pass; inspect wide split and
  narrow fallback in a real terminal.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Display-row offsets diverge from the current line-stream offsets | High | Make the projection pure first; migrate offset/cursor logic only after row mapping and target-equivalence tests exist. |
| A paired replacement selects the old target rather than the current unified target | High | Define target priority at the row boundary: new/right, then old/left only if no new target; regression-test Enter targets. |
| Comment/editor rows break two-column vertical alignment | High | Emit them as explicit full-width rows immediately after their anchor; test old-then-new overlay ordering on paired rows. |
| Split columns become unreadable on common widths | Medium | Enforce the approved 160-column automatic unified fallback, render an explicit reason, and test resize restoration. |
| ANSI/color or wide characters misalign cell separators | High | Lay out escaped, unstyled text using existing display-width helpers, clip before styling, and extend color-removal/alignment tests. |
| Guide detail has a separate assembly path | Medium | Reuse the same projection after guide unit assembly and test repeated file occurrences. |

## Open Questions

None. The approved specification fixes the interactive `S` toggle, tab-local
non-persistence, preserved comment semantics, and automatic unified fallback
below 160 columns.

# Implementation Plan: Calm Review Workspace Layout

## Overview

Implement the approved review-layout specification as four small rendering
slices. The work reuses a few explicit TUI chrome helpers instead of creating a
generic component system: the helpers will represent the header, section
header, selected-row styling, and compact status/footer shared by interactive
surfaces. Existing model state, keys, source lifecycle, and plain output remain
unchanged.

## Architecture Decisions

- Keep layout state inside the existing `Model`; this is a presentation change,
  not a new navigation or persistence model.
- Extract only shared primitives with at least two consumers: app/section
  headers, selection style, and compact status/footer rendering.
- Use textual symbols and priority ordering as the source of truth for status;
  terminal styling reinforces rather than encodes state.
- Preserve the existing `?` page and binding registry, changing its wording and
  grouping rather than adding a second menu/page state.
- Keep the existing 100-column breakpoint and test a single-active-pane narrow
  fallback rather than inventing responsive terminal behavior.

## Dependency Graph

```text
shared chrome + selection semantics
        |
        +-- review header/section/status layout
        |       |
        |       +-- Health & help grouping and documentation
        |
        +-- picker/switcher chrome reuse
                |
                +-- final snapshots and full verification
```

## Task List

The detailed checklist is appended to `tasks/todo.md` under **Calm Review
Workspace Layout**. Tasks are intentionally sequential because they share the
same render paths and golden baselines.

1. Add and test shared chrome/selection primitives.
2. Apply them to review wide/narrow layout and compact health status.
3. Move secondary controls into grouped Health & help and document the changed
   persistent affordance.
4. Apply shared chrome to picker/switcher surfaces and complete snapshots.

## Verification Checkpoints

- After tasks 1–2: focused TUI tests and inspected wide/narrow review output.
- After tasks 3–4: complete `internal/tui` suite, command package tests,
  `./scripts/verify.sh`, and `git diff --check`.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Compact status clips important warnings | High | Establish and unit-test severity-first ordering before layout changes. |
| Styling changes alter colorless output | High | Extend the existing palette-removal/color-profile tests. |
| Shared helpers become an abstraction layer | Medium | Limit helpers to concrete common chrome with two consumers. |
| Golden snapshots obscure behavior regressions | Medium | Add focused state assertions before updating baselines. |

## Open Questions

None. The approved specification keeps `ctrl+p` persistent and moves all
remaining shortcut detail to Health & help.

The architecture review found three reproducible TUI failures despite a passing
race-enabled suite: browser startup calls an unset loader, picker back-navigation
reuses another screen's cursor, and long picker lists hide the selected row.
Application wiring also differs by entry point, offline guide generation is not
blocked, and session listing performs synchronous I/O in the update handler.

## Design

- Preserve pinned source, complete inventory, immutable snapshots, separate
  progress, explicit guide consent, and synthetic-only test fixtures.
- Give each picker its own selection and viewport behavior. Initialize both
  entry screens through a shared model foundation.
- Assemble application operations once for every entry point. Enforce offline
  mode at the operation boundary, including guide generation.
- Move picker storage reads into asynchronous commands and communicate results
  through messages. Cancellation must retain the current review.
- Use the existing Go/Bubble Tea stack for scenario and program tests; add a
  small compiled-binary PTY suite for terminal behavior that model tests omit.
- Keep screen snapshots small and deterministic, with explicit baseline updates.
  Use message/output synchronization with deadlines instead of fixed sleeps.
- Treat automated regression coverage as the normal development gate. Human
  accessibility/usability assessment remains a separate, occasional activity.

## Execution

Tasks and acceptance criteria are tracked in [todo.md](todo.md). Commit this plan
before implementation. Parallel streams own TUI behavior (tasks 1–4), CLI wiring
and contract documentation (tasks 5–6), and binary terminal tests (task 9).
The integrating agent owns CI, snapshots/program harness coordination, review,
and final verification. Shared files must have one writer at a time.

Dependencies: picker scenarios follow tasks 1–4; entry-point program tests follow
task 5; final verification follows every task. Commit coherent verified slices.

## Verification

Baseline: `go vet ./...`, `go test -race -count=1 ./...`, and `go build ./...`
pass on macOS with local fixture servers permitted. The review measured 73.8%
overall statement coverage; coverage is diagnostic, not a substitute for journeys.
The repository has no checked-in CI or verification wrapper at baseline.

Run focused regression tests during each fix, then the complete verification
command including PTY tests. CI must exercise Linux and macOS. Never use real
GitHub/provider credentials or execute reviewed repository code in fixtures.

## Risks

- Async work must finish/cancel before the store closes; verify cancellation and
  failed listing behavior, not just successful navigation.
- Terminal capture needs bounded waits, output diagnostics, cleanup, and stable
  environment/dimensions to avoid hanging or flaky CI.
- Snapshot baselines must not include random session IDs, timestamps, or local
  paths; updating a baseline must be an explicit developer action.
- Existing `.humanlayer/` data is unrelated and must remain untouched.

# Implementation Plan: Agent PR Verifier

## Overview

Implement the approved agent verifier capability map in four small modules. The
live runner is a manually invoked, real-PR acceptance tool; visual artifacts,
measurements, and guide evaluation are additive follow-up capabilities. Normal
verification and CI remain synthetic-only.

## Architecture Decisions

- Keep the live runner outside the standard verification command and use the
  existing read-only pinning path with a fresh private store.
- Emit versioned JSON with relative artifact paths so an agent can summarize a
  run without parsing terminal output.
- Build terminal artifacts from the PTY screen reconstruction, not a user GUI.
- Report timings as observations, separately for local and network-backed work;
  they do not gate success.
- Evaluate only stored guides through the separate read-only `eval-guides`
  command. Live verifier runs never bypass guide-upload consent.

## Dependency Graph

```text
report contract + validation
        |
        +-- live PR PTY journey
        |       |
        |       +-- terminal artifacts
        |       +-- performance reporting
        |
        +-- stored-guide evaluator
```

## Task List

Tasks are recorded in [todo.md](todo.md). The first checkpoint follows the
report contract and live journey; artifact and timing modules then integrate
without changing source pinning. Guide evaluation is independently testable
against synthetic stored sessions.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Go has no portable standard-library PTY API | High | Reuse the repository's Python 3 standard-library PTY protocol behind a bounded, explicit command boundary. |
| Real PR runs vary by host/network | Medium | Keep them manual and report context/timings rather than asserting thresholds. |
| Terminal data can contain hostile bytes | High | Preserve existing escape rules, cap output, and never treat content as instructions. |
| Guide upload consent could be bypassed | High | Only evaluate stored sessions and keep generation out of every new command. |

## Verification

Each module starts with a focused failing test, then runs its package tests.
The final checkpoint is `./scripts/verify.sh`, `git diff --check`, and a
manual real-PR run only when the user supplies a PR and already-authorized
GitHub access.

# Implementation Plan: Persistent PR List and Review Tabs

## Overview

Add a process-local Bubble Tea workspace that keeps one fixed PR browser and
up to eight opened review tabs. Number labels `1` through `9` provide direct
keyboard activation without changing any persisted session schema or the
existing read-only source path.

## Architecture Decisions

- Keep workspace state in `internal/tui.Model`; the existing review view state
  becomes tab-owned so a switch cannot reset another review's selection or
  scroll position.
- Keep `1` as the fixed PRs tab and cap the workspace at nine tabs. `b`, `t`,
  and `T` remain discoverable navigation alternatives, while `tab` preserves
  guide expansion.
- Attach every asynchronous result to its initiating tab before applying it.
  A late open/list result must never replace the currently active review.
- Retain current command entry points, pinning, cancellation, local progress,
  plain output, and synthetic-only verification.

## Dependency Graph

```text
tab state + key routing
        |
        +-- review-view state isolation
        |       |
        |       +-- open/deduplicate/capacity lifecycle
        |
        +-- tab-strip rendering + bindings + documentation
                |
                +-- scenario, snapshot, and program regression coverage
```

## Task List

Tasks are appended to [todo.md](todo.md), after the existing unrelated
checklists. Work is sequential because all slices share `tui.Model`.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Existing `Model` mixes workspace and review state | High | First isolate review state behind tab activation, with a failing state-retention test. |
| Async work lands after a tab switch | High | Carry tab identity in operation results and test late delivery. |
| Number keys collide with review keys or narrow layouts | Medium | Reserve `1`–`9` only at workspace routing, preserve review keys, and snapshot narrow rendering. |
| Dirty worktree overlaps TUI files | High | Preserve current edits; stage/commit nothing unless the user separately asks to isolate them. |

## Open Questions

None. Close/reorder/pin/restore tabs and background PR refresh remain out of scope.

# Implementation Plan: Guide Path Overflow

## Overview

Implement the approved guide-list overflow behavior in two sequential vertical
slices: width-safe static middle truncation first, then an animation state
machine that reveals the selected overflowing path. The change stays entirely
inside the interactive TUI and does not alter guide data, persisted sessions,
plain output, or existing detail-pane horizontal scrolling.

## Architecture Decisions

- Construct guide file rows from stable chrome (selection marker, indentation,
  read marker, and unit suffix) plus a separately measured path region. Only
  that path region may truncate or animate.
- Use terminal display-width operations, preserving existing escaping and
  ANSI-safe final clipping. Do not use byte or raw-rune offsets as horizontal
  positions.
- Keep auto-scroll state scoped to `Model` as transient presentation state; it
  is neither persisted nor folded into `Model.Horizontal`, which remains the
  detail pane's user-controlled scroll position.
- Schedule at most one conditional Bubble Tea tick while the selected guide
  portion is overflowing and the list pane is focused. State changes invalidate
  the current path offset and prevent future ticks from changing a stale row.
- Preserve current key bindings. The feature needs no user-facing settings or
  new dependency.

## Dependency Graph

```text
display-width-safe guide path rendering
        |
        +-- static middle truncation for non-selected rows
                |
                +-- selected-path scroll state + conditional tick
                        |
                        +-- focused regression, snapshot, and full verification
```

## Task List

The detailed checklist is appended to `tasks/todo.md` under **Guide Path
Overflow**. Tasks are sequential because the scroll slice uses the path-region
measurement established by truncation.

1. Add a pure guide-path label helper and static middle-truncation tests.
2. Add transient selected-path scroll state and an invalidatable conditional
   timer, with state-machine tests.
3. Integrate both render paths, review relevant fixed-width output, and run the
   full verification gate.

## Verification Checkpoints

- After task 1: targeted guide rendering tests cover short, long, renamed,
  escaped, and Unicode paths without line overflow.
- After task 2: model tests prove movement, endpoint behavior, reset, and stop
  conditions without relying on wall-clock sleeps.
- After task 3: focused TUI tests, any affected screen baseline comparison,
  `./scripts/verify.sh`, and `git diff --check` pass.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Splitting at rune/byte offsets breaks wide Unicode paths | High | Use display-width-aware windowing and explicit wide-character boundary tests. |
| A stale scheduled tick moves a newly selected path | High | Include a generation/context identity in tick handling and test selection/resize/view changes. |
| Animation causes background work in an inactive pane | Medium | Schedule ticks only while the focused guides-list overflow predicate is true. |
| Unit suffix or selection marker disappears | Medium | Reserve immutable chrome before allocating the path region; test exact suffix/marker retention. |
| Rendering changes leak into diff scrolling | Medium | Keep list state separate from `Model.Horizontal` and regression-test left/right behavior. |

## Open Questions

None. Tick cadence and endpoint pause are implementation constants; their
behavioral state transitions, rather than elapsed real time, will be tested.

# Implementation Plan: Pull Request Line Comments

## Overview

Add one explicitly submitted, line-anchored GitHub review comment without
weakening the frozen, read-only review path. Build the outbound contract first,
then preserve source-line identity in the diff, then add the composer, and
finally wire a mandatory current-revision preflight.

## Architecture Decisions

- Keep writing behind a new `source.ReviewCommenter` interface; do not add it
  to the read-only `source.GitHub` metadata interface.
- Preserve optional target metadata beside rendered diff lines. Never infer
  GitHub lines from escaped terminal text or scroll offsets.
- Send JSON through stdin to `gh api --input -`; user-written text never goes
  in a process argument, session, log, or error.
- Require application-level metadata equality immediately before every post;
  mismatch stops before the GitHub write.
- Make the composer transient, tab-owned UI state; each explicit submit creates
  one immediate GitHub review comment, not a pending batch review.

## Dependency Graph

```text
comment request contract + stdin runner
          |
          +-- diff-line target mapping + cursor
          |             |
          |             +-- composer + injected submit action
          |                            |
          +----------------------------+-- current-head preflight + docs + E2E
```

## Task List

The detailed, agent-sized checklist is appended to `tasks/todo.md` under
**Pull Request Line Comments**. Task 1 establishes the shared contract. Tasks
2–6 then proceed in order because they either consume that contract or edit
the shared `tui.Model` and its detail representation; Task 7 closes the
feature.

## Verification Checkpoints

- Contract checkpoint: source package tests prove validation and no argument
  exposure of the comment body.
- TUI checkpoint: raw and guide line targeting plus cursor/composer scenarios
  pass with no changed plain output.
- Delivery checkpoint: preflight/no-write paths, program journey, documents,
  full verification, and an authorized manual PR comment are complete.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Wrong line/path is posted | High | Derive target during raw hunk parsing and unit-test every patch grammar case. |
| A stale snapshot posts feedback to a moved PR | High | Require exact metadata equality immediately before POST; use frozen SHA. |
| Comment text leaks through process inspection or persistence | High | JSON stdin only; memory-only composer; bounded/sanitized errors. |
| Async action changes another open review | High | Attach target tab and transient composer state to action results; program-test tab switch behavior. |
| `Enter` disrupts existing navigation | Medium | Preserve list-pane behavior; only diff-pane Enter opens a composer. |

## Open Questions

None. The approved first slice excludes replies, batch reviews, line ranges,
and comment history.

# Implementation Plan: Inline Review Comment UX Extension

## Overview

Replace the full-page line-comment composer with a local inline editor and add
a read-only, per-tab comment overlay. The change preserves the existing
explicit-write and freshness contract; it improves only the reviewer’s context
before and after a post.

## Dependency Graph

```text
bounded comment read + canonical POST response
                     |
detail row expansion + target-only cursor invariants
                     |
inline rune-aware editor (including Backspace/Delete)
                     |
per-tab overlay load/refresh + immediate post insertion
                     |
help, docs, synthetic paths, manual PR acceptance
```

## Decisions

- Inline rows belong to the review view and never become selectable diff
  targets.
- The overlay is fetched on an online review start and through explicit `c`
  refresh; it is never persisted or fetched offline.
- Only exact frozen-head/path/side/line matches render. Omit ambiguity rather
  than inventing an anchor.
- A successful POST uses the returned canonical comment to render immediately;
  a later refresh reconciles the tab.
- The inline editor owns its own rune cursor, so `Backspace` and `Delete` are
  deterministic for Unicode input.

## Task List

The ordered agent-sized tasks and verification checkpoints are appended under
**Inline Review Comment UX Extension** in `tasks/todo.md`. The first four
tasks touch shared source/model/rendering code and therefore run sequentially;
the documentation and acceptance task follows their integration.

# Implementation Plan: Inline Review Comment Actions

## Overview

Extend the read-only inline comment overlay with an explicit action path for
replying, deleting a viewer-owned comment, and adding a reaction. Diff
navigation will be able to select anchored comment boxes as well as diff
targets; pressing `enter` on a selected comment opens a local action menu.
Every mutation remains intentional, fresh-targeted where applicable, and
memory-only outside GitHub.

## Architecture Decisions

- Keep comment-box selection distinct from line-target selection. A selected
  comment carries its stable GitHub comment ID and frozen anchor; it never
  becomes a draft target by inference.
- Add narrow source interfaces for reply, delete, reaction, and authenticated
  viewer identity. All writes use JSON stdin, bounded response parsing, and
  safe errors; no comment/action content reaches process arguments or sessions.
- The action menu is local TUI state. `enter` opens it only after an explicitly
  selected comment; choosing a mutation performs one request, while escape
  cancels without a write.
- Delete is offered only when the loaded author equals the authenticated viewer.
  GitHub remains the authority: authorization failures leave the overlay intact
  and report an escaped, actionable error.
- Reactions use an explicit, finite picker. Replies and successful reactions
  update the ephemeral overlay immediately from canonical responses; successful
  deletion removes only that matching comment box from its originating tab.

## Dependency Graph

```text
viewer + bounded action source contracts
                 |
comment-row selection and action menu
                 |
reply / delete / reaction lifecycle with tab isolation
                 |
docs, snapshots, synthetic + manual acceptance
```

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| A comment selection is mistaken for a diff target | High | Use separate selection value types and regression-test every enter path. |
| Delete appears for another reviewer | High | Compare to bounded authenticated viewer identity before enabling it; retain GitHub authorization as the final boundary. |
| Action results land in a different tab | High | Attach each action to tab ID, comment ID, and generation; ignore stale results. |
| Reactions/replies leak into persisted state | High | Keep all action state and canonical responses tab-local and exclude them from sessions/plain output. |

# Implementation Plan: Threaded Inline Reply and Reaction Rendering

## Overview

Refine the inline comment-action overlay so reply drafting and replies read as
one indented thread, reaction selection is digit-driven, and reaction totals
appear as compact chips in a comment box's bottom border.

## Architecture Decisions

- Keep the existing `commentActionMenu` as the action controller, but represent
  reply editing as a dedicated local editor state rather than status-line text.
- Store reply parentage in ephemeral overlay state keyed by canonical comment
  ID. Render a reply under its direct parent with one fixed indentation level.
- Use a single ordered reaction table shared by picker rendering and digit
  dispatch. Render its GitHub values with emoji labels in UTF-8 locales and
  bounded token fallbacks in explicit non-UTF-8 locales. Aggregate per-comment
  reaction values at render time so the map stays memory-only and deterministic.

## Dependency Graph

```text
thread/reply local state + ordered reaction table
                    |
         threaded box and border-chip rendering
                    |
      keyboard dispatch, snapshots, documentation
```

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Reply draft looks like status text | High | Render a bordered editor below the parent; test no draft in status. |
| Reaction digit collides with workspace tabs | Medium | Consume digits only while the reaction picker is active. |
| Border chips overflow narrow boxes | Medium | Clip the chip region within the existing inner width. |

# Implementation Plan: PR Context Views

## Overview

Ship the approved `Changes | Description | Commits` PR-scoped views in thin
slices. The tab shell and frozen Description flow come first; the commit list
uses the same shell only after its separate capture contract is proven.

## Architecture Decisions

- Keep selected context view and context scroll state process-local in the
  existing per-review-tab state; use `v`/`V` so workspace tab keys `1`–`9` keep
  their current meaning.
- Treat presentation data as immutable session content. Pin equality compares
  source identity and SHAs only, not an edited PR description.
- Preserve legacy snapshot readability with explicit absence states; context
  tabs and offline resume must never cause network access.

## Dependency Graph

```text
view enum + keyboard routing
        |
        +-- description capture, snapshot, cache semantics, render
        |
        +-- commit capture, snapshot, render
```

## Task List

1. Context tab shell and process-local per-review view state.
2. Frozen description source/session contract, including revision comparison.
3. Description rendering, keyboard scrolling, snapshots, and documentation.
4. Frozen commit source/session contract with bounded consistency checks.
5. Commit-list rendering, keyboard behavior, snapshots, and documentation.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Presentation edits perturb source pinning | High | Test `SamePinnedRevision` independently from source content. |
| Remote text injects terminal controls | High | Validate at source boundary and escape before render. |
| Context state leaks between open PRs | Medium | Persist it only in `reviewTabState` and add tab-isolation tests. |
| Commit response races a changed head | High | Re-read metadata and discard/retry mismatched results. |

## Open Questions

None. The approved specs defer rich Markdown, pagination beyond 100 commits,
commit-diff drill-in, and live context refresh.

## Amendment: Single-review workspace

The PR selector replaces, rather than supplements, the active review. Remove
the in-memory review-tab workspace and its capacity, duplicate, numeric-key,
and delayed-result routing. Preserve the active session while a selector open
is pending; install the new session and reset transient state only on success.
Disk-backed snapshots and the explicit session picker remain unchanged.
