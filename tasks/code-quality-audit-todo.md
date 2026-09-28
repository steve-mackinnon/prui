# Code quality audit follow-up

Audit baseline: `b80df45`. Requested 2026-09-28. Each task ships as its own
commit, with focused verification and a final integrated quality gate.
Existing task plans remain unchanged. Sol subagents own implementation in
isolated worktrees; the parent agent reviews and integrates their commits.

## Q-01: Preserve text when inserting inside editor drafts

- [x] Implement one safe rune insertion helper for inline comments, replies,
  and review summaries, including newline insertion.
- [x] Cover beginning, middle, and end insertion with ASCII, Unicode, and
  multiline text; verify suffix preservation and cursor movement.
- [x] Run focused editor tests and commit the fix separately.

Evidence: inserting X into abcd at cursor 1 currently produces aXXcd.
Scope: editor code and focused tests. No dependency changes.
Dependencies: none.

## Q-02: Restore meaningful layout verification

- [x] Diagnose all six failing TUI tests and the PTY smoke failure against the
  current framed layout, preserving behavioral assertions.
- [x] Repair stale border, viewport, text, and divider assumptions; fix any
  demonstrated implementation defect instead of weakening its test.
- [x] Pass the complete TUI suite and compiled PTY smoke; commit separately.

Scope: affected test fixtures/assertions and only proven layout defects.
Dependencies: none; coordinate any production changes with the parent.

## Q-03: Cache immutable Files-mode diff assembly

- [x] Reuse immutable diff assembly across Files-mode navigation and rendering,
  including side-by-side projection, with explicit session invalidation.
- [x] Keep comment overlays and drafts current and protect cached source from
  mutation; preserve selection, scrolling, and comment targets.
- [x] Add a Files-mode allocation regression test and benchmark; demonstrate
  lower allocations against the 50-hunk audit baseline (~44,872 allocs/op,
  7.5 MB/op), then commit separately.

Scope: diff assembly/cache and focused regression tests.
Dependencies: none; isolated worktree required because Q-04 overlaps.

## Q-04: Make tab state canonical

- [x] Replace duplicated Model/reviewTabState fields and manual save/restore
  copying with one canonical tab-owned state; workspace services/dimensions
  remain workspace-owned.
- [x] Preserve cursor anchors, drafts, navigation, tab-local preferences, and
  asynchronous result routing, including inactive tabs.
- [x] Add meaningful ownership/routing regressions, update architecture docs,
  run focused/race tests, and commit separately.

Dependencies: Q-01, Q-02, Q-03 integrated before implementation.
Scope: TUI state and result handlers; no public behavior/storage changes.

## Integration checkpoint

- [x] Review all four diffs for correctness, simplicity, architecture, security,
  and performance; resolve concrete issues.
- [x] Run ./scripts/verify.sh, configured static analysis, and git diff --check.
- [x] Record commit IDs, verification results, and any tool/version limitations.


## Completion record

All four tasks were implemented by separate GPT-6 Sol subagents, reviewed,
and integrated as individual commits:

| Task | Commit | Result |
| --- | --- | --- |
| Q-01 | `7c60dbc` | Shared safe insertion across all three editors; suffix and rune-cursor regressions. |
| Q-02 | `ec36365` | Six TUI assertions and the PTY divider check corrected for framed geometry without removing coverage. |
| Q-03 | `2254f5b` | One-session immutable Files diff cache, lazy split projection, live overlays, and allocation budgets. |
| Q-04 | `85ba6b1` | Canonical tab state, direct result routing, inactive viewer completion, and cursor/draft ownership regressions. |

Final verification on macOS/arm64 with Go 1.26.8:

- `./scripts/verify.sh` passed: formatting, vet, all race-enabled tests,
  compiled PTY tests, and build.
- Exact CI-pinned `golangci-lint` v2.13.2 (`go run ...@v2.13.2 run ./...`)
  reported zero issues.
- `git diff --check` passed.
- Synthetic unified Files scrolling benchmark (50 hunks): approximately
  44,867 to 800 allocations/op, 7.52 to 0.98 MB/op, and 4.28 to 0.67 ms/op.
  Timings are local measurements, not production guarantees; allocation tests
  protect both unified and split views.

No dependency changes, live GitHub writes, provider uploads, or Linux runtime
verification were part of this work.
