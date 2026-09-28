# TUI reliability and testing tasks

# Split-Diff Pane Cursor and Comment Targeting tasks

## 1. Semantic cursor selection foundation

**Description:** Replace split mode's row-only cursor authority with a
semantic selection that can name either a full frozen source target or a
stable comment ID. Expose all valid cell targets for a projected row while
retaining `RIGHT`, then `LEFT` as the initial/fallback preference.

**Acceptance criteria:**
- [ ] A paired deletion/addition row can resolve both its exact `LEFT` and
      `RIGHT` targets; context resolves only its `RIGHT` target.
- [ ] Selection restoration across unified/split projection finds the same
      target rather than relying on a stale display-row offset.
- [ ] Comment-card selection remains ID-based and distinct from source-cell
      selection.

**Verification:**
- [ ] Add focused table/model regressions, then run
      `go test ./internal/tui -run 'Test(SideBySideCursor|DiffCursor|CommentCursor)' -count=1`.

**Dependencies:** approved `docs/spec/SPEC-split-diff-pane-cursor.md`.

**Files likely touched:**
- `internal/tui/model.go`
- `internal/tui/model_test.go`

**Estimated scope:** Small (2 files).

## 2. Cell-level split cursor rendering

**Description:** Render a color-independent cursor gutter in each split source
cell, with the chevron on the selected target, while retaining stable gutters,
separator alignment, and shared horizontal clipping.

**Acceptance criteria:**
- [ ] The initial split selection visibly marks the `RIGHT` cell on paired and
      context rows, or the `LEFT` cell on deletion-only rows.
- [ ] Both cells reserve identical cursor-gutter width; color removal, escaped
      control input, and wide-rune clipping preserve alignment.
- [ ] Structural rows and comment cards remain full-width and do not fabricate
      a cell cursor.

**Verification:**
- [ ] Add focused render tests, then run
      `go test ./internal/tui -run 'Test(SideBySide(Render|Cursor)|Color)' -count=1`.

**Dependencies:** task 1.

**Files likely touched:**
- `internal/tui/render.go`
- `internal/tui/render_test.go`
- `internal/tui/style.go`
- `internal/tui/style_test.go`

**Estimated scope:** Medium (4 files).

## Checkpoint: target identity and visible cursor

- [ ] Focused cursor/render/color tests pass at 160 columns, with a visible
      chevron on a right-only, left-only, and paired source row.
- [ ] `git diff --check` passes.

## 3. Side toggle and composer flow

**Description:** Route `P` only from a focused eligible split diff. It switches
between valid cells on the current row, preserves vertical position, and makes
`enter` open the existing composer against the selected target. Document the
binding and active side.

**Acceptance criteria:**
- [ ] `P` alternates a paired row between precise `RIGHT` and `LEFT` targets;
      one-sided rows retain selection and give a non-error notice.
- [ ] `enter` opens the composer for the selected side; composer/menu/action
      input cannot silently change that target.
- [ ] `P` is unavailable in unified/narrow fallback and on comment cards;
      help and the visible side label agree with key behavior.

**Verification:**
- [ ] Add focused model/binding regressions, then run
      `go test ./internal/tui -run 'Test(SideBySideCursor|DiffEnter|CommentComposer|.*Help)' -count=1`.

**Dependencies:** tasks 1–2.

**Files likely touched:**
- `internal/tui/model.go`
- `internal/tui/model_test.go`
- `internal/tui/bindings.go`
- `internal/tui/bindings_test.go`
- `README.md`

**Estimated scope:** Medium (5 files).

## 4. Projection-boundary regressions and completion checks

**Description:** Prove that cell selection remains correct through the places
that change detail projection or vertical placement: resize, unified fallback,
tab switching, guide/raw views, remote comment overlays, and drafts. Update
deterministic screen baselines if their documented renderer requires it.

**Acceptance criteria:**
- [ ] A selected `LEFT` or `RIGHT` source target survives `S`, 159/160-column
      transitions, tab switching, and guide/raw navigation whenever that target
      remains in the active detail.
- [ ] Overlay ordering and comment-card actions remain unchanged; no invalid
      old-context target can be selected.
- [ ] Documentation, snapshots, plain output, and the full test suite agree.

**Verification:**
- [ ] Run `go test -race -count=1 ./...`.
- [ ] Run `go build ./...`, `./scripts/verify.sh`, and `git diff --check`.
- [ ] Manually inspect a 160-column split diff: select both sides of a paired
      replacement, resize below/above 160, and open/cancel a composer on each.

**Dependencies:** task 3.

**Files likely touched:**
- `internal/tui/model_test.go`
- `internal/tui/guides_test.go`
- `internal/tui/snapshot_test.go`
- `internal/tui/testdata/screens/*`
- `README.md`

**Estimated scope:** Medium (5 files plus baselines).

## Completion checkpoint

- [ ] All success criteria in `docs/spec/SPEC-split-diff-pane-cursor.md` are met.
- [ ] Full automated verification passes with no GitHub contract, persistence,
      or plain-output change.
- [ ] The 160-column split interaction is manually inspected before release.

# Side-by-Side Diff View tasks

## 1. Pure aligned hunk-row projection

- [x] Add a pure typed projection from text-hunk `diffLine` values to aligned
      old/new/full-width display rows, without changing current unified output.
- [x] Pair each deletion run only with its immediately following addition run;
      context duplicates to both cells and unequal runs produce blank cells.
- Acceptance: table-driven cases prove escaped cell text, line numbers,
      semantic classes, full-width structural lines, and immutable `LEFT`/
      `RIGHT` target equivalence.
- Verify: write focused failing tests, then run
      `go test ./internal/tui -run 'TestSideBySideProjection' -count=1`.
- Depends on: approved `docs/spec/SPEC-side-by-side-diff.md`.
- Files: `internal/tui/{render,render_test}.go`.
- Estimated scope: Small (2 files).

## 2. Split renderer and automatic unified fallback

- [x] Render projected rows as two aligned code cells with stable gutters,
      markers, separator, shared horizontal scrolling, and full-width cards.
- [x] Retain current unified rendering exactly, and select it automatically
      below 160 columns while showing the required fallback wording.
- Acceptance: width 160 renders split text hunks; width 159 renders unified
      detail with `side-by-side needs 160 columns`; clipping and color removal
      preserve separators, text ordering, and terminal widths.
