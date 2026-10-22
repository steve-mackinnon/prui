# Spec: TUI feature ownership and coordination

Status: Approved by the maintainer on October 21, 2026. Written against `bd5a70d`.

## Objective

Migrate prui's terminal UI from features implemented as methods on one broad
`*Model` to features that own their state and expose small interfaces. Keep one
Bubble Tea root model to coordinate input, navigation, layout and asynchronous
results. A feature must be understandable and testable without constructing the
root or obtaining access to another feature's mutable state.

This is one behavior-preserving architecture migration across existing
capabilities. It introduces no new product capability. Existing feature specs,
[CONSTRAINTS](../../CONSTRAINTS.md), and source/storage/write policies remain
authoritative. The [feature removal audit](../FEATURE-AUDIT.md) is a separate
proposal; none of its removals are authorized by this spec.

Success is explicit ownership and a smaller interface between features and the
workspace. File count or line reduction alone does not establish success.

## Assumptions and scope

- Preserve current commands, shortcuts, mouse behavior, wording, layouts, themes,
  progress, offline behavior, source provenance and all existing features.
- Preserve stored formats and compatibility with existing sessions and drafts.
- Keep Go 1.26.9 and the pinned Bubble Tea v2.0.9 and other existing dependencies.
- Keep service implementations and external-effect authorization in `cmd/prui`
  and their existing domain packages. This migration changes UI ownership and
  wiring, not GitHub policy or Git/source acquisition behavior.
- Do not add an event bus, global registry, dependency-injection framework,
  generic screen framework, or a separate Bubble Tea program for each feature.
- Do not move capture work to a different time, change background refresh policy,
  change limits, or redesign the UI as part of extraction.

## Current problem

`internal/tui` contains 19,097 physical production Go lines across 69 files.
Features generally receive all root state implicitly through `*Model` receivers.
The root embeds `*reviewTabState`, exposing active-tab fields directly to every
method. The central model and UI lifecycle files contain 3,917 lines combined.

Feature-specific state types already exist, but most behavior remains on the
root. Event routing, feature logic, rendering, persistence coordination and
cross-view navigation are interleaved. Async work already has important tab and
generation guards. Draft persistence assembles data across editors and must
remain durable before a remote request is attempted.

Existing shared modules are useful starting points: draft editing, diff rendering,
pane framing, modal presentation, viewport composition and semantic cursor
restoration. Extraction must retain their guarantees rather than recreate them
independently in each feature.

## Target architecture

```text
Bubble Tea messages
        |
        v
Root coordinator: workspace, tabs, focus, navigation, layout, routing
        |
        +--> feature input --> feature-owned state --> rendering/result/action
        |
        +--> authorized effect --> tea.Cmd --> typed completion --> origin owner

Features --> shared UI primitives and small protocol types
Features --> injected service callbacks/interfaces --> existing application layer
```

The coordinator owns the application shell. Features own interactions and local
state. Ordinary local interactions use direct method calls. Cross-feature changes
use typed actions returned to the coordinator. Async work uses Bubble Tea messages.
No publish/subscribe mechanism is needed for communication among UI features.

### Dependency direction

1. `cmd/prui` constructs the root and injects existing authorized operations.
2. The root imports feature packages and composes their output.
3. Features import domain types, shared primitives and small neutral UI protocol
   types where necessary. They cannot import the root or sibling feature packages.
4. Shared primitives cannot import the root or features. A primitive can depend
   on relevant domain types when needed, such as frozen diff anchors.
5. Existing domain/service packages must not acquire a dependency on UI features.

Use concrete feature types for composition. Go's implicit interface satisfaction
is used for capabilities a feature actually consumes, not to create a universal
interface for every screen. A feature need not implement `tea.Model` or expose
an `Update(tea.Msg)` method. Explicit methods are suitable when they make routing
and state changes clearer.

## Ownership contract

