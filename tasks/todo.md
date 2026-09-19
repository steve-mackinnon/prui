# TUI reliability and testing tasks

# Calm Review Workspace Layout tasks

## 1. Shared chrome and selection semantics

- [x] Add concrete shared app/section header, compact status/footer, and
      selected-row presentation helpers for interactive TUI surfaces.
- [x] Selection has a textual `›` marker and distinguishable focused/unfocused
      treatment without relying on color.
- Acceptance: helper tests prove clipping, status priority, and palette removal
      preserve every visible marker and label.
- Verify: write focused failing tests, then run
      `go test ./internal/tui -run 'Test(Chrome|Selection|Color)' -count=1`.
- Depends on: approved `SPEC-tui-review-layout.md`.
- Files: `internal/tui/{style,render}_test.go`, `internal/tui/{style,render}.go`.

## 2. Review workspace layout

- [x] Replace verbose review chrome with the shared app header, section header,
      selected rail, and one-line health status bar.
- [x] Preserve guide/file/inventory navigation and retain a one-pane narrow
      fallback with the highest-severity state visible.
- Acceptance: wide review contains no shortcut dump; narrow review retains the
      current selection, progress, and health signal; guide and raw modes work.
- Verify: write focused failing tests, then run
      `go test ./internal/tui -run 'Test(Review|Guide|Raw|Snapshot)' -count=1`.
- Depends on: task 1.
- Files: `internal/tui/{model,render}_test.go`, `internal/tui/model.go`,
      `internal/tui/testdata/screens/{review_wide,review_narrow,guides}.golden`.

## Checkpoint: review surface

- [x] Focused TUI tests pass and wide/narrow text baselines are inspected.

## 3. Health & help

- [x] Group every existing binding under Navigate, Review, Views, Diagnostics,
      or App in the existing `?` page.
- [x] Keep only PR switching discoverable in persistent review chrome and
      update README keyboard/layout wording.
- Acceptance: every binding appears once in Health & help; help and README
      agree; no key routing changes.
- Verify: write focused failing tests, then run
      `go test ./internal/tui -run 'Test.*Help' -count=1`.
- Depends on: task 2.
- Files: `internal/tui/{bindings,model}_test.go`, `internal/tui/{bindings,model}.go`,
      `README.md`.

## 4. Picker and switcher consistency

- [x] Reuse shared chrome and selected-row treatment in PR switcher and picker
      screens without changing their filtering, error, empty, or cancel flows.
- [x] Update deterministic picker/switcher baselines.
- Acceptance: all interactive picker surfaces use the same header/status and
      visible selected-row language; existing picker behavior regressions pass.
- Verify: write focused failing tests, then run
      `go test ./internal/tui -run 'Test(Picker|CommandSwitcher|ScreenSnapshots)' -count=1`.
- Depends on: tasks 1–3.
- Files: `internal/tui/{picker,lifecycle,snapshot}_test.go`,
      `internal/tui/{picker,lifecycle}.go`, `internal/tui/testdata/screens/*.golden`.

## Completion checkpoint

- [x] `go test ./internal/tui -count=1`, `go test ./cmd/pr-review -count=1`,
      `./scripts/verify.sh`, and `git diff --check` pass.
- [x] The final diff only changes the approved layout, shared chrome, tests,
      snapshots, and reviewer documentation.

## 1. Safe PR browser startup
- [x] Share model initialization and make browser `Init` safe without a loader.
- [x] Regression test starts the browser through its actual initialization path.
- [x] Verify: focused browser startup tests and `go test ./internal/tui`.

## 2. Screen-owned picker state
- [x] Repository, PR, and session pickers own independent cursors; returning to a
      parent restores its selection and cannot index outside its list.
- [x] Regressions cover multiple PRs, one/multiple repositories, empty results,
      back-navigation, and reopening a picker.
- [x] Verify: focused picker scenario tests. Depends on task 1.

## 3. Picker viewport behavior
- [x] All picker screens keep the selected row visible as selection/size changes.
- [x] Regressions cover long lists and narrow/short terminals with bounded output.
- [x] Verify: picker layout tests. Depends on task 2.

## 4. Asynchronous picker storage reads
- [x] Session/repository listing runs in commands instead of blocking `Update`.
- [x] Loading, errors, cancellation, and quit retain the current review and safely
      finish workers before the store closes.
- [x] Verify: deterministic async scenario tests and race detector. Depends on 2.

