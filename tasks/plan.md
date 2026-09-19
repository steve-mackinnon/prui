# TUI reliability and automated verification

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