| Owner | State and responsibility |
| --- | --- |
| Root workspace | Stable tab identities, active tab, application lifetime, terminal capabilities, workspace geometry, navigation and modal stack, shared preferences, input ownership and command dispatch. |
| Review tab coordinator | Comparison identity, references to immutable captured data, instances of tab features, progress coordination, and composition of their presentations. Owns no duplicate selection/editor/loading state. |
| Files and Guide | Their independent selections, scroll/cursor anchors, filtering, search presentation, context expansion and view-specific caches. Shared diff mechanics belong in reusable primitives. |
| Commits and incremental review | Their local selection, comparison projection, reading positions and historical presentation. Canonical PR source and eligibility remain governed by existing domain policy. |
| Overview and discussions | Activity selection, expansion, discussion overlays, local refresh state and navigation intents. Pass explicit discussion/anchor data to other owners. |
| Review writing | Inline/range/file/suggestion composition, pending comments, replies, review summary/decision, published actions and their local submission state. The package may contain multiple focused types. |
| Draft coordination | Existing persisted draft composition, generations, recovery, attempt records and save barriers. Collects explicit contributions from writing owners; does not reach into their fields. |
| Auxiliary features | Readiness, PR lifecycle controls, issue context, inbox, PR/repository/saved-session selection, guide consent, theme selection, help and Actions own local interaction state. Their workspace/tab lifetimes remain consistent with current behavior. |
| Shared primitives | Local text editing, list/viewport mechanics, modal framing, diff rendering and presentation utilities. No GitHub actions, sibling navigation or persistence policy. |
| Existing application services | Source capture, validated remote reads/writes, storage transactions, privacy/consent enforcement and exact-revision preflight. |

These are responsibility groups, not a requirement to create one package per row
or per file. Files and Guide may share a cohesive diff workspace implementation
while retaining separate state. Discussion reading and writing may be one cohesive
feature where splitting would introduce mutual dependencies. Choose a package
only when ownership and a small interface are clear; cycles indicate a bad split.

Every mutable field must have one declared owner. Other modules request changes
through that owner's methods or through a coordinator action. Do not expose
pointers to feature state, sibling instances, or mutable maps/slices through an
interface. Shared captured source is read-only by contract and must not expose
the mutable session/progress record as a general feature host.

Read inputs should be the smallest useful projections. Do not copy entire large
inventories per event merely to prevent mutation: use immutable views/accessors
and explicit ownership of underlying data. Copy mutable request payloads and draft
contributions wherever necessary to preserve their identity after dispatch.

## Interface and code style

Use existing Go naming, `gofmt`, concrete domain types and small consumer-owned
interfaces. For example, a feature can declare the following optional capability:

```go
// Declared in the consuming feature package.
type NoticeHost interface {
	ShowNotice(string)
}

func (f *Feature) NextMatch(host NoticeHost) {
	if len(f.matches) == 0 {
		host.ShowNotice("No matches")
		return
	}
	f.selected = (f.selected + 1) % len(f.matches)
}
```

The coordinator can pass itself if it satisfies this interface. The interface
must not return `*tui.Model`, a broad shared context, `*review.Session`, or an
unrestricted state getter/setter. The example illustrates interface ownership;
it does not prescribe final names or require notices to be shared globally.

Pass dimensions, palette, focus and review projections as explicit values where
practical. Inject service functions at construction or a defined rebind point.
Never retain a callback that resolves "the active tab" later to identify the
origin of an already-started request.

Cross-feature intents use concrete typed payloads, such as opening a file at a
frozen anchor. Prefer a feature-owned result structure with explicit fields or a
closed set of action variants over string names and `map[string]any`. Do not
force every feature into a large common result/interface just for uniformity.

## Input, actions and rendering

- The root resolves input ownership once: active modal/editor before ordinary
  view input, then the relevant feature and existing global controls. An input
  handled by an owner cannot also activate another feature's shortcut.
- Keyboard paste remains literal text and never becomes action dispatch.
- Mouse events use the allocated region and existing hit-testing rules. A feature
  receives local coordinates or an explicit rectangle, rather than reading
  another feature's geometry.
- Resize, theme and relevant data notifications may reach multiple features;
  ordinary key events must not be broadcast indiscriminately.
- A direct local method may mutate only its owner's state. It returns commands
  and/or typed actions when coordination is needed.
- The coordinator validates action context and asks the destination owner to
  perform a transition. No feature mutates a sibling directly.
- Local navigation actions are handled synchronously and in a defined order.
  Do not use `tea.Cmd` merely to deliver a local action on a later event-loop turn.
- Rendering performs no I/O or changes to progress, drafts, navigation or source.
  Feature-owned bounded presentation caches may be maintained without mutating
  shared source or changing observable behavior.

Example: Discussions returns an action containing comparison identity and the
selected raw anchor. The coordinator selects Files, asks Files to reveal that
anchor, and retains the return location using existing navigation semantics.
Resizing or changing theme cannot reinterpret the anchor as a rendered row index.

## Asynchronous work and lifetime

The UI event-loop goroutine remains the owner of UI state mutation. Commands
capture immutable inputs and invoke injected operations; they must not mutate a
feature, root, shared session, map or slice from a worker goroutine.

