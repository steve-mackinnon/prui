# Spec: Commit Filter in the Diff Workspace

Status: Implementation authorized 2026-10-04. User confirmed one net diff and
requested sub-agent implementation using the incremental implementation skill.

## Objective

Let reviewers choose which captured commits contribute to one net diff in the
Files/Guide workspace. That diff hydrates both file navigation and code content. Add a visible `Commits [C]` filter control without replacing the
dedicated Commits tab or changing its navigation. The supplied screenshot is a
visual reference for a filter control and checkbox list, adapted to this TUI.

## Assumptions and Scope

- Files and Guide share the filter; Description and the dedicated Commits tab
  remain independent. No new top-level Diff tab is proposed.
- Uppercase `C` opens the filter in Files/Guide. Lowercase `c` retains discussion
  refresh. Editors and existing modal input take precedence over shortcuts.
- Default is `All changes`: the existing frozen full-PR comparison, including
  the existing guide, progress, comments, and split/unified preference.
- Selected-commit mode derives one transient comparison from immutable captured
  source. Filtering requires no network, changes to the user checkout, or guide
  request. Offline synthesis requires an optional source-payload extension; the
  existing patches alone are not sufficient for general net-diff composition.
- This is one capability; it does not need a separate capability map.

## Filter Interaction

Render a mouse-selectable `Commits [C]` control above the diff workspace, visible
at both wide and narrow widths. Show `All changes` or `N selected` beside it so
filter state remains clear when the picker is closed.

The picker contains `All changes` first, then captured commits with checkboxes,
escaped subject, author, and abbreviated SHA. Preserve captured order and show
the capture limitation notice when membership is incomplete. Clip long metadata
to terminal cell width; identity and checkbox state must not depend on color.

- Opening focuses the active choice; opening alone changes nothing.
- Up/Down or j/k moves focus; Space or Enter toggles the focused commit. Toggling
  from All changes starts an explicit selection containing that commit.
- Enter/Space on All changes restores the full PR comparison.
- Selection updates the workspace immediately and leaves the picker open for
  further choices. Escape or C closes it, retaining the applied selection.
- Deselecting the last commit shows `No commits selected` and an empty reading
  surface, with All changes still reachable. It must not silently reset.
- Selecting all commits in a complete captured list resolves to All changes,
  guaranteeing the existing canonical PR diff. Selecting all entries of a capped
  list remains a subset; uncaptured commits must not be silently included.
- Mouse clicks use the same transitions; wheel navigation stays within the
  picker. The picker consumes input before workspace navigation.
- New/resumed comparisons default to All changes. Process-local selection is
  retained across Files/Guide and workspace PR switches, keyed by review identity
  and SHA. It is independent of the dedicated Commits tab's selected SHA.

## Net Diff Semantics

Every successful selection produces exactly one old/new comparison and one
inventory of files, metadata, hunks, patches and syntax. A changed path appears
once, with its accumulated final content. Changes that cancel out disappear.
Both the left file list and right diff consume that same derived inventory;
neither may continue to render the unfiltered inventory under a filtered label.
Reuse unified/split rendering and its responsive fallback.

Proposed composition rule: start from the first selected commit's first-parent
tree (empty tree for a root), apply selected commit changes in verified ancestry
order, and compare that baseline with the resulting virtual tree. Each selected
commit contributes its change relative to its first parent. For a contiguous
first-parent range, use the equivalent direct comparison from the oldest selected
parent to the newest selected commit. A single selection matches its dedicated
commit diff. For complete full membership, use the canonical full PR comparison.

For A → B → C, selecting A and C applies A then C, excluding B's changes. If C
requires B, a clean three-way application may fail; show a conflict identifying
the affected commit/path. Never silently include B, drop C, use fuzzy application,
render conflict markers as reviewed source, or show a partial result as success.
Merge selections use first-parent changes, with that fact visible. Reject
ambiguous branch ordering that cannot be safely composed under this rule.

