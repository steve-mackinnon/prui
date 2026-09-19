# Spec: Pull Request Line Comments

## Objective

Allow a reviewer of a pinned GitHub pull request to select a concrete diff
line, write Markdown in a terminal composer, and explicitly create one
line-anchored GitHub review comment. The feature serves reviewers who need to
leave actionable inline feedback without leaving the TUI.

Only an explicit submit may write to GitHub. The comment must target the exact
repository, pull request, head commit, path, side, and line represented by the
frozen comparison. If GitHub metadata no longer matches that comparison during
the submit preflight, the application must make no write and must direct the
reviewer to open a new comparison.

## Tech Stack

- Go 1.26.8+
- Bubble Tea v2.0.9
- Lip Gloss v2.0.2
- Authenticated GitHub CLI, using `gh api` for the GitHub REST endpoint
- Existing synthetic Go tests, Bubble Tea program tests, and screen snapshots

The GitHub REST endpoint is `POST /repos/{owner}/{repo}/pulls/{pull_number}/comments`.
It receives `body`, `commit_id`, `path`, `line`, and `side`; `RIGHT` applies
to additions and context and `LEFT` to deletions. The deprecated `position`
parameter is not used. See [GitHub's review-comment API](https://docs.github.com/en/rest/pulls/comments#create-a-review-comment-for-a-pull-request).

## Commands

```sh
go test ./internal/source -count=1
go test ./internal/tui -count=1
go test ./cmd/pr-review -count=1
go test -race -count=1 ./...
go build ./...
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/source/       typed GitHub comment contract, GH CLI implementation, runner
internal/tui/          line provenance, cursor, composer, rendering, key routing
cmd/pr-review/         online/freshness preflight and lifecycle injection
README.md              user-facing interaction and permission requirements
CONSTRAINTS.md         revised explicit-write boundary
tasks/                 implementation plan and ordered task checklist
```

## Interaction Contract

1. The diff pane has one visible selected display line. The cursor is separate
   from its scroll offset and survives ordinary pane navigation.
2. `Enter` in the list pane retains its current behavior: focus the diff.
   `Enter` in the diff pane opens the composer only for a commentable line.
   Headers, hunk markers, binary/metadata/unavailable content, and invalid
   paths are visibly non-commentable.
3. The composer identifies the frozen target as `path`, side, and line.
   Printable input and `Enter` build multiline Markdown; `Ctrl+Enter` submits;
   `Esc` discards the draft. Drafts are process-local and are never persisted.
4. Submit first reads current PR metadata. It proceeds only when base repository,
   head repository, base SHA, and head SHA equal the frozen comparison.
5. The request uses the frozen head SHA and never silently retargets a changed
   PR. A post-preflight race can still yield an outdated GitHub comment, which
   GitHub represents as such.
6. On success the TUI reports that the comment was posted. A failure reports a
   safe actionable error and retains the draft/target for an intentional retry.
   A canceled operation leaves no claim that a comment was not created.

## Comment Contract

Define a narrow additive interface in `internal/source`, separate from
`GitHub`:

```go
type ReviewComment struct {
	Target ReviewCommentTarget
	Body     string
}

type ReviewCommentTarget struct {
	Identity Identity
	CommitID string
	Path     string
	Side     string // LEFT or RIGHT
	Line     int
}

type ReviewCommenter interface {
	CreateReviewComment(context.Context, ReviewComment) error
}
```

The boundary validates nonempty UTF-8 body/path, a validated repository and
40-character commit SHA, positive line, and the two allowed sides. `GH`
serializes this structure with `encoding/json` and sends it via stdin to
`gh api --method POST --input -`; text entered by the reviewer must not become
a process argument. `gh api` documents `--input -` as stdin request-body
support. [GitHub CLI API manual](https://cli.github.com/manual/gh_api)

The response is untrusted data. The initial contract only requires successful
completion; no GitHub response body, comment body, token, or raw stderr is
persisted or rendered.

## Diff-Line Targeting

Each rendered detail line must retain optional target metadata rather than
attempting to derive a target from escaped presentation text:

| Patch line | Target |
| --- | --- |
| addition (`+`) | `NewPath`, `RIGHT`, current new line |
| deletion (`-`) | `OldPath`, `LEFT`, current old line |
| context (` `) | `NewPath`, `RIGHT`, current new line |
| diff/hunk header or no-newline marker | none |

Line counters begin from the stored hunk ranges and advance according to the
unescaped patch grammar. This mapping must propagate through both raw-unit and
guide detail rendering. Paths containing invalid UTF-8 are rendered safely but
cannot be submitted to the JSON GitHub endpoint.

## Code Style

Keep target derivation pure and use a small, explicit value type. Rendering
continues to escape source bytes before styling; target metadata never alters
the visible patch content.

```go
type diffLine struct {
	styledLine
	target *source.ReviewCommentTarget
}
```

Do not add a generic form library, change stored session schemas, or add a
background write queue. TUI services are injected narrow functions, following
the existing lifecycle pattern.

## Testing Strategy

- Start every implementation task with a focused failing test.
- Unit-test hunk mapping for additions, deletions, context, empty ranges,
  multiple hunks, renames, deletion-only files, and invalid-byte paths.
- Test the GH request's exact method, endpoint, stdin JSON, validation, output
  limit, cancellation, and absence of a typed body in arguments.
- Test TUI cursor visibility, non-commentable rejection, composer edit/cancel,
  successful submit, failure draft retention, and pane/tab isolation.
- Test application preflight mismatch and offline refusal prove no commenter
  call; use fakes only, never GitHub credentials or network.
- Add a bounded Bubble Tea program path for compose → submit → success/failure
  and update snapshots only after reviewing the textual change.

## Boundaries

- **Always:** validate at external boundaries; escape all displayed source and
  error text; use the frozen target; preflight fresh metadata; bound subprocess
  output; keep drafts memory-only; test synthetic success/error/cancel paths.
- **Ask first:** new dependencies; persisted comment history/drafts; batch
  review submission; timeline comments; multiline ranges; changed CLI/plain
  output; any change that broadens GitHub write permissions.
- **Never:** send a comment automatically; write during refresh/open/resume;
  silently retarget after a head change; log/store credentials or comment text;
  send user text in a command argument; enable comments in offline mode.

## Success Criteria

1. A reviewer can visibly select a valid added, deleted, or context diff line
   in raw and guide views, and the selected target has the correct path, side,
   and line.
2. A reviewer can compose multiline Markdown, cancel without a write, and
   explicitly submit exactly one review comment.
3. The request uses GitHub's modern `line`/`side` API parameters, frozen head
   SHA, stdin JSON, and an authenticated existing `gh` installation.
4. A changed PR, offline run, unavailable/non-text line, invalid path, empty
   body, or failed preflight makes no external write.
5. Existing review navigation, plain output, pinning, local progress, guide
   behavior, terminal escaping, and tab state remain intact.
6. Focused tests, race-enabled suite, build, verifier script, and diff check
   pass; a real PR comment remains an authorized manual acceptance check.

## Open Questions

None for the first release. Timeline comments, replies, comment listing,
batch reviews, and multi-line ranges are intentionally deferred.
