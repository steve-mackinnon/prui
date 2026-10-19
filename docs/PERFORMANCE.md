# Startup and PR opening performance

## Goal and measurement contract

Target: make startup and PR opening 10 times faster without changing core
behavior. **Active; not achieved.** Compare medians on the same host, build
configuration, fixture and cache state. A stage-level speedup does not establish
an end-to-end speedup. Keep network latency separate from application work.
The user clarified that **selecting a PR for the first time** is the primary
scenario; cold opens take priority in subsequent iterations. Startup remains
part of the original goal.

- Startup: process launch to the first readable offline review frame in a PTY.
- Cold open: Git pinning, main inventory, commit capture/composition, evidence,
  durable session creation and freshness checks, without a prior snapshot.
- Cached open: load and validate frozen source, create a new session, restore
  reading progress and mark freshness unchecked. Online freshness remains a
  separate background operation.
- Preserve pinned source, coverage/limits, syntax diagnostics, commit history,
  drafts, freshness checks, offline operation and explicit write/upload consent.
  No validation or required source capture is skipped to improve a number.

## Latest verified interactive selection

| Real PR | Original | Latest candidate | Overall improvement |
| --- | ---: | ---: | ---: |
| #19: 1 file, 3 commits | 5.80 s | 2.27 s | 2.56x |
| #11: 26 files, 3 commits | 8.93 s | 2.58 s | 3.46x |

These are Enter-to-readable-overview medians in fresh private stores, with real
GitHub/Git access and identical complete snapshots. See the tenth-iteration
ledger for both paired series and their limits. Populated PR-list startup still
typically takes about 0.6 seconds. A separate release-build comparison with
1,000 saved read marks improved offline startup from 96.6 to 63.5 ms; this is
an incremental 1.52x gain against `5752542`. Neither startup scenario has reached
10x. Absolute offline times vary by fixture, build and host conditions.

## Design findings

The CLI opens SQLite before constructing the terminal workspace. Interactive
saved PRs already open from frozen storage and check freshness in the background.
Offline startup still initializes linked packages, validates storage, decodes
and validates the saved snapshot, and renders the terminal. Cached opening also
serializes the frozen snapshot to create the new independent session.

Cold opening builds an isolated Git object view, verifies ancestry, captures
the main inventory and each commit, gathers composition source and evidence,
then stores the result. Git commands remain isolated from checkout config,
hooks, filters and helpers. Inventory uses individual blob reads and blob-pair
diff subprocesses. Main inventory now batch-checks blob sizes and reads eligible
objects in a bounded Git batch. Repeated objects occur across the main comparison,
commit diffs, composition and evidence; these formerly launched Git each time.
Canonical blob-pair diff output is now also reusable inside the same view.
Eligible paths now share one bounded tree diff, with canonical blob headers
and byte-identical Git hunk bodies; unsupported paths retain blob-pair diffs.

Evidence retrieval formerly scanned all changed paths for every tree entry,
both when ranking candidates and choosing deleted-side evidence. This makes
ranking proportional to tree size times changed-file count, despite the small
final evidence budget. A directory/deleted-path index removes that repeated work.

## Experiment ledger (2026-10-18)

Apple M2 Max, macOS arm64, Go 1.26.9. Original implementation: `78cb664`.
End-to-end numbers are medians of three one-operation benchmark trials, with
synthetic GitHub responses, real Git subprocesses and private SQLite stores.
Fixtures contain one PR commit. There is no network/fetch time in these numbers.
These diagnostic runs share a development host; they are not CI thresholds.

| Experiment | Before | After | Decision |
| --- | ---: | ---: | --- |
| Index evidence paths: 10,000 tree files, 1,000 changed paths | 3,184 ms | 11.94 ms | Keep; about 267x for this stage |
| Cold open, 100 text changes with repeated boilerplate, 1,000 tree files | 9,172 ms | 3,968 ms | Keep combined changes; 2.31x overall |
| Cold open, 100 distinct text changes, 1,000 tree files | 11,210 ms | 6,225 ms | Keep combined changes; 1.80x overall |
| Cold open, 1,000 mode-only changes, 10,000 tree files | 4,650 ms | 649 ms | Keep combined changes; 7.17x overall |
| Cached open, repeated text fixture | 10.30 ms | 9.29 ms | Essentially unchanged; not the cold-path target |
| Cached open, distinct text fixture | 11.04 ms | 11.22 ms | Essentially unchanged |
| Cached open, mode-only fixture | 48.44 ms | 47.27 ms | Essentially unchanged |
| Offline first review frame, one-file saved review, 11 launches | 47.72 ms | 49.71 ms | No startup improvement established |

Indexing alone reduced the mode-change cold open to a median 1,662 ms.
Adding bounded blob reuse reduced it further to 649 ms. Repeated-boilerplate
text opens remained approximately 9 seconds after indexing alone; blob reuse
accounts for most of their improvement. The distinct-content fixture prevents
the repeated-content fixture from being mistaken for a general 2.31x result.

The blob cache lives only inside one isolated `source.View`, keyed by complete
object ID. It retains at most 8 MiB and 2,048 entries, never caches failures,
checks cancellation and each caller's size limit, returns independent byte
slices and releases entries on close. It does not persist source or change
freshness policy. The existing repeated-blob microbenchmark moved from about
11.15 ms to 1 microsecond per read; that is a warm per-view measurement, not
an end-to-end claim. Cold distinct reads still require Git.

An initialization trace identifies eager Chroma lexer/style registries and
SQLite libc network-database initialization as startup costs. The observed
process-launch floor (`--version`) is about 20 ms. Achieving a 4.8 ms first
review frame therefore needs more substantial startup work; the cold-open
optimizations do not address it. No dependency replacement has been attempted.
One startup trial had a 794 ms outlier; further measurements should interleave
binaries on an otherwise quiet host. A one-iteration UI CPU profile had too few
samples to support an optimization and was not used to justify changes.

### Second iteration: first-time PR selection

| Experiment | Prior checkpoint | Current median | Original → current |
| --- | ---: | ---: | ---: |
| Cold open, repeated text | 3,968 ms | 467.74 ms | 19.6x |
| Cold open, distinct text | 6,225 ms | 2,445.77 ms | 4.58x |
| Cold open, large mode-only PR | 649 ms | 735.94 ms | 6.32x |

