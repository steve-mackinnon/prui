# Pinned code context and navigation (#30)

## User workflow

Launch `prui --cache-full-source`, `prui open <PR> --cache-full-source`, or
`prui prs [owner/repo] --cache-full-source` to consent to additional **local**
complete-source capture for comparisons opened by that process. For a new
comparison from an existing session use `prui resume <id> --new --cache-full-source`.
The option is rejected on ordinary/offline resume and on verify. Consent is not
remembered across launches; reopening a saved snapshot reads its captured data.
Without the option, ordinary patch reviews retain their existing storage behavior.

On Files:

- Ctrl+E toggles all available unchanged context around the saved hunks.
- Ctrl+O / Ctrl+N show complete pinned OLD / NEW versions across changed files.
- Ctrl+D restores the ordinary diff.
- Ctrl+W hides equal-length replacement runs that differ only in whitespace.
  A visible marker reports the number of hidden canonical rows; toggle to inspect
  or comment on them. This is a local presentation projection, not a new diff or
  a claim that the underlying change is semantically harmless.
- Ctrl+F searches the current source scope. OLD/NEW searches the captured side;
  expanded mode includes available additional context; ordinary Files searches
  canonical saved patches. Guide continues to search its current section.
  Results display side/line and matches; Enter jumps, F3/Shift+F3 navigate next/
  previous with wrapping. Activating a canonical result reveals any hidden
  whitespace change. Filename filters are cleared when needed to reveal a result.
- Alt+Down / Alt+Up navigate loaded authoritative unresolved threads, opening
  thread detail. Unknown and retained stale resolution are excluded; timeline replies are
  deduplicated by canonical thread identity. Partial retrieval remains
  visible. Historical or unplaceable threads stay readable in discussion detail,
  with the existing explicit original-context action and fallback snippet/link.

Expanded and full-source rows are escaped and navigable in unified, split and
narrow fallback views. Additional context and full-file rows are read-only.
Only canonical patch targets may compose comments. Missing source never gains
an invented comment anchor. Review-unit IDs, frozen patches and read progress
never change when any projection changes.

## Source and storage policy

The canonical raw inventory supplies blob OIDs for merge-base OLD and frozen-head
NEW. Capture uses the existing isolated `source.View.Blob` (`git cat-file blob
<OID>`), never working files, checkout switching, textconv, filters, hooks or
model output. No additional fetch is performed for this cache. Source capture is
opt-in and displays the local storage bounds/no-upload notice while opening.

Capture retains complete UTF-8 text without NUL bytes only: 1 MiB per blob
(or a lower caller limit), 4 MiB of distinct blob bytes, and 100,000 source lines
per snapshot. It deduplicates OIDs and has a 60-second maximum capture deadline
(or a shorter caller operation limit). Missing objects, binary/invalid UTF-8,
submodule content, limits, cancellation and old snapshots without full source
are explicitly unavailable in source views. Empty and absent sides are distinct
from missing blob content. There are no partial-blob successes, automatic retries
or lazy credential/network access when navigating or searching.

Captured source is stored with the immutable SQLite source payload, outside the
reviewed checkout with the existing private file permissions, checksum,
transaction and snapshot-generation protections. Git blob identity is checked
at capture and session validation; unreferenced OIDs, oversized/tampered data,
invalid UTF-8 and unsupported source are rejected. An optional omitted field
preserves the canonical encoding of older snapshots. Existing overall 128 MiB
source-payload and bounded commit-capture rules still apply.

Complete files can contain sensitive unchanged material previously omitted by
patch storage, including paths excluded from AI analysis. Local raw data are
unencrypted. The flag authorizes only this local storage; it does not authorize
AI upload or GitHub writes. The guide upload boundary continues to construct
only its existing policy-filtered patches/evidence, never this cache. No query,
result, draft or live discussion becomes persisted source. Tests compare guide
inputs with and without full-source capture.

Captured source remains available on `resume --offline` after Git objects or
working files change. It is keyed by the immutable inventory/snapshot and blob
OID, never reused across different comparisons by path. Refreshing freshness
does not replace frozen source or read progress. A new comparison captures its
own cache only with new process consent. Delete uses existing transactional
session deletion and unreferenced payload cleanup; it does not promise removal
from backups or forensic erasure. Missing capture requires a new consented
comparison; offline navigation never attempts to fill it.

Search is local, literal, case-insensitive and capped at 10,000 results, preserving
PR #42's explicit refine-query notice and visible highlights. Scope and skipped
content are shown in the find panel; individual files show source unavailability.
Expanded gaps are emitted only when OLD/NEW captured regions agree exactly.
Any inconsistent gap has a visible unavailable marker rather than invented text.

## Provenance and dependencies

This change stacks on #44 (`codex/issue-28-thread-actions`, pinned start
`6eae376ce08ae1175c9379c9d2a746eb01183aa6`), itself stacked on #40.
It cherry-picks PR #42's `bfbdb80bebdff107471f4f921382308211312e3d` as
`85ac444`, retaining authorship, and resolves conflicts to preserve #28's PR
switcher and published actions. This PR includes that unmerged search code and
its tests; #42 should not subsequently be applied blindly as a duplicate.
`codex/diff-commit-filter` at `804caf9` was inspected, particularly its bounded
OID capture/validation pattern. No commit-filter implementation is included;
net commit filtering remains a separate dependency/feature. Complete-file
navigation here concerns the frozen PR comparison, not selected net commit sets.

Official plumbing reference: [git-cat-file](https://git-scm.com/docs/git-cat-file)
provides object content; filters/textconv are distinct options and are never used.
[git-diff](https://git-scm.com/docs/git-diff) documents canonical hunk coordinates;
whitespace presentation here preserves the existing frozen diff rather than
invoking a replacement Git comparison.

## Verification

Synthetic tests cover consent, pinned bytes despite uncommitted working files,
blob deduplication/bounds/tampering, restart/deletion, no changed guide upload,
OLD/NEW and expanded search, unavailable coverage, zero-count middle/EOF
insertion/deletion, whitespace reveal/progress invariance, unresolved/unknown
resolution, and canonical targets across unified/split/width changes.
Existing PR #42 offline search, wrapping, highlighting, grouping and limit tests
are retained and independently reviewed as part of the complete stacked diff.
Human terminal usability/accessibility QA remains unverified unless separately
recorded; screen fixtures and synthetic PTY tests are not human QA.
