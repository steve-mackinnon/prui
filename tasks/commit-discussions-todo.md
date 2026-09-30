# Commit Discussions Tasks

Spec: [commit discussions](../docs/spec/SPEC-pr-commit-discussions.md).
Plan: [implementation plan](commit-discussions-plan.md).
Read-side implementation, supported head-commit posting, and synthetic
verification are complete. Historical RIGHT addition/context delivery is now
verified on disposable PR #19; LEFT/rename/copy/root/merge cases remain gated.
Use focused failing tests for any subsequent behavioral changes. Listed paths include tests; split a task if
its final scope exceeds roughly five files. The user authorized local implementation with sub-agents on 2026-09-30.
The user authorized disposable PR #19 for testing on 2026-09-30.

## CD-01: Establish GitHub interoperability evidence

- [x] Verify official GraphQL field names/nullability for threads, original
  commits/anchors, replies, numeric IDs, statuses and both pagination levels.
- [x] Record first-parent targeting support for additions/deletions/context,
  renames, root/merge commits and lines changed again; distinguish documented,
  synthetic and live evidence. Explicitly mark unsupported/unverified cases.
- [x] Check `D` key conflicts and existing safe browser-launch behavior; record
  decisions in the spec and an API fixture note. Resolve the gate before writes.

Verify: official sources plus reviewed synthetic request/response fixtures;
no credentials or real comments in automated tests. If live evidence is needed,
request authorization for a named disposable repository after preparing cases.
Dependencies: none. Scope: S/M.
Files: `docs/spec/SPEC-pr-commit-discussions.md`,
`docs/spec/commit-discussions-api-evidence.md`, source fixture files (new).

## CD-02: Normalize readable historical discussion records

- [x] Introduce typed thread/read-anchor/state records preserving root identity,
  original/current coordinates, replies, snippet and validated permalink.
- [x] Parse null anchors and immediately outdated write responses without rejecting
  valid readable comments; keep outbound target validation strict.
- [x] Preserve numeric action IDs and current-head action compatibility; malformed
  remote fields cannot become targets, unsafe URLs or terminal controls.

Verify: `go test ./internal/source -count=1`; fixtures cover independent
outdated/resolved state, missing anchors, multiline/file threads and POST success.
Dependencies: CD-01 read contract. Scope: M.
Files: `internal/source/discussions.go` (new), `internal/source/discussions_test.go`
(new), `internal/source/github.go`, `internal/source/comment_list_test.go`.

## CD-03: Fetch bounded threads and nested reply pages

- [x] Implement narrow GraphQL reader using stdin JSON, explicit thread/comment
  cursors and the spec's aggregate/deadline/material bounds.
- [x] Return completeness and safe partial/error states; distinguish complete
  empty results from unavailable data and incomplete threads.
- [x] Deduplicate by canonical identity, retain orphan/partial context honestly,
  and never drop a historical thread because it lacks a current line.

Verify: `go test ./internal/source -count=1`; fake runner checks exact requests,
multiple nested pages, cancellation, malformed pages, budget and permission failures.
Dependencies: CD-02. Scope: S/M.
Files: `internal/source/discussions.go`, `internal/source/discussions_test.go`,
source API fixture files (new, keep task within five files).

## CD-04: Expose a review-owned Discussions overlay

- [x] Wire the reader through online lifecycle with metadata verification,
  identity/generation-scoped results and atomic refresh publication.
- [x] Add `D` list/detail with root/reply bodies, statuses, snippets and safe
  permalink display; `c` refresh and Escape return preserve context state.
- [x] Preserve prior results with stale/error notices on failure; offline and
  replacement reviews cannot fetch or receive another review's late response.

Verify: `go test ./internal/tui ./cmd/prui -count=1`; fake program reads an outdated
unplaceable thread; lifecycle tests prove offline refusal and result isolation.
Dependencies: CD-03. Scope: M.
Files: `cmd/prui/lifecycle.go`, `internal/tui/lifecycle.go`,
`internal/tui/discussions.go` (new), `internal/tui/discussions_test.go` (new),
`internal/tui/model.go`. Extend existing test files in a separate increment if needed.

## Checkpoint A: Historical discussions are discoverable

- [x] Complete/partial/unavailable states and status labels are truthful.
- [x] Main-diff comments and existing reply/delete/reaction actions regressions pass.
- [x] No comment data enters sessions, guides, plain output or logs.

## CD-05: Derive commit diff anchors and cursor provenance

- [x] Derive optional targets from immutable selected-commit hunks using SHA,
  path/side and raw line counters; keep unsupported rows non-commentable.
- [x] Add commit-owned cursor state and semantic mouse targets without changing
  rail navigation, reading offsets, main-diff state or progress.
- [x] Cover rename/deletion/context/multiple hunks, root/merge eligibility,
  invalid paths and unavailable/partial/non-text diffs.