The mode-only fixture has no textual blob-pair diffs and is not a target of the
new optimizations. Its host-level variation needs a quiet interleaved comparison
before attributing the difference to code. Cached-open measurements remain in
roughly the same 10–52 ms range. Eleven interleaved original/current offline
startup launches, after verification finished, measured medians of 42.73 ms and
44.82 ms respectively. There were large first-launch outliers (148 ms original,
813 ms current); no startup gain is established.

Changes were measured separately before combination:

- Bounded main-inventory batch reads reduced distinct-text opens from 6,225 ms
  to 4,384 ms. The protocol checks sizes first, reads at most the view's available
  8 MiB cache budget and validates every header, object identity, length, separator
  and final framing before exposing any bytes. Objects not read because of the
  batch cap retain the ordinary single-object path. A failed attempted read is
  returned to inventory, never silently retried. See the
  [Git batch protocol](https://git-scm.com/docs/git-cat-file#_batch_output).
- Reusing canonical immutable blob-pair diffs reduced distinct-text opens
  further to 3,818 ms, and repeated-text opens to 1,799 ms. This has a separate
  8 MiB / 2,048-entry cap. Only the inventory's exact fixed options and two full
  object IDs qualify; other options, paths, trees, refs and mutating commands
  are never cached. Size limits, cancellation, ownership and close semantics
  still apply, and every consumer charges its normal content budget.
- A CPU profile of complete opens then attributed 64% of sampled CPU to Chroma
  filename selection. The pinned registry tries 13 editor/package backup suffixes
  against every glob even when the filename has none. Checking actual suffixes
  once per filename, then testing the original basename and applicable stripped
  variants, preserves primary-pattern precedence, registry order, one match per
  pattern and Chroma's priority tie-breaking. Differential tests compare against
  the actual registry across every registered language, filename aliases, backup
  suffixes, case-sensitive names, unknown names and invalid UTF-8 paths.
  Go selection fell from a median 2.64 ms to 79 microseconds; TypeScript from
  2.22 ms to 77 microseconds. This optimization uses no warmed result cache.

The complete-open benchmark now reports subprocess counts. Distinct-text
opening currently launches six `cat-file` calls and 100 blob-pair `diff` calls;
the repeated-text case needs only one `diff`. The remaining distinct diffs are
the next measured bottleneck. The current measurements retain complete commit
capture, composition, evidence, persistence and freshness work.

Inspection also found that the verifier's existing metadata-timing wrapper hid
the optional commit-reader interface. Measured production opens could therefore
omit commit capture. The wrapper now retains that interface only when the
underlying client supports it. A regression test checks both supported and
unsupported clients and requires byte-equivalent captured history with timing
enabled or disabled. The synthetic benchmark numbers above never enabled this
wrapper and always included commits.

### Third iteration: reserved parallel diffs and broader fixtures

| Cold-open workload | Original median | Current median | Speedup |
| --- | ---: | ---: | ---: |
| Distinct content, shared basename | 11,210 ms | 1,495.69 ms | 7.49x |
| Distinct content and basenames | 12,782.32 ms | 1,513.29 ms | 8.45x |
| Ten commits, distinct content and basenames | 13,780.70 ms | 2,468.29 ms | 5.58x |

The last two baselines were newly measured on the original `78cb664`
implementation using the identical expanded benchmark file. Each result is a
median of three one-operation trials. The one-commit shared-basename workload
fell from the previous checkpoint's 2,445.77 ms to 1,495.69 ms in this iteration.
The benchmark now requires every expected commit and captured composition to
be present; omitted history causes failure instead of appearing as a speedup.

Known blob pairs can now use up to eight diff workers. The actual process runner
advertises a concurrency limit capped by `GOMAXPROCS`; custom runners retain
serial execution unless they explicitly advertise safe parallel use. The
benchmark counter wrapper uses a mutex and forwards that capability.

Planning follows original file order and stops before an unknown budget consumer
or an unfittable reservation. Every eligible file reserves a conservative byte
and line allowance, including repeated pairs; execution deduplicates identical
pairs. The reservation covers payload, line markers, missing-newline notices,
fixed OID headers, hunk headers/function context, and the per-hunk header copies
created by inventory. It is intentionally conservative and can leave large-file
work on the existing serial path. Bounds are checked before multiplication to
avoid integer overflow. The pool separately rejects aggregate output reservations
above the view's content budget.
The fixed function-context allowance follows the 80-byte buffer in
[Git's xdiff emitter](https://github.com/git/git/blob/master/xdiff/xemit.c).

Results preserve request order. The existing inventory loop applies its ordinary
byte and line checks and builds identical units; failed prefetched diffs are not
retried. Cancellation stops queued work and joins all active workers before
returning. No metadata, fetch, helper, upload or write operation is parallelized.
The canonical Git argument builder is shared by serial and parallel paths.

Regression coverage compares complete parallel and serial inventories, including
tight byte/line budgets, binary data, renames, mode changes and byte paths. It
checks actual Git patches with separated hunks, very long lines, absent trailing
newlines and late NUL bytes against reservation bounds. Pool tests cover result
order, the eight-worker cap, serial custom runners, invalid objects, aggregate
caps and cancellation cleanup. Full verification and lint passed; the complete
distinct-name and ten-commit benchmarks also passed under race detection (race
timings are excluded from this table).

Process counts explain the remaining work: the distinct-name fixture uses six
`cat-file` calls, 100 `diff` calls and five `ls-tree` calls; the ten-commit fixture
uses 24, 100 and 14 respectively. Original counts were 704/200/5 and 722/200/14.
Concurrency reduces waiting but retains a process per distinct blob pair.
Per-commit acquisition overhead also remains. The overall goal is still active:
these representative cold opens and startup have not established a 10x gain.

### Fourth iteration: remove per-file diff processes

The distinct-name fixture now performs one additional `diff-tree` operation
instead of 100 separate `diff` operations. Main/commit inventories reuse its
canonical blob-pair output. The complete loose-object fixtures measured medians
of 1,382.36 ms (one commit) and 2,295.74 ms (ten commits), compared with
1,513.29 ms and 2,468.29 ms at the preceding checkpoint. These are about 9.25x
and 6.00x faster than the original 12,782.32 ms and 13,780.70 ms. Removing forks
helps, but does not make these loose-object opens universally 10x faster.

`TreeDiffBatch` selects simple ASCII paths of at most 256 bytes, full pinned
object IDs and supported addition/modification/deletion shapes. Literal
pathspecs prevent expansion; additions/deletions that could select changed
descendants are excluded. Reservations retain the original file order and
aggregate content budget, with an additional 64 KiB argument cap. Unsupported
or unreserved requests remain unread and may use the existing worker pool.
An attempted batch failure is explicit and never falls through to a retry.

The command runs inside the same private bare Git view: checkout attributes,
hooks, filters and config remain unavailable. Frame paths, modes, old/new IDs,
headers and complete output are validated before caching any result. Only the
metadata header is normalized to the canonical blob-to-blob header; Git's hunk
bytes are copied unchanged. Tests compare independent blob-diff output for
adds/deletes, symlinks, executable mode changes, missing final newlines and
committed language attributes. Inventory equivalence also covers renames,
binary/empty/spaced/byte paths, file/directory replacements and tight quotas.
Malformed/missing/duplicate frames, cancellation and output failures cannot
seed the cache or become empty successful source. Checkout snapshots remain
unchanged. Full verification (including race and terminal tests), build and
configured lint passed.

Object layout now matters substantially. Packed variants run `git gc` only in
their disposable synthetic fixture, before timing; runtime code never packs,
prunes or alters the reviewed repository. Identical benchmark code was run
sequentially on the original revision, the preceding `511e6c8` checkpoint and
the new implementation, with no verification workload running. Medians of
three one-operation trials, including complete commit/composition capture:

| Packed fixture (1,000 tree files, 100 changed files) | Original | Prior checkpoint | Tree batch | Original → current |
| --- | ---: | ---: | ---: | ---: |
| One PR commit, distinct content and basenames | 12,528.54 ms | 625.11 ms | 436.08 ms | 28.7x |
| Ten PR commits, distinct content and basenames | 13,221.55 ms | 1,371.08 ms | 1,151.81 ms | 11.5x |

The isolated batching change reduces these packed opens by approximately 30%
and 16%. Cached medians remain approximately 13 ms and 17 ms. An earlier
original-revision trial overlapped repository verification and was excluded
from this comparison because of contention. These are synthetic local-work
measurements, excluding network latency and process initialization; the source
store is packed before every revision's measurement. They establish 10x gains
for these two shapes, not for loose stores, all PRs, live selection or startup.

A diagnostic loose-object open took
1,325 ms overall, including 542 ms creating the isolated view, 151 ms verifying
the merge base, 249 ms building the main inventory and 77 ms retrieving evidence.
These one-trial phase timings identify work to investigate, not stable speedup
claims. `NewView` walks and hard-links the complete object store, so its cost
scales with stored history as well as the selected comparison. The development
repository also contains many loose objects duplicated in its pack, but safely
avoiding redundant links needs independent object validation and lifetime
guarantees; pointing the view at checkout alternates would violate isolation.

### Fifth iteration: bounded object-file linking

The mirror now creates object-file hard links with at most four workers, capped
by `GOMAXPROCS`; a single worker uses the original serial walk. Directory
validation/creation stays in walk order. The existing entry validator still
rejects symlinks and unsupported files and excludes metadata/alternates. The
queue contains at most four entries. The first failure stops further useful
work, and every worker joins before `NewView` returns or removes private data.
There are no retries and no changes to the reviewed object store.

A disposable filesystem microbenchmark compared serial linking with one,
two, four and eight workers, including removal of the borrowed view. Payloads
are synthetic object files; complete PR benchmarks separately use real Git
objects. Medians of three one-operation trials:

| Object entries | Serial | Four workers | Eight-worker experiment |
| --- | ---: | ---: | ---: |
| 2,000 | 585.31 ms | 350.11 ms | 484.95 ms |
| 20,000 | 6,095.48 ms | 3,752.96 ms | 4,751.22 ms |

Four workers won at both sizes. Eight workers were slower and were discarded;
the permanent benchmark compares the supported one/two/four-worker variants.
The serial checkpoint (`0dcf962`) and current implementation were then measured
sequentially, without verification running, on the identical complete-open
fixtures. These medians include pinning, inventories, commit/composition capture,
evidence, durable storage and cleanup, with synthetic GitHub responses:

| Cold-open fixture | Serial checkpoint | Four workers | Original → current |
| --- | ---: | ---: | ---: |
| Loose objects, one commit, 100 distinct files | 1,365.04 ms | 1,094.50 ms | 11.7x |
| Loose objects, ten commits, 100 distinct files | 2,323.91 ms | 2,048.29 ms | 6.73x |
| Packed objects, one commit, 100 distinct files | 427.52 ms | 424.05 ms | 29.5x |
| Packed objects, ten commits, 100 distinct files | 1,158.09 ms | 1,142.42 ms | 11.6x |

The isolated change improves loose-object opens by approximately 20% and 12%.
Packed objects and cached opens are essentially unchanged; their small deltas
are inside host noise. This does not establish a startup gain or a 10x gain for
the loose ten-commit case. Regression tests check identical borrowed paths,
hard-link identity and survival after removal of the original files, excluded
metadata, directory/file symlinks, cancellation and complete private-directory
cleanup after a failed concurrent borrow. Real Git inventory equivalence and
checkout-immutability coverage remain in the complete verification suite.
Full verification passed, including race tests, terminal journeys and build;
configured lint reported zero issues.

### Live first-open check: repository PR 109

The real [PR #109](https://github.com/steve-mackinnon/prui/pull/109) provides a
less favorable, important counterexample to the synthetic results. It has nine
changed files and one captured PR commit. Three original/current CLI opens were
interleaved with a fresh private session store per run, actual GitHub responses,
the same checkout/object store, and unchanged base/head pins:
`46e5f056cf5f28aef146e9fdd3c12f9917c40505` /
`95eb482523a9a864c8978ff0e48f1df2bc4c1508`.

| Process-to-complete plain open | Original `78cb664` | Current `f6cc14a` |
| --- | ---: | ---: |
| Trial 1 | 6,843.97 ms | 3,805.86 ms |
| Trial 2 | 5,696.83 ms | 3,786.07 ms |
| Trial 3 | 5,597.58 ms | 3,711.19 ms |
| Median | 5,696.83 ms | 3,786.07 ms |

This is **1.50x**, not 10x. Every run captured a complete inventory, complete
commit membership and captured composition. Decoded immutable snapshot payloads
were identical across each original/current pair, including inventory/patches,
syntax, commits/composition, context, description, slices and unit ownership.
No prior snapshot was reused. The original binary was rebuilt from an archive
of `78cb664`; timing instrumentation was disabled in both comparison binaries
to avoid the original wrapper's optional-commit-reader bug.

A separate current-only diagnostic took 4,094 ms overall, with 1,328 ms in view
setup, 465 ms in metadata reads, 130 ms verifying the merge base, 298 ms in the
main inventory and 457 ms retrieving evidence; no object fetch was required.
These individual phase numbers are one observation, not median speedup claims.
The real mixed object store and network costs make the favorable packed-fixture
results insufficient evidence for the user's primary scenario. Next work should
reduce mirroring of redundant/unneeded historical objects with independent
integrity/lifetime proofs, and profile evidence/teardown and the remaining
network sequencing. Preserve all source and freshness checks.

For reproduction, launch `open <PR-URL> --plain` in the same checkout using each
binary, a new `HOME` directory for every trial and the caller's existing
`GH_CONFIG_DIR` (as `journeyEnvironment` already does). On macOS,
`XDG_DATA_HOME` alone does **not** redirect app storage. An initial probe used
that ineffective override; its one newly created local session was deleted
and verified absent. It is excluded from the comparison above. All subsequent
probes used isolated homes. Local trial artifacts are in
`/tmp/prui-live-compare-19osfm43` for this development session.

### Sixth iteration: verified copies replace redundant loose links

The real object store contains one pack with 9,019 objects and more than 9,000
duplicate loose entries. Mirroring every loose entry costs substantially more
than the selected nine-file comparison. The optimized path copies a bounded
pack/index pair into private storage and lets Git independently verify it before
omitting the corresponding loose links. It never packs or prunes the checkout.

Eligibility requires at least 2,048 duplicate loose objects, one supported index
pair, at most 10,000 indexed objects, and at most 16 MiB of retained compressed
pack/index input within the caller's limits. SHA-1 v2 indices without large
offsets are supported. Safe bounded basenames include the real store's
`loose-…` naming as well as `pack-…`; unsupported names/layouts use the ordinary
mirror. Candidate hard links are read only. Final copies use exclusive creation
and cannot overwrite a borrowed object. Successful verification alone authorizes
omission; a checksum-valid but forged index cannot bypass that verification.

The resource preflight verifies checksums and exact sequential zlib framing,
including every indexed stream boundary. It bounds raw object/instruction sizes
and delta result sizes by the caller's blob limit (at most 1 MiB), rejects thin
references, and bounds delta depth/cycles. Cumulative discarded verification
work has a separate 96 MiB cap; captured source retains its existing 50 MiB and
line quotas. Input copies remain within the 16 MiB ceiling. The verifier is
`index-pack --verify --no-rev-index --threads=1 --max-input-size=16777216` inside
the private view. It checks object identities and the complete index. Reverse
index handling is disabled explicitly; checkout bytes and file modes stay
unchanged. See [Git's pack format](https://git-scm.com/docs/gitformat-pack) and
the [verification implementation](https://github.com/git/git/blob/v2.39.5/builtin/index-pack.c).

Copies live under `borrowed/pack`, keeping the existing fetch reachability
barrier and fetch-storage accounting intact. An ordinary verification failure
authorizes no omissions and retains ordinary mirroring; the verifier itself
is not retried. A resource failure or cancellation stops opening and cleans up.
Regression tests cover real REF/OFS delta packs, corrupt and forged indices,
size/overflow/depth/cycle limits, false record boundaries, cancellation, failed
verification, symlink rejection even for covered duplicates, survival after
removing the original object store, exclusive-copy safety, fetch reachability,
pure-pack bypass, and unchanged checkout contents/modes.

The actual store's setup/release microbenchmark, including a pinned commit read,
uses three one-operation trials without network or concurrent verification:

| Real object-view lifecycle | Prior checkpoint | Verified copies |
| --- | ---: | ---: |
| Complete create/read/release median | 1,986.66 ms | 439.19 ms |
| Setup median | 1,376.96 ms | 407.93 ms |
| Release median | 563.52 ms | 17.96 ms |

A first correct streaming implementation allocated about 404 MB cumulatively
per lifecycle because it created a decoder per record. Reusing the decoder
within one preflight reduced that to about 37 MB and lowered lifecycle time
from 471.02 ms to 439.19 ms. These are allocation totals, not peak resident
memory; the prior mirror allocated about 7.8 MB. The earlier disposable prototype
took about 272 ms but lacked complete resource preflight and was not adopted.
An initial guarded build declined the real pack and measured no improvement
(3.817 s prior / 3.890 s experimental). Diagnostic inspection identified its
noncanonical basename and 75.5 MB of cumulative verification work. Those
declined-path trials are not counted as optimization wins.

Identical complete-open benchmarks were then run sequentially against the prior
checkpoint (`b141322`, same code as `f6cc14a`) and the final implementation.
Mixed fixtures run `repack -a` only during disposable setup, retaining loose
duplicates. Medians of three one-operation trials:

| Cold-open fixture, 100 distinct files | Prior | Verified copies |
| --- | ---: | ---: |
| Loose objects, one commit | 1,075.33 ms | 1,075.16 ms |
| Loose objects, ten commits | 2,021.89 ms | 2,029.98 ms |
| Packed objects, one commit | 415.30 ms | 422.12 ms |
| Packed objects, ten commits | 1,114.99 ms | 1,125.96 ms |
| Mixed duplicate objects, one commit | 819.18 ms | 472.73 ms |
| Mixed duplicate objects, ten commits | 1,524.06 ms | 1,194.01 ms |

The isolated change reduces mixed-store opens by about 42% and 22%. Loose,
pure-packed and cached results are essentially unchanged; their small deltas
are inside host noise. The mixed cases perform one bounded `index-pack` check;
all four loose/pure-packed cases perform zero. Every trial requires complete
inventory, commit membership and composition capture.

Live measurements must preserve authentication as well as isolate storage.
On this host, changing `HOME` prevents GitHub CLI from finding its normal macOS
keychain token even with `GH_CONFIG_DIR` preserved. Earlier isolated-home
measurements used public unauthenticated requests and eventually hit that
quota. Subsequent comparisons obtain the existing token through `gh auth token`
in the normal environment and pass it only in subprocess memory via `GH_TOKEN`.
No credential is logged or saved. Do not mix authenticated and unauthenticated
samples when computing speedups. The first authenticated three-way comparison
measured medians of 6,534.38 ms original, 4,636.34 ms prior, and 3,082.18 ms for
the verified-copy implementation before decoder reuse. All nine opens captured
complete source and produced identical decoded snapshots within each group.
Artifacts for that comparison are `/tmp/prui-sixth-verified-50hp7rhi`.

The final decoder-reuse build was rechecked in another three-way authenticated
comparison, with fresh private homes and no overlapping benchmark/verification
work. Complete process-to-plain-open times:

| Trial | Original `78cb664` | Prior `b141322` | Final verified copies |
| --- | ---: | ---: | ---: |
| 1 | 6,504.40 ms | 4,620.71 ms | 3,838.81 ms |
| 2 | 6,584.52 ms | 4,614.75 ms | 3,122.87 ms |
| 3 | 6,401.43 ms | 4,477.98 ms | 3,054.43 ms |
| Median | 6,504.40 ms | 4,614.75 ms | 3,122.87 ms |

The isolated change reduces this real open by about 32%; the full branch is
about 2.08x faster than the authenticated original baseline. Every open captured
all nine files, complete commit membership and composition. All three groups
produced identical decoded snapshot payloads across the original, prior and
final implementations, with the same base/head pins recorded above. The first
optimized trial is an outlier; all three remain below every prior
trial. Artifacts: `/tmp/prui-sixth-verified-mo26r0k7`. This still does not prove
the requested 10x real-selection gain or any startup improvement.

Eleven interleaved original/final offline PTY launches measured first readable
review-frame medians of 43.91 ms and 44.66 ms (ranges 42.07–100.76 ms and
42.39–46.12 ms). No startup improvement is established. Parser fuzzing also
checks mutations with repaired outer checksums so framing/resource checks remain
reachable; its result supplements the explicit adversarial fixtures.
The 30-second fuzz run completed 6,296 executions with no crash; a five-second
run with minimization disabled completed another 32,688 executions. Full
repository verification passed, including vet, race tests, terminal journeys,
installation checks and build. Configured lint reported zero issues.

### Seventh iteration: bounded evidence acquisition

Supporting evidence still used one Git process per uncached blob. A local
complete open of the recorded real PR used 40 `cat-file` calls; the synthetic
changed-file fixtures hid this cost because their first-ranked evidence was
already in the main inventory's blob cache.

The new reader uses the existing validated `BlobBatch` capability in windows
of at most eight potential excerpts. Every candidate reserves one possible
file and its maximum excerpt bytes, including repeated OIDs. Path/privacy,
symlink/gitlink, seen-path and OLD/NEW-side checks run before lookahead selects
an ID. The ordinary loop still emits results and omissions in the original
order, applies content privacy and truncation, and updates the same budgets.
A final partial excerpt or a single distinct ID stays serial. Only the current
window retains blob contents (at most 256 KiB of blob data under default excerpt
limits, plus the existing bounded protocol framing). At most
2,048 failed batch IDs are retained locally to prevent automatic retries;
batching stops before that set fills. Omitted/unread IDs alone may fall back
to `Blob`. No file, excerpt, time, cache or source-capture limit was increased.

Tests compare full evidence bundles and logical read scope against serial
retrieval, including tight file/byte limits, repeated OIDs, deleted-side source,
path/content exclusions, unavailable blobs, partial backend selection, bounded
failure retention and cancellation. Real Git tests compare pinned source and
verify that checkout bytes/modes remain unchanged.

A new complete-open fixture contains 1,000 original tree files, 100 changes,
and 100 additional distinct, unchanged supporting test files near the changes.
The benchmark requires all 100 supporting excerpts, complete inventory and
complete commit/composition capture, with no exhausted evidence budget. Setup
is outside the child timer. Median of three one-operation trials:

| Complete open, network excluded | Prior `d11ea1f` | Batched evidence |
| --- | ---: | ---: |
| Distinct unchanged supporting files | 2,233.31 ms | 1,432.52 ms |
| `cat-file` processes | 106 | 35 |
| Recorded real PR, full ordinary open/store path | 1,597.49 ms | 1,446.64 ms |
| Recorded real PR `cat-file` processes | 40 | 25 |

These are about 36% and 9.4% reductions in local complete-open work. The recorded
benchmark uses the real checkout and frozen PR metadata in fresh private stores;
it checks identical inventory, context and commits against the saved reference.
It makes no network request. Its disposable diagnostic source is in
`/tmp/prui-perf-sixth` / `/tmp/prui-perf-seventh`; it is not a CI fixture.
An initial optimized supporting-file run overlapped a build and was excluded;
the reported three-trial timings ran alone. Separate one-operation runs with
the stricter all-excerpts assertions also passed for both implementations.

Three authenticated real first opens then ran interleaved, with new private
homes and existing credentials passed only in process memory:

| Trial | Prior | Batched evidence |
| --- | ---: | ---: |
| 1 | 3,338.63 ms | 3,822.11 ms |
| 2 | 3,122.30 ms | 2,936.88 ms |
| 3 | 3,122.11 ms | 2,938.14 ms |
| Median | 3,122.30 ms | 2,938.14 ms |

The median improved about 5.9%, with one slower optimized outlier. Every run
captured nine changed files, complete commit membership/composition and all
33 reference excerpts; decoded snapshots were identical within every pair.
Pins were unchanged from the prior real-PR comparison. Artifacts:
`/tmp/prui-seventh-live-kb_i59e9`. The current median is approximately 2.2x
faster than the earlier authenticated original baseline, still short of 10x.
This change does not address startup or remove any network/freshness check.
Full verification passed, including race tests, terminal journeys, installation
checks, vet and build. Configured lint reported zero issues.

## Eighth iteration: reuse connections for capture reads

The ordinary first-open path performs three independent metadata reads and one
commit-list read. These are consistency boundaries: none was removed, combined
or served from a response cache. Four sequential authenticated requests through
separate CLI processes took a median 1,425 ms; the same endpoints through one
reusable HTTP client took 1,074 ms (three trials, about 25% less for this stage).
The diagnostic reused existing credentials only in memory. It excluded CLI
credential-export cost; complete-open measurements below include that cost.

Only `Metadata` and `ListPullRequestCommits` now share a bounded client. Every
request still resolves the active account through `gh auth token --hostname
github.com`; no token is cached. Custom runners preserve the CLI boundary.
If token export fails before an HTTP attempt, the original CLI read remains
available. Cancellation, deadline and resource-limit failures do not fall back;
neither do attempted HTTP reads. Writes and all other API operations retain the
original CLI path. Offline startup still constructs no GitHub client.

The client fixes HTTPS/API origin, bounds connections, headers, decompressed
response bytes and the total authentication-plus-request deadline. Redirects
are bounded and confined to the same API origin. Error bodies and transport
errors never enter diagnostics. Existing UTF-8, JSON, identity, pins and commit
validation remain the final authority. The chosen API version retains the
[documented default](https://docs.github.com/en/rest/about-the-rest-api/api-versions);
credential lookup uses the CLI's
[active-account behavior](https://cli.github.com/manual/gh_auth_token).

Three real PR 109 first opens ran interleaved with new private homes, no previous
snapshots, and existing authenticated credentials passed only in memory:

| Trial | Prior `99791ff` | Reused connection |
| --- | ---: | ---: |
| 1 | 3,063.19 ms | 3,351.67 ms |
| 2 | 2,933.78 ms | 2,592.97 ms |
| 3 | 2,879.32 ms | 2,608.90 ms |
| Median | 2,933.78 ms | 2,608.90 ms |

The median improved about 11%, with a slower optimized first trial. Every pair
had identical decoded snapshots: nine files, complete commit membership and
composition, and 33 evidence excerpts. Pins matched the preceding live trials.
Artifacts: `/tmp/prui-eighth-live-fiemku9o`. This is approximately 2.5x faster
than the earlier authenticated original baseline, still short of the 10x goal.

A separate five-pair confirmation alternated the order on each trial:

| Trial | Prior | Reused connection |
| --- | ---: | ---: |
| 1 | 3,029.26 ms | 2,646.35 ms |
| 2 | 2,878.25 ms | 2,637.95 ms |
| 3 | 2,907.70 ms | 2,810.49 ms |
| 4 | 2,961.33 ms | 2,770.89 ms |
| 5 | 3,148.56 ms | 2,775.25 ms |
| Median | 2,961.33 ms | 2,770.89 ms |

This confirms a smaller 6.4% end-to-end improvement, with identical complete
snapshots in all five pairs. Artifacts: `/tmp/prui-eighth-confirm-1q_cpkpu`.
Use the observed 6–11% range for this incremental change rather than treating
the first three-trial median as a universal result. The confirmation is about
2.35x faster than the original authenticated baseline. A separate four-request
probe using the normal macOS keychain, resolving credentials afresh on every
request, improved from 1,559 to 1,200 ms (three trials); the benefit survives
ordinary credential lookup without setting a benchmark-only token path.

Eleven interleaved offline startup trials measured 44.33 ms originally and
44.85 ms now. The first series included a 133 ms original and an 813 ms new
maximum; their causes were not established. A second eleven-trial series
measured 43.96 / 44.90 ms, with maxima 44.84 / 46.74 ms. Every second-series
sample is recorded in `/tmp/prui-eighth-startup-confirm.json`. There is no
startup improvement, and the first-series tail is not claimed as eliminated.

Synthetic tests exercise actual TLS connection reuse, per-read credential
changes, same-origin redirect policy, response bounds, cancellation during
body reads, common authentication/request deadlines, malformed responses,
no fallback after dispatched reads, and unchanged CLI writes/custom runners.
The full verification script passed (installer fixtures, vet, race tests, terminal
journeys and build), and configured lint reported zero issues.

## Ninth iteration: reuse successful syntax capture within an open

A CPU/allocation profile of the complete recorded PR 109 open identified syntax
capture as about 35% of sampled CPU and 67% of allocated bytes. Main inventory
and commit inventory repeated the same successful lexing. The profile included
benchmark warmup/setup; its percentages identify work, not end-to-end savings.
Profiles and disposable diagnostic module: `/tmp/prui-perf-ninth`.

Each open attempt now carries a fresh bounded highlighting cache through its
child contexts. Exact path plus SHA-256 of the complete source identifies a
result. Only successful captures enter the 8 MiB / 128-entry cap, with conservative
accounting for keys, maps, slices and spans. Stored values and returned hits are
independent copies; source bytes and failure statuses are never retained.
Cancellation is checked before hits and after copying. Independent attempts do
not share entries. Existing source retrieval, identity/integrity validation,
per-blob limits, inventory byte/span limits and elapsed-work budgets all remain
active. The cache saves actual lexer work, so it may make more optional syntax
available before the time budget expires; unavailable source is never upgraded.

Complete ordinary open/store path, recorded GitHub metadata and real immutable
Git source, new private stores, three one-operation trials without concurrent
builds/tests or profiling overhead:

| Trial | Prior `c8a0298` | Reused syntax |
| --- | ---: | ---: |
| 1 | 1,448.35 ms | 1,238.61 ms |
| 2 | 1,472.78 ms | 1,231.10 ms |
| 3 | 1,429.85 ms | 1,227.21 ms |
| Median | 1,448.35 ms | 1,231.10 ms |
| Median allocated bytes | 186,644,408 | 123,005,680 |
| Median allocations | 2,341,479 | 1,263,802 |

This is about 15% less local complete-open time, 34% fewer allocated bytes, and
46% fewer allocations. It does not include network time. Every run requires
identical inventory (including syntax/statuses), context and commit/composition
capture against the same saved reference. Git command counts remain unchanged:
25 `cat-file`, three `diff-tree`, five `ls-tree`, and one `index-pack` per open.

Tests cover exact identity, result independence, failure non-retention, entry
and memory caps, cancellation, independent scopes, concurrent use, language and
invalid-text equivalence. Warm-cache inventory tests preserve the original
time, byte and span exhaustion statuses. `BenchmarkTokenizeCaptureReuse` models
two captures of one blob, starting a fresh scope on every iteration.

Five authenticated live PR 109 first-open pairs alternated their trial order,
using fresh private homes and in-memory credentials:

| Trial | Prior | Reused syntax |
| --- | ---: | ---: |
| 1 | 2,924.65 ms | 3,279.09 ms |
| 2 | 2,757.96 ms | 2,573.05 ms |
| 3 | 2,691.79 ms | 2,591.63 ms |
| 4 | 2,900.72 ms | 2,401.16 ms |
| 5 | 2,678.85 ms | 2,482.35 ms |
| Median | 2,757.96 ms | 2,573.05 ms |

The measured reduction is about 6.7%, including one slower optimized first
trial. Every decoded snapshot was identical within its pair, including all
nine files, 33 evidence excerpts and complete commit membership/composition.
Artifacts: `/tmp/prui-ninth-live-rv173vyt`. The current median is about 2.53x
faster than the authenticated original baseline, still below the 10x goal.
The PR-list opener delegates its uncached path to the same ordinary open; guide
generation is a separate consent action and does not block that opener. Actual
interactive selection-to-readable-frame timing still needs separate measurement.

The isolated two-capture benchmark measured 7.137 ms without reuse and 3.549 ms
with it (median of three ten-iteration trials). Allocated bytes fell from
3,304,503 to 1,688,136. Each timed iteration creates a new scope; this is not a
cross-open cache-hit measurement. Eleven interleaved offline launches measured
43.84 ms originally and 45.05 ms now, with maxima 136.02 / 46.44 ms. All samples
are in `/tmp/prui-ninth-startup.json`; there is no startup gain.

Full verification passed (installer fixtures, vet, race tests, terminal journeys
and build). Configured lint reported zero issues; the final focused race checks
also passed. This change retains every pinning/freshness read and source check.

## Tenth iteration: measure actual selection and overlap pin inputs

The earlier live checks used complete plain-command opens. The actual PR browser
was then exercised through a PTY: launch with no arguments from the checkout,
wait for the populated PR list, select a PR, then measure Enter to the first
readable review frame with its `N/M read` indicator. Each trial uses a fresh
private home/store, real authenticated read-only GitHub requests and real Git.
No guide generation, review write, source upload or old snapshot is involved.
The captured initial frame is the overview, not a selected file's expanded diff.

Three pairs alternated their order per PR. Original `78cb664` versus `ff8282f`:

| PR | Changes / commits | Original selection | Prior optimized selection |
| --- | --- | ---: | ---: |
| #19 | 1 file / 3 commits | 5,799.98 ms | 2,716.55 ms |
| #11 | 26 files / 3 commits | 8,934.10 ms | 2,999.65 ms |

Every pair had identical decoded snapshots, complete inventory, complete commit
membership/diffs and captured composition. PR #19 pins: base
`b39e451b3ef96e3bfda250a489ebe10da559984e`, head
`bddf8d8912fa419fcb75f4aca89fb535cb483c76`. PR #11 shares that base, with head
`49239c1f4c528b8b0713ccb033b3b65081250902`. Screens and per-trial data:
`/tmp/prui-selection-compare-32a8oo7t`. An initial current-only probe also captured
a complete PR #19 review in 2,934 ms; it was not part of the paired medians.

Initial metadata and private view setup were sequential, despite having no
shared input or checkout writes. They now overlap for runners that advertise
parallel use. The metadata result retains error precedence. Both tasks must
complete successfully before ancestry/pinning, fetch or capture can proceed.
Metadata failure cancels and joins setup; every successful speculative view is
closed before returning an error. Parent cancellation, view resource failures,
serial-runner ordering, retry/fetch and all freshness reads remain enforced.
No partial view is reused. Retry attempts still build their new view after the
updated metadata, in their original order.

Another three pairs per PR compare `ff8282f` against overlap, alternating order:

| PR / trial | Prior | Overlapped inputs |
| --- | ---: | ---: |
| #19 / 1 | 2,817.36 ms | 2,267.63 ms |
| #19 / 2 | 2,731.06 ms | 2,233.35 ms |
| #19 / 3 | 2,748.60 ms | 2,331.78 ms |
| #19 median | 2,748.60 ms | 2,267.63 ms |
| #11 / 1 | 3,015.74 ms | 2,566.46 ms |
| #11 / 2 | 3,117.22 ms | 2,601.33 ms |
| #11 / 3 | 2,966.51 ms | 2,584.01 ms |
| #11 median | 3,015.74 ms | 2,584.01 ms |

This saves about 17.5% / 14.3% of actual first selection, respectively.
All six pairs had identical complete snapshots. A separate cross-series check
also matched each of the six latest snapshots against its PR's original
reference. Original-to-current ratios are about 2.56x / 3.46x, still below 10x;
these original timings come from the immediately preceding paired series.
Artifacts: `/tmp/prui-pin-overlap-selection-o2e6xw75`.

Populated PR-list launch time is measured separately from selection. Typical
trials remained around 0.6 seconds, with a 1.35-second first candidate launch
outlier. This change runs after selection and does not accelerate that startup
path. Individual view-setup and metadata durations now overlap; do not add them
or treat their sum as complete-open time.

Barrier-based tests cover actual overlap without timing-speed assertions,
serial ordering, metadata/view failure precedence, resource failure, parent
cancellation, joining setup and removing completed/failed speculative views.
The complete verification script passed, including installer fixtures, vet,
race tests, terminal journeys and build. Final focused race checks passed and
configured lint reported zero issues. Offline startup has not been remeasured
for this candidate; overlap is entered only after a PR source-open request.

## Eleventh iteration (2026-10-19): startup profiling and progress-row reuse

Release-equivalent builds use `CGO_ENABLED=0`, `-trimpath`, and `-ldflags='-s -w'`.
Original/current offline PTY startup medians were 141.17 / 146.34 ms in eleven
interleaved trials under the host conditions of this run. Version-only launch
medians were 207.51 / 124.44 ms, with first-execution maxima of 2.83 / 2.18 s;
these do **not** demonstrate an application startup improvement. Three separate
initialization traces per build put the largest median package costs around
14–16 ms each for Chroma lexers, Chroma styles and libc's service database.
Samples/traces: `/tmp/prui-release-startup-results`. Do not compare these absolute
timings with the earlier approximately 45 ms development-build launches across
different host/cache conditions.

A disposable 120-FPS renderer experiment did not establish an improvement:
eleven interleaved offline trials measured 184.31 ms at the normal cadence and
255.69 ms at 120 FPS, with broad ranges/outliers. No renderer change was kept.
Samples: `/tmp/prui-startup-fps-samples.json`. Eager dependency initialization
would require a separate compatibility-preserving design; no fork, lazy-loading
shortcut, integrity-check removal or durability reduction was introduced.

`BenchmarkOfflineStartupComponents` now separates connection opening, snapshot
load/validation, and model loading through the first rendered frame. Fixtures
use synthetic committed Git objects, private storage and captured Markdown;
there is no online client or credential use in timed offline model loading.
One shape has an unread single-file comparison; another has 1,000 mode changes
and all 1,000 saved read marks. Source/schema setup is outside child timers.
Package/process initialization and actual terminal I/O are not included.

The profile exposed per-mark SQL validation plus deletion/reinsertion on every
freshness reset. `UpdateState` now checks unchanged nonempty progress in one
transactional joined query. It requires exact IDs/order, contiguous ordinals and
membership in the session's current snapshot. Only a completely equivalent valid
set skips rewriting rows. Changed/corrupt sets retain the old validation/rewrite
path. Empty requested progress retains the original single-DELETE path, avoiding
extra work for first-time opens and ordinary unread sessions.

The generation/reference CAS, status/timestamp update and durable commit still
run on **every** update. An unchanged row set cannot bypass stale-writer checks,
parent cancellation, snapshot membership or error rollback. Tests deterministically
count progress writes, verify durable generation/freshness advancement and stale
rejection, reject unknown existing IDs, repair noncontiguous ordinals and preserve
reordered/cleared marks. The optimizer bounds returned IDs to 256 bytes and
retains the original path for longer requested legacy IDs. Oversized malformed
stored IDs are rejected or repaired without materializing them in Go.

Median of three three-operation synthetic state-update trials:

| Saved read marks | Prior `5752542` | Reused rows |
| --- | ---: | ---: |
| 0 | 0.432 ms | 0.407 ms |
| 1 | 0.588 ms | 0.491 ms |
| 1,000 | 20.382 ms | 1.947 ms |
| 10,000 | 228.332 ms | 20.846 ms |
| 10,000 allocated bytes | 15,331,704 | 1,325,512 |

The 1,000-/10,000-mark **state update** is about 10.5x / 11.0x faster. This is
not a 10x startup or first-PR-selection result. At zero/one mark the sub-millisecond
difference is not treated as a material win. Baseline/final logs:
`/tmp/prui-progress-before-final.txt`, `/tmp/prui-progress-final.txt`.

Complete offline model loading through a readable frame (median of three
three-iteration trials; connection open/package initialization/PTY excluded):

| Fixture | Prior | Reused rows |
| --- | ---: | ---: |
| 1,000 files, all read | 49.557 ms | 28.164 ms |

This is about 43% less model-load time. The comparison uses the same benchmark
source in an isolated prior archive, retains complete inventory and validates
the expected read-count indicator. Logs: `/tmp/prui-startup-model-before.txt`
and `/tmp/prui-startup-model-final.txt`. The one-file unread control remained
about 1.5 ms for this component; its empty progress path is unchanged.

Final release-equivalent whole-process PTY startup with the retained synthetic
1,000-file/all-read snapshot measured **96.62 ms before / 63.51 ms after**
(eleven interleaved trials, one untimed warmup per executable). Ranges were
94.98–98.08 / 60.44–64.74 ms. Every trial reached the expected `1000/1000 read`
frame; source-payload hashes and the 1,000 stored marks were unchanged afterward.
Samples: `/tmp/prui-progress-startup-samples.json`. This is about 34% less full
startup time, or 1.52x faster, against the prior optimized revision `5752542`;
it is not the overall 10x goal. The retained fixture is private, synthetic and
works with its generating checkout removed.

The final complete verification script passed: installer fixtures, vet, race
tests, terminal journeys and build. Configured lint reported zero issues. The
final benchmark also requires complete commit capture/composition in both source
fixtures. All original freshness writes, durability settings and source validation
remain enforced. The overall goal remains active.

## Reproduction and validation

```sh
go test ./internal/context -run '^$' -bench BenchmarkRetrieveLargeTree -benchtime=1x -count=3
go test ./cmd/prui -run '^$' -bench BenchmarkPullRequestOpen -benchtime=1x -count=3
go test ./internal/source -run '^$' -bench BenchmarkBlobRead -benchtime=1s -count=3
go test ./internal/source -run '^$' -bench BenchmarkBorrowObjects -benchtime=1x -count=3
go test ./internal/syntax -run '^$' -bench BenchmarkLexerSelection -benchtime=100x -count=3
go test ./internal/syntax -run '^$' -bench BenchmarkTokenizeCaptureReuse -benchtime=10x -count=3
go test ./internal/session -run '^$' -bench BenchmarkSQLiteFreshnessProgress -benchtime=1x -count=3
go test ./cmd/prui -run '^$' -bench BenchmarkOfflineStartupComponents -benchtime=3x -count=3
./scripts/verify.sh
```

For baseline comparisons, copy the benchmark test file into a separate checkout
of the original revision and run the identical command. Fixture setup is outside
each child benchmark's timer. Cold trials bypass predecessor selection and build
new source; cached trials reuse the stored snapshot and never claim it is fresh.
On macOS the sandbox can make Git emit a `confstr` warning into synthetic fixture
SHA output and block local HTTP servers; run verification with the needed host
access instead of altering the tests.

Regression tests cover directory/path ranking and deleted-side selection,
immutable bytes, tighter limits on reuse, cancellation, closed views, cache
caps and retry after failed reads. Additional coverage checks batch framing,
size preflight, cap enforcement before content reads, malformed metadata,
no automatic resource-limit retries, canonical diff reuse, complete inventory
equivalence, registry matching equivalence and timing-enabled commit capture.
The complete repository verification script passed after the second iteration,
including vet, race tests, PTY tests and build; configured lint reported zero issues.

## Next iterations

1. Broaden the interactive first-selection fixtures beyond PRs #19 and #11,
   including more commits and missing-object fetches. These two real PRs now
   open about 2.56x / 3.46x faster, still below 10x. Typical populated PR-list
   startup remains around 0.6 seconds; median offline startup is about 45 ms.
2. Profile the remaining GitHub request/process/connection overhead and per-commit
   work. Preserve every pinning, freshness and completeness check; investigate
   further transport work before changing consistency boundaries. Connection
   reuse is now in place for capture reads. Reuse authenticated
   benchmark setup without saving credentials. Evidence reservations remain
   conservative; inspect remaining serial/oversized reads before extending APIs.
3. Expand full-source sizes/change shapes and measure fresh-process first-time
   selection. The new distinct-basename and ten-commit fixtures show that the
   result is not yet universally 10x. Investigate the per-commit overhead while
   retaining membership, ancestry, completeness and freshness checks.
4. Profile large cached snapshot decoding/validation and session creation.
   Any storage optimization must keep integrity validation, isolation and stale
   writer rejection. Do not trade these checks for a faster first frame.
5. Measure eager package initialization separately before considering dependency
   isolation. A startup rewrite needs evidence of a user-visible benefit.
6. Broaden verified-copy eligibility only with evidence from larger/multiple-pack
   stores. Current unsupported layouts retain ordinary mirroring. Retain all
   integrity, work, cancellation, fetch-barrier and cleanup guarantees.

Continue comparing complete opens after each change. Do not mark the goal
complete based on a favorable fixture, cache hit or isolated stage.
