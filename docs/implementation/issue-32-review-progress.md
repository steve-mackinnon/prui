# Changes since a previous review

Issue #32 from roadmap #39 originally stacked on #45 after #41. Those parents
are merged. PR #47 now targets main, with feature-only commits restacked onto
`7f64f19ffa26ac5bd5d032c3ff668d4221fb6f84`, including the merged published-thread
actions from #44. Only the coordinator merges after final review and green checks.

`N` opens a newly pinned comparison using the currently selected saved session
as its predecessor. An online open of an existing PR uses its latest valid saved
session. Background refresh carries the ID of the session it actually opened,
not a latest-head lookup. `5` opens Changes since previous review. `n/p` selects
captured files, arrows and Page Up/Down scroll, left/right pan, and `d` switches
to private draft anchor outcomes. `o` loads the exact original session and its
private draft recovery; Escape returns to the current review. `s` lists all saved
snapshots. Opening the original with `o` replaces the visible review; use `s` to
return to another saved comparison.

## Source and proof

The captured comparison is a direct old-head-tree/new-head-tree diff, not an
ancestry-based merge-base diff and not a status summary. Each head is the complete
40-character SHA with its recorded repository identity. Objects are read through
the existing isolated `source.View`, including a bounded exact-SHA fetch when a
head is missing. No working files, checkout refs, ref switching, external diff,
filters, or textconv supply source. Repositories changing identity disable the
incremental capture rather than joining evidence from another fork. Normal pushes
and rewritten history are distinguished using committed ancestry. Rebases and
force pushes use the same direct tree comparison even without a shared ancestor.
The base/head repositories and pins are rechecked after capture. Failed checks,
missing objects, limits, and incomplete raw inventory stay explicitly unavailable
or partial; they never become a successful empty comparison.

Read progress is file-granular. Transfer requires complete prior and current PR
inventories, a complete captured head diff with exactly matching prior/current
pins, identical inventory version/settings, both blob OIDs, both modes, both raw
paths, change status, and every raw unit range/kind/patch digest and patch bytes.
Unavailable units never prove unchanged. Equal paths, displayed text, or new-side
blobs alone are insufficient. A base change may retain progress only if both sides
of the whole PR file comparison satisfy that proof; changing old-side content
requires another review even if the head-side text is identical. Renames and mode
changes do not transfer progress. Newly changed files retain the ordinary unread
marker. A base-only change with identical head trees can have an empty incremental
diff while still changing the PR diff and invalidating read marks.

The predecessor ID, immutable snapshot reference, generation, full pins, ancestry
classification, and direct diff are captured in the new immutable source payload.
The generation and reference are checked again in the transaction that inserts
the new snapshot and proven initial progress. Conflicting writers fail without
changing original progress. No progress is inferred from the current checkout.

## Private drafts and reconciliation contract

`incremental.Assess` accepts complete `source.ReviewCommentTarget` values and
returns a typed outcome, the original target, and an advisory proposed target
only for a fully proven unchanged PR file. It preserves range start/end/side and
file subject shapes. The contract is payload-agnostic for later suggestion
payloads. Nothing in it authorizes a remote write or mutates/copies a draft.

Outcomes distinguish unchanged (explicit recreation required), changed, renamed,
deleted/absent from the new PR diff, unavailable, historical, and uncertain
attempted delivery. Exact full file evidence takes precedence over other records;
equal-content copies are never rename proof. Raw rename records require old/new
path evidence. Reply drafts retain their original thread identity. Review summaries
and decisions remain on the old comparison. Historical anchors remain original
captured-commit targets; they are not promoted into current-head targets.

Pending comments, line/range/file editors, replies, review summaries, and immutable
attempted payloads remain in the separate private mutable `review_drafts` table.
The outcomes view loads the predecessor by ID/reference and reads its draft key.
It shows anchor metadata and outcomes, not bodies. Uncertain attempts require the
existing explicit outcome check in the original snapshot; no remapped attempt,
automatic retry, submit, or remote write is possible from this view. To comment
on the new comparison, select and compose a new target explicitly in its PR diff.
Existing freshness/consent and captured-target validation still govern submission.
Missing or corrupt predecessors/drafts show an unavailable outcome and preserve
stored records. The ordinary recovery view exposes original text offline.

## Storage and offline behavior

