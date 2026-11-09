# Line-anchored agent conversations

Status — approved on 2026-11-04; inline editor mode refinement approved on 2026-11-05. Human terminal review remains to be recorded.

## Objective

During guided PR review, a reviewer can select one or more diff lines, ask the
model configured for guides a question, inspect its answer beside the diff, and
ask follow-up questions without losing their review position. Files and Guide
share this interaction. Success means useful answers grounded in the selected
hunk and existing captured evidence, with missing evidence made explicit.

This specification describes one conversation feature, including the source
selection needed to start it. Repository search is a subsequent extension.

## First-pass decisions

- Use the current confirmed guide provider, model, and destination. A saved
  guide's historical model does not override the reviewer's current selection.
- Make one non-streaming model request for each explicit Send action. Show a
  pending state immediately and allow cancellation while the reviewer reads.
- Retain conversations in memory for the lifetime of the open review tab.
  Document the lifetime in the reference; keep the inline composer minimal.
- Support multiple anchored threads per comparison and follow-ups within each
  thread. Opening Ask agent on an identical normalized anchor reopens its thread.
- Use only the canonical PR comparison in Files and Guide for this first pass.
  Historical commit views, derived commit subsets, and expanded source rows
  without canonical patch coordinates show an unavailable reason.
- Treat answers as private review assistance. Publishing to GitHub remains an
  explicit action through the existing comment workflow.

## Source selection

### Shared anchor contract

An anchor contains the comparison identity, pinned revision, path, side, and
inclusive start/end lines. A single-line anchor has equal endpoints. Resolve
these values from canonical patch targets. Screen coordinates are used only to
hit-test a target. Guide repetitions resolve to the same canonical anchor.

Ranges must share a comparison, path, side, and hunk, with contiguous captured
source coordinates. Normalize reverse selections. A paired deletion/addition
row in split view has distinct LEFT and RIGHT targets. The selected column
determines the side. Headers, blank split cells, hunk markers, thread cards,
binary rows, unavailable rows, and extra full-file context cannot become anchors.

Use the existing range validation for both questions and GitHub comments.
An invalid selection gives a visible reason and cannot open a targeted composer.
Reject an attempted cross-boundary extension while retaining the last valid
selection. Do not silently clamp it to a different file, hunk, or side.

### Keyboard and mouse

- Ctrl+V starts a range from the current canonical target. Existing diff movement
  extends the endpoint. Ask agent consumes the displayed range. Ctrl+V again or
  Escape clears range mode. A single cursor target remains sufficient to ask.
- Clicking a canonical source line selects it and establishes a fixed range
  origin. Shift-clicking another valid line selects the inclusive range from
  that origin. Further Shift-clicks adjust only the endpoint. A plain click
  establishes a new origin. Shift-click with no origin selects a single line.
- Pressing the left mouse button on a canonical source row and dragging over
  valid rows previews an inclusive range. Release commits the selection. A press
  and release on one line selects one line. Both forward and reverse dragging
  work, starting in either the source text or its line-number gutter.
- Preview and committed selection highlight all selected source cells and show
  a textual side/line-range label. Wrapped display fragments map to their one
  source line. Horizontal scrolling and narrow unified fallback preserve anchors.
- Scrolling between the first click and Shift-click preserves the origin.
  Dragging is bounded to the visible viewport in this release. Moving outside
  the pane retains the last valid endpoint without automatic edge scrolling.
- Resizing, switching tabs/views, opening a modal, or receiving a keyboard action
  during a drag cancels the unfinished gesture and restores the prior committed
  selection. A source comparison change clears selection. Completed selection
  survives ordinary scrolling and layout changes when its targets remain valid.
- Terminal applications receive only mouse events delivered by the terminal.
  Document the keyboard fallback when a terminal reserves Shift gestures for its
  own selection. Verify Shift-click in a terminal that forwards modifiers.

### Clipboard interaction

Today source text dragging participates in the app's release-to-copy behavior.
For canonical diff source rows, dragging selects source lines
and adds an explicit Copy selected lines action. Copy uses the selected side's
source text, without diff markers or line numbers. Releasing a range gesture
does not emit clipboard output. Text dragging in prose and other existing text
surfaces retains its existing copy behavior. Divider dragging keeps priority
within its own hit region. Expose selection and Copy in Actions and help.

This mouse behavior change is approved for the first pass.

## Conversation flow

1. Select a line or range in Files or Guide and press Enter to open the shared
   inline editor. Tab changes from the default Comment mode to Ask agent as
   the second option, before eligible Suggestion and File comment modes. The
   editor displays only the selected model ID in the box's top border.
2. Enter a nonblank question. Enter explicitly sends this turn with the
   bounded captured evidence and prior turns. Escape closes the composer.
   Closing and reopening within the tab retains unsent text.
3. Show pending state on the anchored thread. Allow continued review, tab
   switching, and explicit Cancel. Permit one active request per review tab.
   A second Send is disabled until the active request finishes or is cancelled.
