# Conversation Markdown implementation

Spec: [Readable conversation Markdown and HTML](../docs/spec/SPEC-conversation-markdown.md).
The user requested specification followed by implementation on 2026-10-05.

Starting source: the full working tree beside `/Users/steve/.codex/worktrees/512a/review/prui`,
including its uncommitted Overview work and draft conversation-rendering tests.
Implementation is isolated in `/Users/steve/.codex/worktrees/85e0/review`.

- [x] Specify display semantics, expansion, sanitization and raw-body boundaries.
- [x] Confirm starting tests fail for the missing renderer.
- [x] Convert HTML outside code and render readable Markdown with concise links.
- [x] Integrate Overview/detail and bound the tab-owned render cache.
- [x] Verify nested/malformed details, HTML tables, references and controls.
- [x] Verify code literals, Unicode wrapping, resize/theme and semantic row IDs.
- [x] Review wide, narrow and expanded Vercel screens and update documentation.
- [x] Finish full repository verification, mutation check and runnable build.

Focused checks passed:

```sh
go test ./internal/tui -run 'ConversationMarkdown|OverviewRenders|OverviewConversationMarkdown|DescriptionMarkdown|ProgramCommitDiscussionReadFallbackJumpReturn|DiscussionsUnplaceable|OverviewThreadFallback|ScreenSnapshots' -count=1 -args -update-golden
```

Screen changes retain code context, authors, source-navigation links and freshness
labels. Bodies are indented and preserve internal paragraph spacing; outer
document margins are removed because cards already provide their own spacing.
The legacy detail assertion strips trusted ANSI before checking prose; the
program-return assertion checks the activity list rather than an offscreen
description heading. No tests or security checks are disabled.

Live GitHub and subjective human terminal QA remain unverified. Tests use only
synthetic comments and local fixtures; no remote images or GitHub writes occur.

Review: inverting the collapse visibility condition made all nested/malformed
details regression cases fail; the source was restored automatically. HTML
preformatted code uses a fence longer than any backtick run in the input, and
nested detail visibility uses a counter rather than scanning the stack per token.
The inherited CLI PTY fixture expected the old Description tab label; its three
expectations now match Overview without changing its journeys or assertions.

Final verification: `./scripts/verify.sh` passed formatting, vet, the full
race-enabled suite including PTY smoke, and build. `go build -o prui ./cmd/prui`
produced the executable in this worktree. `git diff --check` passed.
