# Spec: Frozen Pull Request Description View

## Objective

Populate the Description context view with the PR author's frozen GitHub body.
It lets a reviewer read the PR's rationale, change summary, and review notes
inside the same pinned review session—including offline/resumed sessions—without
claiming that prose is source truth or current remote state.

## Tech Stack

- Go 1.26.8+ with `encoding/json` and Bubble Tea/Lip Gloss
- GitHub CLI `gh api` through the existing bounded `source.GH` boundary
- Existing immutable session snapshot and private atomic store

## Commands

```sh
go test ./internal/source -run 'Test.*(Metadata|Description)' -count=1
go test ./internal/session ./internal/review ./internal/tui -count=1
go test -race ./internal/source ./internal/session ./internal/review ./internal/tui
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/source/github.go      validated GitHub PR presentation data
internal/source/git.go         pin revision comparison that ignores presentation-only fields
internal/review/review.go      attach frozen description while constructing a session
internal/session/store.go      immutable snapshot field and validation
internal/tui/{model,render}.go Description view state and safe read-only rendering
internal/{source,session,review,tui}/*_test.go  boundary, persistence, and display coverage
README.md                      frozen-description and offline behavior
```

## Data and Lifecycle Contract

`source.Metadata` continues to be the single response boundary for
`GET repos/{owner}/{repo}/pulls/{number}`. It gains a presentation-only
description field with the exact GitHub `body` string; `null` maps to the empty
string. The external response is rejected when body is not valid UTF-8 or
exceeds 1 MiB. It is never treated as Markdown instructions or executed HTML.

The source package must provide a comparison-only predicate, equivalent to:

```go
func SamePinnedRevision(a, b Metadata) bool {
    return a.Identity == b.Identity &&
        a.BaseRepository == b.BaseRepository && a.HeadRepository == b.HeadRepository &&
        a.BaseSHA == b.BaseSHA && a.HeadSHA == b.HeadSHA
}
```

Pin/fetch retry logic uses that predicate rather than Go struct equality. A
description edit with unchanged source revisions is therefore allowed to create
a new frozen session without forcing a misleading source re-fetch; changed
repository identities or SHAs retain today's retry/failure semantics.

`session.Snapshot` gains an optional `PullRequestDescription` string. Opening a
new review copies the validated body into that immutable field before the first
store write. A cached comparison only reuses a snapshot if it contains the same
validated frozen presentation data returned by the metadata call; otherwise it
creates a new snapshot session from the existing frozen source material. This
keeps Description accurate for the session while source cache identity remains
base/head based.

Old snapshots lacking the optional field remain valid and readable. Their
Description view displays a clear, non-error empty state: `Description was not
captured for this session.` It makes no background request and offline mode
continues to access no GitHub client or credentials.

## Rendering Contract

- Empty newly captured bodies display `No description provided.`
- Nonempty bodies render as escaped, wrapped plain text. Markdown is not
  interpreted in this increment; headings, lists, links, images, ANSI escapes,
  and control sequences stay literal safe text.
- The view starts at top on each newly opened PR tab. Its scroll offset is
  tab-owned, stays independent of diff/guide offsets, survives view switches
  during the process, and resets after restart.
- The view includes compact provenance: `Frozen from GitHub when this review
  opened.` It must not say or imply that the description is current.

## Code Style

Validate once at the GitHub boundary, use value types for frozen presentation
data, and return concise generic availability errors without raw provider
output. Rendering receives already-frozen data and must not call `source.GH`.

```go
func descriptionText(s *review.Session) string {
    if s.PullRequestDescription == "" { return "No description provided." }
    return Escape(s.PullRequestDescription)
}
```

## Testing Strategy

- Unit-test valid body, null body, invalid UTF-8, oversized data, and errors
  without using a real `gh` process or credential.
- Prove `SamePinnedRevision` ignores description edits but not source pins.
- Round-trip a new snapshot and an old fixture with no field; test cache reuse
  and re-session behavior for changed bodies at unchanged SHAs.
- Test escaped, colorless, narrow, empty, absent-old-session, online, and
  offline description rendering.

## Boundaries

- **Always:** freeze, validate, escape, bound, and label GitHub description
  data; retain old session readability and strict offline behavior.
- **Ask first:** schema-version migration, Markdown rendering, live refresh
  action, changing source-cache identity, or storing remote comments.
- **Never:** execute/render GitHub HTML, fetch on tab selection or resume,
  overwrite a snapshot, or imply description text is verified source truth.

## Success Criteria

1. A newly opened online review displays its validated PR description after
   switching to Description and remains readable offline after restart.
2. A changed body alone cannot break pinned-source comparison/retry semantics.
3. Empty and legacy-absent descriptions are unambiguous, safe, and non-networked.
4. Untrusted body content cannot inject terminal control sequences or formatting.
5. Source, session, review, and TUI regressions pass without live GitHub access.

## Open Questions

None. Rich Markdown presentation and a manual remote refresh are intentionally
out of scope; both would need a separate specification because they alter the
trust and freshness contract.