4. Display the completed answer in a collapsible Agent Q&A card below the
   anchor's ending row. Its placement identifies the source anchor; the card
   title shows the pinned recipient without repeating side or line numbers.
   Each turn shows a styled Question: block followed by a separate answer
   block. Keep the reviewer's viewport stable when a response arrives away
   from the thread.
5. Inspect source references or enter a follow-up in the same thread. The
   follow-up editor sits immediately below that thread's answer card. A card
   under the diff cursor has a solid focused border so its keyboard target is
   clear. Closing an evidence view returns focus and scroll position to the
   thread. Collapsing the thread restores the source review position.

The inline Ask agent mode uses Enter for explicit Send and Shift+Enter for
a newline, like the existing comment editor. The mode is visibly distinct
from GitHub comments; Ask agent drafts never enter
GitHub submission or durable comment-draft paths. The Q&A thread uses the
existing inline comment card framing and readable per-turn grouping. The
separate Ask agent action and shortcut are retired by this refinement.

The model selection is pinned for a thread so follow-up history has one recipient.
If the configured guide selection changes, existing threads show their original
recipient and new threads use the new selection. A changed recipient requires
starting a new thread with explicit Send consent. Credentials are resolved at
send time and held only in memory.

## Request material and evidence

Build a frozen request manifest before sending. It contains the question, anchor,
selected hunk with selected lines marked, eligible evidence excerpts, bounded
thread history, and a ledger of omitted material. Source excerpts include stable
IDs, revision, blob ID where available, path, side where applicable, and line
ranges. Treat questions and source as data, and source content as untrusted
evidence. The prompt asks for an answer grounded in this manifest and a specific
description of any missing evidence needed to answer confidently.

Reapply the existing privacy policy to every turn, including historical material.
Honor whole-file exclusions already captured in the omission ledger. Local
source-navigation consent does not authorize uploading cached full files.
Exclude review drafts, unrelated conversations, and locally expanded source.
An excluded selected hunk blocks Send with a sanitized reason.

Select at most three existing `Snapshot.Context` excerpts. Rank same-path,
overlapping-line evidence first, then evidence sharing exact identifiers with
the question or selection. Use evidence ID as the deterministic final tie-break.
Discard duplicates and unrelated candidates. Preserve each excerpt's revision
so an old-side question never presents new-side evidence as old source.

Proposed initial limits are 32 KiB of source, 8 KiB for the current question,
16 KiB of prior history, a 128 KiB serialized request ceiling, 32 KiB of answer
text, and a 60-second deadline. These are application bounds and must also fit
the provider transport's existing limits. Include the latest complete prior
turns that fit. Show when earlier turns are omitted. Do not summarize omitted
history with another model call.

Prioritize the full selected hunk, then evidence. If the hunk alone exceeds its
source budget, preserve every selected line and add nearby canonical rows up to
the limit, explicitly marking omitted ranges. Block sending if the selected
lines themselves cannot fit. Bound UTF-8 text without splitting code points.
When history quotes previously supplied source, include it in the source budget
conservatively or omit that prior turn. Never silently truncate the question.

The first pass performs zero new repository searches, checkout reads, source
fetches, or model-directed tool operations. Sparse or absent captured evidence
is a supported state. The model must distinguish its inference from the supplied
source and describe uncertainty rather than claim repository-wide coverage.

## Answers and citations

Use a bounded response containing Markdown answer text and references to supplied
evidence IDs. Reuse the pinned provider library's structured-response facilities
where supported. A rejected schema, refusal, malformed result, or unsupported
model produces a recoverable error without an automatic repair request.

Resolve source links exclusively through the request manifest. Validate any
requested line subrange against that excerpt. Unknown IDs and out-of-range
references appear as unavailable references and never navigate to invented
source. Citation validity establishes provenance, not correctness of the answer.
Inspection displays the exact excerpt sent, including its revision and omissions.
Apply the existing safe Markdown and terminal escaping rules to answers, errors,
paths, and evidence views.

## Lifecycle and failure behavior

Each async request carries tab, comparison, thread, and request-generation IDs.
Cancellation invalidates its generation immediately. Late results are discarded.
Switching tabs may allow the originating request to finish in its own tab.
Closing a tab cancels its work and discards its conversations. Replacing the
comparison cancels pending work and discards its threads after warning about the
in-memory lifetime. An unchanged comparison retains threads and selections.

Failures retain the question for explicit retry. Retry creates a fresh request
generation and cannot duplicate a completed answer. Disable automatic provider
fallback and application retries. Offline mode blocks provider operations before
client or credential access. Missing credentials leave review usable and show
the configured provider's setup error. Logs and persisted snapshots exclude
questions, answers, request payloads, and credentials.

## Integration and project structure