Selection drives one atomic result: show a computing/unavailable state until both
file list and code are ready for the current selection. Stale asynchronous results
must not replace a newer selection. A failed combination retains the requested
checkboxes, explains the failure, and leaves All changes readily accessible;
do not display the previous diff under the new selection label.

## Frozen Source and Composition Contract

Current commit bundles retain hunks, object IDs and token spans, not complete
file versions. Do not infer missing file content from those hunks. Extend capture
with optional, bounded, deduplicated material sufficient for the selected baseline
and three-way application: tree/path/mode identity and required old/new blob bytes
for touched paths, including binary files, renames and deletions. Untouched paths
must not appear in the derived inventory. Treat gitlinks as metadata; never recurse
into submodules. An implementation plan must establish the precise source schema
and composition engine before coding.

Capture remains in the isolated pinned open path. Reuse the existing additional
50 MiB source budget, 100,000 diff-line bound, 60-second capture deadline, and
128 MiB encoded source-payload ceiling; account for the new material within these
limits rather than increasing them. Apply existing per-blob/entry limits to
composition too. Store an explicit unavailable status when source is insufficient.

Validate identities, content/object correspondence, references, and bounds on
save/load. Keep the extension optional so legacy canonical payloads and digests
remain valid. Legacy sessions retain All changes and the dedicated commit browser;
net filtering explains missing composition data and never backfills on resume.
No SQL table migration is assumed. Offline filtering must survive Git object
pruning and operate only on frozen material. Any temporary object store/index
must be app-owned, disposable, bounded, and separate from the user checkout;
no hooks, external diff/textconv, user index writes, checkout or commits.

Guide retains its existing full-PR interpretation, visibly labeled as such.
Use existing validated unit-to-file associations to organize derived file
entries only where paths match unambiguously. Do not claim filtered patches have
their own generated explanation. Place unmatched/ambiguous paths in an explicit
`Selected commit changes outside guide` group; display every derived file once.
An absent guide remains absent; generation never starts from a filter action.

Derived patches are read-only in this first addition. Disable marking and line
composition, explaining `Selected commits: reading only; use All changes to mark
files or Commits to discuss a commit.` Keep pending drafts intact and the existing
PR-level review action available. Reuse historical discussion targeting only in
the dedicated Commits tab; do not route filtered rows through head-based targets.

Keep the All changes cursor, file selection, scroll, guide expansion and progress
separate from filtered reading state. Returning to All changes restores them.
After selection changes, retain a still-visible derived file selection; otherwise
choose the first available entry and clamp the scroll position.

## Safe States

- Missing legacy bundle, failed capture, or empty captured list: keep the control
  discoverable and explain why selection is unavailable; All changes still works.
- Capped membership: list only captured entries and disclose that more may exist.
- Missing/partial composition material or conflict: no successful derived diff;
  show the affected commit/path and safe reason, without omitting selected input.
- Empty commits or cancelling changes: show `No net changes in selected commits`.
- An empty full PR diff does not hide the filter or historical changes.
- Narrow/short terminals: scrollable picker with focused row visible, bounded to
  available screen size, no negative geometry or source navigation underneath.

## Stack, Structure, and Style

Go 1.26.8; Bubble Tea 2.0.9; Lip Gloss 2.0.4. Reuse current dependencies.

- `internal/tui/model.go`, `lifecycle.go`: workspace rendering/input dispatch.
- `internal/tui/commits.go`: existing captured data access/rendering to reuse.
- `internal/tui/commit_filter*.go` (proposed): picker state and derived inventory.
- `internal/tui/mouse_review.go`, `bindings.go`: mouse controls and help.
- Adjacent `*_test.go` and `internal/tui/testdata/screens`: tests/fixtures.
- `internal/commits/types.go`, `internal/session/store.go`: existing immutable
  data contracts to extend with optional frozen composition material.
