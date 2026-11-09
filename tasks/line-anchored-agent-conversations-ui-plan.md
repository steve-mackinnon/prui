# UI refinement plan: inline ask agent mode

Approved by the maintainer on 2026-11-05: Enter on a canonical diff target opens the existing inline editor; Tab cycles Comment, Ask agent, Suggestion (when eligible), and File comment. Ask agent is second. Enter in Ask agent sends one private request; Shift+Enter adds a newline. A later readability refinement keeps only the selected model ID at the top right of the editor box and removes its extra scope and instruction rows. The bounded request, tab lifetime, and privacy behavior remain unchanged. No repository search, transcript persistence, GitHub submission, or provider behavior changes.

## Tasks and ownership

1. **Composer mode integration** — dedicated implementation subagent owns `internal/tui/comment_modes.go`, `internal/tui/lifecycle.go`, `internal/tui/draft_editor.go`, and focused tests. It adds Ask agent as the second Tab mode, keeps its draft out of persisted GitHub comment modes, routes Enter to the agent service and never to GitHub, and preserves comment/suggestion/file behavior. It coordinates narrow hooks with Task 2 before editing shared files. Verification: `go test ./internal/tui -run 'Test.*(CommentType|Agent.*Composer|CommentComposer)' -count=1`.
2. **Agent inline presentation** — dedicated implementation subagent owns `internal/tui/agentchat_ui.go`, `internal/tui/features/agentchat/**`, `internal/tui/comment_surface.go` if a shared frame primitive is needed, and focused tests. It provides composer activation/send hooks and renders Q&A with the same framed inline card language as published comments, clear question/answer/citation grouping, wrapping, pending/error states, and preserved citation navigation. Verification: `go test ./internal/tui -run 'Test.*(Agent|CommentCard)' -count=1`; `go test ./internal/tui/features/agentchat -count=1`.
3. **Coordinator integration** — primary agent owns `internal/tui/model.go`, `internal/tui/actions.go`, `internal/tui/bindings.go`, `internal/tui/diff_overlay_cache.go`, `docs/REFERENCE.md`, `TESTING.md`, this plan/checklist, and golden/PTY updates. It replaces the separate Ask action/modal entry with the inline mode, checks all routes and terminal widths, runs required gates, and resolves cross-task failures. Verification: affected Go package tests, `go build ./...`, `sh scripts/verify.sh`, lint, and `git diff --check`.
4. **Integrated review** — separate review subagent checks the combined interaction against this approval and the original spec. Primary fixes actionable findings.

## Invariants and acceptance

- Comment remains the default. One Tab selects Ask agent when available; Shift+Tab cycles backward. The selector visibly marks the active mode.
- Agent drafts and answers remain in the review tab's memory only. Cycling modes, Escape, and failed Send retain the agent draft; comment/suggestion/file drafts retain their existing storage behavior.
- Agent mode can never invoke comment submission, pending-draft persistence, or GitHub write preflight. Each Enter Send retains the original privacy, bounded manifest, one-request, offline, cancellation, and tab/comparison guards.
- Inline Q&A uses readable framed cards and wraps safely at narrow widths, with citation rows still inspectable and pending/cancel/retry/follow-up visible. Viewport and guide position stay stable.
- No new repository search or model-directed tools.
