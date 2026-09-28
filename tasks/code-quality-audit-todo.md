# Code quality audit follow-up

Audit baseline: `b80df45`. Requested 2026-09-28. Each task ships as its own
commit, with focused verification and a final integrated quality gate.
Existing task plans remain unchanged. Sol subagents own implementation in
isolated worktrees; the parent agent reviews and integrates their commits.

## Q-01: Preserve text when inserting inside editor drafts

- [ ] Implement one safe rune insertion helper for inline comments, replies,
  and review summaries, including newline insertion.
- [ ] Cover beginning, middle, and end insertion with ASCII, Unicode, and
  multiline text; verify suffix preservation and cursor movement.
- [ ] Run focused editor tests and commit the fix separately.

Evidence: inserting X into abcd at cursor 1 currently produces aXXcd.
Scope: editor code and focused tests. No dependency changes.
Dependencies: none.

## Q-02: Restore meaningful layout verification

- [ ] Diagnose all six failing TUI tests and the PTY smoke failure against the
  current framed layout, preserving behavioral assertions.
- [ ] Repair stale border, viewport, text, and divider assumptions; fix any
  demonstrated implementation defect instead of weakening its test.
- [ ] Pass the complete TUI suite and compiled PTY smoke; commit separately.

Scope: affected test fixtures/assertions and only proven layout defects.
Dependencies: none; coordinate any production changes with the parent.

## Q-03: Cache immutable Files-mode diff assembly

- [ ] Reuse immutable diff assembly across Files-mode navigation and rendering,
  including side-by-side projection, with explicit session invalidation.
- [ ] Keep comment overlays and drafts current and protect cached source from
  mutation; preserve selection, scrolling, and comment targets.
- [ ] Add a Files-mode allocation regression test and benchmark; demonstrate
  lower allocations against the 50-hunk audit baseline (~44,872 allocs/op,
  7.5 MB/op), then commit separately.

Scope: diff assembly/cache and focused regression tests.
Dependencies: none; isolated worktree required because Q-04 overlaps.

## Q-04: Make tab state canonical

- [ ] Replace duplicated Model/reviewTabState fields and manual save/restore
  copying with one canonical tab-owned state; workspace services/dimensions
  remain workspace-owned.
- [ ] Preserve cursor anchors, drafts, navigation, tab-local preferences, and
  asynchronous result routing, including inactive tabs.
- [ ] Add meaningful ownership/routing regressions, update architecture docs,
  run focused/race tests, and commit separately.

Dependencies: Q-01, Q-02, Q-03 integrated before implementation.
Scope: TUI state and result handlers; no public behavior/storage changes.

## Integration checkpoint

- [ ] Review all four diffs for correctness, simplicity, architecture, security,
  and performance; resolve concrete issues.
- [ ] Run ./scripts/verify.sh, configured static analysis, and git diff --check.
- [ ] Record commit IDs, verification results, and any tool/version limitations.