- `internal/commits/`, `internal/inventory/`: bounded net-comparison engine.
- `internal/review/`, `internal/session/`: pinned capture and codec validation.
- `docs/spec/`: specification; `tasks/`: plan/checklist after spec approval.
- `README.md`, `docs/REFERENCE.md`: shipped controls updated during implementation.

Use typed state, SHA identity, existing Escape helpers, terminal-cell measurement,
and gofmt. Follow the existing presentation/state separation, for example:

```go
func (m *Model) commitEntries() []commits.Entry {
	if m.Session == nil || m.Session.Commits == nil {
		return nil
	}
	return m.Session.Commits.Entries
}
```

Do not fabricate `Session.Inventory` or mutate immutable patches to reuse a
renderer. Cache derived rows by review, ordered selection, width/layout and theme.

## Commands and Testing Strategy

```sh
go run ./cmd/prui --help
go test ./internal/tui ./internal/commits ./internal/inventory ./internal/session ./internal/review -count=1
go test ./internal/tui -run '^TestScreenSnapshots$' -count=1
./scripts/verify.sh
git diff --check
```

Use synthetic frozen bundles and local fixtures, no live GitHub/provider calls.
Model tests cover multi-select, empty/reset, keyboard precedence, mouse parity,
Files/Guide shared state, PR isolation, restoring All changes, and independent
Commits navigation. Composition tests use disposable Git fixtures with known expected old/new bytes:
single commit, contiguous range, noncontiguous independent changes, dependent
conflict, repeated edits to one path, cancellation/reverts, rename chains,
add/delete/mode/binary/gitlink, root/merge/empty commits and ambiguous ordering.
Assert a single correct inventory and unchanged immutable source. Capture/codec
tests cover budget exhaustion, optional-field legacy digest compatibility,
corruption, deduplication and offline filtering after original objects disappear.
Assert no progress, comment target, draft, network or guide-generation side effects
from filtering; test stale-result rejection and atomic list/content updates.

Screen fixtures cover wide/narrow/short dimensions, colorless checkbox markers,
long escaped labels, grouped headings and all unavailable states. Existing
split/unified and compiled-binary PTY checks remain required. Run the repository
gate during implementation; human terminal usability remains required by
CONSTRAINTS.md. No new numeric coverage threshold is introduced.

## Boundaries

- Always: preserve frozen provenance, label comparison/limitations, escape source,
  retain full-review state, and follow CONSTRAINTS.md and the verification gate.
- Ask first: change the proposed baseline/conflict semantics, expand capture
  limits/pagination, add filtered writing/progress, or add dependencies.
- Never: change checkout, fetch during filtering, regenerate guides implicitly,
  submit filtered lines as PR-head targets, weaken tests, or alter stored source.

## Success Criteria

1. Files and Guide expose a visible Commits control and uppercase C picker.
2. All changes initially reproduces the existing full comparison; the dedicated
   Commits tab and lowercase c retain their behavior.
3. Selected commits produce one net diff with one entry per resulting changed
   file; cumulative edits combine, cancelling changes disappear, and excluded
   changes are not silently included. Conflicts/material gaps are explicit.
4. Selected files remain reachable even when absent from the final net PR diff
   or the existing guide; missing material is disclosed.
5. Changing filters cannot write, mark files, retarget drafts, or mutate snapshots.
6. All changes restores reading state; PR switching preserves isolated filters.
7. Keyboard/mouse, narrow layouts, offline/legacy cases and repository gates pass.

## Open Decisions

The user confirmed a singular net diff driving all displayed files and code.
The baseline/application rule above is the proposed meaning of arbitrary subset
selection; dependent skipped commits can produce conflicts. Implementation follows the proposed baseline rule, Guide
organization and read-only filtered mode. The implementation plan must detail the optional source representation and bounded composition engine;
existing first-parent patches cannot fulfill this requirement by themselves.
