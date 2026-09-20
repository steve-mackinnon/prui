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

# Guide Path Overflow tasks

## 1. Display-width-safe guide path truncation

- [x] Introduce a pure helper that separates guide-file row chrome from its
      path region and middle-truncates only overflowing unselected paths.
- [x] Preserve selection/read markers and unit suffixes; test short paths,
      tight widths, renamed paths, escaped content, and wide Unicode paths.
- Acceptance: each rendered guide-list row fits its available width and an
      overflowing unselected file path retains both a useful prefix/suffix when
      space permits.
- Verify: write failing focused tests, then run
      `go test -race ./internal/tui -run 'TestGuide.*Path|Test.*Truncat' -count=1`.
- Depends on: approved `SPEC-guide-path-overflow.md`.
- Files: `internal/tui/guides.go`, `internal/tui/guides_test.go`.
- Estimated scope: Small (2 files).

## 2. Selected guide-path scroll state machine

- [x] Add transient list-scroll state and a conditional Bubble Tea tick that
      advances only an overflowing selected guide portion while the list is
      focused.
- [x] Reset or stop on selection, focus, view, width, or path changes without
      changing `Model.Horizontal` or existing `left`/`right` behavior.
- Acceptance: deterministic message-driven tests prove advance, endpoint
      pause/restart, reset, and all stop conditions without timing sleeps.
- Verify: write failing focused tests, then run
      `go test -race ./internal/tui -run 'TestGuide.*Scroll|Test.*Path.*Scroll' -count=1`.
- Depends on: task 1.
- Files: `internal/tui/model.go`, `internal/tui/model_test.go`,
      `internal/tui/guides.go`, `internal/tui/guides_test.go`.
- Estimated scope: Medium (4 files).

## Checkpoint: guide-list overflow behavior

- [x] Focused guide/model tests pass with the race detector.
- [x] Inspect wide and narrow render output to confirm fixed chrome, suffixes,
      and no overflow.

## 3. Integration verification and baselines

- [x] Integrate static and selected render paths in the guides list without
      changing file/inventory/plain views or detail-pane scrolling.
- [x] Update a fixed screen baseline only if the approved static label output
      changes it; review the textual baseline diff manually.
- Acceptance: every success criterion in `SPEC-guide-path-overflow.md` is
      covered by focused tests; no unrelated dirty worktree changes are
      modified.
- Verify: `go test -race ./internal/tui -count=1`,
      `go test ./internal/tui -run '^TestScreenSnapshots$' -count=1`,
      `./scripts/verify.sh`, and `git diff --check`.
- Depends on: tasks 1–2.
- Files: `internal/tui/{guides,model}_test.go`, optional
      `internal/tui/testdata/screens/*.golden`.
- Estimated scope: Small (2–3 files).

## Completion checkpoint

- [x] All Guide Path Overflow success criteria are met and verified.

# Pull Request Line Comments tasks

## 1. Comment request contract and safe GH transport

**Description:** Define the narrow review-comment request/interface, add bounded
stdin support to the subprocess runner, and implement `GH.CreateReviewComment`
with JSON stdin and the documented GitHub REST endpoint.

**Acceptance criteria:**
- [x] Invalid identity, SHA, side, line, UTF-8 path/body, and empty body are rejected before `gh` runs.
- [x] A valid request invokes `gh api --hostname github.com --method POST --input - repos/{repo}/pulls/{number}/comments`; its stdin JSON has exactly body, commit_id, path, line, and side.
- [x] Typed comment text is absent from program arguments, errors, and persistence; cancellation and bounded output remain enforced.

**Verification:**
- [x] Write failing source-package tests, then run `go test ./internal/source -count=1`.
- [x] Run `go test -race ./internal/source -count=1`.

**Dependencies:** None.

**Files likely touched:** `internal/source/{github,process}.go`,
`internal/source/{source_test,process_test}.go`.

**Estimated scope:** Medium (4 files).

## 2. Provenance-preserving diff-line targets

**Description:** Preserve an optional GitHub target alongside each rendered
raw/guide detail line. This is a pure mapping change; it introduces no user
interaction yet.

**Acceptance criteria:**
- [x] Additions/context resolve to `RIGHT` new-file lines; deletions resolve to `LEFT` old-file lines; all structural/non-text lines are non-commentable.
- [x] The mapping works across hunk boundaries, renames, deleted files, raw inventory, deterministic file plan, and guide detail.
- [x] Existing rendering text, escape behavior, color classification, and plain output remain unchanged.