- Verify: write focused failing tests, then run
      `go test ./internal/tui -run 'Test(SideBySideRender|SideBySideFallback|Color)' -count=1`.
- Depends on: task 1.
- Files: `internal/tui/{model,render,style}_test.go`,
      `internal/tui/{model,render,style}.go`.
- Estimated scope: Medium (5 files).

## Checkpoint: projection and responsive rendering

- [ ] Focused projection/render/color tests pass, and the 159/160-column
      rendered output is inspected for readable unified fallback and aligned
      split gutters.

## 3. Tab-owned layout preference and comment-safe row navigation

- [x] Add the process-local `unified`/`side-by-side` preference to review-tab
      state and route `S` only in the interactive Changes view.
- [x] Adapt offset, cursor, editor, and remote-overlay insertion to rendered
      rows, preserving target priority: `RIGHT` where present, otherwise
      `LEFT` for deletion-only rows.
- Acceptance: preferences remain isolated across tabs and survive width
      transitions; Enter selects exactly the prior unified target; paired
      old/new remote threads remain deterministic old-then-new rows.
- Verify: write focused failing tests, then run
      `go test ./internal/tui -run 'Test(SideBySide(Tab|Resize|Comment|Cursor|Overlay))' -count=1`.
- Depends on: tasks 1-2.
- Files: `internal/tui/{model,model_test,render_test}.go`.
- Estimated scope: Medium (3 files).

## 4. Guide integration, reviewer documentation, and complete verification

- [x] Route guide-detail text hunks through the active row layout while
      preserving repeated-file anchors, raw inventory navigation, marks, and
      non-text cards.
- [x] Document `S`, the default unified layout, and automatic 160-column
      fallback in keyboard/layout documentation and Health & help.
- [x] Add/inspect deterministic wide split and narrow fallback baselines.
- Acceptance: file, inventory, and guide paths render equivalent source and
      comment targets; plain output remains unified; help and README agree.
- Verify: `go test -race -count=1 ./...`, `go build ./...`,
      `./scripts/verify.sh`, and `git diff --check`; manually inspect both
      layouts in a terminal.
- Depends on: task 3.
- Files: `internal/tui/{guidedetail,bindings,bindings_test,snapshot_test}.go`,
      `internal/tui/testdata/screens/*`, `README.md`.
- Estimated scope: Medium (5 files plus baselines).

## Completion checkpoint

- [x] All automated success criteria in `docs/spec/SPEC-side-by-side-diff.md` are met.
- [ ] Wide split / narrow fallback are manually inspected in a real terminal
      before a release; automated rendering coverage passes without a TTY.
- [x] Full automated verification passes without changing frozen source,
      persistence, plain output, or GitHub comment semantics.
- [ ] The final diff is committed as an atomic feature change after the user
      reviews the unrelated working-tree files.
- [ ] The wide split / narrow fallback manual inspection occurs before release
      (not performed by this non-interactive run).

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
- Depends on: approved `docs/spec/SPEC-tui-review-layout.md`.
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
- Depends on: approved `docs/spec/SPEC-guide-path-overflow.md`.
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
- Acceptance: every success criterion in `docs/spec/SPEC-guide-path-overflow.md` is
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

## Follow-up: Emoji reaction labels

- [x] Render all eight documented GitHub reactions as emoji in UTF-8 locales,
  with bounded GitHub-token fallbacks for explicit non-UTF-8 locales.
- [x] Cover picker labels, bottom-border chips, locale selection, and the
  representative threaded-comment snapshot.

# PR Context Views

## 1. Context tab shell

**Acceptance criteria:**
- [x] New and restored in-process review tabs default to Changes; `v`/`V`
  cycle the three views without changing source review state.
- [x] View selection is isolated across open PR tabs, not persisted, and
  visible without terminal color at wide and narrow widths.

**Verification:**
- [x] Start with focused failing TUI model/render tests.
- [x] Run `go test ./internal/tui -run 'Test(PRContextView|ReviewTab|Snapshot)' -count=1`.

**Dependencies:** Approved `docs/spec/SPEC-pr-view-tabs.md`.

**Files likely touched:** `internal/tui/{model,render,bindings}.go`, matching
tests and screen baselines. **Estimated scope:** Medium.

## 2. Frozen description contract

**Acceptance criteria:**
- [x] A validated GitHub body is stored with a new immutable session; legacy
  sessions remain readable and require no network access.
- [x] Description-only edits do not alter source pin equality or force fetches.

**Verification:**
- [x] Start with focused failing source/session/review tests.
- [x] Run `go test ./internal/source ./internal/session ./internal/review -count=1`.

**Dependencies:** Task 1. **Files likely touched:** `internal/source/{github,git}.go`,
`internal/review/review.go`, `internal/session/store.go`, matching tests.
**Estimated scope:** Medium.

## 3. Description view

**Acceptance criteria:**
- [x] Description displays escaped frozen text plus clear empty and legacy
  states; its tab-owned scroll is independent of Changes.
- [x] Offline/resume, plain output, progress, and comment behavior are unchanged.

**Verification:**
- [x] Start with focused failing TUI tests and inspect wide/narrow baselines.
- [x] Run `go test ./internal/tui ./cmd/pr-review -count=1`.

**Dependencies:** Tasks 1–2. **Files likely touched:** `internal/tui/{model,render}.go`,
tests, screens, `README.md`. **Estimated scope:** Medium.

## Checkpoint: Description

- [x] Focused source/session/review/TUI tests and `go build ./...` pass.
- [ ] Manual terminal check confirms literal, safely escaped text and tab-local state.

## 4. Frozen commit contract

**Acceptance criteria:**
- [ ] A bounded, validated commit list is stored only when a post-load metadata
  check matches the frozen source revision.
- [ ] A capped 100-item result and legacy absence are represented explicitly.

**Verification:**
- [ ] Start with focused failing source/session/review tests.
- [ ] Run `go test ./internal/source ./internal/session ./internal/review -count=1`.

**Dependencies:** Task 1. **Files likely touched:** `internal/source/github.go`,
`internal/review/review.go`, `internal/session/store.go`, matching tests.
**Estimated scope:** Medium.

## 5. Commit list view

