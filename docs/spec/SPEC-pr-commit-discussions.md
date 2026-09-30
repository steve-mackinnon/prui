# Spec: Commit Discussions and Outdated Review Comments

Status: Read-side scope and verified head-commit composer implemented
2026-09-30 after user authorization. Nonhead historical posting remains gated
pending separately authorized live interoperability evidence. Feature id: `pr-commit-discussions`.

## Objective

Let a reviewer read and post PR review comments while inspecting an individual
commit, and keep historical discussions discoverable when their current diff
anchor or original commit is unavailable. Outdated describes a code anchor;
it never implies that the concern was resolved.

This is one capability with successive read and write slices. It extends
`SPEC-pr-commits.md` and `SPEC-pull-request-review-comments.md`: commit browsing
becomes commentable and fetched discussions are no longer discarded merely
because they cannot match the frozen PR head. All other pinning, source capture,
progress, offline, and terminal-escaping contracts remain applicable.

## Scope and Decisions

- PR review threads and comments, including replies, current and original anchors,
  original commit identity, a bounded historical snippet, and a GitHub permalink.
- Inline discussions in the main diff and the selected commit's first-parent diff.
- Textual outdated indication, thread counts in the commit rail, and a PR-wide
  Discussions overlay for every loaded thread, including unplaceable threads.
- Immediate, explicit single-line comments targeting a captured commit SHA.
- Historical comments remain separate from pending comments submitted with a
  head-based PR review. Existing main-diff pending-review behavior stays intact.
- Discussion data and drafts remain memory-only. Offline sessions show captured
  commit diffs but cannot fetch or post comments. No schema migration.
- Exclude standalone repository commit comments, timeline comments, new resolve/
  unresolve actions, mixed-SHA review batches, persisted discussion history,
  automatic relocation, hidden polling, and fetching removed Git objects.
- Existing reply/delete/reaction capabilities must continue working in the main
  diff. New historical surfaces are initially read-only apart from composing a
  new line comment; action parity is a follow-up, avoiding new write semantics.

Assumptions: the intended workflow is PR review discussions created from a
selected PR commit, rather than standalone repository commit comments; a
historical comment posts immediately; existing commit membership stays capped.

## User Experience

### Commit list and selected commit

Preserve current rail navigation, focus, narrow-layout behavior, and per-commit
reading position. Add a compact `2 threads · 1 outdated` suffix/count line when
space permits. Counts refer to threads whose root's original SHA equals that
commit, not number of replies. With limited width prioritize selection, SHA,
and total thread count. Incomplete results say `Loaded threads` rather than
implying a complete count; never present unavailable data as zero threads.

Selecting a commit renders its original-anchor threads beneath exactly matching
raw diff lines. Show author/body/replies using existing escaped comment-card
presentation. Add `Outdated on current PR` when GitHub reports that state. A
thread may also appear at its exact current anchor in the main diff; these are
views of the same thread, not duplicated records.

If an original path/side/line does not match captured material, leave the comment
in Discussions. Never guess an anchor using text similarity, line offsets, or
another commit's matching line number. Multiple-line and file-level threads
remain readable in Discussions; no new range/file comment creation is included.

```text
Commits · 2/3                    Commit diff · b7c8d9e01234
› Wire the commits pane          + return result
  Alice · b7c8d9e01234            ┌ alice · Outdated on current PR
  2 threads · 1 outdated         │ Handle an empty result here?
                                 └ 1 reply
```

### Discussions overlay

Use `D` from any review context view to open the PR-wide overlay; `D` currently
must be checked for conflicts during implementation. Keep `c` as explicit
refresh, including from Commits and Discussions. Overlay controls: j/k or arrows
select, Enter shows the thread and historical snippet, Escape returns one level.
Thread detail offers `View original commit` and a displayed, selectable GitHub
permalink. Opening a browser requires an explicit action and validated HTTPS
GitHub URL, using the existing launcher if available. A plain displayed URL is
an acceptable fallback. Mouse selection follows the same semantic targets.

All loaded threads are listed, including outdated, file-level, multiline,
unmatched, and missing-original-commit records. Order by root creation time then
stable ID; no automatic filtering hides outdated threads. Selection survives a
refresh by stable thread ID, with a deterministic fallback when deleted.

Labels distinguish independent facts:

| Fact | Label/behavior |
| --- | --- |
| GitHub says thread is outdated | `Outdated on current PR` |
| GitHub says thread is resolved | `Resolved`; may coexist with Outdated |
| State could not be obtained | `Status unknown`; never infer resolved |
| Original SHA is absent from captured list | `Original commit not captured` |
| Original SHA cannot be obtained | `Original commit unavailable` |
| Original coordinate cannot be placed | `Original context unavailable`; show bounded snippet if supplied |
| Live PR pins differ from snapshot | `Discussions from live PR · snapshot differs`; writes blocked |