Verify: `go test ./internal/tui -count=1`; pure target assertions and keyboard/mouse
model tests include no-net-main-diff sessions and narrow focus restoration.
Dependencies: CD-01, CD-02. Scope: M.
Files: `internal/tui/commits.go`, `internal/tui/commits_test.go`,
`internal/tui/mouse.go`, `internal/tui/mouse_test.go`, `internal/tui/render.go`.
Confirm actual mouse file ownership before editing; use existing semantic routing.

## CD-06: Render shared discussions inline and count commit threads

- [x] Index roots by original SHA and exact anchors; show original-anchor cards
  in commits and freshness-qualified current-anchor cards in the main diff.
- [x] Count roots rather than replies, show outdated labels and incomplete counts,
  and keep unmatched records in Discussions without guessed placement.
- [x] Include overlay generation in rendering caches and cursor geometry; cards
  never become new source targets or corrupt progress/state.

Verify: `go test ./internal/tui -count=1`; render tests cover cross-commit same-line
collisions, replies, stale pins, refresh invalidation, no color and short screens.
Dependencies: CD-04, CD-05. Scope: M.
Files: `internal/tui/discussions.go`, `internal/tui/discussions_test.go`,
`internal/tui/commits.go`, `internal/tui/render.go`, `internal/tui/lifecycle.go`.

## CD-07: Navigate original context and integrate overlay controls

- [x] `View original commit` selects captured SHA and exact available line;
  missing capture retains thread detail with snippet/link and honest labels.
- [x] Return restores prior view/focus/selection/cursor/scroll; support keyboard
  and mouse controls and safe explicit permalink opening/display fallback.
- [x] Complete help and wide/narrow screens, including capped list, unknown
  status, partial replies, file/multiline threads and unavailable commit diffs.

Verify: `go test ./internal/tui -count=1`; reviewed screens at widths
60/99/100/120 and model tests prove no navigation network requests.
Dependencies: CD-06. Scope: M; split fixture-only updates if exceeding five files.
Files: `internal/tui/discussions.go`, `internal/tui/discussions_test.go`,
`internal/tui/help.go`, `internal/tui/mouse.go`, `internal/tui/testdata/screens/`.

## Checkpoint B: Commit viewing is complete

- [x] Every loaded thread is reachable, inline or via fallback.
- [x] Thread identity, counts, status and main/commit views agree after refresh.
- [x] Main review navigation, actions, progress and frozen source are unchanged.

## CD-08: Validate historical submission at the application boundary

- [x] Introduce explicit main-diff versus commit target provenance; historical
  requests must match active immutable commit membership and patch coordinates.
- [x] Retain full current-metadata freshness preflight, offline refusal and
  one-shot REST submission; unsupported cases cannot write.
- [x] Keep head-based PR-review batch validation strict; historical drafts cannot
  be smuggled into it through a forged submission.

Verify: `go test ./cmd/prui ./internal/source -count=1`; rejected SHA/anchor,
changed base/head/repository, offline and unsupported targets make zero writes.
Dependencies: CD-01 write gate, CD-05. Scope: M.
Files: `cmd/prui/lifecycle.go`, `cmd/prui/lifecycle_test.go`,
`internal/tui/lifecycle.go`, `internal/source/github.go`,
`internal/source/source_test.go`.

## CD-09: Compose explicit historical line comments

- [x] Reuse inline editor with historical SHA/path/side heading and immutable
  target; Enter posts, Shift+Enter inserts newline and Escape cancels.
- [x] Guard queue-for-review with explanation; preserve existing head drafts and
  review form; switching contexts/commits cannot move an active draft.
- [x] Retain draft on ordinary failure; include commit drafts in existing quit/
  review-replacement protection; never persist or log draft text.

Verify: `go test ./internal/tui -count=1`; compose/cancel/post routing,
keyboard isolation, pending-review exclusion and draft-protection tests.
Dependencies: CD-07, CD-08. Scope: M.
Files: `internal/tui/commits.go`, `internal/tui/lifecycle.go`,
`internal/tui/review_submit.go`, `internal/tui/commits_test.go`,
`internal/tui/review_submit_test.go`.

## CD-10: Reconcile created and uncertain historical comments

- [x] Confirmed POST clears draft and immediately inserts canonical record into
  shared overlay/counts, even with null current anchor or outdated state unknown.
- [x] Place only exact returned anchors; enrich authoritative status on refresh
  without losing the created record in partial pages or duplicating its ID.
- [x] Distinguish confirmed rejection from uncertain delivery; show refresh-first
  guidance, retain draft and never automatically retry a write.

Verify: `go test ./internal/source ./internal/tui ./cmd/prui -count=1`;
fake creation with immediately outdated response, post-preflight movement,
malformed success/transport ambiguity, cancellation and stale generation cases.
Dependencies: CD-09. Scope: M.
Files: `internal/tui/lifecycle.go`, `internal/tui/discussions.go`,
`internal/tui/discussions_test.go`, `cmd/prui/lifecycle_test.go`,
`internal/source/comment_list_test.go`.

## Checkpoint C: Historical posting is safe and visible