**Verification:**
- [x] Write failing TUI mapping tests, then run `go test ./internal/tui -run 'Test.*(Comment|Guide)' -count=1`.
- [x] Run `go test -race ./internal/tui -count=1`.

**Dependencies:** Task 1 contract types.

**Files likely touched:** `internal/tui/{render,guidedetail,style}.go`,
`internal/tui/{render,guides}_test.go`.

**Estimated scope:** Medium (5 files).

## 3. Scroll-aware selected diff-line cursor

**Description:** Add a visible cursor over provenance-bearing detail lines and
keep it valid as the reviewer moves, scrolls, changes panes, or switches tabs.

**Acceptance criteria:**
- [x] The selected detail line remains visible and visibly marked while the diff scrolls; only a line with target metadata is commentable later.
- [x] Cursor movement does not change existing list selection, file, guide, horizontal-scroll, or tab-isolation behavior.

**Verification:**
- [x] Write failing model/cursor tests, then run `go test ./internal/tui -run 'Test.*(Cursor|Scroll|Tab)' -count=1`.
- [x] Run `go test -race ./internal/tui -count=1`.

**Dependencies:** Task 2.

**Files likely touched:** `internal/tui/{model,render,style}.go`, `internal/tui/model_test.go`.

**Estimated scope:** Medium (4 files).

## Checkpoint: safe selectable targets

- [ ] Source contract and all target grammar cases pass under the race detector.
- [ ] Existing raw/guide/file/inventory navigation and plain output remain unchanged apart from the interactive cursor marker.

## 4. Composer state and key routing

**Description:** Add tab-owned draft/target state, page routing, and the
injected submit action, without first changing the main review rendering.

**Acceptance criteria:**
- [x] Diff-pane Enter opens only for a commentable target; list-pane Enter still focuses the diff.
- [x] Enter adds a newline, Ctrl+Enter submits, and Esc discards without a write.
- [x] Failed submission retains draft/target; success clears them; a late action result cannot alter a different active tab.

**Verification:**
- [x] Write failing model/lifecycle tests, then run `go test ./internal/tui -run 'Test.*(Comment|Composer|Tab)' -count=1`.
- [x] Run `go test -race ./internal/tui -count=1`.

**Dependencies:** Tasks 1–3.

**Files likely touched:** `internal/tui/{model,lifecycle}.go`, `internal/tui/{model,lifecycle}_test.go`.

**Estimated scope:** Medium (4 files).

## 5. Composer rendering, help, and program journey

**Description:** Render the multiline composer as a focused modal/page, expose
its keys in help, and prove the synthetic interactive journey.

**Acceptance criteria:**
- [x] The composer identifies the frozen path/side/line, safely renders a multiline draft, and advertises Enter, Ctrl+Enter, and Escape semantics.
- [x] Program coverage proves compose → submit → success as well as error and cancellation paths without a real GitHub request.

**Verification:**
- [x] Write failing render/program tests, then run `go test ./internal/tui -run 'Test.*(Composer|Comment|Program)' -count=1`.
- [x] Run `go test -race ./internal/tui -count=1`.

**Dependencies:** Task 4.

**Files likely touched:** `internal/tui/{render,bindings}.go`, `internal/tui/{render,program}_test.go`.

**Estimated scope:** Medium (4 files).

## 6. Application freshness preflight and lifecycle wiring

**Description:** Inject the explicit comment operation from the application,
enforce online/current metadata equality just before the write, and preserve
the existing offline and setup-error boundaries.

**Acceptance criteria:**
- [x] Offline mode, unavailable GH, invalid target, or preflight mismatch calls no commenter.
- [x] An exact metadata match calls the commenter once with the frozen target and head SHA.
- [x] A canceled/unknown delivery outcome makes no false success claim and never auto-retries.

**Verification:**
- [x] Write failing command/lifecycle tests, then run `go test ./cmd/pr-review -count=1`.
- [x] Run `go test -race ./cmd/pr-review ./internal/review -count=1`.

**Dependencies:** Tasks 1 and 4.

**Files likely touched:** `cmd/pr-review/{lifecycle,wiring}.go`,
`cmd/pr-review/{lifecycle,wiring}_test.go`.

