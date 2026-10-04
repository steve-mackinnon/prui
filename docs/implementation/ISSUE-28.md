# Issue #28: published comment and review thread actions

Branch: `codex/issue-28-thread-actions`, stacked on reviewed parent PR #40,
`codex/issue-27-conversation` at `d1d89b5a67ea05c478a6c4211f91f9e09fa7997f`.
Merge #40 first, then rebase/retarget this PR to main and repeat verification.
No merge or auto-merge is performed by this implementation.

## Behavior and boundaries

- Inline comment actions offer `e` to edit and `z` to resolve/reopen; discussion
  detail offers the same actions for individual inline activity, including replies,
  outdated threads and threads without a placeable code anchor. General PR comment
  detail also offers editing. Enter explicitly confirms; Escape discards.
- Authenticated viewer identity gates editing. Submission re-reads the comment's
  membership/author or the thread's server viewer permissions, with frozen PR
  metadata checks before and after retrieval. Unknown/missing permissions deny.
- Comment PATCH responses preserve numeric identity and canonical author/body;
  thread GraphQL results preserve node identity and authoritative resolution and
  permissions. Code anchors, outdated status, timestamps and parent IDs are retained.
  General PR comments and review summaries remain distinct; review summary editing
  is outside this issue's published-comment scope.
- All mutation data uses JSON stdin, never process arguments. Offline refusal is
  before client/credential access. Edits, attempts and live results are memory-only.
- Failed writes retain editable text. Unknown outcomes preserve the immutable
  attempted body/desired resolution and block Enter until explicit refresh yields
  a complete verified result for that identity. A match blocks resubmission; a
  mismatch permits intentional retry. An absent identity remains uncertain.
- Generations invalidate reads begun before each attempt and before canonical
  success. Newer verified reads may show external edits/reopens/deletions. Partial
  refreshes merge only omitted identities into canonical threads, with individual
  stale labels and no current overlay placement; they do not claim complete counts. No automatic mutation retries.

## Official API evidence

Checked 2026-10-04 against GitHub's official documentation:

- [PullRequestReviewThread and resolve/unresolve mutations](https://docs.github.com/en/graphql/reference/pulls):
  `viewerCanResolve`, `viewerCanUnresolve`, `resolveReviewThread`,
  `unresolveReviewThread` and `threadId` are supported. Permission is not inferred
  from outdated status or repository roles.
- [Update review comment](https://docs.github.com/en/rest/pulls/comments#update-a-review-comment-for-a-pull-request):
  PATCH `repos/{owner}/{repo}/pulls/comments/{comment_id}`; Pull requests write.
- [Update issue/PR comment](https://docs.github.com/en/rest/issues/comments#update-an-issue-comment):
  PATCH `repos/{owner}/{repo}/issues/comments/{comment_id}`; Issues write or
  Pull requests write. Server authorization remains authoritative at the write.

## Verification and review

Focused synthetic source/TUI/application tests cover identity-preserving JSON
stdin mutations, canonical thread permissions, offline refusal, ownership and
membership rejection, moved PR pins, both entry points, unplaceable/outdated
threads, failure retention, immutable unknown attempts, stale/partial refresh,
external edits/reopens and confirmed identity retention. Source tests first failed
on the missing mutation methods before their implementation.

Final tested implementation SHA: `b6271ac7d1e8e652c4082b0fe4fb6f5d348ee68d`.
The final documentation-only PR head SHA is recorded in PR #44's evidence; this
file cannot contain the hash of its own containing commit.

Passed on local macOS for that implementation:

- `go vet ./...`.
- `go test -race -count=1 -timeout=5m ./...` (all packages, including compiled
  binary PTY tests and the real Bubble Tea published-edit journey).
- `go build ./...`.
- `GOFLAGS=-p=2 ./scripts/verify.sh` (formatting plus all three commands above).
  Reduced build parallelism limited temporary disk use; no checks were weakened.
- `GOFLAGS=-p=2 golangci-lint run ./...`: 0 issues.
- `git diff --check`.

The initial full gate exhausted disk space while another implementation was
building. Removed only this chat's duplicate temporary cache, switched to the
existing cache and serialized full gates. The next complete test run identified
four intentional discussion-footer baseline changes. Updated only those four
baselines, inspected their diffs, and passed the final full gate after correction.

Independent fresh reviewer `independent_review` did not implement the feature.
It requested one P1 fix at `be2570b7b0ae191957ccb3cb0df085b8d01a4a7e`:
partial refresh appended entire old threads per omitted edited comment, duplicating
threads and allowing stale retained siblings to seed an editor instead of the
newer canonical body. Disposable probes reproduced both cases. The fix in
`92ca871c51c49591f39ac7be073b89d25cdc3c75` merges only missing comment identities,
marks them retained individually, rejects retained edit/current-overlay targets,
and uses the same merge in composed selection/confirmed-creation refresh paths.
Permanent tests cover both failures, repeated refreshes and general-event dedup.
The reviewer approved that fix, then approved exact final implementation SHA
`b6271ac7d1e8e652c4082b0fe4fb6f5d348ee68d` after reviewing the footer/contract.

A preliminary coordinator finding that exact-value convergence could hide later
external edits/reopens was also resolved: pre-write reads are invalidated and
newer verified reads remain authoritative. Regression tests cover external edits
and reopens after success, and omitted identities during partial refresh.

Reviewer's isolated mutation changing the complete/verified reconciliation guard
from OR to AND failed the intended partial-refresh regression; original source
was restored immediately. No required review findings remain. No synthetic test
contacts GitHub or a model provider. Final PR metadata records its exact head,
final documentation re-review and CI outcomes separately.

Limitations: unknown outcomes cannot be made transactional with other clients;
a complete verified read can establish the current remote state but cannot prove
which writer caused it. Local synthetic tests do not establish human terminal
usability/accessibility or Linux runtime behavior.
