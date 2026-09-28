# Spec: PR-List Guide Loading

## Objective

When a reviewer selects a PR through the interactive PR list or switcher, open
the pinned local comparison and automatically resolve its guide. First reuse a
valid local guide cache entry; otherwise generate from the same already-pinned,
bounded material and persist the successful result. While work runs, the TUI
must clearly state whether it is opening, reusing, or generating a guide and
allow cancellation.

This changes only interactive PR-list selection. Direct `open`, `resume`,
`--plain`, offline mode, and `verify` retain their current non-generating
behavior. The explicit `g` key remains available as a manual retry action.

## Tech Stack

Go 1.26.8 and Bubble Tea v2. Reuse typed lifecycle messages in `internal/tui`,
the current source-pinning path, `guide.Analyzer`, and the new `guide-cache`
module. No dependency is added.

## Commands

```sh
go test ./internal/tui ./cmd/pr-review -count=1
go test -race ./internal/tui ./cmd/pr-review -count=1
go vet ./...
go test -race -count=1 ./...
go build ./...
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
cmd/pr-review/lifecycle.go         open, cache-or-generate orchestration
cmd/pr-review/wiring.go            lifecycle wiring and analyzer creation
cmd/pr-review/*_test.go            command lifecycle tests
internal/tui/{model,lifecycle}.go  tab-aware async guide resolution/loading
internal/tui/*_test.go             selection, cancellation, and late-result tests
internal/tui/{render,bindings}.go  loading copy and manual-retry wording
README.md                          changed guide-upload and PR-list behavior
CONSTRAINTS.md                     approved exception to opt-in-only generation
```

## Code Style

Associate each asynchronous result with the initiating PR/tab and never infer
its destination from whichever tab is active when it arrives:

```go
type PullRequestGuideResult struct {
    Target   int
    Identity source.Identity
    Session  *review.Session
    CacheHit bool
    Err      error
}
```

Use the established `beginAction`, `finishAction`, and cancellation path. A
guide failure yields a readable raw session with a stated unavailable guide;
it must not discard the pinned comparison or corrupt another tab.

## Testing Strategy

- TUI scenarios prove that a PR-list selection renders a loading notice while
  guide resolution is blocked and associates a late completion with its source
  tab.
- Command lifecycle tests prove cache hit skips analyzer creation/provider work,
  cache miss generates once and persists, and a reopen after a new process uses
  the cache.
- Test cancellation before and during generation: raw session is retained,
  no successful artifact is written, and PR browser/review tabs remain usable.
- Test provider failure: raw review opens with the unavailable status; reopening
  retries rather than treating it as a cache hit.
- Preserve existing synthetic-only, local-HTTP provider fixtures and plain
  output/terminal safety checks.

## Boundaries

- Always: pin and build the raw comparison before cache lookup or guide use;
  preserve raw inventory as the complete review surface.
- Always: show text-based running state and permit Escape cancellation.
- Always: retain async tab ownership and escape all visible provider text.
- Ask first: extend auto-generation to direct open/resume, background prefetch,
  change privacy limits, add configuration, or alter CI.
- Never: generate in plain/offline/verifier paths; execute reviewed code; send
  unpinned source; or store API keys/provider transcripts.

## Success Criteria

- Selecting a PR from the interactive list automatically resolves a guide.
- A visible, cancellable loading state appears during generation.
- A cache hit opens the guided review without an analyzer/provider call.
- A cache miss generates once, stores only a successful guide, and displays it.
- Reopening the identical immutable comparison after restart reuses the guide.
- A changed base or head regenerates; an unavailable result retries next time.
- Existing non-PR-list entry points remain non-generating.

## Open Questions

None. The user approved automatic guide generation only for PR-list selection
and successful-result-only caching.