**Estimated scope:** Medium (4 files).

## 7. Documentation, snapshots, and full regression verification

**Description:** Update the public read-only/write-boundary contract, keyboard
reference, and affected deterministic screens; run the full verification gate.

**Acceptance criteria:**
- [x] README and constraints state that comment posting is explicit, fresh-head-gated, permission-dependent, and unavailable offline/plain.
- [x] Help/composer chrome identifies submission and cancel keys without leaking draft text into a persisted artifact.
- [x] Snapshot and program coverage demonstrate a successful synthetic journey and safe error/cancel paths.

**Verification:**
- [x] Run `go test -race -count=1 ./...` and `go build ./...`.
- [x] Run `./scripts/verify.sh` and `git diff --check`.
- [ ] Conduct one authorized manual real-PR comment as the final acceptance check.

**Dependencies:** Tasks 1–6.

**Files likely touched:** `README.md`, `CONSTRAINTS.md`,
`internal/tui/testdata/screens/*`, relevant `*_test.go` files.

**Estimated scope:** Small (3–5 files).

## Completion checkpoint

- [x] All spec success criteria are verified with synthetic tests; no unrelated dirty worktree changes were modified.
- [x] An authorized manual real-PR comment succeeds at the intended line.

# Inline Review Comment UX Extension

## 1. Canonical comment read/create source contract

**Description:** Extend the narrow GitHub boundary so a successful create
returns one validated canonical comment, and add a bounded read-only listing
operation for pull-request review comments.

**Acceptance criteria:**
- [x] `ReviewCommentReader` lists a documented, bounded number of comments and
  rejects malformed anchors before they reach TUI state.
- [x] `CreateReviewComment` returns the validated GitHub response needed for
  immediate rendering; no typed text is exposed in arguments, logs, sessions,
  or unsanitized errors.
- [x] Listing and POST parsing enforce output bounds, cancellation, valid UTF-8,
  allowed sides, positive lines, and an exact target shape.

**Verification:**
- [ ] Start with failing source tests for endpoint/query, response parsing,
  malformed values, bounds, cancellation, and no body leakage.
- [ ] Run `go test -race ./internal/source -count=1`.

**Dependencies:** None.

**Files likely touched:** `internal/source/{github,process}.go`,
`internal/source/{source,process}_test.go`.

**Estimated scope:** Medium (4 files).

## 2. Expand detail rendering into anchored display rows

**Description:** Preserve existing target-bearing diff rows while allowing
zero-or-more read-only comment rows and one inline-editor row after a target.

**Acceptance criteria:**
- [x] Raw and guide views display comment/editor rows directly below their
  exact target without changing source text, hunk mapping, or plain output.
- [x] Cursor and scroll logic select only target-bearing diff rows, skip
  inserted rows, and retain target identity across tabs/panes/view changes.
- [x] Escaped remote body/login/path data cannot inject terminal controls.

**Verification:**
- [ ] Start with failing render/model tests for row order, target-only
  selection, wrapping, escape behavior, and scroll-anchor stability.
- [ ] Run `go test -race ./internal/tui -run 'Test.*(Comment|Cursor|Detail|Guide)' -count=1`.

**Dependencies:** Task 1 type contract.

**Files likely touched:** `internal/tui/{model,render,guidedetail,style}.go`,
relevant `*_test.go` files.

**Estimated scope:** Large (5–7 files).

## 3. Inline rune-aware composer and deletion keys

**Description:** Remove the standalone composer page and route editing to the
inline target-owned editor, with predictable Unicode-aware cursor semantics.

**Acceptance criteria:**
- [x] Diff-pane `Enter` opens the inline editor; list-pane `Enter` remains
  unchanged; non-commentable lines still refuse composition.
- [x] `Backspace` and `Delete` work independently at boundaries and in
  multibyte text; left/right/Home/End/Enter/Ctrl+Enter/Esc have documented
  editor semantics.
- [x] Submitting or discarding restores normal navigation without moving the
  selected target; failed posts retain the exact draft and cursor state.

**Verification:**
- [ ] Start with failing table-driven editing tests, including actual Bubble Tea
  delete/backspace messages and Unicode input.
- [ ] Run `go test -race ./internal/tui -run 'Test.*(Inline|Composer|Comment|Delete|Backspace)' -count=1`.

**Dependencies:** Task 2.