**Acceptance criteria:**
- [ ] Commits render a textual selection, independent scroll, and truthful
  zero/truncated/legacy states without affecting diff progress or comments.
- [ ] Documentation, wide/narrow screens, and help agree with the shipped views.

**Verification:**
- [ ] Start with focused failing TUI tests and inspect screen baselines.
- [ ] Run `go test ./internal/tui ./cmd/pr-review -count=1`.

**Dependencies:** Tasks 1 and 4. **Files likely touched:** `internal/tui/{model,render,bindings}.go`,
tests, screens, `README.md`. **Estimated scope:** Medium.

## Completion checkpoint

- [ ] `./scripts/verify.sh` and `git diff --check` pass.
- [ ] Review confirms source pinning, session immutability, offline behavior,
and text-only selected-state contracts remain intact.

## 6. Replace multi-PR workspace with selector replacement

**Acceptance criteria:**
- [ ] The model owns one active review; `ctrl+p` selects a replacement PR and
  has no opened-review rows, numeric navigation, deduplication, or capacity.
- [ ] Successful selection atomically resets transient UI state; cancel/error
  retains the old review and disk-backed session behavior remains unchanged.

**Verification:**
- [ ] Start with focused failing selector/model/lifecycle tests.
- [ ] Run `go test ./internal/tui ./cmd/pr-review -count=1` and the full gate.

**Dependencies:** Tasks 1–3. **Files likely touched:** `internal/tui/{model,lifecycle,picker}.go`,
matching tests/screens, `README.md`. **Estimated scope:** Large; split into
state removal and selector lifecycle slices before implementation.

# Markdown PR Description Rendering tasks

## 1. Safe Markdown renderer adapter

**Description:** Add the Glamour v2 dependency and a focused Description
renderer adapter that normalizes line endings, makes unsafe controls visible,
uses a configured terminal style and width, and returns ANSI-rendered lines.

**Acceptance criteria:**
- [x] CRLF and lone-CR Markdown render with no literal `\\r` artifact.
- [x] Common GFM constructs render readably; raw HTML remains literal, and
      unsafe terminal-control input cannot escape into output.
- [x] The adapter has a narrow, testable API independent of `Model` state.

**Verification:**
- [x] Start with focused failing adapter tests, then run
      `go test ./internal/tui -run 'TestDescriptionMarkdown' -count=1`.
- [x] Glamour module metadata contains only Glamour and its transitive dependencies.

**Dependencies:** Approved `docs/spec/SPEC-pr-description.md`.

**Files likely touched:** `go.mod`, `go.sum`, new `internal/tui/description_markdown.go`,
new `internal/tui/description_markdown_test.go`.

**Estimated scope:** Medium (4 files).

## 2. Cached Description-view integration

**Description:** Replace per-key plain-text wrapping with a tab-owned cache of
the adapter's ANSI lines. Invalidate it for the frozen body, available width,
or theme/style changes while retaining all current empty/legacy and scrolling
semantics.

**Acceptance criteria:**
- [x] Description scrolling slices cached output and does not reparse the
      frozen body on navigation.
- [x] Resize/theme/review replacement rebuilds cached wrapping; empty and
      legacy views remain non-rendered explicit states.
- [x] The Description tab has no effect on diff state, comments, or offline
      lifecycle behavior.

**Verification:**
- [x] Add focused model/view regressions, then run
      `go test ./internal/tui -run 'TestDescriptionView' -count=1`.

**Dependencies:** Task 1.

**Files likely touched:** `internal/tui/model.go`, `internal/tui/description_view_test.go`,
`internal/tui/style.go`, `internal/tui/style_test.go`.

**Estimated scope:** Medium (4 files).

## Checkpoint: safe interactive rendering

- [x] Focused renderer and Description-view tests pass in color and colorless
      modes at narrow and wide widths.
- [x] `git diff --check` passes.

## 3. User-facing regression completion

**Description:** Update frozen-description documentation and deterministic
screens, then run the complete regression gate against the integrated feature.

**Acceptance criteria:**
- [x] README describes rendered Markdown as display-only and documents the
      literal-HTML/no-link-activation scope.
- [x] Screen snapshots reflect readable Markdown at wide and narrow widths.
- [x] Full tests preserve source/session/review contracts and plain output.

**Verification:**
- [x] Run `go test -race -count=1 ./...`, `go build ./...`, and
      `git diff --check`. `./scripts/verify.sh` is blocked by pre-existing
      formatting in `internal/tui/program_test.go`.
- [ ] Manually inspect a Description containing headings, a list, a fenced
      block, a link, raw HTML, and CRLF input in a real terminal.

**Dependencies:** Task 2.

**Files likely touched:** `README.md`, `internal/tui/snapshot_test.go`,
`internal/tui/testdata/screens/{description_wide,description_narrow}.golden`,
`internal/tui/description_view_test.go`.

**Estimated scope:** Medium (4 files).

## Completion checkpoint

- [ ] All success criteria in `docs/spec/SPEC-pr-description.md` are met.
- [ ] Full automated verification passes with no GitHub lifecycle, snapshot,
      source-pin, or link-activation behavior change.

# Mouse Selection and Panel Resizing tasks

Implemented with subagents. See the matching section in `tasks/plan.md`
for scope, current-code evidence, interaction semantics, and API contracts.
Existing unchecked tasks above are preserved. All agents must use synthetic fixtures and preserve CONSTRAINTS.md.

## M1: Establish shared screen geometry and input routing

Description: Extract the existing review geometry and visible list calculation
without changing output, then add a centrally gated mouse dispatch seam. Keep
capture disabled until supported handlers land. Coordinator owns this slice.

Acceptance criteria:
- [x] Rendering and hit testing share body/rail/detail/divider rectangles and
  visible list rows; duplicate fixed-width rendering math is removed. Normal
  snapshots and narrow fallback remain unchanged.
- [x] Rectangles use clipped half-open terminal-cell bounds; negative/outside
  coordinates, empty comparisons, headers/footers, and description rows have
  no item target. Label spans account for ANSI-free display width.
- [x] Central routing enforces modal/loading/editor ownership and defines the
  workspace width/drag fields and handler signatures used by downstream agents.