Incremental captures use the existing private, local SQLite source payload store,
with digest/reference validation, canonical encoding, and durable transactions.
They have no model input, guide upload, telemetry, credentials, or remote cache.
The field is optional, preserving existing source payloads; older binaries reject
new unknown source fields rather than misinterpret them. Capture uses existing
limits (10,000 files, 50 MiB materialized content, 1 MiB blobs, 100,000 diff lines,
60 seconds and 512 MiB isolated fetch storage). Source payloads retain the existing
128 MiB aggregate ceiling; an oversized combined snapshot fails explicitly.
There are no automatic limit retries. Temporary objects are disposed with the view.

`resume --offline` renders captured changes without source retrieval, repository
access, GitHub, or provider calls. Deleting a predecessor removes its own drafts
under the existing last-snapshot policy, but the independently captured head diff
remains readable in the newer snapshot. `o` then reports an unavailable original;
it cannot synthesize the deleted draft. Deleting a new comparison follows the
existing unreferenced-payload cleanup. Source remains unencrypted private data;
there is no forensic-erasure claim.

## Verification

Synthetic/offline regressions cover forward pushes, rewritten history, base-only
and base-content changes, mode/settings/partial-proof rejection, renames/deletions,
duplicate-content ordering, LEFT/RIGHT ranges and file anchors, generation
conflicts, immutable originals, captured patches after checkout removal and prior
session deletion, and private uncertain attempts without writes or uploads.

Required full gates and the independent final reviewer are recorded in the PR
once run. Human terminal usability QA is unverified; render tests do not establish
accessibility. No live GitHub or provider calls are used by tests.

Historical pre-restack evidence: independent reviewer `/root/final_review` approved implementation SHA
`2331d0615cdbe8788dbeb7062e26c4496d0595ba` against parent
`5dd00d102ae6cfdd8051d0050a0edb6f2284e8d7`, after fixing its required finding:
an existing raw rename record now requires renewed review even on a later push.
The reviewer independently ran `go test -count=1 ./internal/incremental` and
`git diff --check` successfully on that SHA. Isolated Go-overlay mutations of
the complete-capture guard and rename rejection each made the corresponding
regression fail. Shared source was not mutated. `golangci-lint run` passed with
zero issues. Earlier wider verification attempts failed from disk exhaustion
during overlapping runs; those attempts are not pass evidence. Exact final
commit review and complete serialized gate/CI results are recorded on PR #47.


## Main integration and lease-safe restack

The recorded previous remote #47 head was
`4b3bee2b8cfdf7e59a1f9b2ad54ec462a5774728`. Only the three #32 feature commits
were rebased, followed by new combined regressions. The remote rewrite uses an
explicit force-with-lease for that SHA; no other author branch is changed.
A reset-path conflict preserves both main's commit-filter cancellation and
incremental-view reset. The contract append conflict preserves both the published
comment/thread contract and the incremental-review contract.

The combined synthetic regression exercises normal pushes, base-content changes,
renames and deletions with complete RIGHT/LEFT ranges, a file editor, an associated
reply root, review decision and summary. It asserts only proven reading progress
transfers, exact anchor outcomes, cancelled derived-filter/discussion work, ignored
stale conversation results, original snapshot/draft immutability, no draft copies,
and exact offline recovery after the checkout is removed. The complete rebased
feature diff receives a fresh independent reviewer; its exact head, focused and
broad results, current-main base and CI are recorded on PR #47. Pre-restack test
evidence above does not establish verification of this rewritten head. Human
terminal QA remains unverified.

The final navigation integration rebases only this feature and its maintenance
repair onto main `f0a3dea37f58fccd96cf6e51b8f2b80b82412b9c`, preserving both
`CacheFullSource` consent and the explicit predecessor. The previous remote head
for this rewrite is `051ddfd04eabdaf8ae6cab17af8e48d814ceda8c`. The combined
regression also captures consented full source, reloads it offline, and asserts
that comparison reset cancels old search workers, discards search/navigation
state and ignores old search results while preserving draft target shapes.

Ubuntu CI exposed a pre-existing borrower race: Git's transient
`objects/maintenance.lock` could disappear after directory enumeration but before
hard-linking. The repair excludes that exact metadata file. A deterministic
production-callback regression captures its directory entry, removes the file,
then invokes borrowing; it reproduced the link error before the repair. The same
ordering still rejects a missing real loose object. A NewView integration test
also checks that a present maintenance lock is never borrowed. No fixture
maintenance settings or existing assertions were weakened.