## 5. Consistent application wiring and offline enforcement
- [x] Every entry path installs the same refresh/new/guide/browser operations.
- [x] Offline mode rejects every network operation before creating clients or
      reading credentials, while saved sessions remain usable.
- [x] Test both entry paths, cancellation, and offline operations with fakes;
      guide cancellation does not replace the current session.
- [x] Verify: `go test -race ./cmd/pr-review ./internal/review`.

## 6. Reconcile the public contract
- [x] README/constraints match accepted CLI flags, current keys, interactive guide
      consent, derived sessions, and offline restrictions.
- [x] Document automated testing layers, fixture requirements, snapshot updates,
      and the remaining scope of human usability/accessibility assessment.
- [x] Verify documented commands against CLI parser and testing scripts. Depends
      on 5 and final testing infrastructure.

## 7. Scenario and real-program testing infrastructure
- [x] Add reusable, bounded helpers that execute `Init`, commands/messages, and
      complete navigation sequences without fixed sleeps.
- [x] Run actual Bubble Tea programs for load and browser entry paths, quit,
      cancellation/failure/retry, and persistence where applicable.
- [x] Keep existing domain/provider tests; add regressions rather than weaken
      assertions. Verify with the race detector. Depends on 1–5.

## 8. Deterministic screen snapshots
- [x] Check in small representative screens for review, pickers, narrow layout,
      loading/error states, and guides using fixed dimensions and fixture data.
- [x] Provide an explicit baseline-update command; normal tests only compare.
- [x] Verify failure output is actionable and selected-row/width assertions remain
      behavioral tests independent of the baselines. Depends on 1–4.

## 9. Compiled-binary PTY smoke tests
- [x] Exercise actual startup, keyboard input, resize, Ctrl+C/quit, and terminal
      restoration on macOS/Linux with bounded waits and diagnostics.
- [x] Exercise saved-session marking and restart/resume using temporary stores;
      fixture GitHub/provider behavior must never use live credentials/network.
- [x] Verify both entry paths and run the suite locally. Depends on 1–5 for green.

## 10. Verification command and CI
- [x] Add one checked-in command for formatting checks, vet, race-enabled Go
      tests, build, and PTY smoke tests; fail visibly on missing prerequisites.
- [x] Add Linux/macOS CI using the module's Go version and synthetic fixtures.
- [x] Document the per-feature requirement for a successful journey plus relevant
      error/cancellation regressions. Depends on 7–9.

## Completion checkpoint
- [x] Review integrated changes for architecture, correctness, and test robustness.
- [x] Run the full verification command and record results in this checklist.
- [x] Commit all completed work in coherent slices; preserve unrelated files.

## Verification record

- 2026-09-17, macOS arm64, Go 1.26.8: `./scripts/verify.sh` passed
  formatting, vet, the full race-enabled test suite (including PTY smoke tests),
  and build.
- Real-program tests cover initial loading/quit, persisted marking across model
  restart, browser cancellation/failure/retry/back/open. Picker tests cover
  one/multiple repositories, empty lists, bounded layouts down to one line, and
  asynchronous listing cancellation/error/retry.
- Application tests cover all three entry commands, offline dependency rejection,
  usable offline saved-session browsing, and cancellation before/during analysis
  without a derived session. PTY tests use the compiled binary and verify arrows,
  resize across the pane breakpoint, marking, help/back, Ctrl+C/q, terminal
  restoration, and durable progress after process restart with no GitHub calls.
- Eight small screen baselines were inspected. Updates require `-update-golden`;
  visibility and dimension assertions run even when baselines are updated.
- `sh -n scripts/verify.sh`, YAML parsing, and `git diff --check` passed. The
  Linux/macOS CI matrix is configured but has not run remotely; Linux execution
  and branch-protection configuration are not claimed as locally verified.
- Changes were reviewed for state ownership, cancellation, offline boundaries,
  preservation of source/snapshots, escaping, bounded test waits/output, and
  subprocess cleanup. No dependency or stored-session schema changes were needed.

# Agent PR Verifier tasks

## 1. Versioned verifier report contract

- [x] Add `internal/verify` result types, relative artifact-path validation,
      stable JSON encoding, and exit-status mapping.
- Acceptance: every completed outcome has one JSON object and secrets or
      absolute paths cannot appear in it.
- Verify: write and run focused failing-then-passing package tests.
- Depends on: none. Files: `internal/verify/*`.