Every completion carries enough identity to distinguish its originating stable
tab instance, frozen comparison, operation and request generation. A tab's numeric
position alone is insufficient because tabs can be closed, reordered or replaced.
Request generations must not collide after a feature instance is replaced.

The coordinator routes a completion to its origin even when that tab is inactive.
The owner rejects an obsolete read generation or mismatched comparison. Focus
changes alone do not retarget, restart or discard valid work. Closing or replacing
an owner cancels its reads and invalidates future presentation updates.

Remote writes require additional care: discarding a stale presentation result
does not establish that the write did not happen. Durable attempted-request and
uncertain-delivery records must survive owner closure, cancellation, comparison
replacement and tab switching under the existing contract. Results cannot update
an unrelated tab or clear a newer draft. Do not automatically retry a write or
weaken outcome checks to simplify ownership.

Keep a small shared request-identity helper only if multiple extracted features
demonstrably need the same mechanism. Read cancellation, remote delivery outcomes
and feature-specific freshness policies must not be collapsed into one generic
"success/error" state.

## Drafts, progress and compatibility

Features export typed draft contributions and accept validated recovery data.
Draft coordination assembles those contributions into the existing durable
format. Exported mutable payloads must be independent of later editor mutations.
All existing recoverable text, pending comments, immutable attempts and generation
checks remain supported.

Before dispatching a remote write, the current attempted payload must pass the
existing durable save barrier. A feature cannot bypass this by returning a write
command directly to Bubble Tea. The coordinator/draft owner authorizes dispatch
only after the required save succeeds; failure leaves the request unattempted.
Existing local-save ordering may remain synchronous where required; moving it
to async I/O is a separate change requiring a preserved barrier.

Canonical progress changes go through its designated owner/service. Features
cannot mark a file read merely by rendering it or changing views. Independent
Files/Guide/commit reading positions remain independent, and existing progress
carry-forward and new-comparison rules remain unchanged.

No database migration, new on-disk configuration, draft version change or silent
data cleanup is needed. UI owner identities are process-local; existing durable
comparison and draft keys retain their current meaning.

## Project structure

The following describes package roles; concrete feature names can be settled
during planning without changing these dependency rules:

```text
cmd/prui/                      Existing application composition and effect policy
internal/tui/                  Root Bubble Tea model and workspace coordination
internal/tui/protocol/         Minimal shared identities/actions only where needed
internal/tui/features/<name>/  Feature state, interaction, rendering and tests
internal/tui/components/       Cohesive shared primitive packages and tests
internal/source/               Existing Git/GitHub operations
internal/review/               Existing review orchestration
internal/session/              Existing snapshot/state/draft persistence
docs/spec/                    Architecture contract and existing feature specs
tasks/                        Later feature-specific plan and checklist
```

Avoid a new `common` package that accumulates all state and services. Move shared
request/response types out of the root only as needed to prevent imports back
into it. Temporary type aliases or thin adapters may preserve `cmd/prui` wiring
during migration; final feature packages cannot depend on those root aliases.
Keep `tui.Plain` working independently of interactive feature initialization.

## Migration constraints

This spec defines the destination and verification gates, not an implementation
task list. Planning follows approval in a separate turn.

Start with a bounded feature that exercises local state, rendering and an async
read, such as readiness. Establish package separation, routing and origin-safe
completion handling there before extracting interdependent diff and writing flows.
Use existing shared primitives rather than replacing them wholesale.

Migrate cohesive behavior at a time. Each intermediate state must compile and
pass the relevant integration journeys. A transitional root adapter may delegate
to a feature, but must not maintain a second authoritative copy of its state.
Explicitly identify remaining legacy accesses and remove adapters as their
consumers migrate. Do not claim completion while retaining a broad Model-shaped
host interface, the embedded shared tab-state escape hatch, or duplicate owners.

The completed migration covers every existing interactive capability, including
advanced and auxiliary screens. A smaller pilot is a checkpoint, not completion
of this specification. Feature removal requires a separately approved scope change.

## Testing strategy

Use Go's `testing` package and current synthetic fixtures. New feature tests live
beside the extracted feature and exercise its interface using explicit inputs,
fake service functions and fake small hosts; they do not instantiate the root.
Preserve relevant behavior tests while relocating them. Replace tests of removed
internal representations only with equivalent assertions of behavior/ownership.

Root integration tests retain the composed journeys: input priority, navigation,
tab switching, rendering, draft recovery, progress, command dispatch and stale
result routing. Add structural dependency checks if imports alone do not enforce
the required direction; do not add tests that merely mirror field layout.

