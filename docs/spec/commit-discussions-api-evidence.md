# Commit discussion API evidence

Investigated 2026-09-30 using official documentation and authenticated probes
on the user-authorized disposable [PR #19](https://github.com/steve-mackinnon/prui/pull/19).
Four labeled test comments were posted; one deletion request returned HTTP 422.

## Read contract

[GitHub GraphQL pull requests reference](https://docs.github.com/en/graphql/reference/pulls)
provides `PullRequest.reviewThreads`, the `PullRequestReviewThread` node ID,
`isOutdated` and `isResolved` (independent non-null booleans), `diffSide`, `path`,
nullable `line`, `originalLine`, `startLine`, and `originalStartLine`, and
`subjectType`. Both the thread connection and each nested `comments` connection
support `first`, `after`, and `pageInfo` pagination.

The comment provides nullable `originalCommit`, `commit`, and `replyTo`,
`diffHunk`, `url`, `createdAt`, and nullable `author`. Use `fullDatabaseId`
(BigInt) for REST action identity; `databaseId` is deprecated and cannot represent
all 64-bit IDs. Deleted authors are displayed as `[deleted]`. A missing root
never becomes a manufactured root or inferred commit owner.

Thread original coordinates and the root's original commit identify ownership.
Multiline/file threads remain readable but cannot be represented by the
single-line target contract. Renamed paths are retained as supplied; no path
translation or inferred relocation is performed. URL display is restricted to
control-free github.com HTTPS permalinks without credentials or query parameters.

## Historical write delivery gate

[GitHub REST review comments](https://docs.github.com/en/rest/pulls/comments#create-a-review-comment-for-a-pull-request)
documents `commit_id`, `path`, `line`, and `side`, recommends the latest commit,
and explains that an older commit can make the comment outdated. It does not
establish that PRUI's first-parent diff coordinates equal GitHub's historical PR
diff coordinates for every case. REST responses contain original/current commit
and line fields; a confirmed comment without a current line remains readable.

| Case | Evidence | Historical posting |
| --- | --- | --- |
| RIGHT additions and context, regular single-parent A/M files | Live original SHA/path/line/side matched exactly | Enabled |
| Changed-again RIGHT addition | Live original anchor matched; GraphQL isOutdated=true | Enabled |
| LEFT deletion | Second-commit parent line absent cumulative PR diff; HTTP 422 | Disabled |
| Rename/copy | Original-path semantics unverified | Disabled |
| Root/merge commit | PR coordinate equivalence unverified | Disabled |

The runtime requires a complete captured single-parent diff, an added or modified
file without path translation, and exact RIGHT raw-hunk membership. Head targets
retain their existing requirement to also appear in the frozen PR diff. Delivery
preflight still verifies both repository identities and base/head SHAs; no target
is rewritten to head and uncertain delivery is never retried automatically.

### Live results

Fixture path: `docs/testing/commit-discussions-fixture.md`.

| Request commit | Line/side | Result |
| --- | --- | --- |
| `9e18f6ea2cabf611b5e0f55415dda86dffe91119` | 8 RIGHT, stable addition | Comment 4149188001; exact original SHA/line; current anchor at head; isOutdated=false |
| `9e18f6ea2cabf611b5e0f55415dda86dffe91119` | 9 RIGHT, changed again | Comment 4149188112; exact original SHA/line; current line null; isOutdated=true |
| `1f2c5c1486c4b14f8e03db93c69f0b72cca55319` | 9 RIGHT, replacement | Comment 4149188245; exact original SHA/line; current anchor at head; isOutdated=false |
| `1f2c5c1486c4b14f8e03db93c69f0b72cca55319` | 8 RIGHT, context | Comment 4149188404; exact original SHA/line; current anchor at head; isOutdated=false |
| `1f2c5c1486c4b14f8e03db93c69f0b72cca55319` | 9 LEFT, deletion | HTTP 422: pull_request_review_thread.line could not be resolved |

GraphQL reviewThreads independently verified each original commit and outdated
flag. The head at probe time was `bddf8d8912fa419fcb75f4aca89fb535cb483c76`.

## Local interaction evidence

Keyboard conflicts and launcher behavior are checked in the TUI implementation
slice. Displaying a validated permalink is the permitted fallback; fetching
missing commits is outside scope.