**Files likely touched:** `internal/tui/{model,render,bindings,lifecycle}.go`,
relevant `*_test.go` files.

**Estimated scope:** Large (4–6 files).

## 4. Per-tab overlay lifecycle and immediate read-back

**Description:** Load and explicitly refresh comment overlays online, filter
them against the frozen snapshot, and add the canonical POST result immediately
after success.

**Acceptance criteria:**
- [x] Online open loads an ephemeral per-tab overlay; `c` refresh replaces only
  its originating tab; offline mode does neither.
- [x] Only entries exactly matching frozen head SHA/path/side/line render; a
  list failure is bounded, escaped, non-blocking, and cannot erase a just-posted
  successful local item.
- [x] Async read/post results cannot leak into another tab or review, and a
  stale response cannot overwrite a newer refresh generation.

**Verification:**
- [ ] Start with failing lifecycle/application tests for initial load, refresh,
  filtering, post insertion, failures, offline refusal, and tab isolation.
- [ ] Run `go test -race ./internal/tui ./cmd/pr-review -count=1`.

**Dependencies:** Tasks 1–3.

**Files likely touched:** `internal/tui/{model,lifecycle,render}.go`,
`cmd/pr-review/{lifecycle,wiring}.go`, relevant `*_test.go` files.

**Estimated scope:** Large (6–8 files).

## 5. Documentation, regression gate, and manual acceptance

**Description:** Update the interaction reference and capture the full
synthetic and real-PR validation of inline compose and visible read-back.

**Acceptance criteria:**
- [x] README, constraints, help, and screen coverage describe inline editing,
  explicit `c` refresh, ephemeral remote comments, and offline behavior.
- [ ] Existing read-only, plain-output, pinning, guide, and terminal-escaping
  behavior remain unchanged outside the interactive overlay.
- [ ] A manual comment on PR #7 is visible inline after submission and after an
  explicit refresh; both deletion keys work in the running TUI.

**Verification:**
- [ ] Run `go test -race -count=1 ./...`, `go build ./...`,
  `./scripts/verify.sh`, and `git diff --check`.
- [ ] Review changed snapshots and complete one authorized manual acceptance
  pass against the pull request.

**Dependencies:** Tasks 1–4.

**Files likely touched:** `README.md`, `CONSTRAINTS.md`,
`internal/tui/testdata/screens/*`, relevant tests.

**Estimated scope:** Medium (3–6 files).

## Completion checkpoint

- [ ] Inline composition, both deletion keys, and exact anchored read-back are
  covered by synthetic tests and full regression verification.
- [ ] A manual PR comment is visibly rendered inline immediately after posting
  and after an explicit refresh.

# Inline Review Comment Actions

## 1. Bounded viewer and comment-action source contracts

**Description:** Add narrow, explicit GitHub boundaries for authenticated viewer
identity, replying to a review comment, deleting a reviewer's own comment, and
adding a selected reaction.

**Acceptance criteria:**
- [x] Every action validates repository, PR, stable comment ID, UTF-8 body or
  finite reaction value, and uses stdin JSON rather than command arguments.
- [x] Viewer identity and canonical reply/reaction responses are bounded and
  validated; delete has no assumed response body.
- [x] Error, cancellation, output-limit, malformed-response, and body-leakage
  tests cover every endpoint.

**Verification:**
- [x] Start with failing `internal/source` tests for methods, endpoints, stdin,
  validation, bounds, cancellation, and safe errors.
- [x] Run `go test -race ./internal/source -count=1`.

**Dependencies:** None.

**Files likely touched:** `internal/source/{github,process}.go`, source tests.

## 2. Comment-box cursor and local action menu

**Description:** Extend diff navigation so it can visibly select comment boxes,
without allowing a comment row to become a line-comment target; Enter on a
selected comment opens a local action menu.

**Acceptance criteria:**
- [x] Scrolling and `j`/`k` can select target rows and exact anchored comment
  boxes; selected boxes are clearly highlighted and stay visible.
- [x] Enter retains current semantics on a diff target and opens an action menu
  only for a selected comment; Escape is write-free.
- [x] The menu offers reply/react for every loaded comment and delete only when
  its escaped author matches the authenticated viewer.

**Verification:**
- [x] Start with failing TUI tests for cursor order, highlighting, target/menu
  dispatch, unsafe author handling, tab isolation, and cancel behavior.
