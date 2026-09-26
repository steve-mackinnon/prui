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

# Implementation Plan: Markdown PR Description Rendering

## Overview

Replace Description's escaped plain-text presentation with safe, cached,
width-aware GitHub-flavored Markdown rendering. This fixes CRLF bodies showing
literal `\\r` while preserving frozen-description lifecycle, offline behavior,
tab-owned scroll, and non-interactive handling of untrusted content.

## Architecture Decisions

- Use `charm.land/glamour/v2` with an explicitly selected repository style and
  current Description body width. Do not use environment-selected styles.
- Normalize CRLF and lone CR before parsing. Represent disallowed C0/DEL input
  controls visibly before parsing; never pass user-supplied escape sequences
  through as terminal control output.
- Raw HTML stays literal text; links are informational only, and images have
  no fetch or display behavior.
- Cache rendered ANSI lines by frozen body, width, and active style. Invalidate
  on resize, style change, or a replacement review; navigation only slices the
  cache.

## Dependency Graph

```text
input normalization + renderer adapter
                 |
                 +-- Description cache and scroll integration
                         |
                         +-- snapshots, documentation, full regression gate
```

## Task List

See the `Markdown PR Description Rendering tasks` section in `tasks/todo.md`.
The renderer adapter must land before model integration; final documentation
and snapshots follow the completed interactive path.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| PR text injects terminal controls | High | Normalize/sanitize input before parsing and test ANSI/OSC payloads. |
| ANSI output wraps or slices incorrectly | High | Use configured width, preserve ANSI lines, and assert stripped display widths. |
| Re-rendering a 1 MiB frozen body on every keypress | Medium | Cache by body, width, and style; test cache reuse during scroll. |
| Markdown HTML differs from GitHub's display | Medium | Keep HTML literal and document the display-only scope. |

## Open Questions

None. Raw HTML literal rendering is approved; link activation, remote images,
and manual description refresh remain out of scope.

# Implementation Plan: SQLite Storage — Fresh Start

