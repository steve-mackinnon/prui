# Cross-repository review inbox (#35)

`prui inbox` opens cached account triage; `--refresh` explicitly reads GitHub.
`--offline` rejects refresh before client/credential access. `--plain` prints
escaped metadata and exits 2 for incomplete results. In the existing repository
picker, `i` opens the inbox; in the PR picker/switcher, `ctrl+o` opens it.
No timer, notifications, alerts, AI upload or remote writes are introduced.

Views cover the authenticated account's accessible GitHub repositories, without
restricting results to remembered repositories. Requested-from-me unions
`user-review-requested:@me` and `team-review-requested-user:@me`. Authored uses
`author:@me`. Participated unions `involves:@me` and `reviewed-by:@me` so a review
without a conversation comment is included. Team membership uses GitHub search
rather than inferring it from team names. The item repository is authoritative.

`--account LOGIN` selects a specific cached account; online refresh must match
the authenticated account. With no account selected, cached mode displays the
most recently captured account visibly. Captures and read marks remain isolated
per account; a refresh always reloads that same account.

Filters: `--view requested|authored|participated`, `--repository owner/repo`,
`--author LOGIN`, `--review all|none|required|approved|changes_requested`,
`--state open|closed|all`, `--draft all|yes|no`,
`--requests all|personal|team`, `--activity all|unknown|read|changed`.
Review decisions come from GraphQL readiness facts; none requires an authoritative
zero review count. Unknown decision does not mean no reviews. Personal/team
noise filters apply to all views. TUI `f` opens filters: Tab selects a field,
Left/Right cycles choices, repository/author accept typing, Enter loads the cache
for those filters and `r` explicitly refreshes. Escape discards filter edits.

Each search stream is limited to ten 100-item pages (at most 20 requests and 2,000
union matches); the entire operation has a 60-second deadline and each response
is bounded to 1 MiB. GitHub's 1,000-result search limit, missing/null fields,
unknown request connection coverage, malformed or repeated cursors, invalid
identities, mismatching authoritative filters, changing counts and page failures
are visibly incomplete. Prior successful pages remain visible. Access-limited
repositories cannot be enumerated as a guarantee: the UI says they may be absent.
Request details are capped at 100 per PR and partial coverage remains explicit.
Outside a team-scoped search, team names show requested teams, not proof of the
viewer's membership. Filtering reduces visible rows, never promotes completeness.

`m` explicitly marks the selected captured evidence read locally. First capture
is unknown until marked read; unchanged evidence stays read; changed head,
updated conversation time, state, draft/readiness, title/author or requested team
facts becomes changed. Activity annotations do not modify the frozen review.
The separate private `inbox.sqlite` inside app-owned session storage contains
only triage metadata, integrity hashes, account-scoped marks and query captures.
It stores no credentials, source bodies, conversation bodies or checkout paths.
Captures survive restart; stale captures cannot overwrite later observations.
Cached data always identifies account and observation time; refresh failures do
not imply access or an empty complete inbox. Invalid/unsupported caches are
retained and rejected. Local cache data is unencrypted.

Enter switches immediately to an already open identity. Otherwise it resumes that
PR's own frozen saved session, without automatically refreshing pinned source,
readiness or conversation. Frozen sessions stay readable offline without a live
checkout; their live freshness is labelled unchecked. With no saved session,
opening requires the selected repository's own remembered checkout and verifies
its current origin matches the item. Missing/mismatching checkouts show the
concrete `prui open URL` action from the appropriate checkout; another repository's
checkout is never borrowed. Existing ordinary repository picker controls remain.
Async generations reject superseded inbox refresh/mark/open results.

Prepared suggestion drafts can return to the review with Escape and enter the
inbox through the PR switcher while retaining private delivery intent. An uncertain
suggestion application keeps its existing reconciliation modal: keyboard Escape
does not leave it or discard its immutable attempt. Inbox support does not relax
that guard. Tests distinguish the reachable prepared keyboard flow from background
tab/storage invariants for uncertain attempts, including restart recovery and exact
private SQLite payload/generation preservation.

Validation uses synthetic fixtures. Live account search and human terminal
usability remain unverified until performed.

Sources: [GitHub search qualifiers](https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests)
(including direct/team request and reviewed-by semantics),
[GitHub GraphQL search](https://docs.github.com/en/graphql/reference/queries#search).