Verification: Add geometry/routing tests, run `go test ./internal/tui -count=1`,
`go build ./...`, and `git diff --check`. Inspect unchanged wide/narrow frames.
Dependencies: None. Scope: Medium.
Files likely touched: `internal/tui/model.go`, new `workspace_geometry.go`,
`workspace_geometry_test.go`, `mouse.go`, `mouse_test.go` in `internal/tui`.

## M2: Select review navigation items and visible tabs

Description: Implement click-to-select for Files, Inventory, and Guides plus
existing visible context/mode tabs. Own `mouse_review.go`; request shared model
integration from the coordinator rather than editing it concurrently.

Acceptance criteria:
- [x] Left press on a visible item selects its semantic row and focuses the
  rail, reusing existing navigation effects (unit synchronization, offsets,
  horizontal reset, path-scroll restart). Wrapped explanation/padding rows
  and hidden panes are no-ops; selection never marks progress or opens editors.
- [x] Clicking visible context/mode labels invokes the corresponding existing
  transition; clipped labels have only their visible cells clickable. Guide
  expansion and activation retain their existing keyboard actions.
- [x] Real Bubble Tea click messages route through Update with cell-motion
  capture enabled on supported review surfaces; repeated clicks do not
  activate items, write data, or generate guides. Keyboard behavior still works.

Verification: Add state tests for scrolled lists, repeated guide file portions,
99/100 columns, clipping/Unicode, and tab transitions. Run
`go test ./internal/tui -count=1` and `go build ./...`; inspect selection in a
synthetic review frame. Dependencies: M1. Scope: Medium.
Files likely touched: `mouse_review.go`, `mouse_review_test.go`, `guides.go`,
plus coordinator edits to `model.go` and `mouse.go` in `internal/tui`.

## M3: Select visible picker results

Description: Share picker viewport math and implement mouse selection for
repository, session, PR browser, filtered switcher, and theme pickers. This
agent owns the picker-specific helper and handler; keep shared integration
with the coordinator.

Acceptance criteria:
- [x] Renderer and handler use one picker-window calculation including truncated
  headers, optional details, footer, and selected-dependent capacity. Clicking
  a filtered switcher row selects the displayed result identity/index.
- [x] Left press only changes picker selection. Enter still opens/switches or
  applies a theme. Empty/loading/error/details/footer rows have no item target;
  filter text and off-screen items are unaffected.
- [x] Short and long lists, changing detail reservations, Unicode, tiny heights,
  and independent picker cursors retain correct selection and keyboard behavior.

Verification: Add focused picker mouse tests and run
`go test ./internal/tui -count=1`, `go build ./...`. Inspect a filtered switcher
and tiny picker fixture. Dependencies: M1; can run alongside M2/M5. Scope: Medium.
Files likely touched: `internal/tui/picker.go`, new `picker_geometry.go`,
`mouse_picker.go`, `mouse_picker_test.go`, coordinator-owned `mouse.go`.

## Checkpoint: M1–M3

- [x] Geometry contract reviewed, focused tests/build pass, existing snapshots
  still agree, and no shared-file changes were overwritten.
- [x] Selection-only handlers produce no source/network/storage writes and do
  not bypass loading, composer, consent, or review-confirmation ownership.

## M4: Select semantic diff cells and comment cards

Description: Extend review selection to unified and split details using the
existing split-cursor work's complete-target selection contract. This is a
sequential follow-up by the review-selection agent, not a second cursor rewrite.

Acceptance criteria:
- [x] Clicked visible detail row includes its viewport offset; unified rows and
  comment cards resolve to immutable source targets or stable comment IDs.
  Structural rows, empty paired cells, and separators do not select a neighbor.
- [x] Split clicks select the actual old/new cell using shared rendered cell
  bounds (including cursor/line gutters). Context retains the existing
  right-only commentability rule; the old context cell cannot invent LEFT.
- [x] Selection survives layout toggles/reflow and subsequent Enter opens the
  correct existing composer/action menu. A click itself opens neither and
  cannot post a comment. Horizontal clipping cannot alter target identity.

Verification: Add synthetic paired deletion/addition, context, overlay,
159/160-column, and horizontal-scroll tests. Assert exact target equality and
zero submitter calls after clicks. Run `go test ./internal/tui -count=1` and
`go build ./...`. Dependencies: M2 and completion/reconciliation of the earlier
Split-Diff Pane Cursor semantic-state tasks. Scope: Medium.
Files likely touched: `internal/tui/mouse_review.go`, `mouse_review_test.go`,
`render.go`, `render_test.go`, coordinator-owned `model.go`.

## M5: Drag the navigation/detail divider

Description: Implement bounded workspace rail width and a transient divider
drag lifecycle through the established geometry/dispatch contracts.

Acceptance criteria:
- [x] Press on the three-column divider starts drag; motion adjusts effective
  width with preserved grab offset and main’s shared 18-column rail/40-column detail
  minimums. Default widths stay unchanged until
  dragged. Release ends drag without selecting/activating an item.
- [x] No drag exists below 100 columns or outside Changes. Resize, keyboard,
  page/tab/view changes, loading/editor entry, stale fresh presses, and lost-left
  motion cancel drag. Requested width survives narrow mode and is saved per review
  in memory, never persisted. The inner split divider is unchanged.
- [x] Reflow preserves semantic selection and progress, clamps offsets,
  maintains editor visibility, and refreshes guide wrapping/path scrolling.
  Odd widths, edge clamps, and repeated motion produce no overflow or panic.

Verification: Add press/motion/release and cancellation sequences, boundary
width tests, default-render parity, and anchor-retention assertions. Run
`go test ./internal/tui -count=1`, `go build ./...`; inspect both drag extremes.
Dependencies: M1; can run alongside M2/M3 using coordinator integration.
Scope: Medium. Files likely touched: `internal/tui/mouse_resize.go`,
`mouse_resize_test.go`, coordinator-owned `model.go`, `mouse.go`, and
`workspace_geometry.go`.

## Checkpoint: M4–M5

- [x] Full TUI tests/build pass with exact source-side targets after drag and
  split/unified fallback; verify a drag cannot finish as a background click.
- [x] Coordinator integrates all handlers and rechecks modal precedence,
  preference ownership, visible bounds, and mouse capture policy together.

## M6: Verify terminal behavior and document controls

