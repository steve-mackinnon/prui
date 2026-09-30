# Spec: Pull Request Commit Browser

Status: Implemented 2026-09-30. Verification and review evidence is recorded in
`tasks/commit-browser-todo.md`.

## Objective

Replace the Commits tab's placeholder with a commit list in the left pane and
the selected commit's diff in the right pane. Moving through the list immediately
updates the diff so reviewers can understand how the PR evolved.

This supersedes this document's earlier list-only contract and the single-surface
Commits contract in `SPEC-pr-view-tabs.md`. Description retains its existing
behavior. This remains the `pr-commits` capability in the existing capability map.

## Scope and Defaults

- List the active PR's commits frozen at review opening, not checkout history.
- Preserve GitHub's returned order; select the first returned commit initially.
- Compare each commit to its first parent, not the preceding visible commit or
  PR base. Merge commits explicitly say `Compared with first parent`; root
  commits compare to the empty tree. Empty commits show `No tree changes.`
- Capture bounded diffs during opening, allowing instant, offline navigation.
- Retain the earlier first-page, 100-commit bound with an explicit truncation
  notice. Pagination is a follow-up.
- Commit browsing is read-only. Commit comments, review marks, checkout/reset,
  search, commit message bodies, and remote refresh are outside this release.
  The existing PR-level Submit review action remains available.

## UI Contract

Keep the existing `Diff [1]`, `Description [2]`, and `Commits [3]` tabs.
`3` and `v`/`V` select the populated Commits view.

```text
owner/repo #42 · Add commit browser
Diff [1]  Description [2]  › Commits [3]
┌ Commits · 2/3 ────────────┬ Commit diff · b7c8d9e01234 ──────────┐
│   Add commit capture     │ Wire the commits pane                │
│   Alice · a1b2c3d45678    │ Alice · b7c8d9e01234                  │
│ › Wire the commits pane  │ Compared with a1b2c3d45678            │
│   Alice · b7c8d9e01234    │ ── internal/tui/commits.go            │
│   Cover navigation       │ @@ -10,2 +10,3 @@                     │
│   Bob · c3d4e5f67890      │  existing line                       │
│                          │ +new line                            │
└──────────────────────────┴─────────────────────────────────────┘
```

### Commit rail

- Each selectable item occupies two lines: subject, then author and 12-character
  SHA. Clip to display width rather than wrapping. Empty subjects display
  `(no subject)`; missing authors display `Unknown author`.
- Use the existing textual `›` marker plus focus styling. Selection stays visible
  with detail focused and with color disabled. The header shows position/count.
- Keep the selected item fully visible where height permits. Very short
  terminals still show its subject and permit selection.
- Selecting another item immediately replaces the detail content and heading;
  Enter is not required. Never show the old patch under the new heading.

### Commit detail

- Show subject, author, SHA, and comparison parent above one scrollable stream
  containing all changed files and hunks. Reuse file dividers, line numbers,
  escaping, and theme classes. There is no nested file rail or third pane.
- Use unified diff presentation initially. The main Diff tab's `S` preference
  stays independent; `S` is a no-op here.
- Include additions, deletions, rename paths, and mode changes. Binary and
  submodule changes use informational cards. Merge/root semantics are labeled.
- Start at the top on the first visit to a commit; revisiting restores its saved
  vertical offset, clamped after resize. These rows have no PR comment targets.

### Input

Bindings apply only in Commits, after overlays and global controls are handled.

| Input | Rail focused | Diff focused |
| --- | --- | --- |
| `j` / `k`, Down / Up | Next / previous commit | Scroll one line |
| `n` / `p` | Next / previous commit | Next / previous commit, retaining focus |
| PageDown / PageUp, `d` / `u` | Move by a visible page of items | Scroll a visible page |
| Home / End | First / last commit | Start / end of diff |
| `l`, `ctrl+l`, Enter | Focus selected diff | No action |
| `h`, `ctrl+h`, Escape | Stay in rail | Return to rail |
| `[` / `]` | Resize rail when both panes are visible | Same |

Selection clamps without wrapping. Clicking a commit selects it and focuses the
rail; clicking detail focuses detail. Wheel events over the rail move selection
and over detail scroll content, matching existing workspace behavior.

Enter in detail never opens a comment composer. File/guide/inventory controls,
marking, source cursors, and Tab's guide expansion cannot fall through to Diff.
Keep existing view/PR switching, themes, help, quit, and PR-level Submit review
routing available. Document applicable keys in help and pane labels.

