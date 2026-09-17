# TUI reliability and automated verification

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
