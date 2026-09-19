# Implementation Plan: Automatic PR-List Guides

## Overview

Add a successful-guide cache for immutable PR comparisons, then use it from
interactive PR-list selection to either reuse a guide or generate one with
visible, cancellable progress. Existing raw review opening remains the source
of truth and stays available if analysis fails or is canceled.

## Architecture Decisions

- Cache by repository, PR number, base SHA, and head SHA. The pair of commits
  defines the diff; a head-only cache can return guides for stale review units.
- Store only validated `generated` bundles. Unavailable results remain in their
  derived sessions but never block a later retry.
- Keep provider behavior unchanged outside interactive PR-list selection.
- Make the guide-resolution result tab-aware, matching existing PR-open result
  safety for late asynchronous messages.
- Preserve existing immutable source sessions and derived guided sessions;
  the cache is an additional reusable artifact, not a session rewrite.

## Dependency Graph

```text
typed comparison cache key + durable validated artifact
                         |
                         v
          cache-or-generate application orchestration
                         |
                         v
     tab-aware TUI loading, cancellation, and result application
                         |
                         v
         documentation and full synthetic verification
```

## Task List

### Phase 1: Cache foundation

- [x] Task 1: Add typed guide-cache identity and private, atomic generated-guide
  storage.
  - Acceptance: exact immutable-comparison lookup returns only a structurally
    valid generated bundle; changed base/head and invalid artifacts are misses.
  - Verify: `go test ./internal/session ./internal/guide -count=1`.
  - Depends on: approved `SPEC-guide-cache.md`.
  - Files: `internal/session/store.go`, `internal/session/store_test.go`.
  - Estimated scope: Small.

- [x] Task 2: Add application-level cache-or-generate orchestration for a
  pinned PR-list comparison.
  - Acceptance: cache hit skips analyzer creation; cache miss generates and
    writes only generated bundles; unavailable result remains retryable.
  - Verify: `go test ./cmd/pr-review -run 'Test.*Guide' -count=1`.
  - Depends on: Task 1.
  - Files: `cmd/pr-review/lifecycle.go`, `cmd/pr-review/wiring.go`,
    `cmd/pr-review/lifecycle_test.go`.
  - Estimated scope: Medium.

### Checkpoint: persisted guide path

- [x] Focused session, guide, and command lifecycle tests pass with a cache
  hit, cache miss, changed base/head, restart, and provider failure.

### Phase 2: Interactive PR-list workflow

- [x] Task 3: Add tab-aware guide-resolution lifecycle messages and a
  cancellable loading state after PR-list selection.
  - Acceptance: UI states whether it is generating or using a cached guide;
    Esc cancels safely; late results cannot replace another active tab.
  - Verify: `go test ./internal/tui -run 'Test.*(PullRequest|Guide|Loading)' -count=1`.
  - Depends on: Task 2.
  - Files: `internal/tui/model.go`, `internal/tui/lifecycle.go`,
    `internal/tui/{lifecycle,model}_test.go`.
  - Estimated scope: Medium.

- [x] Task 4: Update interactive wording, guide manual-retry copy, and public
  policy documentation.
  - Acceptance: README, key help, and constraints accurately limit automatic
    upload to interactive PR-list selection and explain cache reuse/retry.
  - Verify: focused TUI snapshots/tests and `git diff --check`.
  - Depends on: Task 3.
  - Files: `internal/tui/{bindings,render,model}.go`, relevant TUI tests and
    goldens, `README.md`, `CONSTRAINTS.md`.
  - Estimated scope: Medium.

### Checkpoint: user journey

- [x] Synthetic TUI journey proves PR list → visible generation → guided review,
  then reopen → cache hit with no generation; cancellation and a failed provider
  leave a usable raw review and allow a retry.

### Phase 3: Complete verification

- [x] Task 5: Run the full quality gate and inspect the final change for
  privacy, persistence, cancellation, and cross-tab regressions.
  - Acceptance: full test/build/verifier suite passes; no real provider,
    credential, GitHub, or reviewed-code execution occurs in tests.
  - Verify: `go vet ./... && go test -race -count=1 ./... && go build ./... && ./scripts/verify.sh && git diff --check`.
  - Depends on: Tasks 1–4.
  - Files: tests/documentation only if verification reveals a scoped defect.
  - Estimated scope: Small.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Head-only reuse returns a guide for a changed diff | High | Key by both base and head SHA, plus repository/PR identity. |
| A late result overwrites another tab | High | Carry initiating tab and identity in the typed result; scenario-test tab switching. |
| Cancellation loses the pinned raw review | High | Separate raw-session persistence from guide completion; assert raw session remains usable. |
| Failed response suppresses useful later work | Medium | Cache generated bundles only. |
| Automatic upload silently broadens existing consent | High | Limit to explicit interactive PR-list selection; update README and constraints; exclude CLI/plain/offline/verifier paths. |
| Cache artifact becomes corrupt or incompatible | Medium | Private atomic writes plus load-time structural/inventory validation and fail-closed miss behavior. |

## Open Questions

None. The approved scope excludes background prefetch, configuration flags,
direct-open/resume automation, cache UI/management commands, and changing
provider/prompt invalidation policy.