Required cases include:

- Independent feature instances in two tabs; interacting with one cannot change
  the other's selection, cursor, draft, progress or loading state.
- A slow request started in A completes while B is active and reaches only A.
- Reordered, closed or replaced tab/feature instances cannot consume old results;
  superseded read generations and comparison identities are rejected.
- An uncertain remote write remains recorded after cancellation or owner change;
  no duplicate write occurs, and no later draft is cleared by its completion.
- A failed draft save blocks a write; successful recovery preserves all currently
  supported draft forms and pending attempts through existing storage fixtures.
- Modal/editor/paste exclusivity, mouse regions, narrow/wide layouts, split-side
  anchors, search selection and active-editor visibility survive extraction.
- Offline/plain operation performs no prohibited network, credential or write
  action; guide consent and exact-head preflight remain unchanged.
- Shared rendering/caches never mutate source or duplicate large parsed streams
  per keystroke. Existing allocation regressions remain meaningful.

Use existing screen goldens unchanged for retained output. Any proposed baseline
change must explain a separately authorized observable change; architecture alone
is not a reason to regenerate goldens blindly.

## Commands and performance verification

Run from the repository root. Focused commands expand to descendant packages as
features move:

```sh
go test ./internal/tui/... ./internal/review/... ./internal/session/... -count=1
go test ./cmd/prui -run '^TestPTYSmoke$' -count=1
go build ./cmd/prui
./scripts/verify.sh
golangci-lint run ./...
git diff --check
```

`verify.sh` runs formatting checks, installer checks, `go vet`, race tests with
PTY coverage, and build. The pinned expanded lint version remains the one in CI.
Retain the other existing repository security/dependency gates; no dependency
changes are authorized here.

Record the current Files scroll benchmark before and after relevant extractions:

```sh
go test ./internal/tui -run '^$' -bench BenchmarkFilesScrollTransition -benchmem -count=3
```

If that benchmark moves, document the equivalent full command. Compare on the
same machine/toolchain and preserve warmed cache behavior. Investigate repeatable
regressions in time or allocations before accepting the extraction; report noisy
measurements rather than claiming a speedup or prescribing an unsupported target.

## Boundaries

Always preserve source/progress/draft/consent/write invariants, one state owner,
the event-loop mutation rule, typed origin identity, package dependency direction,
and meaningful behavior verification. Update ARCHITECTURE and testing commands
when the migration actually changes their structure.

Ask before expanding this migration to remove features, change storage or CLI
behavior, introduce dependencies, or revise the agreed interaction contract.
Routine package naming and adapter placement within these constraints do not
require additional approval.

Never expose the entire root through a feature interface, replace it with a broad
mutable context, import siblings to mutate them, mutate UI state in commands,
weaken preflight/recovery checks, automatically retry uncertain writes, delete
saved work, or suppress checks to make the refactor pass.

## Acceptance criteria

1. The root remains the only application-level Bubble Tea model and owns focus,
   tabs, navigation, layout composition and event routing. Reusable Bubbles may
   retain their existing update interfaces.
2. Each existing feature has an explicit state owner and can be exercised through
   a small interface without constructing or importing the root. No feature
   imports a sibling or receives unrestricted mutable session/workspace state.
3. Extracted feature behavior is no longer implemented as methods on `*Model`.
   Remaining root methods coordinate features or operate the application shell;
   merely moving methods into more files does not meet this condition.
4. Shared primitives contain mechanics, not PR workflow policy. No event bus or
   universal screen/host abstraction is introduced.
5. Cross-feature navigation uses typed actions and owner methods; input routing
   preserves existing priorities and semantic source coordinates.
6. Async completions remain safe across tab switches, reorder/closure, owner
   replacement and new comparisons. Durable write uncertainty and save barriers
   survive these transitions.
7. Existing sessions, drafts, reading progress, commands, features and rendered
   output remain compatible; the composed tests and repository gates pass.
8. Temporary broad adapters and duplicate state are removed. The architecture
   guide records the actual owner/package map and all retained legacy exceptions,
   if any, are explicitly reviewed rather than described as finished migration.
9. Existing cache/allocation protections pass and relevant before/after benchmark
   results are recorded without unsupported claims of line or speed reductions.

## Approval and open questions

There are no unresolved product requirements under the stated behavior-preserving
scope. Exact package splits, names and per-slice file lists belong to the later
implementation plan. Maintainer approval confirms the ownership/dependency
contracts and preservation scope; it does not select any feature-audit removals.
