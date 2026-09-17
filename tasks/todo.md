# TUI reliability and testing tasks

## 1. Safe PR browser startup
- [ ] Share model initialization and make browser `Init` safe without a loader.
- [ ] Regression test starts the browser through its actual initialization path.
- [ ] Verify: focused browser startup tests and `go test ./internal/tui`.

## 2. Screen-owned picker state
- [ ] Repository, PR, and session pickers own independent cursors; returning to a
      parent restores its selection and cannot index outside its list.
- [ ] Regressions cover multiple PRs, one/multiple repositories, empty results,
      back-navigation, and reopening a picker.
- [ ] Verify: focused picker scenario tests. Depends on task 1.

## 3. Picker viewport behavior
- [ ] All picker screens keep the selected row visible as selection/size changes.
- [ ] Regressions cover long lists and narrow/short terminals with bounded output.
- [ ] Verify: picker layout tests. Depends on task 2.

## 4. Asynchronous picker storage reads
- [ ] Session/repository listing runs in commands instead of blocking `Update`.
- [ ] Loading, errors, cancellation, and quit retain the current review and safely
      finish workers before the store closes.
- [ ] Verify: deterministic async scenario tests and race detector. Depends on 2.

## 5. Consistent application wiring and offline enforcement
- [ ] Every entry path installs the same refresh/new/guide/browser operations.
- [ ] Offline mode rejects every network operation before creating clients or
      reading credentials, while saved sessions remain usable.
- [ ] Test both entry paths, cancellation, and offline operations with fakes;
      guide cancellation does not replace the current session.
- [ ] Verify: `go test -race ./cmd/pr-review ./internal/review`.

## 6. Reconcile the public contract
- [ ] README/constraints match accepted CLI flags, current keys, interactive guide
      consent, derived sessions, and offline restrictions.
- [ ] Document automated testing layers, fixture requirements, snapshot updates,
      and the remaining scope of human usability/accessibility assessment.
- [ ] Verify documented commands against CLI parser and testing scripts. Depends
      on 5 and final testing infrastructure.

## 7. Scenario and real-program testing infrastructure
- [ ] Add reusable, bounded helpers that execute `Init`, commands/messages, and
      complete navigation sequences without fixed sleeps.
- [ ] Run actual Bubble Tea programs for load and browser entry paths, quit,
      cancellation/failure/retry, and persistence where applicable.
- [ ] Keep existing domain/provider tests; add regressions rather than weaken
      assertions. Verify with the race detector. Depends on 1–5.

## 8. Deterministic screen snapshots
- [ ] Check in small representative screens for review, pickers, narrow layout,
      loading/error states, and guides using fixed dimensions and fixture data.
- [ ] Provide an explicit baseline-update command; normal tests only compare.
- [ ] Verify failure output is actionable and selected-row/width assertions remain
      behavioral tests independent of the baselines. Depends on 1–4.

## 9. Compiled-binary PTY smoke tests
- [ ] Exercise actual startup, keyboard input, resize, Ctrl+C/quit, and terminal
      restoration on macOS/Linux with bounded waits and diagnostics.
- [ ] Exercise saved-session marking and restart/resume using temporary stores;
      fixture GitHub/provider behavior must never use live credentials/network.
- [ ] Verify both entry paths and run the suite locally. Depends on 1–5 for green.

## 10. Verification command and CI
- [ ] Add one checked-in command for formatting checks, vet, race-enabled Go
      tests, build, and PTY smoke tests; fail visibly on missing prerequisites.
- [ ] Add Linux/macOS CI using the module's Go version and synthetic fixtures.
- [ ] Document the per-feature requirement for a successful journey plus relevant
      error/cancellation regressions. Depends on 7–9.

## Completion checkpoint
- [ ] Review integrated changes for architecture, correctness, and test robustness.
- [ ] Run the full verification command and record results in this checklist.
- [ ] Commit all completed work in coherent slices; preserve unrelated files.
