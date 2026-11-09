# Line-anchored agent conversations checklist

## Task 1: Bounded manifest and answer contract

- [x] Build a frozen manifest from a canonical validated anchor, selected hunk and at most three ranked, deduplicated captured context excerpts. Reapply privacy and omission rules; block excluded or oversized selected source.
- [x] Enforce question, source, history, serialized request and answer limits with UTF-8-safe boundaries. Include deterministic omission ledger and only complete latest history turns that fit.
- [x] Validate structured answer references against supplied evidence IDs and subranges; expose exact sent excerpt for inspection.

**Owner:** dedicated contract subagent. **Dependencies:** none. **Files:** `internal/agentchat/**`. **Verification:** `go test ./internal/agentchat -count=1`; `go test ./internal/context ./internal/privacy -count=1`.

## Task 2: Configured guide-recipient answer service

- [x] Reuse the current confirmed guide selection and existing protected Fantasy/provider transport for one non-streaming structured answer call per Send.
- [x] Enforce 60-second deadline, response bound, no automatic retry or fallback, sanitized failures, and fake credential/local endpoint tests. Keep credential lookup at Send and offline refusal before lookup.
- [x] Expose a narrow service for the TUI and wire it from `cmd/prui`, with recipient changes applying to new threads only.

**Owner:** dedicated provider subagent. **Dependencies:** Task 1 answer contract. **Files:** `internal/guide/agentchat_*`, `cmd/prui/agentchat_*`, narrowly required `cmd/prui/wiring.go`, and tests. **Verification:** `go test ./internal/guide ./internal/guideconfig ./cmd/prui -count=1`.

## Task 3: Canonical source selection and explicit copy

- [x] Make Ctrl+V, cursor movement, click/Shift-click, and forward/reverse viewport drag yield the same canonical inclusive range; reject boundary crossings without losing valid selection.
- [x] Highlight source cells with side/range label across Files/Guide, split/unified, wrapping, scroll and resize. Cancel unfinished drags on modal/tab/view/keyboard transitions.
- [x] Route canonical source drag to selection with no release-to-copy; add explicit selected-source Copy action. Preserve prose copy and divider priority.

**Owner:** dedicated selection subagent. **Dependencies:** none. **Files:** `internal/tui/comment_targets.go`, `internal/tui/text_selection.go`, `internal/tui/mouse.go`, `internal/tui/mouse_review.go`, selection helpers/tests, narrowly required actions/help/render files. **Verification:** `go test ./internal/tui -run 'Test.*(Range|Selection|Mouse|Copy)' -count=1`; `go test ./internal/tui -count=1`.

## Task 4: Inline conversation feature

- [x] Open or reopen a thread by normalized anchor in Files/Guide; retain independent per-anchor drafts, answers and bounded history. The shared Ask agent editor shows the model ID; each Enter Send authorizes the bounded request documented in the reference.
- [x] Show pending, Cancel, retry and follow-up; dispatch only on explicit Send. Route by stable tab/comparison/thread/generation and discard late or replaced results.
- [x] Render collapsible escaped/Markdown Agent Q&A below ending row, citation inspection of exact sent evidence, unavailable references, and stable review position.

**Owner:** dedicated conversation subagent. **Dependencies:** Tasks 1–3 contracts and source changes. **Files:** `internal/tui/agentchat_*` and narrowly required model/actions/render files, plus tests. **Verification:** `go test ./internal/tui -run 'Test.*(Agent|Conversation)' -count=1`; `go test ./internal/tui -count=1`.

## Integration and acceptance

- [x] Primary agent resolves interfaces and all cross-task regressions; records consequential decisions in the plan and updates `CONSTRAINTS.md`, `docs/REFERENCE.md`, and `TESTING.md`.
- [x] Automated acceptance covers the listed behavior, including no pre-Send/offline request, privacy, malformed citations, cancellation, comparison/tab isolation, unchanged guide/progress/GitHub writes.
- [x] Run `go test ./internal/tui ./internal/context ./internal/privacy ./internal/guide ./internal/guideconfig ./cmd/prui`, `go test ./internal/agentchat`, `go build ./...`, `sh scripts/verify.sh`, `golangci-lint run ./...`, `go run ./cmd/prui sessions`, and `git diff --check`.
- [x] Perform feasible terminal verification: compiled PTY exercises source drag, forwarded SGR Shift-click, zero copy on release, explicit Alt+Y OSC 52 copy, and review navigation; human terminal judgment remains pending.
- [x] Separate review subagent checks integrated diff against the spec; actionable findings fixed and re-review found no remaining concrete defect.

## Outstanding external acceptance

- [ ] A human terminal review with an actual guided PR records hunk-only,
  captured-test-evidence, and missing-caller questions; answer readability,
  scrolling/narrow layout, real mouse/Shift delivery, clipboard acceptance,
  cancellation, and resuming review. This needs a reviewer terminal and a
  configured provider key; no key should be shared in this checklist. Record
  observations in `tasks/line-anchored-agent-conversations-manual-qa.md`.
