# Spec: Overview Conversation and Inline Code Context

Status: Approved and implemented on 2026-10-05.

## Objective

Make the PR conversation discoverable while reading the PR's intent, and let a
reviewer understand an inline comment and open its exact location in Files to
read more context or reply. This is one conversation-review workflow extending
`SPEC-pr-conversation.md`, `SPEC-pr-description.md`, and
`SPEC-pr-commit-discussions.md`.

## Assumptions and Recommended Design

- Keep the tab order Overview, Files, Guide, Commits and shortcuts 1–4.
- Put the conversation below the description in Overview. Keep D as a direct
  jump to the conversation, with a visible `Discussions [D]` section shortcut.
- Keep the description frozen and conversation live; label both independently.
- Reuse existing comment actions in Files rather than introducing another
  inline reply composer in Overview.

Alternative considered: add Discussions as the second tab. This makes the
  entry point permanently visible and gives discussion a dedicated surface,
  but separates the description from its responses and requires a fifth tab
  or changed numeric shortcuts. Prefer the combined Overview unless review
  shows that long descriptions make the conversation too difficult to reach.

## Existing Behavior

`pageDiscussions` is an overlay reached through D. The conversation contains
general PR comments, submitted reviews, and individual inline comments/replies,
ordered chronologically. Enter opens detail. Historical diff snippets appear
after comment bodies; `o` opens the original captured commit. There is no
equivalent action that selects the comment in Files.

Overview currently renders a frozen Markdown description with its own scroll.
Files already renders comment cards with stable comment IDs and supports cursor
restoration and inline actions. Existing conversation refresh, partial-read,
freshness, draft, delivery, and escaping contracts remain authoritative.

## Overview Interaction Contract

1. Overview contains the description followed by a `Discussions` section and
   chronological activity cards. Reviews with no body still show their decision.
   Each card shows author, time, kind, body, and applicable thread status.
2. The section entry and D shortcut are visible near the start of Overview,
   even when the description fills the viewport. D from any review tab selects
   Overview and reveals/focuses Discussions. Repeated D preserves the selected
   event; it does not reset reading position or refresh.
3. Overview uses one vertical document. Arrow keys/j/k scroll; Tab/Shift+Tab
   move between interactive cards/actions and reveal the focused item. Enter
   opens focused activity detail; Escape returns to its card. Home/End and
   page scrolling work across the whole document. Mouse targets perform the
   same actions. Numeric tab selection and v/V retain their existing behavior.
4. Discussion controls apply only while that section/detail has focus. In
   discussion focus, n creates a general PR comment and c refreshes explicitly;
   r on a general PR comment keeps the existing @mention reply behavior. The
   footer advertises actions appropriate to the selected activity. Editors
   consume their own keys, including tab shortcuts, as today.
5. Switching tabs preserves Overview scroll, focus, expanded detail, and stable
   activity selection. Refresh retains these by identity where possible; if
   removed, select the nearest remaining event deterministically. PR replacement
   resets transient navigation and never transfers drafts to another PR.
6. Reuse the current review's discussion loading lifecycle. Rendering, focusing,
   or switching tabs does not start another read. c performs explicit refresh;
   there is no polling. Loading, unavailable, stale, partial, offline, and
   verified-empty states remain distinct and appear in the section itself.
7. Distinguish `Description · captured when review opened` from
   `Discussions · live PR activity`. Counts on partial reads say loaded activity;
   unavailable data never becomes a zero count or an empty conversation claim.

## Inline Code Context

Each inline activity card places a compact code block immediately before its
comment body. Replies repeat compact context so their chronological position
remains meaningful; detail shows the complete loaded thread in reply order and
selects the originating comment. General PR comments and reviews have no code
block or Files action.

Prefer an exact current anchor in the captured PR comparison when verified and
not outdated. Show path, side, line or range, line numbers, diff markers, and a
textual marker on the commented line(s). Show up to three captured diff rows
before and after the target; do not invent unchanged source outside the capture.
File-level comments show the path and `File comment`, without selecting an
arbitrary line. Detail can show a larger bounded excerpt from the same capture.

If current context cannot be verified, use exact original captured commit
context where available. Otherwise show the supplied bounded historical hunk,
labeled `Historical snippet`; mark target lines only when its coordinates can
be established. Never represent that snippet as current source. If no context
is available, show `Code context unavailable` and retain comment text and link.

Resolved, outdated, retained/stale, and comparison mismatch are independent
labels. Deleted/renamed paths, left-side deletions, ranges, and missing commits
must not relocate using text matching or guessed offsets. Source follows the
existing diff rendering, syntax coloring, and terminal escaping rules. Narrow
terminals stack code and comment vertically, preserving path/line identity and
the target marker without requiring color.

## Open in Files and Return

- Show `Open in Files` on inline cards/detail when the verified current anchor
  and selected comment exist in the captured Files diff. Proposed shortcut:
  f while discussion has focus, subject to a binding conflict check.
- Select Files, the exact path, the appropriate diff side, and the exact comment
  ID (including a selected reply). Reveal its card and focus the diff. Temporarily
  reveal a hidden thread or remove a conflicting file filter when necessary;
  preserve that prior navigation state for return. Honor the active diff layout.