Absence from a frozen, possibly capped list does not prove a force-push removed
that commit. Do not label it removed without authoritative membership evidence.
`View original commit` switches to Commits only when that SHA is captured, then
focuses and scrolls to the anchor if available. Otherwise thread detail remains
readable with its snippet and link; no Git fetch occurs. Returning restores the
prior context view, selection, focus, cursor, and reading position.

### Composer and delivery

Enter in focused commit detail opens the composer only on a valid target row.
Reuse editor controls, including Enter to post, Shift+Enter for newline, and
Escape to cancel. Header includes `Comment on <SHA> · <path>:<line> · <side>`
and, for non-head commits, `Historical commit`. Context/commit switching cannot
move an active draft to another target; composer routing consumes those keys.
Ctrl+R explicitly refreshes discussion data without discarding the draft, so an
uncertain delivery can be checked before an intentional retry. Drafts are
review-owned and target-keyed. Confirm discarding on review replacement
or quitting consistently with existing unsent-draft behavior.

`ctrl+p` queue-for-review is unavailable in a historical composer, with an
explanation that historical comments post immediately. The existing PR-level
review form remains available and contains only its current head-based drafts.

Submit checks frozen repository identities, base SHA and head SHA against fresh
metadata and verifies the requested SHA is an entry of the active frozen commit
bundle. Validate provenance against that entry's patch and target; do not accept
an arbitrary caller-supplied SHA or anchor. First-parent and GitHub coordinates
must be verified before enabling this write path (see delivery gate below).

A successful POST is retained even if GitHub immediately marks it outdated or
returns no current line. Update Discussions immediately and place it inline
only when the returned original/current anchor matches exactly. Clear its draft
only after confirmed creation. No automatic write retry. If cancellation or a
transport failure leaves delivery uncertain, say `Posting outcome unknown;
refresh discussions before retrying` and retain the draft without claiming it
was not posted. Canonical reconciliation uses comment ID; do not deduplicate
unrelated comments merely because their bodies match.

## Source and State Contract

Introduce a narrow read-only discussion reader alongside current write/action
interfaces. Use GitHub GraphQL review threads for authoritative `isOutdated` and
`isResolved`; use REST for existing single-comment writes. Query via bounded
`gh api graphql` with JSON input on stdin, never interpolating reviewer text.
REST's original/current anchor fields remain useful for write responses; absence
of a current line alone is not a substitute for authoritative thread state.

Separate a discussion record from a writable `ReviewCommentTarget`: a readable
record can lack a current anchor and must not become invalid merely for that
reason. The proposed contract is illustrative; finalize exact fields after the
API fixture investigation, preserving existing action IDs.

```go
type Discussion struct {
    ID string
    OriginalCommitID string
    OriginalAnchor *ReviewCommentTarget
    CurrentAnchor *ReviewCommentTarget
    Outdated *bool // nil means unknown, not false
    Resolved *bool
    Comments []ReviewComment
    DiffHunk string
    URL string
}
```

Root comments determine historical ownership; replies stay with their root even
when their own coordinates differ or are absent. Retain node IDs for thread
identity and numeric REST IDs for existing actions. Missing/deleted parents and
partial reply pages are visible incomplete states, never manufactured roots.

Bound each refresh: 100 threads/page, at most 5 thread pages, 500 threads and
2,000 total comments, 100 comments/thread, 4 MiB decoded material and a single
60-second deadline. Comment bodies <=64 KiB; logins <=256 bytes; paths <=4 KiB;
historical snippet <=64 KiB. Reuse stricter existing runner output bounds where
applicable. Explicitly paginate both threads and nested comments; query only
needed fields. When a budget or permission prevents completion, preserve usable
validated results with an incomplete notice. Truncate display material honestly;
never let display limits alter an anchor or write target. Zero results implies
no discussions only after a complete successful fetch.

No per-commit network request on navigation. Fetch once through the existing
online comment-loading lifecycle, and on explicit `c`; cancel on review
replacement. Build shared indexes by stable thread ID, original SHA, and exact
current/original target. Escape all remote fields before measurement/rendering.
Read current metadata around refresh; movement or failed freshness checks label
the discussion snapshot uncertain and disable exact current-diff placement until
its pins are verified. Historical ownership can still be shown explicitly.

Publish a completed refresh atomically. On failure preserve the prior overlay
with a visible stale/error notice. Async responses carry review identity and
request generation; late results cannot enter a replacement review. Counts,
inline cards, and Discussions all derive from the same bounded overlay. No
comment data enters session payloads, guides, logs, plain output, or disk.

## Delivery Gate and Risks

Before historical posting, establish GitHub behavior for first-parent diffs:
additions, deletions, context, renames, root/merge commits, changed-again lines,
and immediately outdated POST responses. Official docs allow a requested commit
SHA, but do not establish that every first-parent coordinate is valid in a PR.
Record evidence and supported cases; unsupported cases remain non-commentable
with an explanation. Do not silently convert to a head comment, use deprecated
position targeting, or switch to standalone commit comments.