- [x] Run `go test -race ./internal/tui -run 'Test.*(Comment|Cursor|Action)' -count=1`.

**Dependencies:** Task 1.

**Files likely touched:** `internal/tui/{model,lifecycle,render,bindings}.go`,
TUI tests and snapshots.

## 3. Reply lifecycle and immediate overlay insertion

**Description:** Submit an explicit reply to the selected comment and render
the canonical GitHub response in the same ephemeral anchored overlay.

**Acceptance criteria:**
- [x] Reply draft is rune-aware, local-only, cancelable, and may only submit
  from an intentional action-menu choice.
- [x] Success adds the canonical reply to the originating tab; failure retains
  its draft and selection for retry.
- [x] Offline mode, stale tab/action results, and invalid selected comments
  make no write.

**Verification:**
- [ ] Start with failing lifecycle/application tests for success, failure,
  cancellation, offline refusal, and tab isolation.
- [ ] Run `go test -race ./internal/tui ./cmd/pr-review -count=1`.

**Dependencies:** Tasks 1–2.

## 4. Viewer-owned deletion and reactions

**Description:** Add explicit delete confirmation for comments authored by the
authenticated viewer and a finite reaction picker for loaded comments.

**Acceptance criteria:**
- [x] Delete is unavailable for another author, requires an explicit final
  confirmation, and removes only the canonical matching box after success.
- [x] Reaction picker exposes only documented allowed values; success updates
  the ephemeral comment state without changing sessions/plain output.
- [x] Failures leave the overlay usable and do not erase comment boxes or
  selection; offline mode cannot reach either action.

**Verification:**
- [ ] Start with failing source/TUI/application tests for authorization gating,
  confirmation, picker values, success/failure/cancellation, and stale results.
- [ ] Run `go test -race ./internal/source ./internal/tui ./cmd/pr-review -count=1`.

**Dependencies:** Tasks 1–2.

## 5. Documentation and acceptance

**Description:** Document comment-box selection and explicit actions, then
verify reply/delete/reaction journeys without persisting remote data.

**Acceptance criteria:**
- [x] README, constraints, help, and snapshots describe action selection,
  confirmation, viewer-owned deletion, reactions, and offline refusal.
- [ ] Full regression verification is clean; manual acceptance confirms each
  permitted action on an authorized test PR.

**Verification:**
- [x] Run `go test -race -count=1 ./...`, `go build ./...`,
  `./scripts/verify.sh`, `git diff --check`, and `golangci-lint run`.

**Dependencies:** Tasks 1–4.

## Completion checkpoint

- [x] Comment selection, action dispatch, reply, viewer-owned deletion, and
  reactions have focused synthetic coverage and pass the full verification gate.
- [ ] Authorized manual acceptance proves each action is explicit, exact, and
  never persisted to a session or plain output.

# Threaded Inline Reply and Reaction Rendering

## 1. Threaded reply editor and canonical reply box

**Acceptance criteria:**
- [x] Reply opens a bordered rune-aware editor beneath, and visibly indented
  from, its selected parent comment rather than in the action-menu status.
- [x] Canonical replies render beneath their parent with the same indentation;
  cancel and failure retain no remote state and preserve the local draft.
- [x] Anchoring, escaping, scrolling, and tab/generation isolation remain exact.

**Verification:**
- [x] Start with focused failing TUI lifecycle/render tests.
- [x] Run `go test -race ./internal/tui -run 'Test.*(Reply|Comment)' -count=1`.

## 2. Numbered reaction picker and bottom-border count chips

**Acceptance criteria:**
- [x] The reaction picker presents documented values numbered `1`–`8`, and a
  digit is consumed only while that picker is active.
- [x] The bottom border renders one escaped chip per reaction value with an
  ephemeral count, bounded to the comment box width.
- [x] Reactions remain tab-local and are omitted from sessions and plain output.

**Verification:**
- [x] Start with focused failing TUI render/lifecycle tests and inspect snapshots.
- [x] Run `go test -race ./internal/tui -run 'Test.*(Reaction|Comment)' -count=1`.

## Completion checkpoint

- [x] Full required verification passes and PR #9 is updated.

## Follow-up: Reply-to-reply fallback

- [x] Resolve a selected reply to its top-level thread comment before explicit
  reply submission; keep reaction/delete scoped to the selected comment.
- [x] Regression test proves the canonical response remains in the root thread.