Description: Coordinator integrates the completed slices, adds real protocol
coverage, and publishes accurate mouse help and compatibility limitations.

Acceptance criteria:
- [x] A bounded synthetic PTY journey sends SGR click/drag/release sequences,
  observes selection and panel width changes, verifies keyboard input still
  works, and checks terminal capture cleanup on exit. No live service is used.
- [x] README and Health & help explain selection vs Enter activation, supported
  divider, bounds/narrow behavior, memory-only width, and terminal-native text
  selection behavior. Annotate the old no-mouse boundary in the layout spec.
- [x] Full verification and diff checks pass or unrelated baseline failures are
  reported accurately; human terminal usability inspection is recorded as done
  or pending, never inferred from automated render tests.

Verification: Run focused PTY tests (`go test ./cmd/pr-review -run PTY -count=1`),
then `./scripts/verify.sh` and `git diff --check`. Inspect mouse use in a real
terminal and a terminal multiplexer where available, including releasing a
button outside the terminal and modifier-based text copying.
Dependencies: M2, M3, M4, M5. Scope: Medium.
Files likely touched: `cmd/pr-review/testdata/pty_smoke.py`,
`cmd/pr-review/pty_test.go`, `internal/tui/help.go`, `README.md`,
`docs/spec/SPEC-tui-review-layout.md`. If help content belongs in `bindings.go`, use that
instead of `help.go`; do not add a fake keyboard shortcut for mouse actions.

## Completion checkpoint

- [x] All M1–M6 acceptance criteria satisfied and reviewed against the proposed
  interaction contract; any changed scope is reflected in the plan.
- [x] Existing keyboard, frozen-target, offline, plain-output, and tab-isolation
  behavior preserved; no new dependency, schema, or implicit write path.
- [ ] Human terminal/multiplexer usability check completed (text copying and
  release outside the window); automated checks do not establish usability.


# SQLite Storage — Fresh Start tasks

Status: Completed on 2026-09-26.
Evidence: [SQLite verification](../docs/SQLITE-VERIFICATION.md).