Use synthetic fixtures for automated verification. A real GitHub investigation
that creates comments needs explicit authorization for a disposable repository;
this specification request does not authorize external writes. If live evidence
is needed, complete all read-side work while that gate remains open.

## Project Structure and Style

Go/Bubble Tea/Lip Gloss and existing bounded GH runner; no new dependency.

- `internal/source/`: normalized records, bounded GraphQL parsing/pagination,
  existing REST response handling, fake-runner fixtures.
- `cmd/prui/lifecycle.go`: injection, offline refusal, freshness and provenance
  preflight; application lifecycle tests.
- `internal/tui/`: shared discussion indexes, commit anchors/cursor, inline cards,
  overlay navigation, composer routing, help/mouse geometry, render caches.
- `internal/tui/testdata/screens/`: deterministic wide/narrow/no-color screens.
- `docs/REFERENCE.md`, `docs/ARCHITECTURE.md`, `CONSTRAINTS.md`: shipped behavior
  and narrow amendments to current-head-only contracts during implementation.

Use typed boundaries and pure anchor derivation from raw hunk counters. Format
Go with gofmt. Never derive targets from styled text or reuse fake sessions to
render commit material. Cache keys include discussion generation, commit SHA,
width and theme so refresh cannot leave stale cards/counts.

## Testing and Commands

Automated tests use synthetic GitHub responses and isolated Git fixtures.

- Source: current/original coordinates, null fields, resolved/outdated independence,
  nested pagination, missing roots, malformed records, bounds, partial failures,
  canonical POST responses without current anchors, safe URLs/control bytes.
- TUI: exact placement in both surfaces, no guessed placement, counts, reply
  grouping, removed/uncaptured contexts, unavailable diffs, zero net PR diff,
  widths 60/99/100/120, short height, no color, cursor/mouse and state restoration.
- Lifecycle: immutable membership and patch provenance, mismatched pins, offline
  refusal before credential access, late results, cancellation/unknown delivery,
  immediate update, pending-review isolation, main-diff action regressions.
- Program/PTY journey: read historical thread, inspect fallback, jump to commit,
  compose/cancel/post with injected fake transport, refresh and restore main diff.

```sh
go test ./internal/source ./internal/tui ./cmd/prui -count=1
go test -race ./internal/source ./internal/tui ./cmd/prui -count=1
./scripts/verify.sh
git diff --check
```

Human terminal checks cover keyboard/mouse ergonomics, no-color labels and short
screens; automated snapshots do not establish accessibility.

## Boundaries

- Always: preserve frozen source and review state; validate external data;
  label incomplete/unknown/live states; require explicit writes; keep drafts
  memory-only; retain discussions that cannot be placed inline.
- Ask first: dependencies, persisted history/drafts, removed-commit fetching,
  standalone commit-comment support, resolve actions, mixed-SHA batching,
  external writes for live API experiments.
- Never: silently retarget, infer resolution from outdated status, claim an
  uncaptured SHA was removed, auto-post/retry, execute reviewed code, weaken
  CONSTRAINTS.md beyond the explicitly scoped historical-comment amendments.

## Acceptance Criteria

1. Selecting a captured commit shows every loaded thread with a matching
   original SHA and exact available anchor, with replies and outdated labels.
2. Every loaded thread is discoverable in Discussions even without a captured
   commit/diff/line; fallback context and URL remain readable.
3. Resolved, outdated, unknown status and unplaceable anchors are distinct;
   counts and empty states disclose incomplete retrieval.
4. Current-diff placement remains exact and freshness-qualified; commit
   navigation makes no network request and preserves main review state.
5. A supported historical target can explicitly post one PR review comment
   using its SHA after strict freshness/membership/provenance validation.
6. Confirmed creation survives an immediately outdated response; uncertain
   delivery never invites an automatic duplicate write.
7. Offline/plain/session/guide behavior and head-based queued reviews retain
   their existing contracts; historical drafts never enter that queue.

## References and Remaining Investigation

- [GitHub PR review comments](https://docs.github.com/en/rest/pulls/comments):
  current/original fields, requested commit SHA, line/side targeting, responses.
- [GitHub GraphQL review threads](https://docs.github.com/en/enterprise-cloud@latest/graphql/reference/pulls):
  authoritative outdated/resolved state and thread/comment connections.
- [GitHub comment types](https://docs.github.com/en/rest/guides/working-with-comments):
  PR review discussions and standalone commit comments are different resources.

Implementation task CD-01 must verify exact GraphQL field/nullability contracts,
rename path semantics, and first-parent eligibility. These are technical delivery
gates, not permission to broaden the agreed product scope.