Status: Approved for implementation on 2026-09-26; Sol agents executing.
Spec: [SPEC-sqlite-storage.md](../SPEC-sqlite-storage.md), approved for planning.
Task list: [SQLite Storage — Fresh Start tasks](todo.md#sqlite-storage--fresh-start-tasks).
The user authorized appending these sections; all earlier work remains intact.

## Outcome and scope

Deliver in-process SQLite persistence for newly created macOS/Linux stores.
Delete the old session/guide/registry filesystem backend completely. No import,
legacy reader, compatibility Save, fallback, dual writes, or Windows port.
Keep theme configuration, temporary Git acquisition, and in-memory rendering
outside this replacement. Users install only the application executable, with
no SQLite executable, library, service, or database setup prerequisite.

## Architecture decisions

1. **One concrete backend.** `internal/session` retains domain validation and
   exposes Store; `internal/session/storage` owns the independent SQL connection,
   initialization, schema, and transaction helpers. No ORM or generic backend
   interface. SQL and transaction objects never reach CLI/TUI consumers.
2. **Embedded driver.** Evaluate and pin `modernc.org/sqlite` (v1.59.0 is the
   documented starting candidate). SQL-01 verifies the exact dependency graph,
   target builds, license, and reachable vulnerabilities before accepting it.
   Record the actual selected version here. The [driver reference](https://pkg.go.dev/modernc.org/sqlite@v1.59.0)
   documents the CGo-free engine; runtime isolation tests establish the packaging
   claim for our binary. No driver has been installed during planning.
3. **Durability.** DELETE journal, EXTRA synchronous, foreign keys enabled, one
   connection per Store, five-second busy bound, context cancellation, and short
   explicit write transactions. Verify connection-local settings each time a
   connection is created. Never retain query rows while requesting the only
   connection again. Roll back all failed multi-statement operations.
4. **Deduplicate source, not sessions.** Canonical versioned source JSON blobs
   omit checkout, guides, and DerivedFrom. Immutable session metadata references
   source/bundle digests; independent sessions retain independent progress.
   Logical SnapshotReference includes all immutable session fields, making a
   wrong reference or generation fail state updates without re-reading source.
5. **No compatibility API.** Replace all seven production Save calls and the
   direct CLI test call with UpdateState; delete Save at cutover. Keep the
   existing Entry callback type but replace its full Record with summary fields.
6. **Local storage identity.** Fresh defaults use `pr-review/storage`, not
   `pr-review/sessions`. `--store` remains a directory. An unknown/old nonempty
   directory is rejected without parsing or changing its records.
7. **Validation stays layered.** Verify source/bundle digests and domain references
   on create/load/cache reuse, enforce relational constraints in SQL, and check
   indexed identities against decoded payloads. Listing is explicitly metadata
   only; corruption of a source blob is discovered on load, not by loading all
   patches during a list operation. Database failures are not cache misses.

## Shared contract to freeze in SQL-02

### Session boundary

- `UpdateState(ctx, id, expectedGeneration, snapshotReference, StateUpdate)
  (State, error)` accepts only progress and freshness. It returns committed state;
  callers assign it only after success. Retain context through review.Mark by
  adding a context parameter and changing its callers/tests in the cutover wave.
- `Entry`: ID, Repository, Number, HeadSHA, ReviewedCount, SliceCount,
  RevisionStatus, UpdatedAt, Err. No Record pointer or lazy blob-bearing field.
  Store.List keeps its signature shape, returns ordered summaries, and exposes
  already known validation failures without claiming every blob was checked.
- Retain Create/Load/Delete and guide/registry/comparison method semantics;
  add context to internal SQL operations without spreading database details.
  Existing context-less public operations use the bounded Store operation policy;
  cancellable startup/create/query paths should receive context where needed.
- Typed errors distinguish closed/read-only, not-found, stale generation/reference,
  invalid record, unsupported schema, busy/canceled, and database corruption/I/O.
  Invalid individual guide payloads become misses; engine errors propagate.

During staging, freeze equivalent private SQLite contract types without changing
public Entry.Record or Store signatures used by the still-running application.
The private test constructor opens the real new implementation directly; it is
not a production backend selector. SQL-16 publishes the final contract and
removes those staging names in the coordinated cutover batch.

### Schema v1

Use SQLite application_id `0x50525256` and user_version `1`, separate from payload
and prompt versions. SQL-02 freezes complete DDL and storage DTOs before delegates
write queries. Use CHECK/NOT NULL/UNIQUE constraints and enabled foreign keys.

| Table | Primary key / required attributes | Relationships / indexes |
| --- | --- | --- |
| snapshots | source digest; payload version, bounded BLOB, normalized repo/PR/base/head, base/head repository, inventory ID, file count | Comparison index on repo/PR/base/head/inventory; identity rechecked on load |
| snapshot_files | (snapshot digest, file ID); unique ordinal | FK to snapshots, cascade only when unreferenced snapshot is removed |
| guide_bundles | bundle digest; payload version and bounded BLOB | Both sessions and guide_cache may reference a bundle |
| sessions | random 32-hex ID; source digest, optional bundle digest, checkout bytes, DerivedFrom, logical reference, repo/PR/head summary, freshness, UTC update time, generation | FKs to source/bundle; index (repo, PR, updated_at DESC, ID ASC) |
| progress | (session ID, file ID); unique ordinal per session | Session FK with cascade; membership validated transactionally against snapshot_files |
| guide_cache | (repo, PR, base SHA, head SHA, inventory ID, prompt version); bundle digest | FK to guide_bundles; generated bundles only |
| repositories | normalized repository; absolute checkout, stable ordinal | Preserve insertion order on checkout replacement |

DerivedFrom is provenance text, not an FK to the parent. Create validates a live
parent in the same transaction, but deleting a parent does not delete its child.
Generation is a positive signed 64-bit SQLite integer; reject Go uint64 values
outside that range and increments beyond its maximum. Timestamps use a uniform
UTC representation with deterministic chronological ordering, not variable-width
RFC3339 strings for SQL ordering. Progress ordinals preserve returned order.
Bounds are checked via length metadata before fetching/allocating payload BLOBs.

### Bootstrap protocol

Use minimal new SQLite ownership control artifacts, not a revived file cache:
`.sqlite-owner` records format/identity and `initializing` or `ready` phase;
`.sqlite-init.lock` serializes only opening/initialization checks. They contain no
sessions, progress, guides, or registry. Their I/O lives in the new bootstrap
helper, not the deleted JSON store. Normal persistence is SQLite only.

1. Validate private root/recognized artifacts and reject old/unrelated contents.
   A root containing only the private regular bootstrap lock is an identifiable
   empty initialization attempt: acquire/wait and recheck, then initialize if
   still otherwise empty. Either process may acquire the lock first. Test the
   lock-creation-to-identity-publication interleaving explicitly. Protect
   artifacts and journal paths against unsafe links and broad permissions.
2. For a fresh root, durably publish the initializing identity, privately create
   the database, and transactionally commit schema, application ID/version, and
   matching identity in an internal store-info row. No user records yet.
3. Validate committed identity/schema, durably publish ready, and release the
   lock before returning the Store. Ordinary transactions do not use this lock.
4. Initializing plus no DB or an empty identifiable initialization attempt can
   resume under the lock; a committed matching DB can complete ready publication.
   Pre-identity DB/other artifacts are ambiguous and rejected intact with a
   choose-new-store error; the otherwise-empty lock-only case above is allowed.
   Ready plus missing DB is an error. Never repair a foreign/corrupt DB by
   deleting it or replacing it with an empty database.
5. Read-only opening validates ready and opens mode=ro without touching the lock,
   marker, journal, or database. A hot journal requiring recovery is a visible
   error directing the user to a writable open. Do not use immutable mode.

All phase writes, directory durability, concurrent first-open races, and failures
are tested in SQL-03/15/18. Existing `.format`, `.lock`, and JSON store helpers are
removed, not repurposed. The tiny ownership marker is necessary to distinguish a
missing established DB from a genuinely fresh empty root; it is not a data cache.

## Dependency graph and delivery checkpoints

```text
SQL-01 driver evidence -> SQL-02 shared contracts -> SQL-03 real DB startup [A]
                                                   |
                              SQL-04 source roundtrip -> SQL-05 state [B]
                                                   |
                            +-- SQL-06 guides -----+
                            +-- SQL-07 registry ---+-- SQL-09 delete [D]
                            +-- SQL-08 summaries --+ [C]

[D] + frozen contracts -> coordinated cutover patches:
  SQL-10 review callers | SQL-11 CLI callers | SQL-12 TUI state -> SQL-13 picker
  SQL-14 persistence tests -> SQL-15 safety/read-only/description tests
  all patches -> SQL-16 production switch + old-backend deletion [E]
  SQL-17 docs -> SQL-18 final regression/performance/packaging proof [F]
```

SQL-06/07/08 may proceed in parallel after SQL-04/05; SQL-09 follows all three.
No agent changes shared DDL after SQL-02 without the coordinator updating the
contract and notifying dependents. TUI tasks are sequential because they share
`internal/tui/lifecycle.go`; test replacement tasks have one sequential owner.

**Working-state rule:** SQL-01 through SQL-09 develop and exercise the actual SQL
implementation with a private construction seam; the existing CLI remains
working until the switch. There is no runtime backend selector or dual-write
behavior. SQL-10 through SQL-16 are one coordinated cutover batch, divided into
small owned patches, not individually releasable partial API changes. Stage
those patches in isolated scratch checkouts until the batch can compile/test
as a whole. Do not integrate or mark a cutover task complete while dependents
are broken. Remove the temporary construction seam and old implementation in
SQL-16. Every released checkpoint builds and passes its applicable checks.

## Delegation protocol

The coordinator owns the final merge, shared schema/API changes, task status,
and verification evidence. At most three sub-agents run simultaneously alongside
the coordinator. User authorization covers delegation after plan review.

| Lane | Ownership / assignments | Concurrency constraint |
| --- | --- | --- |
| A — storage | SQL-01..05, SQL-09, SQL-16 | Sole editor of shared contract, schema, store facade and module dependencies |
| B — guides/registry | SQL-06 then SQL-07; later SQL-11 | No edits to shared schema or monolithic old store_test.go |
| C — queries/verification | SQL-08; then SQL-14/15; later SQL-18 | Owns replacement session tests; hand off fixture changes explicitly |
| Integration delegation | SQL-10, SQL-12 then SQL-13 | Dispatch to available lanes only after D; CLI/review/TUI file ownership stays disjoint |
| Coordinator/documentation | SQL-17, plan/task tracking, all checkpoints | No concurrent overwrite of another agent's files |

Each dispatch names task IDs, prerequisite commit/contract, exact allowed files,
acceptance tests, excluded work, and required evidence. Agents return a patch,
commands/results, and unresolved concerns. They must not commit unrelated edits,
change scope, suppress checks, or write to a real user store. If file scope grows
past about five implementation/test files, split the work before dispatch.

Each checkpoint requires coordinator review. Pause for human input only if a
scope/contract choice changes or a blocker requires it; routine verification and
already authorized work do not require renewed permission.

## Risks and mitigation

| Risk | Mitigation / owning task |
| --- | --- |
| Driver adds unexpected runtime or platform dependencies | CGo-disabled build, stripped-environment child test, Linux linkage inspection, dependency audit (01/18) |
| Initialization mistakes overwrite or recreate a store | Durable identity/phase, brief lock, foreign-store rejection, crash matrix (03/15/18) |
| Deleting Save breaks every package before callers move | Stage and land the coordinated cutover batch, no compatibility shim (10..16) |
| Dedup changes corruption-test expectations | Corrupt a session-specific field or use genuinely different source rows; one corrupt shared blob affects every referencing session (14) |
| Full source still read on small updates | State-only API and query/blob-read assertions (05/08/18) |
| Parent/session deletion destroys shared data or cached guides | Provenance without parent FK; reference-aware transactional deletion (09) |
| SQL locks, connection reuse, or retries lose progress | Separate-process CAS tests, close rows, bounded waits, no uncertain replay (05/18) |
| Old file tests removed without replacing protections | Map privacy, corruption, crash and immutability tests to new backend before deletion (14/15/16) |
| Native Linux behavior inferred from cross-build | Require Linux CI results; report pending evidence honestly (18) |

## Verification and evidence

Use synthetic data only. Retain existing full macOS/Linux gates. Benchmark the
same machine before/after: 50 sessions with 256 KiB evidence plus 100 identical
reopenings and large-payload state updates. Report binary size, allocations,
payload reads, distinct source-row count, and runtime; do not invent latency
thresholds. No performance gains are established by this plan.

Required final commands include `./scripts/verify.sh`, `golangci-lint run ./...`,
`go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`, and `git diff --check`.
SQL-18 also builds with `CGO_ENABLED=0`, proves embedded persistence without a
SQLite executable/shared library, and links actual Linux CI evidence. Existing
unrelated formatting/test failures must be diagnosed and disclosed, never hidden.

## Review gate and unresolved implementation evidence

The user approved this plan and SQL-01..18 for Sol-agent implementation.
Two read-only planning agents reviewed storage and call-site boundaries; their
findings are incorporated above. Implementation follows the dependency gates.

No product decision remains open. Driver version/security/build suitability,
measured overhead, and native Linux behavior remain evidence to establish in
SQL-01/18, not assumptions of completed validation. The ownership-marker protocol
is an implementation refinement for the already approved crash-safety requirement.
