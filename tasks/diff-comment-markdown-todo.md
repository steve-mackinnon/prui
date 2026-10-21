# Readable Diff Comment Markdown and HTML Tasks

Plan: [diff-comment-markdown-plan.md](diff-comment-markdown-plan.md).
Status: all implementation tasks pending. File destinations approved by the
user on 2026-10-21; existing `tasks/plan.md` and `tasks/todo.md` are untouched.

## Task 1: Render readable diff comment bodies

**Description:** Route published diff comment bodies through the existing
HTML/Markdown adapter at each card's inner width. Show details content expanded
and preserve formatted output through card assembly. Establish a synthetic
Devin-style regression fixture before changing the rendering path.

**Acceptance criteria:**
- [ ] The fixture hides HTML comments/attributes, formats emphasis/code/links,
  displays summary and detail prose, and turns the picture badge into its linked
  alt label without fetching assets.
- [ ] Body lines fit wide and narrow card interiors, including indented replies;
  HTML within inline/fenced/indented code remains literal.
- [ ] Raw bodies and suggestion parsing remain unchanged; existing safe-input,
  fallback and bounded-cache behavior remain in effect.

**Verification:**
- [ ] Add focused failing card tests, then make them pass alongside
  `go test ./internal/tui -run 'DiffCommentMarkdown|ConversationMarkdown|SelectedComment' -count=1`.
- [ ] Build succeeds: `go build ./...`.
- [ ] Inspect the synthetic card's rendered text at wide and narrow widths.

**Dependencies:** None.

**Files likely touched:**
- `internal/tui/model.go`
- `internal/tui/conversation_markdown.go`
- `internal/tui/diff_comment_markdown_test.go` (new)

**Estimated scope:** Medium: 3 files.

## Task 2: Preserve comment interactions and styling

**Description:** Verify and repair compatibility between formatted body rows,
final painting and the existing comment interactions. Keep changes limited to
the shared comment surface and focused regressions.

**Acceptance criteria:**
- [ ] Every body row retains its comment ID; selection and actions keep the same
  comment and immutable source target in unified and split layouts.
- [ ] Nested replies, reactions, file comments, suggestion previews and historical
  discussion navigation remain attached to their existing identities/anchors.
- [ ] Emphasis/code/link styles survive final painting and selected borders;
  colorless output remains readable without raw control sequences.

**Verification:**
- [ ] Focused tests pass:
  `go test ./internal/tui -run 'DiffCommentMarkdown|SelectedComment|Comment|Historical|ThemeRenderer' -count=1`.
- [ ] Build succeeds: `go build ./...`.
- [ ] Inspect selected/unselected cards in unified and split layouts, including
  one nested reply and one historical comment.

**Dependencies:** Task 1.

**Files likely touched:**
- `internal/tui/comment_surface_test.go`
- `internal/tui/diff_comment_markdown_test.go`
- `internal/tui/model.go`
- `internal/tui/style.go` (only if final painting needs adjustment)

**Estimated scope:** Medium: 3–4 files.

## Checkpoint: Readability and interaction

- [ ] Tasks 1–2 pass their focused checks and builds.
- [ ] Wide/narrow sample output shows formatted prose and expanded details with
  no metadata clutter, truncated borders or incorrect comment actions.
- [ ] Record observed readability and any limitations before further expansion
  of scope; new disclosure behavior is outside this plan.

## Task 3: Verify cache freshness and safe rendering cost

**Description:** Add integration coverage for width-aware cached cards, unsafe
input and repeated overlay assembly. Adjust the existing cache access only if
the card integration requires it; avoid a second rendering cache.

**Acceptance criteria:**
- [ ] Resize, indentation, theme and body edits produce fresh output; shared
  caching remains bounded at 128 entries and 4 MiB including raw/rendered text.
- [ ] Unsafe URLs, terminal controls and encoded controls stay neutralized in
  final cards; malformed HTML retains readable content or sanitized fallback.
- [ ] Unchanged cards reuse rendering work and satisfy the existing overlay
  scroll allocation budget without weakening its threshold.

**Verification:**
- [ ] Focused tests pass:
  `go test ./internal/tui -run 'DiffCommentMarkdown|ConversationMarkdown|DiffOverlay' -count=1`.
- [ ] Build succeeds: `go build ./...`.
- [ ] Inspect card reflow after resizing and switching themes; confirm comment
  selection remains attached to the same ID.

**Dependencies:** Tasks 1–2.

**Files likely touched:**
- `internal/tui/conversation_markdown.go`
- `internal/tui/conversation_markdown_test.go`
- `internal/tui/diff_comment_markdown_test.go`
- `internal/tui/diff_overlay_cache_test.go`

**Estimated scope:** Medium: 3–4 files.

## Task 4: Document and validate the experiment

**Description:** Update the conversation rendering contract to include diff
overlays, review intentional snapshot changes and complete repository checks.
Record evidence and remaining limitations without claiming browser fidelity.

**Acceptance criteria:**
- [ ] The spec and relevant constraints describe formatted diff comments with
  expanded details, local-only conversion and preserved raw-body boundaries.
- [ ] Intentional comment-card snapshots reflect the new presentation; unrelated
  screen behavior and existing checks remain intact.
- [ ] Repository verification passes and actual wide/narrow terminal inspection
  is recorded separately from automated evidence.

**Verification:**
- [ ] Run screen tests; update affected goldens explicitly and inspect each diff:
  `go test ./internal/tui -run 'ScreenSnapshots|ConversationMarkdown' -count=1`.
- [ ] Run `./scripts/verify.sh` (includes `go build ./...`).
- [ ] Run `git diff --check` and review the final change for scope and safety.
- [ ] Inspect the synthetic bot comment in a terminal: wide/narrow cards,
  unified/split layouts, nested replies, selected border and expanded details.

**Dependencies:** Tasks 1–3.

**Files likely touched:**
- `docs/spec/SPEC-conversation-markdown.md`
- `CONSTRAINTS.md`
- Affected comment screen goldens under `internal/tui/testdata/screens/`
- `tasks/diff-comment-markdown-todo.md` (evidence)

**Estimated scope:** Medium: 3–5 files. If more than two goldens need updates,
split snapshot review into another small verification task.

## Completion Checkpoint

- [ ] All task acceptance criteria and checks are complete.
- [ ] Record focused checks, full verification and terminal inspection evidence.
- [ ] Confirm no new dependency, asset fetching, posting/persistence changes or
  per-details UI state entered the implementation.
- [ ] Summarize readability gains and remaining fidelity limits for user review.
