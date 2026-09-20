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
   `Enter` in the diff pane opens an inline composer immediately below the
   selected commentable line.
   Headers, hunk markers, binary/metadata/unavailable content, and invalid
   paths are visibly non-commentable.
3. The inline composer identifies the frozen target as `path`, side, and line.
   Printable input and `Enter` build multiline Markdown; `Backspace` removes
   the preceding rune; `Delete` removes the following rune; `Ctrl+Enter`
   submits; `Esc` discards. Drafts are process-local and are never persisted.
4. Submit first reads current PR metadata. It proceeds only when base repository,
   head repository, base SHA, and head SHA equal the frozen comparison.
5. The request uses the frozen head SHA and never silently retargets a changed
   PR. A post-preflight race can still yield an outdated GitHub comment, which
   GitHub represents as such.
6. On success the TUI reports that the comment was posted and immediately
   renders it below its anchored diff line. A failure reports a safe actionable
   error and retains the draft/target for an intentional retry. A canceled
   operation leaves no claim that a comment was not created.
7. When online, the TUI loads review comments as read-only, ephemeral overlay
   data. It renders every comment anchored to the frozen head commit whose
   `path`, `side`, and `line` match a visible target. `c` explicitly reloads
   that overlay. Comments that cannot be matched exactly are omitted rather
   than guessed at or rendered at an incorrect line.

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

Read-only comment overlays use `GET /repos/{owner}/{repo}/pulls/{pull_number}/comments`
with bounded pagination. The implementation retains only a bounded current-head
set in memory and never serializes it into a review session. A successful POST
returns its canonical GitHub comment so the overlay can update immediately;
the same escaping rules apply to both fetched and newly created text.

GitHub response data is untrusted. Comment body, login, path, and errors are
escaped before rendering; comment text, tokens, and raw stderr are never
persisted or logged.

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
the visible patch content. The review view expands a target into zero or more
escaped, read-only comment rows and, when active, one editor row directly
beneath it. Cursor navigation continues to select only target-bearing diff
rows; overlay rows and editor rows cannot become targets.

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
- Test TUI cursor visibility, non-commentable rejection, inline composer
  edit/cancel, both deletion keys, successful submit, failure draft retention,
  inline overlay placement, refresh, and pane/tab isolation.
- Test source comment-list parsing, bounds, exact-target filtering, POST
  response parsing, and terminal escaping of all remote comment fields.
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
6. Posted and fetched comments appear inline under the exact selected diff
   line; unmatched, stale, malformed, or oversized remote data never shifts a
   comment to another line or prevents review rendering.
7. Focused tests, race-enabled suite, build, verifier script, and diff check
   pass; a real PR comment remains an authorized manual acceptance check.

## Open Questions

The inline-overlay extension intentionally excludes timeline comments, replies,
batch reviews, multi-line ranges, editing/deleting remote comments, and
persisted comment history.

## UX Amendment: Threaded Replies and Reaction Chips

### Objective

Make reply composition and read-back visually communicate a GitHub-style
thread, while making the finite reaction picker fast to operate from a
keyboard. This changes only ephemeral interactive rendering and local action
state; GitHub request contracts, frozen anchoring, and persistence boundaries
remain unchanged.

### Interaction Contract

1. Choosing `r` in a selected comment's action menu opens a separate,
   rune-aware reply editor beneath that comment box. It is indented farther
   right than its parent and retains the parent stable ID and exact frozen
   anchor. It does not place draft text in the action-menu status line.
2. A successful canonical reply is rendered as a separate indented box beneath
   its parent comment. GitHub only accepts replies to top-level review
   comments: choosing reply on an existing reply resolves and submits to that
   thread root, then renders the canonical response beneath the root. Replies
   retain their original anchor and are never inferred from presentation text.
   Escape discards an active reply draft without a write; failed submissions
   retain that editor and draft.
3. Choosing `a` shows the eight documented reaction values in stable numbered
   order with emoji-first labels: `1 👍`, `2 👎`, `3 😄`, `4 😕`, `5 ❤️`,
   `6 🎉`, `7 🚀`, `8 👀`. An explicit non-UTF-8 locale instead shows the
   bounded GitHub tokens (`+1`, `-1`, `laugh`, `confused`, `heart`, `hooray`,
   `rocket`, `eyes`). Pressing a matching digit selects exactly that value;
   other input and Escape are write-free.
