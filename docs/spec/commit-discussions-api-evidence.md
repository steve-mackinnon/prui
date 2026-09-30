# Commit discussion API evidence

Investigated 2026-09-30 using official public documentation. No authenticated
live queries or external writes were performed. Synthetic tests verify our
request construction and normalization; they are not GitHub interoperability proof.

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
| Addition, deletion, context on an older SHA | Parameters documented; first-parent equivalence unverified | Disabled |
| Rename | Original-path semantics unverified | Disabled |
| Root commit | PR coordinate equivalence unverified | Disabled |
| Merge commit | First-parent equivalence unverified | Disabled |
| Changed-again line / immediate outdated response | Outdated behavior documented; synthetic normalization tested | Disabled |

Historical reads are enabled independently. No live mutation is needed to ship
the read slices. Enabling historical writes requires a separately authorized
experiment in a named disposable repository, covering request SHA/path/line/side
and canonical response anchors for each supported class. Unsupported cases must
remain non-commentable. Do not silently fall back to the head SHA or standalone
commit comments.

## Local interaction evidence

Keyboard conflicts and launcher behavior are checked in the TUI implementation
slice. Displaying a validated permalink is the permitted fallback; fetching
missing commits is outside scope.
