# Issue #27 implementation status

Branch: `codex/issue-27-conversation`. Scope: complete live PR conversation only;
no draft persistence changes, no roadmap project updates, no merge/auto-merge.

- Source slice: `311cd7e` adds bounded general-comment/submitted-review retrieval.
- First complete implementation: `e8693c3d0d1577e84c895584b4eb70d1eb317a7c`.
  `scripts/verify.sh` passed: formatting, `go vet ./...`,
  `go test -race -count=1 -timeout=5m ./...`, `go build ./...`.
- Independent reviewer (fresh `independent_review` subagent, not an implementer)
  requested one P2 fix on that SHA: partial refresh discarded earlier general
  activity and selection. Corrected by retaining omitted general/review events
  with stale labels and preserving selected inline context without current anchors.
- Reviewer ran focused source/TUI/application suites successfully and an isolated
  mutation inverting freshness reconciliation failed the intended regression test.
- Follow-up synthetic tests pass for partial retention/deduplication, editor mouse
  ownership/viewport bounds, immutable attempted-body reconciliation after editing,
  byte limits, malformed responses, JSON stdin, offline refusal and stale preflight.
- Final PR description records final tested/reviewed head SHA, full gate outcome
  and final independent verdict. The PR remains draft until both are established.

Limitations: general replies are explicit @mention comments because GitHub offers
no nested general-comment reply endpoint. Uncertain-write matching is conservative
without a client idempotency key. Retrieval can be partial under documented bounds.
Human terminal usability/accessibility and Linux runtime verification are not
established by local macOS tests. No synthetic test contacts GitHub or a provider.