- [x] Establish supported nonhead first-parent cases with live evidence;
  unverified cases currently stay read-only. Verified head targets are enabled.
- [x] A confirmed historical comment cannot disappear due to its current anchor.
- [x] Uncertain outcomes never cause automatic duplicates or false success claims.

## CD-11: Verify complete terminal journeys and regressions

- [x] Add bounded program/PTY journeys for historical reading/fallback/jump/return,
  compose/cancel/post/refresh and wide/narrow layouts with fake GH transport.
- [x] Prove no session/comment persistence, navigation fetch, reviewed-code
  execution, or main-review/pending-review behavior regression.
- [ ] Record human keyboard/mouse/no-color/short-screen usability evidence
  separately; do not present synthetic fixtures as live GitHub evidence.

Verify: `go test -race ./internal/source ./internal/tui ./cmd/prui -count=1`;
run relevant existing PTY tests and inspect the rendered screens.
Dependencies: CD-10. Scope: M.
Files: `internal/tui/workflow_test.go`, `cmd/prui/pty_test.go`,
`cmd/prui/testdata/pty_smoke.py`, `cmd/prui/wiring_test.go`,
`tasks/commit-discussions-todo.md` (evidence).

## CD-12: Update shipped contracts and complete verification

- [x] Amend current-head-only/read-only-commit clauses in constraints and earlier
  specs narrowly; describe shipped limits, status semantics, historical posting
  support, fallback, ephemeral/offline behavior and controls in user docs.
- [x] Review the complete diff against acceptance criteria and repository
  constraints; fix concrete findings and preserve unrelated rules/checks.
- [x] Run full repository verification and diff whitespace check; record results,
  API gate evidence and outstanding human checks honestly.

Verify: `./scripts/verify.sh` and `git diff --check`.
Dependencies: CD-11. Scope: M per increment; split documentation into two
increments if required to keep each at five files or fewer.
Files: first increment `CONSTRAINTS.md`, `docs/spec/SPEC-pr-commits.md`,
`docs/spec/SPEC-pull-request-review-comments.md`, `docs/REFERENCE.md`,
`docs/ARCHITECTURE.md`; completion increment spec/status and checklist evidence,
plus README only if its description changes.

## Completion Checkpoint

- [ ] All seven specification acceptance criteria met with recorded evidence.
  Nonhead RIGHT addition/context commenting (criterion 5) is verified; unsupported
  cases remain explicitly disabled.
- [x] API eligibility and live/synthetic evidence distinctions remain explicit.
- [x] Full automated gate passes; human checks are completed or listed as pending.
- [x] No new dependencies, persisted discussion data, mixed-SHA review batch,
  standalone commit comments or removed-object fetch slipped into scope.


## Implementation evidence — 2026-09-30

- Source GraphQL fixtures verify nested pagination, explicit connection/pageInfo
  presence, 64-bit action IDs, independent statuses, null anchors, partial result
  sorting, global canonical ID deduplication, budgets and safe errors.
- Actual Bubble Tea program journeys verify outdated reading/fallback, original
  jump/return without requests, current-diff freshness suppression, supported
  head composer creation with a null current anchor, and partial refresh retention.
- Four new reviewed golden screens cover widths 60/99/100/120, short height,
  no color, historical detail, unavailable context, partial results and stale pins.
- Independent reviews found/fixed current comments associated with older SHAs,
  canonical reply anchor rejection, hidden short-screen editors, wheel/keyboard
  jumps, partial-refresh creation loss, malformed empty-page success, duplicate
  canonical IDs and unsorted partial results. Focused regressions protect fixes.
- Composer Ctrl+R permits explicit refresh while retaining an uncertain draft.
  Captured raw anchor indexes avoid scanning every thread for every patch line.
- `GOCACHE=/tmp/prui-go-cache ./scripts/verify.sh` passes on native macOS:
  vet, full race suite (including PTY smoke) and build. `git diff --check` passes.
  Linked-worktree fixture tests require sandbox escalation for local Git and
  loopback fixtures; no real credentials or external writes are used.
- Local branch: `codex/commit-discussions`. Source/API, immutable target validation, UI integration, and final
  visibility/documentation increments are saved as local commits.
- Draft protection now covers active composers, and quit confirmation owns input
  before the composer; a failing-then-passing test proves Enter cannot post while
  discarding. Existing program tests explicitly confirm discard and wait for
  confirmed creation, preserving meaningful lifecycle assertions.
- Explicit discussion refresh repositions an active commit editor after newly
  loaded cards; the viewport regression fails before the fix and passes after.
- No external GitHub writes or live interoperability claims.
- Pending: separately authorized disposable GitHub experiment for nonhead
  first-parent targets; human terminal accessibility/usability acceptance.

## Historical delivery follow-up (2026-09-30)

- Verified exact original anchors for additions, context and immediate-outdated
  comments through REST and GraphQL on disposable PR #19.
- Historical LEFT deletion returned HTTP 422; keep LEFT/rename/copy/root/merge
  cases disabled. See the API evidence document for comment IDs and SHAs.
