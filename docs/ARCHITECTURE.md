# Code architecture

`prui` is a local Go CLI. The command layer coordinates GitHub metadata,
an isolated view of Git objects, a frozen review snapshot, SQLite storage, and
either plain output or a Bubble Tea interface. The [README](../README.md)
covers installation and day-to-day use; this document describes the code paths
and boundaries for contributors.

## Package map

| Package | Responsibility |
| --- | --- |
| `cmd/prui` | Parse commands, assemble dependencies, control online actions and GitHub writes |
| `internal/source` | Read GitHub metadata through `gh`; build an isolated Git object view and pin a comparison |
| `internal/inventory` | Turn the pinned comparison into deterministic files, review units, and patches |
| `internal/context` | Retrieve bounded repository evidence for optional guide generation |
| `internal/privacy` | Apply path and content exclusions to material considered for upload |
| `internal/guide` | Build bounded guide input, call an optional analyzer, and validate its output |
| `internal/guideconfig` | Resolve the global provider, model, endpoint, and credential variable without reading credentials |
| `internal/review` | Compose a review from source, inventory, evidence, and guides; manage progress and freshness |
| `internal/session` and `internal/session/storage` | Define snapshots and mutable state; persist and validate them in SQLite |
| `internal/tui`, `internal/theme` | Render and operate the terminal workspace and its appearance |
| `internal/verify`, `internal/guideeval` | Produce terminal acceptance artifacts and evaluate saved guides locally |

## Opening a comparison

The main path starts in `cmd/prui/main.go`. It parses the command, opens the
session store, creates the GitHub and Git runners, and chooses plain output or
the TUI. `cmd/prui/lifecycle.go` owns open, resume, fresh-comparison, and
guide-generation actions. `cmd/prui/wiring.go` gives the TUI those actions
as callbacks.

For a new comparison, `internal/review.OpenWithConfig` runs these stages:

1. `internal/source.PinWithTiming` reads PR metadata and creates a private Git
   object view. It verifies the pinned base and head, finds the merge base, and
   fetches missing objects into temporary storage when needed.
2. `internal/inventory.Build` derives stable file and unit identities and the
   patches shown to the reviewer. Incomplete content remains explicit in the
   inventory.
3. `internal/context.Retrieve` collects bounded supporting evidence. The
   optional guide path applies `internal/privacy` policy and builds the final
   upload input in `internal/guide`.
4. `internal/review` assembles file slices from the inventory. The command layer
   saves the resulting snapshot through `internal/session`.

The source view reads object data from the checkout without using its config,
hooks, filters, or worktree. The frozen inventory and description are what a
review session displays; later changes in the checkout cannot silently alter
that session. A new comparison is an explicit operation.

## Sessions and progress

`internal/session` separates immutable `Snapshot` data from mutable `State`.
The snapshot contains the inventory, file slices, captured PR description,
supporting evidence, and any guide bundle. The state contains reviewed slice
IDs, freshness status, and a generation number. SQLite stores reusable source
snapshots and guides separately from session state; state updates use the
generation and snapshot reference to reject stale writes.

`internal/review.Mark`, `Refresh`, and `Resume` handle reading progress and
freshness. Resume loads the saved snapshot, then checks GitHub metadata unless
offline mode is selected. A changed PR is marked stale; it does not rewrite the
saved comparison. A new comparison creates another session.

## Terminal and external actions

`internal/tui` owns navigation, rendering, per-tab display state, and local
comment drafts. It receives storage, loading, guide, and GitHub action callbacks
from `cmd/prui`, so the command layer remains the boundary for external
effects. Plain output uses the same loaded review data without starting the
interactive program.

Each open review tab owns one `reviewTabState`. `Model` points to the active
tab's state, so navigation, drafts, overlays, and review results update that
state directly. Asynchronous results use their origin tab and generation to
reject stale work, including when another tab is visible. Before any tab is
opened, the model has an initial state for loading and browser screens. Window
dimensions, service callbacks, picker data, theme, and the bounded Files render
cache belong to the workspace; the Files cache is invalidated when the active
session changes. Tab switches save a semantic cursor anchor so a later resize
can restore the selected source line.

Main-review and commit diffs share a pure text-hunk renderer. Escaped source
text, raw line coordinates, and frozen comment targets remain separate; each
view owns its headings, numbering, navigation, and posting eligibility. Each
review tab retains source rows for its selected commit independently of the
discussion/editor overlay cache, so typing and cursor blinking do not reparse
patches. Both views share pane-body framing while retaining their own focus,
widths, border styles, and scroll state.

Guide generation requires an explicit user action. `cmd/prui/wiring.go`
resolves one selection from the XDG guide configuration for the interactive
model. That selection supplies the consent identity, analyzer factory, and
guide-cache check. Credentials are read from the selected environment variable
only after confirmation. `internal/guide` applies limits and privacy policy to
frozen patches and evidence before invoking a Fantasy-backed analyzer for
OpenAI, Anthropic, Google, or an OpenAI-compatible endpoint. The HTTP boundary
caps request and response bodies and refuses redirects. A successful guide
creates a derived session that shares the original source snapshot; it does
not change file ownership or the original session's reading progress. Its
non-secret selection fingerprint prevents automatic reuse after a provider,
model, or endpoint change, while old sessions remain readable.

Review comments and review submissions pass through the command layer. Before
a write, it validates the target and compares current GitHub metadata with the
session's pinned revision. It rejects a write if the PR changed. Offline mode
blocks network actions at the same boundary.

PR review discussions are an ephemeral overlay in `internal/tui/discussions.go`.
`internal/source/discussions.go` reads bounded GraphQL thread/comment connections,
retaining original/current anchors and independent outdated/resolved status.
The command layer checks metadata around retrieval; only verified current anchors
can enter the main diff, while original anchors remain readable in captured
commit diffs. Unplaceable records remain in Discussions. Request generation,
session identity and cancellation prevent stale responses from changing a newer
review. REST keeps the explicit write/action boundary. Commit composer targets
are checked against immutable bundle membership and raw patches; historical
RIGHT additions/context are enabled for complete single-parent A/M diffs based on
live API evidence. Historical LEFT/rename/root/merge cases remain disabled. No discussion
record or draft enters session storage or guide input.


## Where to change behavior

- For Git or GitHub acquisition and limits, start in `internal/source`.
- For file identity, patch construction, or incomplete inventory, start in
  `internal/inventory`.
- For guide inputs and exclusions, inspect `internal/context`, `internal/privacy`,
  and `internal/guide` together. For provider selection, inspect
  `internal/guideconfig` and `cmd/prui/wiring.go`.
- For stored data or reading progress, start in `internal/session` and
  `internal/review`.
- For controls and rendering, start in `internal/tui`; keep external actions in
  the `cmd/prui` callbacks.

See [TESTING.md](../TESTING.md) for verification commands and
[CONSTRAINTS.md](../CONSTRAINTS.md) for the project invariants.
