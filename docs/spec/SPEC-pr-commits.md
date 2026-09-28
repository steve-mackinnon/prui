# Spec: Frozen Pull Request Commit List View

## Objective

Populate the Commit context view with the commits GitHub reports for the frozen
PR head. It provides review history and intent without changing diff units,
source progress, or the pinned Git comparison. This module follows Description;
the tab shell may render an intentional `Commits not available yet.` state until
it ships.

## Tech Stack

- Go 1.26.8+, Bubble Tea v2.0.9, and existing bounded `source.GH`
- GitHub pull-request commits API through `gh api`
- Immutable snapshot storage and existing review/TUI test suites

## Commands

```sh
go test ./internal/source -run 'Test.*Commit' -count=1
go test ./internal/session ./internal/review ./internal/tui -count=1
go test -race ./internal/source ./internal/session ./internal/review ./internal/tui
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/source/github.go      bounded commit-list request and response validation
internal/review/review.go      attach only commit data tied to the frozen head
internal/session/store.go      immutable commit-list snapshot field and validation
internal/tui/{model,render}.go per-tab scrolling and list/empty/legacy states
internal/*/*_test.go           request, pin, persistence, and screen regressions
README.md                      frozen commit-list behavior and limits
```

## Data and Lifecycle Contract

Define an internal immutable `source.PullRequestCommit` with exactly:

```go
type PullRequestCommit struct {
    SHA     string
    Subject string
    Author  string
}
```

The source boundary obtains `GET repos/{owner}/{repo}/pulls/{number}/commits`
after a PR's metadata is pinned. It requests a documented bounded first page
(`per_page=100&page=1`) and rejects a response whose count exceeds 100, whose
SHA is not a 40-character lowercase SHA-1, whose subject/author is invalid
UTF-8, or whose subject/author crosses the stated 4 KiB / 256-byte limits.
Multi-line commit messages are normalized to their first line before validation;
the view never renders body text. Duplicate SHAs are rejected.

The implementation must re-read PR metadata after receiving the list and keep
the list only when `SamePinnedRevision` matches the frozen comparison. A change
while loading causes the normal open/retry path to restart; it never attaches a
possibly different head's commits to a frozen snapshot. The session stores the
validated list and its `Complete` boolean. A list exactly at 100 records is
`Complete=false` and renders an explicit truncation notice rather than silently
claiming all commits are present.

Legacy snapshots have no commit field and show `Commit list was not captured for
this session.` No view selection, resume, or offline workflow contacts GitHub.

## Rendering and Interaction Contract

- Rows show subject first, then author and abbreviated SHA. Ordering is exactly
  GitHub's returned PR-commit order; the UI does not infer chronology.
- The body is one read-only scrollable list. `j`/`k`, arrows, page keys, Home,
  and End use the context view's tab-owned selection/scroll state and must not
  activate a diff cursor or file navigation.
- The selected commit is textual (`›`) and color-supplemented. Selecting a row
  does not show a commit diff, invoke Git, mark review progress, or alter
  comments. Commit-diff drill-in is explicitly out of scope.
- Render distinct safe states for zero commits, truncated list, missing legacy
  data, and unavailable capture. None are errors that erase the frozen diff.

## Code Style

Keep source parsing isolated from TUI formatting. Do not expose raw GitHub JSON
or generic map values beyond `internal/source`.

```go
func commitLabel(c source.PullRequestCommit) string {
    return Escape(c.Subject) + " · " + Escape(c.Author) + " · " + c.SHA[:12]
}
```

## Testing Strategy

- Test the exact `gh api` request and all bounded malformed-response failures
  with fake runners only.
- Test head changes during commit capture, 0/1/100 commits, duplicates,
  multiline subjects, and invalid/untrusted strings.
- Prove snapshot round-trip, cached source reuse, legacy absence, and offline
  refusal before GitHub client/credential access.
- Add wide/narrow list rendering, keyboard scrolling, per-PR-tab isolation,
  textual selection, and no-progress/comment-side-effect regressions.

## Boundaries

- **Always:** bind commits to the frozen PR revision, bound and validate all
  remote data, escape visible fields, and label truncation/absence honestly.
- **Ask first:** pagination beyond 100 commits, commit-diff drill-in, commit
  details/body display, schema migration, or a remote refresh command.
- **Never:** derive the list from local checkout history, change diff units or
  progress, fetch when selecting/resuming a view, or treat a commit list as
  complete when it was capped.

## Success Criteria

1. A new online review can show a frozen, bounded, safely rendered commit list
   for its exact pinned base/head comparison.
2. Head movement cannot attach stale commits to a snapshot.
3. A 100-commit result visibly reports that more commits may exist.
4. Commit navigation has no effect on source review progress, diff position, or
   comment state, and remains independent for each open PR tab.
5. Legacy and offline sessions remain readable without network access; focused
   source/session/review/TUI tests and the full verification gate pass.

## Open Questions

None. Pagination and per-commit diff navigation are deferred because they each
introduce independent freshness, bounds, and source-view semantics.