### Responsive layout and state

- At >=100 columns reuse the existing two-pane frame: default rail width
  `min(36, width/3)`, minimum rail 18, minimum detail 40. Keep a Commits-owned
  rail width preference.
- Below 100 columns show the focused pane at full width. Start with the rail;
  Enter/`l` opens detail and `h`/Escape returns. Preserve tab navigation and
  selected commit identity/position. No overlay or horizontal scrolling needed.
- Keep SHA selection, rail offset, focus, width preference, and per-commit
  reading offsets in process-local review state. Restore on context-view or
  existing workspace-review switches. New reviews/process resumes start at
  the first item with rail focus.
- Do not reuse or mutate main Diff selection, focus, scroll, layout, guide
  expansion, progress, or drafts. Returning to Diff restores its exact state.

## Capture and Storage Contract

### Frozen membership

1. After metadata pinning, request
   `GET repos/{owner}/{repo}/pulls/{number}/commits?per_page=100&page=1`
   through the bounded `source.GH` runner. The
   [GitHub endpoint](https://docs.github.com/en/rest/pulls/pulls#list-commits-on-a-pull-request)
   permits 100 results per page and has a 250-commit ceiling; this release
   intentionally captures only the first page.
2. Validate <=100 unique lowercase 40-character SHAs. Normalize messages to
   their first line and validate UTF-8 subject/author fields against 4 KiB /
   256-byte bounds. Do not persist raw responses or unused message bodies.
3. Re-read metadata after capture. Use `SamePinnedRevision` for repository
   identities and base/head SHAs. Revision movement discards captured material
   and restarts the bounded open path once; repeated movement fails safely.
4. Preserve returned order. Exactly 100 entries conservatively means incomplete:
   show `Showing first 100 commits; more may exist.` A successful shorter list
   is complete; successful empty and unavailable captures remain distinct.

### Frozen commit diffs

- Before closing the isolated `source.View`, verify each commit object and its
  reachability from the pinned head; resolve ordered parents from Git ancestry.
  Membership comes from the PR list, not local branch traversal.
- Reuse inventory's raw-tree/blob pipeline through an explicit commit comparison
  entry point, retaining deterministic rename/binary/submodule/patch rules.
  Do not fabricate PR metadata, replace `Session.Inventory`, generate progress
  slices, or include commit material in guide analysis.
- A missing commit/parent gets a per-commit unavailable state. Fetching stays
  in the existing pin/open path; navigation never fetches. Keep external
  diff/textconv, hooks, and submodule recursion disabled.
- Apply existing entry and per-blob limits plus an aggregate additional capture
  budget of 50 MiB source material, 100,000 diff lines, and a single 60-second
  deadline. Account for metadata too; these bounds do not multiply by commit
  count. Respect the final 128 MiB encoded source-payload ceiling, including
  JSON overhead and existing PR material.
- Represent every captured commit even if its patch exceeds a bound. Store an
  unavailable reason or partial material with `Complete=false`. Once the
  aggregate budget is exhausted, mark remaining diffs unavailable without
  further work. Never label a limited patch complete.
- Ordinary capture failures do not prevent the main PR diff from opening; save
  a safe bounded reason. Cancellation and revision mismatch retain the normal
  open/cancel/retry semantics rather than saving mismatched material.

### Durable representation

- Add an optional immutable commit bundle to `session.Snapshot` and the SQLite
  source payload: pinned base/head identity, capture status/reason, list
  completeness, and ordered entries.
- Entries contain SHA, subject, author, ordered parents, diff status/reason,
  and structured files/hunks/patches. Keep them separate from the main inventory,
  its unit/progress references, and guide input.
- Validate identities, unique SHAs, parent IDs, patch references, bounds, and
  status combinations at save/load. Missing objects never mean empty success.
- Omit the new field when absent so old canonical JSON re-encodes byte-for-byte
  and old digests remain valid. Test the strict decoder and canonical codec;
  no SQL table migration is proposed.
- Source-cache reuse and guide-derived copies retain captured commit material.
  Resume does no Git/GitHub work. Legacy snapshots do not backfill themselves.

## Safe States

| Condition | Display |
| --- | --- |
| Successful empty list | `No commits captured for this PR.`; no selection |
| Legacy bundle absent | `Commits were not captured for this session.` |
| Capture failure | `Commit list unavailable: <safe reason>` |
| Capped list | Persistent truncation notice; captured entries remain usable |
| Missing selected diff | Commit heading plus `Commit diff unavailable: <safe reason>` |
| Partial diff | Visible incomplete notice alongside captured material |

Opening uses the existing blocking loading modal with a `Capturing commits`
phase. Navigation needs no background request, spinner, or response-race state.
Commit history remains available even when the main PR has no net tree changes.

## Tech Stack and Project Structure

Go 1.26.8+, Bubble Tea v2.0.9, Lip Gloss v2.0.4, existing Git/GitHub runners,
inventory pipeline, SQLite source codec, and TUI tests. No new dependency.

```text
internal/source/github.go       bounded PR commit list capture
internal/source/git.go          pinned commit/parent verification
internal/inventory/             explicit commit comparison and shared diff pipeline
internal/review/review.go        capture before closing isolated view
internal/session/store.go        optional bundle and validation
internal/session/sqlite_codec.go canonical encoding and legacy compatibility
internal/tui/commits*.go         dedicated navigation/rendering/cache
internal/tui/model.go            context dispatch and transient review state
internal/tui/{mouse,help}.go      input routing and discoverable controls
internal/tui/testdata/screens/   deterministic screen fixtures
docs/REFERENCE.md, README.md     shipped behavior/controls during implementation
```

## Code Style

Use existing typed boundaries, bounded runners, and `gofmt`. Keep capture
independent of presentation; escape untrusted text before measuring/styling:

```go
func commitMeta(author, sha string) string {
    return Escape(author) + " · " + sha[:12] // SHA validated at source/storage boundaries.
}
```

Extract pure file/hunk rendering helpers if needed; do not construct fake review
sessions to call renderers. Cache rows for the selected commit only, keyed by
snapshot, SHA, width, and theme. Reading offsets can remain a small SHA-keyed map.

## Testing Strategy and Commands

- Fake-runner tests: exact API request, 0/1/100 entries, duplicates, malformed
  IDs, Unicode/control characters, multiline subjects, timeouts, and pin changes.
- Git fixtures: linear history, reverted changes with empty final PR diff,
  merge/root/empty commits, rename/mode-only/binary/submodule changes, missing
  parents, and aggregate exhaustion. Assert first-parent comparisons.
- Storage: round-trip, old canonical JSON/digests, corrupt references/statuses,
  encoded-size bounds, cache/derived-copy retention, and offline resume with
  no Git/GitHub calls.
- Model: immediate selection, clamped navigation, per-commit offsets,
  context/workspace isolation, mouse routing, and no comment/progress mutations.
- Screens: widths 60/99/100/120, short heights, no color, escaped long labels,
  safe states, theme changes, and resize restoration. Cover an empty main diff.

```sh
go test ./internal/source ./internal/inventory ./internal/session ./internal/review ./internal/tui -count=1
go test -race ./internal/source ./internal/inventory ./internal/session ./internal/review ./internal/tui
./scripts/verify.sh
git diff --check
```

## Boundaries

- **Always:** pin, validate, and bound captured data; preserve main review state;
  label limitations; support offline browsing.
- **Ask first:** expanding pagination/bounds, commit comments or progress,
  changing global keys, adding dependencies, or requiring SQL migration.
- **Never:** change the checkout, infer membership from its branch, perform I/O
  on navigation/resume, or submit commit rows as PR comment targets.

## Success Criteria

1. Commits shows a selectable left rail and selected commit's own complete or
   explicitly limited multi-file diff at wide widths.
2. All selection methods immediately update heading/content; navigation clamps
   and first-parent semantics are verified.
3. Narrow terminals expose the same list/detail through focus controls.
4. Revisiting commits/views restores reading state without altering main Diff,
   progress, guide state, or comment drafts.
5. Offline, legacy, failed, capped, and partial captures are handled honestly;
   empty net PR changes do not hide commit history.
6. Capture remains bounded and pinned; focused tests and the repository
   verification gate pass when implementation ships.

## Implementation Decisions

The user approved implementation on 2026-09-30. Defaults are first returned
commit selected, first-parent merge diffs, unified presentation, a 100-commit
cap, and eager bounded capture. Capture work uses 55 seconds of the 60-second
overall deadline, reserving five seconds for the final freshness check.
