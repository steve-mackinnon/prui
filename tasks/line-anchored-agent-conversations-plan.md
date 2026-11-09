# Implementation plan: line-anchored agent conversations

## Scope and decisions

Implement the approved first pass in `docs/spec/SPEC-line-anchored-agent-conversations.md`. Conversations and selections belong to a stable review tab and its frozen canonical comparison; no transcript persistence or repository search. The configured, confirmed guide selection supplies the recipient. Each Send makes one bounded non-streaming request. Captured context is the only optional evidence. Use existing raw-target validation and privacy policy.

The spec file was copied from an untracked sibling worktree into this implementation worktree. Its proposed wording is superseded by the maintainer's explicit approval in this task. Keep `tasks/plan.md` and `tasks/todo.md` intact because they contain other work.

## Dependency graph

1. Immutable request/answer contract and provider transport can proceed independently of selection.
2. Canonical range interaction and explicit copy can proceed independently of request/provider work.
3. Conversation feature UI depends on the contract, transport interface, and selection.
4. Coordinator integration, documentation, automated verification, terminal journey, and independent review follow all slices.

## Task ownership

Each implementation task has one dedicated subagent. Task 1 owns `internal/agentchat/**`; Task 2 owns `internal/guide/agentchat_*`, `internal/guideconfig` only if needed, and `cmd/prui/agentchat_*`; Task 3 owns selection-related `internal/tui` files, agreed with Task 4 before dispatch; Task 4 owns conversation-related `internal/tui` files. Primary agent owns the cross-task wiring, integration checks, documentation and checklist. Agents may add focused tests beside owned code. No simultaneous edits to the same file.

## Risks

- Existing TUI selection and mouse routing overlap comment ranges and release-to-copy. Resolve canonical source hit testing before switching mouse behavior and preserve non-source copy routes.
- Provider APIs differ. Preserve existing transport limits, redirect refusal and sanitized errors; do not retry a rejected schema.
- Async tab state currently uses a broad root model. Capture stable tab/comparison/thread/generation identity at dispatch and route completions to origin without active-tab lookup.
- Human terminal mouse and Shift modifier delivery depend on terminal support. Record actual terminal evidence separately from synthetic tests.

## Integration decisions

- `Alt+Y` is the explicit source Copy shortcut. Enter on a canonical diff
  target opens the shared editor; one Tab selects Ask agent. `X` cancels a
  pending request, `Q` toggles a focused Q&A card, and Enter on a citation
  inspects the exact sent evidence. Existing `Ctrl+V` range behavior remains.
- Local selection checks source coordinates and the frozen row. Outbound comment
  and agent operations retain their full identity and revision validation; this
  preserves legacy comment-composer fixtures with abbreviated synthetic SHAs.
- Reviewer findings were resolved as source-boundary and interaction fixes.
  Conversation state, card assembly, and question/evidence lines now live in
  `internal/tui/features/agentchat`; the root TUI adapts them to terminal
  styling, input, and viewport behavior.

## Checkpoints

- Foundation: contract and provider focused tests pass; source selection tests pass.
- Feature: a fake provider journey proves Send, follow-up, citations, cancellation, and tab isolation.
- Complete: requested Go package tests, build, `scripts/verify.sh`, lint, diff check, feasible terminal checks, and independent spec review pass. Fix actionable review findings.

## Files scroll benchmark evidence

Baseline, unchanged sibling checkout at `44604f5`, Apple M2 Max, Go 1.26.10:
`go test ./internal/tui -run '^$' -bench BenchmarkFilesScrollTransition -benchmem -count=3`.
Width 120, no boundary crossing: 1.623–1.634 ms/op, 129.9 KB/op,
1,745 allocs/op; crossing: 1.621–1.637 ms/op, 129.8–130.0 KB/op,
1,739 allocs/op. Width 180, no crossing: 1.717–1.729 ms/op,
184.5 KB/op, 2,224 allocs/op; crossing: 1.717–1.824 ms/op,
181.9–182.0 KB/op, 2,193 allocs/op. Integrated measurement used the same
host and toolchain after owner extraction.

Integrated uncontended rerun: width 120/no crossing 1.692–1.705 ms/op,
131.2–131.3 KB/op, 1,745 allocs/op; crossing 1.665–1.689 ms/op,
131.0–131.2 KB/op, 1,739 allocs/op. Width 180/no crossing
1.776–1.783 ms/op, 187.1 KB/op, 2,224 allocs/op; crossing
1.758–1.773 ms/op, 184.5–184.6 KB/op, 2,193 allocs/op. The earlier
concurrent run was noisier. Allocation counts are unchanged; per-operation
bytes rose about 1.2–2.7 KB and timings about 2–5% on this host. This is
measured overhead, with no portable performance target or claimed speedup.
