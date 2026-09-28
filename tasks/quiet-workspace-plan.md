# Quiet workspace (option A)

Approved for implementation on 2026-09-25. This plan is separate from the
unfinished historical plans in plan.md and todo.md.

## Design contract

Make source content dominant through a persistent PR identity, understated
context tabs, one quiet pane divider, compact PR browsing, explicit selection,
and a small status/shortcut area. Retain current keys, comment posting versus
pending behavior, review state restoration, source/guide lifecycle, immutable
comment targets, custom themes, and plain output. No new dependencies.

Keep the existing 100-column two-pane threshold and 160-column side-by-side
threshold. Budget header/footer rows centrally; narrow/short screens prioritize
identity, content, severe health states, pending count and Review. Color remains
supplemental; all untrusted text is escaped. High-contrast remains supported.

This supersedes the visual arrangement in docs/spec/SPEC-tui-review-layout.md, including
its single-header and single persistent shortcut choices, only for the approved
option A. It does not authorize the unrelated unfinished PR-state redesign.

## Execution and ownership

Wave 1: workspace_layout owns model.go and workspace helpers; quiet_styling owns
style.go and theme palettes; compact_picker owns picker rendering and viewport.
Root owns chrome.go, bindings.go, documentation, snapshots and integration.
Wave 2: after workspace ownership is released, finish status integration and
file/guide hierarchy polish. Wave 3: inspect snapshots, run full gates and review.
Shared files have one writer at a time; agents request cross-boundary changes.

Detailed acceptance criteria and progress: quiet-workspace-todo.md.

## Verification

Focused behavior tests per task, then ./scripts/verify.sh, golangci-lint run ./...
and git diff --check. Inspect deterministic renders at widths 60, 99, 100, 120,
159 and 160, including short heights and long paths/titles. Exercise selection,
resize, description scrolling, comments, pending submission, switching, and
colorless rendering with synthetic fixtures only. Human terminal usability
acceptance remains distinct from automated success.

## Risks

- Added chrome can obscure content: explicit height budgeting and small-screen tests.
- Changed dividers can shift cursor targets: preserve source provenance and test mappings.
- Compact health may hide warnings: severity-first fitting and recoverable detail in help.
- Concurrent baseline updates can hide regressions: root alone reviews and updates snapshots.
- Existing exact-layout tests need intentional updates, never weakened behavioral assertions.
