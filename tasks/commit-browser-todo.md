# Pull Request Commit Browser tasks

- [x] CB-01: Establish shared bundle/entry/diff types and package ownership.
  - Acceptance: capture, storage, and UI use the same typed immutable contract.
  - Verify: contract compiles through package imports.
- [x] CB-02: Capture bounded PR membership through narrow GitHub interface.
  - Acceptance: fixed first-page request; validated IDs/text; honest cap/failure.
  - Verify: source fake-runner tests; no real credentials/network.
- [x] CB-03: Capture pinned first-parent/root diffs with shared inventory logic.
  - Acceptance: merge/root/empty/binary/rename/mode behavior; aggregate bounds.
  - Verify: commits/inventory Git fixtures and cancellation/pin tests.
- [x] CB-04: Persist and validate optional bundle without changing legacy bytes.
  - Acceptance: SQLite round-trip, canonical compatibility, corrupt rejection.
  - Verify: session tests including cache/offline behavior.
- [x] CB-05: Render rail/detail with independent selection and reading state.
  - Acceptance: immediate diff updates, wide/narrow layout, safe states.
  - Verify: TUI model/render tests at breakpoint widths and short heights.
- [x] CB-06: Route keyboard/mouse safely and document applicable controls.
  - Acceptance: navigation/restoration; no progress/comment side effects.
  - Verify: TUI isolation/mouse/help regressions and program scenario.
- [x] CB-07: Integrate opening with bounded retry and offline durable browsing.
  - Acceptance: captures before source-view cleanup; retry rejects moving heads;
    ordinary failure preserves source review; saved material survives source deletion.
  - Verify: review integration tests plus existing lifecycle suites.
- [x] CB-08: Complete docs, independent review, and repository verification.
  - Acceptance: user reference describes actual controls and limits; all review
    findings resolved; feature and existing workflows pass verification.
  - Verify: ./scripts/verify.sh, relevant program/PTY tests, git diff --check.

Checkpoint: CB-02–04 focused tests pass before integration completion.
Checkpoint: CB-05–07 compose correctly before full gate.
Checkpoint: CB-08 complete before reporting the feature implemented.

## Completion evidence — 2026-09-30

- All checkpoints complete. `GOCACHE=/tmp/prui-go-cache ./scripts/verify.sh`
  passes on native macOS: vet, complete race suite, and build.
- Compiled-binary PTY journey proves offline commit selection, individual patch
  display, scrolling, narrow focus, and main review restoration without GitHub
  calls. Screen decoder region/character operations have split-stream regressions.
- Capture fixtures cover roots, merges, empty/reverted changes, R050 renames,
  binary/mode/submodule material, missing/unreachable objects, budgets,
  cancellation, and changing PR heads. Review integration proves durable source
  deletion/offline resume and bounded retry.
- Two independent reviews found and resolved lossless-path/rename validation,
  informational-unit hidden patches, repeated source serialization, quadratic
  UI grouping, narrow position labels, short cap notices, and freshness reserve.
  Final re-review found no remaining concrete blockers.
- Payload-pruning benchmark with 1 MiB main source and 100 small diffs dropped
  cumulative allocations from about 153 MB to 1.63 MB in the focused measurement.
- `git diff --check` passes. No dependency change or SQL table migration.
  Human accessibility verification is not inferred from automated screen tests.