Spec: [SPEC-sqlite-storage.md](../docs/spec/SPEC-sqlite-storage.md).
Plan: [SQLite Storage — Fresh Start](plan.md#implementation-plan-sqlite-storage--fresh-start).
User authorized appending; preceding task sections remain unchanged.

SQL-10..16 are coordinated cutover patches, not individually releasable changes.
Run their verification against the integrated batch; do not mark them complete
until that batch compiles and passes. The checks below have completed against the integrated batch. Equivalent
real-database tests use the final names in the implementation; benchmark, native
platform, failure-mode and packaging results are recorded in the evidence link.

## SQL-01: Prove the embedded driver and record a baseline

- [x] Complete SQL-01.

**Description:** Measure the current storage benchmark, evaluate the pinned CGo-free driver, and exercise an isolated real database before building the replacement.

**Acceptance criteria:**
- [x] Pin an explicitly reviewed driver version compatible with Go 1.26.8 and macOS/Linux; document license, bundled SQLite, dependency/security results, and any blockers.
- [x] A CGo-disabled storage test executable creates, closes, reopens, and reads a real SQLite file without invoking external SQLite tools.
- [x] Record baseline history benchmark and executable size; do not change CLI persistence or the user’s store.

**Verification:**
- [x] `go test ./internal/session -run '^$' -bench BenchmarkLatestComparisonHistory -benchmem -count=5`
- [x] `CGO_ENABLED=0 go test -count=1 ./internal/session/storage`
- [x] `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`

**Dependencies:** Approved plan.

**Delegation owner:** A.

**Spec coverage:** S12; embedded packaging.

**Files likely touched:**
- `go.mod`
- `go.sum`
- `internal/session/storage/runtime_test.go`

**Estimated scope:** Medium (3 files). Split before dispatch if more files become necessary.

## SQL-02: Freeze storage contracts and schema

- [x] Complete SQL-02.

**Description:** Extract retained domain types/validation without changing current behavior, and define the concrete SQL contract and complete schema before parallel query work.

**Acceptance criteria:**
- [x] Freeze StateUpdate, summary fields, typed error categories, canonical payload versions, and SQL row contracts described in the plan.
- [x] DDL defines source/bundle/session/progress/cache/registry relationships, bounds, recency/comparison indexes, and bootstrap store identity; DerivedFrom is not a parent FK.
- [x] The old running app still builds; no generic backend interface, compatibility Save, or runtime selector is added.

**Verification:**
- [x] `go test -race -count=1 ./internal/session`
- [x] `go build ./...`
- [x] `git diff --check`

**Dependencies:** SQL-01.

**Delegation owner:** A.

**Spec coverage:** S1–S11 contract.

**Files likely touched:**
- `internal/session/store.go`
- `internal/session/types.go`
- `internal/session/validation.go`
- `internal/session/sqlite_contract.go`
- `internal/session/storage/schema.sql`

**Estimated scope:** Medium (5 files). Split before dispatch if more files become necessary.

## SQL-03: Open and initialize a private SQLite store

- [x] Complete SQL-03.

**Description:** Implement the actual engine lifecycle with bounded operations and a minimal bootstrap identity protocol.

**Acceptance criteria:**
- [x] Concurrent first opens converge on one complete schema; ready/missing, foreign, legacy, unsafe-link, and ambiguous initialization states fail without replacing data.
- [x] DELETE/EXTRA, foreign keys, busy bound, application/schema IDs and one-connection policy are applied and verified; startup lock is released before normal operations.
- [x] Read-only opens change no persistent files and never recover a hot journal or initialize a missing store; Close is safe and later operations fail.

**Verification:**
- [x] `go test -race -count=1 ./internal/session/storage`
- [x] `CGO_ENABLED=0 go test -count=1 ./internal/session/storage`

**Dependencies:** SQL-02.

**Delegation owner:** A.

**Spec coverage:** S3/S4/S10/S11.

**Files likely touched:**
- `internal/session/storage/db.go`
- `internal/session/storage/bootstrap_unix.go`
- `internal/session/storage/db_test.go`
- `internal/session/storage/bootstrap_test.go`

**Estimated scope:** Medium (4 files). Split before dispatch if more files become necessary.

## SQLite checkpoint A — real private database

- [x] SQL-01..03 evidence is reviewed; driver/DDL/bootstrap contracts are frozen.
- [x] Storage tests pass and current application still builds; unsupported-driver or unsafe-initialization findings are resolved before delegation expands.

## SQL-04: Round-trip immutable sessions with shared source

- [x] Complete SQL-04.

**Description:** Implement Create/Load against the new SQL engine, using a private construction seam until coordinated cutover.

**Acceptance criteria:**
- [x] Versioned canonical source excludes checkout/guide/parent; equal source is stored once while sessions retain distinct identity and immutable logical references.
- [x] Create/load preserve binary paths, nil/empty descriptions, bounded patches/context and guide provenance; checksum/domain/index identity mismatches fail visibly.
- [x] Derived creation validates the live parent transactionally without making later parent existence a load requirement.

**Verification:**
- [x] `go test -race -count=1 ./internal/session -run 'TestSQLite(Session|Snapshot|Derived|Payload)'`
- [x] `go build ./...`

**Dependencies:** SQL-03.

**Delegation owner:** A.

**Spec coverage:** S1/S2/S6.

**Files likely touched:**
- `internal/session/sqlite_store.go`
- `internal/session/sqlite_codec.go`
- `internal/session/sqlite_store_test.go`

**Estimated scope:** Medium (3 files). Split before dispatch if more files become necessary.

## SQL-05: Commit progress through the state-only API

- [x] Complete SQL-05.

**Description:** Implement small transactional updates that never read or serialize source payloads.

**Acceptance criteria:**
- [x] UpdateState compares expected generation/reference, validates file membership, preserves progress order, and commits freshness/progress/time/generation together.
- [x] Competing independent handles/processes with the same generation produce exactly one success; overflow, busy, canceled and uncertain-commit outcomes are explicit.
- [x] Returned state becomes available only after confirmed commit; SQL tracing or a test seam proves no snapshot BLOB access.

**Verification:**
- [x] `go test -race -count=1 ./internal/session -run 'TestSQLiteState'`
- [x] `go test -race -count=1 ./internal/session/storage`

**Dependencies:** SQL-04.

**Delegation owner:** A.

**Spec coverage:** S4/S5.

**Files likely touched:**
- `internal/session/sqlite_state.go`
- `internal/session/sqlite_state_test.go`

**Estimated scope:** Small (2 files). Split before dispatch if more files become necessary.

## SQLite checkpoint B — source and progress

- [x] Round-trip, deduplication and state-CAS tests pass with no source reads on progress updates.
- [x] No database objects leak into application interfaces; parallel file ownership is confirmed.

## SQL-06: Persist reusable guide bundles

- [x] Complete SQL-06.

**Description:** Implement the existing generated-guide cache semantics using SQLite bundles and comparison keys.

**Acceptance criteria:**
- [x] Only generated, structurally validated bundles become cache entries; full comparison, inventory and prompt identity govern reuse.
- [x] Unavailable and old-prompt session-attached bundles remain readable; invalid individual cache payloads miss without hiding database/I/O errors.
- [x] Cache replacement and bundle references are transactional, with no provider/network invocation or per-guide file write.

**Verification:**
- [x] `go test -race -count=1 ./internal/session -run 'TestSQLiteGuide'`
- [x] `go test -count=1 ./internal/guide`

**Dependencies:** SQL-05; contract frozen in SQL-02.

**Delegation owner:** B.

**Spec coverage:** S1/S2/S8.

**Files likely touched:**
- `internal/session/sqlite_guides.go`
- `internal/session/sqlite_guides_test.go`

**Estimated scope:** Small (2 files). Split before dispatch if more files become necessary.

## SQL-07: Persist remembered repositories

- [x] Complete SQL-07.

**Description:** Replace registry file behavior with bounded SQL records and durable ordering.

**Acceptance criteria:**
- [x] Repository normalization and canonical absolute checkout validation retain current behavior; replacement preserves insertion order.
- [x] Lookup/list work after restart and in read-only mode; invalid rows and database failures remain visible.
- [x] Registry operations write no repositories.json and expose no database objects to application callers.

**Verification:**
- [x] `go test -race -count=1 ./internal/session -run 'TestSQLiteRepositor'`

**Dependencies:** SQL-05; sequential after SQL-06 when using lane B.

**Delegation owner:** B.

**Spec coverage:** S1/S10.

**Files likely touched:**
- `internal/session/sqlite_repositories.go`
- `internal/session/sqlite_repositories_test.go`

**Estimated scope:** Small (2 files). Split before dispatch if more files become necessary.

## SQL-08: Query session summaries and comparison candidates

- [x] Complete SQL-08.

**Description:** Implement indexed lookup and metadata-only listing without loading unrelated patches.

**Acceptance criteria:**
- [x] Summary entries contain counts/identity/state and no Record payload; listing decodes zero source BLOBs.
- [x] Comparison queries use repo/PR/revision/recency indexes, deterministic tie-breaking, and validate selected payload identities before reuse.
- [x] Invalid matching candidates can be skipped, engine errors propagate, and description/nil handling follows the existing domain contract.

**Verification:**
- [x] `go test -race -count=1 ./internal/session -run 'TestSQLite(Lookup|Summary)'`

**Dependencies:** SQL-05; may run alongside SQL-06/07.

**Delegation owner:** C.

**Spec coverage:** S2/S7.

**Files likely touched:**
- `internal/session/sqlite_lookup.go`
- `internal/session/sqlite_lookup_test.go`

**Estimated scope:** Small (2 files). Split before dispatch if more files become necessary.

## SQLite checkpoint C — reusable data and queries

- [x] Guide cache, registry and summary/index tests pass independently.
- [x] Queries propagate engine failures and close rows before nested operations.

## SQL-09: Delete sessions without deleting shared content

- [x] Complete SQL-09.

**Description:** Implement reference-aware deletion after all reference-producing paths exist.

**Acceptance criteria:**
- [x] Deleting a session atomically removes its progress and references; other sessions and children remain readable.
- [x] Only source/bundle rows with no live session or cache reference are reclaimed; independent guide cache and repository records survive.
- [x] Failure rolls back the whole operation, invalid IDs never affect unrelated data, and no automatic VACUUM or forensic-erasure promise is added.

**Verification:**
- [x] `go test -race -count=1 ./internal/session -run 'TestSQLiteDelete'`
- [x] `go test -race -count=1 ./internal/session ./internal/session/storage`

**Dependencies:** SQL-06, SQL-07, SQL-08.

**Delegation owner:** A.

**Spec coverage:** S9.

**Files likely touched:**
- `internal/session/sqlite_delete.go`
- `internal/session/sqlite_delete_test.go`

**Estimated scope:** Small (2 files). Split before dispatch if more files become necessary.

## SQLite checkpoint D — backend ready for cutover

- [x] All SQL persistence slices pass their tests, including shared-content deletion.
- [x] Coordinator freezes the public state/Entry contract and dispatches disjoint cutover patches; no partial API batch is released.

## SQL-10: Move review lifecycle to committed state updates

- [x] Complete SQL-10.

**Description:** Prepare the review-layer portion of the coordinated cutover, including context propagation for Mark.

**Acceptance criteria:**
- [x] Mark, Refresh and Resume use UpdateState rather than Save and assign committed state only after success.
- [x] Add context to Mark and adapt tests; failed/canceled/stale updates leave caller-owned state unchanged.
- [x] Offline resume remains network-free and readable after the checkout is deleted.

**Verification:**
- [x] `go test -race -count=1 ./internal/review`

**Dependencies:** Checkpoint D; integrated with SQL-12/16.

**Delegation owner:** Available integration lane.

**Spec coverage:** S5/S12.

**Files likely touched:**
- `internal/review/lifecycle.go`
- `internal/review/lifecycle_test.go`

**Estimated scope:** Small (2 files). Split before dispatch if more files become necessary.

## SQL-11: Move CLI persistence to SQLite contracts

- [x] Complete SQL-11.

**Description:** Prepare CLI state saves and metadata-only session output for the same cutover.

**Acceptance criteria:**
- [x] Replace both production Save calls and the direct Save test call; update the three review.Mark test calls in lifecycle_test.go/wiring_test.go to pass context and preserve reopening progress.
- [x] sessions output uses summary fields without loading source, and fresh default/error reporting identifies the new store.
- [x] Read-only guide evaluation, registry reopening, plain output and explicit generation behavior remain covered.

**Verification:**
- [x] `go test -race -count=1 ./cmd/pr-review`

**Dependencies:** Checkpoint D and SQL-10 API contract; integrated with SQL-16.

**Delegation owner:** B.

**Spec coverage:** S5/S7/S10/S12.

**Files likely touched:**
- `cmd/pr-review/lifecycle.go`
- `cmd/pr-review/lifecycle_test.go`
- `cmd/pr-review/main.go`
- `cmd/pr-review/wiring_test.go`
- `cmd/pr-review/eval_guides_test.go`

**Estimated scope:** Medium (5 files). Split before dispatch if more files become necessary.

## SQL-12: Move TUI state saves to SQLite contracts

- [x] Complete SQL-12.

**Description:** Prepare event-loop-owned state application and update the review.Mark call for the cutover.

**Acceptance criteria:**
- [x] Replace the Save calls in model.go and lifecycle.go; use the active operation context where present and m.ctx for synchronous Mark calls, never a possibly nil actionCtx.
- [x] Background results cannot overwrite newer reading progress or another tab’s state; apply only successfully committed state.
- [x] Cancel/failure and session replacement preserve established draft/progress ownership behavior.

**Verification:**
- [x] `go test -race -count=1 ./internal/tui -run 'Lifecycle|Background|Program.*Progress|ProgramBackground'`

**Dependencies:** Checkpoint D and SQL-10 API; integrated with SQL-16.

**Delegation owner:** Available integration lane.

**Spec coverage:** S5/S12.

**Files likely touched:**
- `internal/tui/model.go`
- `internal/tui/lifecycle.go`
- `internal/tui/lifecycle_test.go`
- `internal/tui/program_test.go`

**Estimated scope:** Medium (4 files). Split before dispatch if more files become necessary.

## SQL-13: Render the session picker from summaries

- [x] Complete SQL-13.

**Description:** Complete TUI summary integration after the state-save edits to its shared lifecycle file.

**Acceptance criteria:**
- [x] Picker rows use repository/PR/read counts from Entry summaries, with no Record access.
- [x] Successful and unreadable rows render safely, and selection loads source only when opening a session.
- [x] Keep the existing Entry callback type shape so picker transport code requires no unrelated rewrite.

**Verification:**
- [x] `go test -race -count=1 ./internal/tui -run 'Picker|BrowserInitialization'`

**Dependencies:** SQL-12; integrated with SQL-16.

**Delegation owner:** Same owner as SQL-12.

**Spec coverage:** S7/S12.

**Files likely touched:**
- `internal/tui/lifecycle.go`
- `internal/tui/picker_test.go`

**Estimated scope:** Small (2 files). Split before dispatch if more files become necessary.

## SQL-14: Replace filesystem persistence tests with SQLite coverage

- [x] Complete SQL-14.

**Description:** Prepare the core test replacement as one owned patch; retain useful fixtures and behavior coverage.

**Acceptance criteria:**
- [x] Replace per-file corruption/permission and lifetime-lock assumptions with real DB transactions, row corruption and concurrent-process behavior.
- [x] Retain guide/registry/frozen-byte/progress/reference tests and the history benchmark; remove obsolete on-disk legacy round-trip assertions.
- [x] Dedup corruption tests distinguish shared-source corruption from session-specific corruption; missing committed rows fail visibly.

**Verification:**
- [x] `go test -race -count=1 ./internal/session`
- [x] `go test ./internal/session -run '^$' -bench BenchmarkLatestComparisonHistory -benchmem -count=5`

**Dependencies:** Checkpoint D; integrated with SQL-16.

**Delegation owner:** C.

**Spec coverage:** S1/S2/S3/S4/S6/S7/S8/S9.

**Files likely touched:**
- `internal/session/store_test.go`
- `internal/session/lifecycle_test.go`
- `internal/session/lookup_test.go`

**Estimated scope:** Medium (3 files). Split before dispatch if more files become necessary.

## SQL-15: Replace read-only and safety fixtures

- [x] Complete SQL-15.

**Description:** Finish the persistence test conversion without retaining the old filesystem backend.

**Acceptance criteria:**
- [x] Read-only tests inspect all relevant SQLite/ownership artifacts and prove no persisted mutation, including recovery-required cases.
- [x] Private root/database/journal/initialization symlink checks reject unsafe targets unchanged; old nonempty roots are rejected without import.
- [x] Description tests preserve nil/empty round-trips and source-reuse semantics using SQLite; legacy wording/fixtures are removed.

**Verification:**
- [x] `go test -race -count=1 ./internal/session ./internal/session/storage`

**Dependencies:** SQL-14; integrated with SQL-16.

**Delegation owner:** C.

**Spec coverage:** S2/S3/S10/S11.

**Files likely touched:**
- `internal/session/readonly_test.go`
- `internal/session/safety_test.go`
- `internal/session/description_test.go`

**Estimated scope:** Medium (3 files). Split before dispatch if more files become necessary.

## SQL-16: Activate SQLite and delete the file backend

- [x] Complete SQL-16.

**Description:** Integrate the prepared caller/test patches with the production Store switch as one buildable batch.

**Acceptance criteria:**
- [x] Store opens only SQLite at the new default storage path; every session/guide/registry operation uses the completed SQL implementation.
- [x] Delete old JSON I/O, .format/.lock handling, obsolete file helpers, Save method, full-record Entry payload, and private staging seam; no fallback/selector remains.
- [x] All cutover patches are integrated together and compile; unrelated theme/Git/in-memory cache code and user data remain untouched.

**Verification:**
- [x] `go vet ./...`
- [x] `go test -race -count=1 -timeout=5m ./...`
- [x] `go build ./...`
- [x] `rg -n 'func .*Save\(|snapshot\.json|state\.json|repositories\.json|O_NOFOLLOW|Flock' internal/session`

**Dependencies:** SQL-10, SQL-11, SQL-13, SQL-14, SQL-15; SQL-01..09 complete.

**Delegation owner:** A with coordinator integration.

**Spec coverage:** S1–S12. The rg audit may find legitimate new bootstrap/privacy
uses of Flock/O_NOFOLLOW; review each match for ownership rather than requiring
zero matches or removing necessary safety code.

**Files likely touched:**
- `internal/session/store.go`
- `internal/session/sqlite_store.go`
- `internal/session/sqlite_contract.go`
- `internal/session/types.go`

**Estimated scope:** Medium (4 files). Split before dispatch if more files become necessary.

## SQLite checkpoint E — SQLite-only application

- [x] SQL-10..16 are integrated as one working batch; complete application builds and focused/full race tests pass.
- [x] All old session/guide/registry file persistence and compatibility APIs are removed; audit search matches are reviewed, not blindly deleted.

## SQL-17: Document the fresh SQLite storage contract

- [x] Complete SQL-17.

**Description:** Update user/developer guidance to describe shipped persistence and remove misleading old-backend instructions.

**Acceptance criteria:**
- [x] Document new location, no old-cache import, no external SQLite install, read-only behavior, logical deletion and metadata-only listing.
- [x] Update CONSTRAINTS lifetime-lock/atomic-file mechanics to transactions and generation checks without weakening source/privacy constraints.
- [x] Preserve macOS/Linux gates, describe native evidence requirements and benchmarks, and keep Windows explicitly deferred.

**Verification:**
- [x] `git diff --check`
- [x] `rg -n 'writer lock|snapshot.json|state.json|0700|SQLite|sqlite' README.md CONSTRAINTS.md TESTING.md docs/REFERENCE.md`

**Dependencies:** Checkpoint E.

**Delegation owner:** Coordinator.

**Spec coverage:** S10/S11/S12; documentation.

**Files likely touched:**
- `README.md`
- `CONSTRAINTS.md`
- `TESTING.md`
- `docs/REFERENCE.md`

**Estimated scope:** Medium (4 files). Split before dispatch if more files become necessary.

## SQL-18: Verify packaging, failures and measured behavior

- [x] Complete SQL-18.

**Description:** Complete independent acceptance checks against the integrated application and record actual platform evidence.

**Acceptance criteria:**
- [x] Prove 100 identical raw/guided reopenings share source, state saves/listing read zero source BLOBs, and reference-aware deletion preserves survivors.
- [x] Exercise subprocess interruption, contention, cancellation and disk-full/commit failure; prove a CGo-disabled executable persists data with no SQLite executable/shared-library dependency.
- [x] Report same-machine before/after benchmark, allocations and executable size; full static/race/PTY/build/security checks pass and native Linux evidence is recorded without claiming cross-builds are runtime tests.

**Verification:**
- [x] `./scripts/verify.sh`
- [x] `golangci-lint run ./...`
- [x] `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`
- [x] `CGO_ENABLED=0 go build -o /tmp/pr-review-sqlite ./cmd/pr-review`
- [x] `go test ./internal/session -run '^$' -bench 'Benchmark(LatestComparisonHistory|SQLite)' -benchmem -count=5`
- [x] `git diff --check`

**Dependencies:** SQL-17.

**Delegation owner:** C plus coordinator.

**Spec coverage:** S1–S12; embedded packaging.

**Files likely touched:**
- `internal/session/sqlite_acceptance_test.go`
- `internal/session/storage/runtime_test.go`
- `internal/session/sqlite_benchmark_test.go`

**Estimated scope:** Medium (3 files). Split before dispatch if more files become necessary.

## SQLite checkpoint F — completion review

- [x] Every specification acceptance criterion has evidence; packaging requires no user-installed SQLite.
- [x] Full macOS and native Linux results, benchmark deltas, remaining limitations, and code review are recorded.
- [x] Only SQLite tasks are marked complete; no unrelated existing task or user store is modified.

Implementation notes:
- SQL-01..03: embedded driver, lifecycle, bootstrap/schema and security checks.
- SQL-04..09: canonical payloads, state CAS, guides/registry, indexed summaries and deletion.
- SQL-10..16: coordinated callers/tests cutover; obsolete persistence code removed.
- SQL-17..18: fresh-store docs, native macOS/Linux race/PTY gates, CGo-free proof,
  failure tests and before/after measurements. Windows remains deferred.
- Independent review fixes cover orphan bootstrap artifacts, coherent read
  transactions, source membership corruption, typed errors and payload bounds.
- Native Linux additionally exposed a verifier timing precision defect, corrected
  in a separate commit while retaining its original positive-duration assertion.
