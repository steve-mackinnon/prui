# TUI approachability audit — 2026-10-15

## Objective and design contract

A first-time reviewer can browse a PR, read its changes, comment, and submit a
review using visible controls and familiar terminal navigation. Advanced tools
remain available through named actions instead of requiring a hotkey vocabulary.
The user's request authorizes audit, issue filing, and delegated implementation.

References: [GitHub review workflow](https://docs.github.com/en/pull-requests/how-tos/review-pull-requests/reviewing-proposed-changes-in-a-pull-request)
and [Linear Reviews](https://linear.app/docs/diffs). Both keep code, discussion,
and review decisions together; Linear exposes secondary settings through menus.
Apply that hierarchy using this app's terminal components and semantic styles.

Assumptions: retain capabilities and expert shortcuts; simplify their discovery
and the common interaction path. Preserve all source, storage, draft, consent,
and remote-write guarantees. No dependencies or persistence changes are needed.

## Evidence

| Finding | Evidence | Impact |
| --- | --- | --- |
| Features require memorized shortcuts | `bindings.go` has a long catalogue; `model.go` defines 21 page states | Readiness, issues, saved reviews, evidence, and display options are hard to find |
| Familiar focus navigation is obscured | Files' arrows already follow focus, but hints emphasize j/k and Tab expands guide rows | A newcomer must discover pane controls before using the existing arrow path |
| Help is a reference dump | `help.go` appends another set of sections after `healthHelpView` | Getting started requires reading advanced controls |
| Help contradicts navigation | Appended help says Commits (3) and filename filter `/`; actual controls are 4 and F | Users follow incorrect instructions |
| Documentation starts with hotkeys | README essential controls has more than 20 entries and first-review steps mix advanced workflows | Common tasks appear more complicated than necessary |
| Primary chrome is crowded | Footer mixes expansion, navigation, help, status, review submission | Important actions lack clear hierarchy |

## Capability map and build order

| Module | Responsibility | Dependencies |
| --- | --- | --- |
| actions | One searchable menu of named existing review actions | Existing handlers |
| navigation | Arrows operate focus; Tab switches panes | Existing pane state |
| guidance | Short contextual hints and beginner-first help/docs | Actions and navigation contracts |

Build actions and navigation in parallel, then integrate guidance and run the
composed journeys. The root agent owns shared menu hooks, mouse access, goldens,
integration, and verification. Subagents own bounded implementation files.

Filed work: [Actions #102](https://github.com/steve-mackinnon/prui/issues/102),
[navigation #103](https://github.com/steve-mackinnon/prui/issues/103), and
[guidance #104](https://github.com/steve-mackinnon/prui/issues/104). Each was assigned
to a separate implementation subagent before integration.

## Actions acceptance criteria

Space or Ctrl+K opens a searchable Actions overlay from an idle review. Type to
filter; arrows select; Enter activates; Escape closes without changing reading
position. Include view selection, submit review, discussions, readiness, linked
issues, saved reviews, new comparison, guide generation, display options, and
evidence. Context-only actions are omitted or labeled unavailable with a reason.
Existing consent and final confirmation remain authoritative. Opening, filtering,
and cancelling never fetch, upload, post, or change progress. Editors, search,
filters, other modals, and busy state keep input ownership.

## Navigation acceptance criteria

In Files and Guide, Up/Down selects list rows when the list has focus and moves
the diff when it has focus (existing behavior, covered by regression journeys).
Tab/Shift+Tab switches list/diff focus in Files,
Guide, and Commits. Enter and Escape retain activation/back semantics. Preserve
horizontal scrolling, Vim accelerators, comment provenance, and editor-specific
Tab behavior. Guide expansion remains available by a named Actions entry.

## Guidance acceptance criteria

Show concise focused-pane guidance and a visible Actions entry. Keep review
submission and severe health warnings visible. Help starts with a short first
review journey, exposing full shortcuts only on explicit request. Correct stale
labels. Narrow tab strips keep every primary view name visible by omitting
shortcut badges; numeric shortcuts remain available. README teaches the beginner
path before advanced reference controls.
Document immediate posting, queueing, review confirmation, and guide upload.

## Implementation and verification

Go 1.26.9, Bubble Tea v2. Source and adjacent behavior tests live in
`internal/tui`; documentation lives in README, docs/REFERENCE.md, and docs/spec.
Reuse existing state/handlers and gofmt conventions, such as
`func (m *Model) openActions()`; avoid duplicate behavior dispatchers.

Use focused failing tests before implementation, then composed input journeys,
wide/narrow/tiny screen assertions, and intentional golden review. Run
`./scripts/verify.sh`, which includes vet, race tests, PTY tests, and build;
run the pinned lint check. No synthetic test contacts GitHub or a provider.
Terminal accessibility and subjective usability still need human assessment.

Never weaken checks, alter immutable source/session schemas, bypass posting
guards, or generate guides without existing consent. The baseline verify run
initially encountered sandbox denial of the shared Go build cache; use a private
temporary cache or run the gate with the required sandbox access.