Use Go 1.26.10 and the pinned Bubble Tea v2.0.9, Fantasy v0.41.3, and existing
Markdown rendering dependencies. Keep the conversation state in its feature
owner and the root responsible for workspace coordination.

| Area | Responsibility |
| --- | --- |
| `internal/tui` | Semantic selection, input routing, tab-owned conversation state, rendering, async result guards |
| `internal/source` and `internal/tui/comment_targets.go` | Existing canonical target and range validation |
| Proposed `internal/agentchat` | Immutable request manifest, evidence ranking, limits, answer contract and validation |
| `internal/guideconfig` and provider wiring in `cmd/prui` | Reuse confirmed guide selection and transport protections for a separate answer service |
| `internal/context` and `internal/privacy` | Existing evidence representation and upload filtering |
| `docs/spec`, `docs/REFERENCE.md`, `TESTING.md` | Requirements, eventual interaction documentation, verification evidence |

Keep state owned by the conversation feature and pass narrow immutable inputs.
Avoid forcing a question through guide generation's immutable-bundle lifecycle.
Provider failures return errors to the feature and preserve source review.

Use gofmt, descriptive names, explicit errors, and package-local tests. This
existing target-validation style illustrates the intended early checks.

```go
start := *m.rangeStart
if start.Identity != end.Identity || start.CommitID != end.CommitID || start.Path != end.Path || start.Side != end.Side {
    return end, errors.New("range endpoints must share the pinned file and diff side")
}
```

## Boundaries

Always preserve canonical source provenance, explicit per-turn Send consent,
privacy filtering, terminal escaping, comparison ownership, and offline guards.
Run the applicable repository checks before landing implementation.

Seek a scope decision before adding persistent transcripts, provider-session
storage, dependencies, automatic source retrieval, streaming, or GitHub publishing
from an answer. These would change the agreed first-pass contract.

Never execute model-supplied commands, let source content authorize operations,
invent comment anchors, or include credentials in rendered or durable data.

## Verification and acceptance

Use Go tests beside the implementation with fake providers and local `httptest`
endpoints. Cover observable input/output behavior and asynchronous event ordering.
Provider tests use fake credentials. Record human terminal checks separately.

- Keyboard, forward/reverse drag, and click/Shift-click yield the same inclusive
  canonical range in unified, split, and narrow layouts. Test old-side deletions,
  wrapped lines, scrolling, cancelled gestures, and invalid cross-boundary input.
- Source-range release emits zero clipboard output. Explicit Copy returns the
  chosen side's lines. Prose copying and divider drags retain their intended routing.
- Asking on a normalized existing anchor reopens its thread in Files or Guide.
  Two different anchors retain independent questions, answers, and history.
- Request fixtures account for every outgoing field. Excluded content, oversized
  selections, missing evidence, and history limits produce deterministic outcomes.
- Each Send makes at most one provider request. Cancellation, retry, tab switch,
  resize, and comparison replacement cannot duplicate or misroute an answer.
- Zero requests occur before Send or in offline mode. Existing guide/progress
  state and GitHub submission counts are unaffected by asking and cancelling.
- Supplied citations open their exact evidence. Fabricated references, refusal,
  malformed output, timeouts, and unsafe terminal content fail safely and visibly.
- A manual terminal review covers a hunk-only question, a question answered by
  captured test evidence, and a caller question whose evidence is unavailable.
  Verify scrolling stability, narrow layouts, real mouse input and Shift delivery,
  clipboard behavior, cancellation, and resuming guided review.

Run from the repository root after implementation.

```sh
go test ./internal/tui ./internal/context ./internal/privacy ./internal/guide ./internal/guideconfig ./cmd/prui
go test ./internal/agentchat
go build ./...
sh scripts/verify.sh
golangci-lint run ./...
go run ./cmd/prui sessions
```

The `internal/agentchat` command applies once that package exists. The sessions
command is an interactive entry point for the manual review journey.

## Repository search follow-up

Collect representative questions that could not be answered because a definition,
caller, or test was absent. A later spec can add bounded literal search at a
pinned revision and bounded source-range reads, returning evidence through this
same manifest and citation contract. That extension needs explicit source-scope
consent, per-turn tool/time/byte budgets, and comparison against the missed
questions for answer quality and latency.

## Review points

The diff drag/copy behavior, in-memory conversation lifetime, non-streaming
responses, and initial request budgets are approved for the first pass.
The 2026-11-05 inline editor refinement replaces the separate Ask agent action
and shortcut with Ask agent as the second Tab mode. The later readability
refinement removes anchor side, destination, upload, scope, lifetime, and
keyboard instruction rows from the composer. It shows only the model ID in
the box's top-right border; the bounded request contract remains unchanged.

Related requirements live in `CONSTRAINTS.md`, `SPEC-code-navigation.md`,
`SPEC-split-diff-pane-cursor.md`, and `SPEC-MULTI-PROVIDER-GUIDES.md`. The Q&A
send and mouse behavior contracts are recorded in the authoritative docs.