4. Reactions render as compact escaped chips embedded in the bottom border of
   the affected comment box: each distinct value appears once with its current
   ephemeral count. A successful canonical reaction increments only the
   originating tab's matching comment chip. A refresh replaces overlay state.

### Testing Strategy

Use focused TUI tests before implementation for separate reply-editor layout,
rune editing/cancel/failure retention, numbered reaction mapping, and
deterministic bottom-border counts. Snapshot the representative threaded box
only after inspecting its unstyled text. Keep source and application request
tests unchanged except for lifecycle regressions that prove origin-tab
isolation.

### Boundaries

- Always: preserve escaped rendering, exact anchors, tab/generation isolation,
  offline refusal, JSON-stdin writes, and memory-only draft/reaction state.
- Ask first: changing GitHub endpoints, adding reaction persistence, or adding
  a mouse interaction.
- Never: put reply body text in a menu/status line, infer threads from body
  text, let a digit write outside the reaction picker, or alter plain output.

### Success Criteria

- A reply editor and a successful reply are visibly indented relative to the
  selected parent box; the editor remains a distinct text input surface.
- Each picker digit maps to exactly one documented reaction; labels and counts
  remain meaningful with terminal color disabled and with an ASCII locale.
- The bottom border shows deterministic reaction chips and counts without
  exceeding the comment-box width or leaking state across tabs.

## UX Extension: Inline Composer and Comment Overlay

### Why

Manual acceptance proved that the GitHub write succeeds. The full-page
composer breaks the reviewer’s spatial connection to the line being discussed,
and a success toast alone does not provide evidence that the comment landed at
that line. This extension keeps composition and read-back in the diff itself.

### Rendering and Navigation

For the selected target, the view is ordered as follows:

```text
selected diff line
  existing inline comments (zero or more)
  inline composer (only while drafting)
next diff line
```

Comment rows show an escaped author label (when provided) and escaped,
wrapped Markdown source; they do not render Markdown or terminal control
sequences. The editor is visually distinct, retains the selected target label,
and scrolls with the line. Opening, cancelling, submitting, switching tabs,
or scrolling must not change the selected target unexpectedly.

The composer has a rune-aware edit cursor. `Backspace`, `Delete`, left/right,
`Home`, `End`, `Enter`, `Ctrl+Enter`, and `Esc` apply to the draft while it is
active; ordinary diff navigation is suspended until it is submitted or
discarded. Empty drafts cannot be submitted.

### Comment Overlay Data Contract

Add a read-only `ReviewCommentReader` to `internal/source`. Its item type
contains only data needed to render and match a single comment: stable ID,
author login, body, commit ID, path, side, and line. `GH` lists comments using
the documented pull-request review-comment endpoint with explicit request and
response limits. It accepts only comments with a valid exact target; the TUI
then filters them again against the frozen comparison’s head SHA and its own
target map. This double boundary prevents an API response from visually
attaching a comment to a guessed or stale line.

`CreateReviewComment` evolves to return the canonical created comment, which
the TUI appends only after the POST succeeds. A later `c` refresh replaces the
overlay for that tab with the canonical read result. If a read fails, the
review and newly posted local overlay remain usable and the UI shows a bounded,
escaped notice; no read failure blocks a write that has already passed
freshness preflight.

Remote comments are TUI-only, per-tab state. They are never written to session
files, plain output, logs, or error strings. Offline mode performs neither the
initial fetch nor a refresh and shows no synthetic empty-history claim.

### Incremental Delivery Phases

1. Introduce the bounded read/create response contracts and their source tests.
2. Refactor detail rendering into target, comment, and editor rows without
   changing selection semantics.
3. Replace the page composer with the inline rune-aware editor, including both
   deletion keys.
4. Load, refresh, filter, and render the per-tab comment overlay; apply the
   returned POST item immediately.
5. Update help/docs and run an interactive acceptance pass against this PR.