## 2. Bounded live PR terminal journey

- [x] Add `pr-review verify` input validation and a private-store, fixed-PTY
      journey that opens, navigates, marks, quits, and resumes offline.
- Acceptance: synthetic integration proves the production open/resume path;
      real runs stay manual and never execute reviewed code.
- Verify: focused command/PTY tests and `go build ./...`.
- Depends on: task 1. Files: `cmd/pr-review/*`, `internal/verify/*`, testdata.

## Checkpoint: live verifier

- [x] Focused report and journey tests pass; normal test suite remains
      credential-free.

## 3. Deterministic terminal artifacts

- [x] Save bounded transcript, screen text, and deterministic SVG milestones
      under the explicit artifact directory.
- Acceptance: artifact paths are contained, escaped, and listed in the report.
- Verify: focused artifact tests.
- Depends on: tasks 1–2. Files: `internal/verify/*`.

## 4. Timing observations

- [x] Add typed per-run and aggregate lifecycle timing to the report.
- Acceptance: local startup and network-backed work remain distinct and never
      change pass/fail status.
- Verify: focused timing tests.
- Depends on: tasks 1–2. Files: `internal/verify/*`.

## 5. Read-only stored-guide evaluation

- [x] Add `eval-guides SESSION_ID`, structural validation, and curated synthetic
      semantic-corpus evaluation.
- Acceptance: no provider call or credential read; absent guides report
      `not_available`; named expectations fail independently.
- Verify: `go test ./internal/guideeval ./cmd/pr-review`.
- Depends on: none. Files: `internal/guideeval/*`, `cmd/pr-review/*`.

## Checkpoint: complete

- [x] All focused tests, `./scripts/verify.sh`, and `git diff --check` pass.
- [ ] An authorized manual real-PR run passes when a PR URL and checkout are
      supplied.

# Persistent PR list and review tabs tasks

## 1. Workspace tab foundation

- [x] Add process-local fixed PRs tab, numbered tab selection (`1`–`9`), and
      next/previous navigation without changing current review behavior.
- Acceptance: initial review becomes a numbered review tab; `1` selects PRs;
      each populated number selects exactly its matching tab; `tab` still
      expands a guide/section.
- Verify: write the failing model tests first, then run
      `go test ./internal/tui -run 'Test.*Tab' -count=1`.
- Depends on: none. Files: `internal/tui/model.go`, `internal/tui/model_test.go`.

## 2. Isolated review state and tab-aware lifecycle

- [x] Preserve each review's selection, focus, scroll, expansion, errors, and
      action result while another tab is active; associate lifecycle results
      with their originating tab.
- Acceptance: switching between two reviews restores independent state; a
      delayed action cannot overwrite the active tab.
- Verify: failing-then-passing lifecycle tests and
      `go test ./internal/tui -run 'Test.*Tab' -count=1`.
- Depends on: task 1. Files: `internal/tui/model.go`,
      `internal/tui/lifecycle.go`, `internal/tui/lifecycle_test.go`.

## Checkpoint: workspace behavior

- [x] Focused TUI tests pass and `go build ./...` succeeds.

## 3. Open, deduplicate, and cap review tabs

- [x] Open selected PRs as tabs, activate an already-open immutable review,
      and keep PRs usable with a stated nine-tab capacity error.
- Acceptance: two PRs remain switchable; duplicate selection makes no second
      open call; canceled/failed/capacity-limited opens leave other tabs intact.
- Verify: focused lifecycle tests and `go test ./internal/tui -count=1`.
- Depends on: tasks 1–2. Files: `internal/tui/lifecycle.go`,
      `internal/tui/lifecycle_test.go`.

## 4. Render, document, and verify tabs

- [x] Render accessible numbered tab labels across terminal sizes; add bindings,
      README guidance, snapshots, and end-to-end program coverage.
- Acceptance: tab labels survive color removal and width clipping; help/footer
      document keys; plain output stays single-review.
- Verify: `go test ./internal/tui ./cmd/pr-review -count=1`, then
      `./scripts/verify.sh` and `git diff --check`.
- Depends on: tasks 1–3. Files: `internal/tui/{render,style,bindings}.go`,
      `internal/tui/*_test.go`, `README.md`.

## Completion checkpoint

- [x] All tab spec success criteria pass through synthetic tests and the full
      verification command; existing dirty changes remain separately visible.