- Use existing Files reply/thread actions after navigation; navigation alone
  does not open a composer or submit anything. Existing freshness and permission
  checks govern every subsequent write.
- Escape returns to the originating Overview activity after inner overlays or
  editors close. Restore Overview position and the prior Files navigation state.
  Explicit tab navigation abandons this temporary return path, leaving normal
  tab switching intact. Nested jumps must not accumulate a navigation stack.
- When unavailable, retain a disabled action with a specific reason such as
  `Outdated; current Files location unavailable` or `Comparison differs`.
  Keep `View original commit [o]` when the original commit is captured, preserving
  its existing return behavior. Never substitute an original commit for Files
  silently. General activity offers neither source-navigation action.

## Illustrative Layout

```text
Overview [1]  Files [2]  Guide [3]  Commits [4]
Description · captured when review opened     Discussions [D] ↓
...PR description...

Discussions · live PR activity
alice · Inline comment · timestamp · Unresolved
src/parser.go · RIGHT · line 42 · captured PR diff
   41    value := parse(input)
>  42  + return value.Name
   43  }
Could value be nil here?
Open in Files [f] · View original commit [o]
```

## Technical Scope and Code Style

Go 1.26.8; existing Bubble Tea v2.0.9, Lip Gloss v2.0.4, and Markdown/diff
renderers. No new dependencies, API endpoints, persistence schema, or polling.
Fetched discussion content stays outside frozen snapshots, guides, and AI inputs.

Likely source boundaries: `internal/tui/model.go` for Overview/tab dispatch;
`internal/tui/discussions.go` for activity identity, context resolution and
navigation; `internal/tui/bindings.go` and mouse routing for discoverability;
existing diff rendering/cursor helpers for exact comment selection. Tests stay
beside source with screen fixtures under `internal/tui/testdata/screens`.
README and affected older specs must reflect the final approved navigation.

Follow Go formatting and existing typed source targets. Resolve identity before
rendering; keep presentation pure and navigation mutations explicit. For example,
the existing identity-based cursor helper is preferable to display-row offsets:

```go
func (m *Model) restoreCursorAnchor(target *source.ReviewCommentTarget, commentID int64)
```

## Testing Strategy and Commands

Use deterministic Go model/key/message tests and fake readers; no tests contact
GitHub. Cover current left/right anchors, replies, ranges, file-level/outdated
threads, missing historical context, retained events, mismatched comparisons,
hidden threads/filters, navigation return, drafts and PR replacement. Add wide,
narrow, and color-disabled screen coverage and a Bubble Tea journey from
Overview through Files reply actions and back. No new coverage threshold.

Implementation validation commands (not required for this prose-only draft):

```sh
go test ./internal/tui ./internal/source ./internal/session ./internal/review -count=1
./scripts/verify.sh
git diff --check
```

## Boundaries

- Always: preserve existing write preflights, uncertain-delivery handling,
  stable identities, bounded reads/rendering, escaping, and offline behavior.
- Ask first: change numbered shortcuts, add network fetching for missing source,
  expand posting capabilities, persist live content, or add dependencies.
- Never: infer a writable anchor from a historical snippet, auto-post on
  navigation, hide outdated/unplaceable discussions, or weaken write checks.

## Success Criteria

1. A reviewer opening Overview can discover Discussions without knowing D and
   reach it directly despite a long description.
2. Description and conversation share Overview; existing four tab shortcuts work.
3. Inline activity shows correctly labeled code beside its comment in reading
   order, with a clear target marker or an explicit context-unavailable state.
4. Open in Files selects the exact comment/reply and exposes existing reply
   actions; Escape restores the originating activity and prior Files position.
5. Unsupported or stale locations remain readable and never jump to guessed code.
6. Refresh, narrow layouts, tab changes, editors, and PR replacement preserve
   the identity, freshness, and draft guarantees in the existing specs.

## Decisions for Review

Approved: the combined Overview design versus a separate second Discussions tab;
the compact chronological inline cards (with full thread in detail); and the
proposed f/Escape navigation behavior. Implementation follows the approved scope; see
`tasks/overview-conversation-plan.md` and `tasks/overview-conversation-todo.md`.

## In-feed composition

- `n` opens a PR-wide comment composer after the final activity in Overview and scrolls to the bottom. `r` on a PR comment opens a new PR-wide message prefilled with an author mention; it does not create a code-thread reply.
- `r` on an inline comment with verified current context opens the existing code-thread reply composer beneath that activity. Replies target the canonical root and use the existing preflight, private draft, and submission handlers.
- Composing preserves the conversation feed and reveals the editor as it grows. Escape cancels in place. Successful PR comment submission selects the posted message in the feed rather than switching to detail.
- Inline replies requiring unavailable or outdated current context remain unavailable; Files navigation remains a separate explicit action.

## In-place discussion expansion

Enter toggles the selected discussion card between its collapsed presentation and expanded Markdown plus loaded thread content. Description and other discussion cards remain in the same continuous feed. Expansion is stored independently by activity identity; Tab and cursor movement preserve it. Escape collapses the selected expanded card. Expanding retains viewport and cursor position, and collapsing clamps a cursor inside the card to its remaining rows.
