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

## Verified interactive selection baselines

Matched release builds of original `78cb664` and runtime `405c105`, using the
same toolchain/flags and three alternating fresh-profile pairs per real PR:

| Real PR | Original readable | Current readable | Readable speedup | Fully checked speedup |
| --- | ---: | ---: | ---: | ---: |
| #19: 1 file, 3 commits | 6,166.76 ms | 1,066.61 ms | 5.78x | 4.53x |
| #11: 26 files, 3 commits | 9,650.17 ms | 1,217.74 ms | 7.92x | 6.58x |

All six paired decoded snapshots were identical and complete. Readable timing
starts at Enter in the populated picker; fully checked timing also waits for
confirmed-current status. These measurements supersede the summary of the
older development-build series; the complete earlier observations remain in
the ledger. Do not chain per-iteration ratios or compare unlike build cohorts.

Matched offline process launches from identical copied complete snapshots,
eleven interleaved pairs per fixture, preserved source and every read mark:

| Offline first review frame | Original | Current | Speedup |
| --- | ---: | ---: | ---: |
| One file, no read marks | 44.79 ms | 35.93 ms | 1.25x |
| 1,000 files and read marks | 96.27 ms | 54.41 ms | 1.77x |

These are process-cold launches with copied/warm filesystem data, not disk-cache
flushes. All observations, including first one-file samples of 130.73 / 126.24 ms,
remain included. Populated PR-list startup is still typically around 0.6 seconds.
Neither startup nor the real first-selection scenarios have reached 10x. See
the thirty-first iteration for artifacts and later transport experiments.

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

## Twelfth iteration: bounded parent-validation read-ahead

Each non-root captured commit retains four isolated Git validations: commit object/type,
ancestry under the pinned head, exact commit/parent identity, and first-parent
object availability. These commands formerly ran only after the previous
commit's inventory finished. No check has been removed or replaced by an API
membership assumption.

Parallel-capable runners can now validate up to four entries ahead, leaving at
least one runner slot for foreground capture. Entries/results remain in original
API order. Consuming a result admits one more entry; workers cannot eagerly read
the whole list. A ready early result never waits for a later blocked validation.
Serial/unadvertised runners retain lazy direct calls. The reader accepts at most
100 entries, returns failed checks once, and cancels/joins before final commit
freshness verification or view close. Existing per-command output/time limits,
overall capture deadline, source budgets and explicit unavailable states remain.

Barrier/atomic-counter tests cover bounded admission, exact order, one attempt
per entry including resource failures, lazy serial operation, blocked later
entries, cancellation and joined shutdown. Real isolated Git tests compare
ordinary and read-ahead results for a root, descendant, non-ancestor sibling and
blob. Commit/review race tests also passed before the complete verification run.

Three one-operation full cold-open trials with complete inventory, commit and
composition assertions (network excluded, Apple M2 Max):

| Ten-commit fixture | Prior `91d229f` | Read-ahead |
| --- | ---: | ---: |
| Loose objects | 1,770.16 ms | 1,420.22 ms |
| Packed objects | 976.18 ms | 644.52 ms |
| Mixed duplicate loose/packed objects | 1,032.64 ms | 690.82 ms |

These are approximately 20%, 34% and 33% reductions in complete local opening.
The recorded `cat-file`, tree-diff and tree-listing counts stay unchanged: this
iteration overlaps required work. The identical regex selected all three layouts
in both runs (including the mixed prefix). Logs: `/tmp/prui-parents-before.txt`
and `/tmp/prui-parents-after.txt`. Real first-selection timing and the overall
10x target remain separate from these synthetic results.

Three release-equivalent real first-selection pairs per PR then alternated order,
using fresh private homes, existing credentials held only in memory, and complete
source assertions. `91d229f` versus read-ahead:

| PR / trial | Prior | Read-ahead |
| --- | ---: | ---: |
| #19 / 1 | 2,598.43 ms | 2,567.25 ms |
| #19 / 2 | 2,532.74 ms | 2,267.79 ms |
| #19 / 3 | 2,517.27 ms | 2,401.25 ms |
| #19 median | 2,532.74 ms | 2,401.25 ms |
| #11 / 1 | 2,882.83 ms | 2,550.49 ms |
| #11 / 2 | 2,569.45 ms | 2,699.88 ms |
| #11 / 3 | 2,583.42 ms | 2,434.72 ms |
| #11 median | 2,583.42 ms | 2,550.49 ms |

Every pair had identical decoded snapshots, complete main inventory, all three
commits and captured composition. Pins match the earlier interactive studies.
Artifacts: `/tmp/prui-parent-selection-ciz7ye68`. Observed reductions are about
5.2% / 1.3%; the latter is effectively neutral at this sample size/variability.
The larger ten-commit synthetic wins do not establish the same improvement for
these smaller PRs or a universal result. Do not combine historical absolute
numbers from different build/host conditions into a new overall speedup claim.

The complete verification script passed (installer fixtures, vet, race tests,
terminal journeys and build); configured lint reported zero issues. Startup was
not remeasured: this reader is created only during new commit capture. The overall
10x startup/first-selection goal remains active.

## Thirteenth iteration: scoped composition tree queries

Frozen composition needs only paths declared by captured commit inventories.
It previously listed each complete tree and discarded unrelated records. For
safe nonempty scopes of at most 256 paths / 64 KiB path arguments, capture now
uses `--literal-pathspecs ls-tree ... -- <sorted paths>`. The original exact-path
filter still runs, including when a selected old file became a directory with
unselected descendants. Byte-only names, tabs, spaces, option-like names and
pathspec metacharacters retain their literal meaning. Empty/unsafe/large scopes
keep the original full listing. Attempted errors/limits are never retried.

Both paths charge actual returned metadata to the existing shared capture budget;
all final composition/source validation remains intact. Reading fewer irrelevant
records can make a valid bounded projection available where the old broad query
exhausted its budget. No selected source or required validation is skipped.

Three three-iteration listing trials on 10,000 synthetic tree files:

| Selected paths | Full listing | Scoped-policy listing | Returned bytes before/after |
| --- | ---: | ---: | ---: |
| 100 | 12.620 ms | 9.443 ms | 740,000 / 7,400 |
| 256 | 13.141 ms | 9.510 ms | 740,000 / 18,944 |
| 1,000 | 12.963 ms | 13.067 ms | 740,000 / 740,000 |

The last row uses the full-list fallback. An earlier unrestricted 1,000-path
probe took 13.826 ms versus 13.184 ms for a full listing despite returning fewer
bytes; it motivated the conservative matcher-count cap. Do not equate fewer
bytes with faster total execution. Logs: `/tmp/prui-composition-scopes.txt` and
`/tmp/prui-composition-policy.txt`. Scope tests compare byte-exact retained
records in both trees of a file/directory replacement. Invalid UTF-8 names are
constructed directly with synthetic Git objects because the host filesystem
refuses to create those names.

Three one-operation complete ten-commit cold opens, with the same existing
complete inventory/commit/composition assertions and no concurrent builds/tests:

| Object layout | Prior `eb10e00` | Scoped composition |
| --- | ---: | ---: |
| Loose | 1,416.886 ms | 1,203.174 ms |
| Packed | 644.256 ms | 633.712 ms |
| Mixed | 689.545 ms | 694.147 ms |

Loose-object opening improved about 15%; packed/mixed results are essentially
neutral at this sample size. Counts of Git calls stay unchanged; scoped listing
reduces their output and traversal. Logs: `/tmp/prui-composition-before.txt` and
`/tmp/prui-composition-after.txt`. This result is not a universal PR-open or 10x
speedup claim; real first selection is checked separately.

Live first selection used release-equivalent binaries, fresh private homes,
existing credentials held only in memory and alternating order. Timing starts
at Enter in the populated picker and ends at the first readable overview frame.
The first three-pair batch suggested a regression for PR #19, so a separate
five-pair confirmation batch followed without concurrent tests/builds:

| Batch / PR | Prior median | Scoped median |
| --- | ---: | ---: |
| Initial / #19 | 2,283.85 ms | 2,518.39 ms |
| Initial / #11 | 2,569.46 ms | 2,566.40 ms |
| Confirmation / #19 | 2,367.30 ms | 2,417.27 ms |
| Confirmation / #11 | 2,584.55 ms | 2,583.26 ms |

The confirmation #19 difference is about 2.1% slower; #11 is neutral. These
variable network-inclusive samples do not demonstrate a live-selection win or
a consistent regression. Both batches are retained rather than selecting only
the favorable one. The measured benefit is limited to bounded tree work and
the loose-object synthetic cold open above. All 16 paired decoded snapshots
were identical, with complete inventories, all three commits and captured
composition. Artifacts: `/tmp/prui-composition-selection-ppn4oyt3` and
`/tmp/prui-composition-confirmation-yvz9i9ow`.

The complete verification script passed, including race tests and terminal
journeys; configured lint reported zero issues. Startup was not remeasured
because this query runs only during new composition capture. The overall 10x
startup/first-selection goal remains active.

## Fourteenth iteration: overlap commit capture and supporting evidence

Six fresh-home direct opens on the current release-equivalent binary profiled
the remaining serial stages. Evidence retrieval took 293–322 ms on PRs #19/#11,
after commit capture had already finished. Main inventory took about 82–86 ms
for #19 and 572–600 ms for #11. Saving took about 3–11 ms; the subsequent
freshness read still cost hundreds of milliseconds. These observations identify
independent pinned-object work worth overlapping without removing freshness
reads. Artifacts: `/tmp/prui-critical-profile-jwfmfxy3/results.json`.

After the main inventory (and any explicitly requested full-source capture),
parallel-capable runners now capture commits and supporting evidence together.
Both retain their existing independent byte/file/time budgets, pinned source,
privacy filtering and completeness labels. Commit errors cancel and join the
evidence reader before retrying/closing the view. Success joins both before
guide input, payload bounding or persistence. Unadvertised runners retain the
original ordering, and no additional GitHub requests or retries are introduced.
Stage wall durations can overlap and must not be summed as elapsed time.

Tests exercise overlap with barriers, joining on success, cancellation/error
cleanup, serial ordering/failure and exact serial/parallel frozen snapshot
equivalence using synthetic real-Git history and supporting documentation.

Three interleaved release-equivalent first-selection pairs per real PR, with
fresh private homes and complete capture assertions (`1a3d5b1` versus overlap):

| PR / trial | Prior | Overlapped |
| --- | ---: | ---: |
| #19 / 1 | 2,498.61 ms | 1,933.32 ms |
| #19 / 2 | 2,368.53 ms | 2,068.05 ms |
| #19 / 3 | 2,416.75 ms | 2,218.88 ms |
| #19 median | 2,416.75 ms | 2,068.05 ms |
| #11 / 1 | 2,516.57 ms | 2,299.89 ms |
| #11 / 2 | 2,534.20 ms | 2,168.46 ms |
| #11 / 3 | 2,533.75 ms | 2,251.82 ms |
| #11 median | 2,533.75 ms | 2,251.82 ms |

Median reductions are about 14.4% / 11.1%, with improvement in every pair.
Every pair had identical decoded snapshots, complete main inventory, all three
commits and captured composition. Pins match the prior interactive studies.
Timing runs from Enter in the populated picker to the first readable overview;
this is not an expanded-source rendering measurement. Credentials remain only
in memory. Artifacts: `/tmp/prui-details-selection-rp2mbzwy`.
These results do not establish a universal 10x startup/selection improvement.

The identical network-free complete-open benchmark command then ran against an
exact `1a3d5b1` archive and the candidate, three one-operation trials each, with
no concurrent builds/tests:

| Cold fixture | Prior median | Overlapped median |
| --- | ---: | ---: |
| Packed ten commits / 100 files | 634.111 ms | 612.111 ms |
| 100 changed files + 100 distinct supporting excerpts | 1,198.197 ms | 1,090.735 ms |

Observed reductions are about 3.5% / 9.0%. All complete inventory, commit,
composition and supporting-excerpt assertions passed. Git call counts stay
unchanged: packed history uses 24 `cat-file`, 12 `diff-tree`, 14 `ls-tree`;
supporting evidence uses 35, 3 and 5 respectively. Logs:
`/tmp/prui-details-before.txt` and `/tmp/prui-details-after.txt`. The smaller local
effect is expected because these fixtures have no network wait to overlap.

The complete verification script passed (installer fixtures, vet, race tests,
terminal journeys and build); configured lint reported zero issues. Startup
was not remeasured because the changed stage runs only during new comparison
capture. The overall 10x goal remains active.

## Fifteenth experiment: share the PR-list HTTPS transport (reverted)

The PR list already uses a bounded, fixed GraphQL query. A disposable candidate
sent that identical query through the capture client's bounded HTTPS transport,
instead of a separate `gh api graphql` process, to reuse the list connection
during first selection. Query text, variables and parsed results stayed identical;
synthetic tests verified list-to-capture TLS connection reuse, malformed-result
rejection and the existing authentication/limit/write boundaries. Focused race
tests and lint passed before measurement.

Three interleaved release-equivalent first-selection pairs per PR compared
`b509361` against the candidate, using fresh private homes:

| Metric | Prior median | Candidate median |
| --- | ---: | ---: |
| #19 first selection | 2,118.63 ms | 2,015.88 ms |
| #11 first selection | 2,299.71 ms | 2,250.38 ms |
| Populated-list startup, all six trials | 620.55 ms | 673.14 ms |
| Launch + selection, all six trials | 2,817.23 ms | 2,857.65 ms |

Every paired snapshot was identical and complete. First execution of the new
candidate had a 1,172 ms list startup; every trial remains recorded rather than
silently excluding that observation. Subsequent candidate startup ranged
596–722 ms. The small first-selection differences and mixed total timings do
not establish a benefit beyond variability. The production change and its
experiment-only tests were reverted; no transport expansion was retained.
Artifacts: `/tmp/prui-list-http-selection-hibfszsr`.

## Sixteenth experiment: early local view preparation (prototype only)

A disposable `b509361` archive started the existing isolated local object-view
preparation while the PR list loaded. It transferred ownership once, joined
cancelled preparation and cleaned unused views on exit. It did not fetch before
selection, reuse a view across comparisons, change pin validation or retry failed
preparation. The experiment used a context-carried value as a temporary seam;
that interface was not proposed for production.

Three interleaved release-equivalent pairs per PR, fresh private homes:

| First-selection metric | Prior median | Early preparation median |
| --- | ---: | ---: |
| #19 | 2,218.16 ms | 2,116.22 ms |
| #11 | 2,333.01 ms | 2,236.22 ms |

All six paired snapshots were identical and complete. Two individual trials
were slower with the prototype. The candidate's first-execution list startup
was 1,311 ms and is retained in the raw observations. Artifacts:
`/tmp/prui-prewarm-selection-w63nqgcf`. No production preparation API or
application-lifetime ownership state was added on the strength of these small,
variable first-selection differences.

Pinning already overlaps metadata and view preparation, so its critical wait is
roughly the slower of those independent inputs, not their sum. Moving just one
earlier can leave the other dominating. A combined disposable prototype then
tested early view preparation together with the shared list transport, again
with three interleaved fresh-home pairs per PR:

| Metric | Prior median | Combined prototype median |
| --- | ---: | ---: |
| #19 first selection | 2,034.07 ms | 1,933.94 ms |
| #11 first selection | 2,233.57 ms | 2,150.37 ms |
| Populated-list startup, all six trials | 657.50 ms | 660.28 ms |
| Launch + selection, all six trials | 2,830.79 ms | 2,735.81 ms |

Every snapshot was again identical and complete. First execution of this new
prototype had a 1,195 ms list startup and remains in the observations. Artifacts:
`/tmp/prui-combined-selection-wqr_b_06`. The roughly 4–5% selection differences
and 3.4% combined median difference are small relative to trial variability;
startup is essentially unchanged. This does not justify adding application-
lifetime view ownership, cleanup, cross-checkout transfer and cancellation
machinery. Both early-preparation prototypes remain disposable; production
behavior stays at `b509361`. Only this evidence record is retained. Restoring
the verified Go files and `git diff --check` confirmed a documentation-only
worktree change; the rejected candidates are not claimed as fully verified
production implementations.

The next candidate is the selected PR's membership request, which currently
waits for main inventory construction. It can potentially overlap mandatory
local work without speculative preparation before selection. Any experiment
must keep the capture deadline, final freshness read, cancellation/joining,
membership validation and no-retry behavior intact.

## Seventeenth iteration: overlap selected-PR membership with main inventory

The prior profile put main inventory at about 82–86 ms for #19 and 572–600 ms
for #11. Commit membership was requested only after this local work completed.
For parallel-capable runners and clients supporting commit membership, a
single-use preparation now starts that same request immediately after pinning.
Local commit capture and its final freshness read still start after main
inventory and any explicitly requested full-source capture. Supporting evidence
retains the previous overlap with local commit capture.

The preparation owns one request and the original shared 55-second work /
60-second total deadlines, starting at preparation. Consumption never extends
the deadlines, and overlapping local work consumes wall time in that window.
An expired window retains explicit incomplete/unavailable outcomes. In
particular, valid membership arriving at the work deadline stays available while
per-commit work reports exhaustion, preserving the existing deadline behavior.
No failed request is retried. Main-inventory errors cancel and join preparation
before the owned view closes; final freshness waits for the membership reader
to finish even during cancellation. Unsupported clients and unadvertised serial
runners retain ordinary capture. There is no preparation before PR selection.

Tests use barriers to require membership to start during the main tree-diff
operation (the integration test fails with the old ordering), and check
inventory-failure cancellation/view cleanup, single consumption, repeated close,
no freshness on unused preparation, no failure retries, parent cancellation,
unsupported clients and both deadline boundaries. Existing serial/parallel
frozen-snapshot equivalence and moved-revision retry tests also pass.

Three interleaved release-equivalent first-selection pairs per real PR, with
fresh private homes, existing credentials held only in memory and complete
capture assertions (prior runtime `b509361`, unchanged by documentation-only
`20fa99f`, versus prepared membership):

| PR / trial | Prior | Prepared membership |
| --- | ---: | ---: |
| #19 / 1 | 2,133.87 ms | 2,118.78 ms |
| #19 / 2 | 2,215.78 ms | 2,168.18 ms |
| #19 / 3 | 2,217.47 ms | 1,799.89 ms |
| #19 median | 2,215.78 ms | 2,118.78 ms |
| #11 / 1 | 2,117.35 ms | 2,083.73 ms |
| #11 / 2 | 2,350.62 ms | 1,834.64 ms |
| #11 / 3 | 2,268.20 ms | 1,935.00 ms |
| #11 median | 2,268.20 ms | 1,935.00 ms |

Observed median reductions are about 4.4% / 14.7%, with every pair improving.
The smaller #19 result remains variable; #11 has more mandatory local inventory
work available to overlap. Every paired decoded snapshot was identical, with
complete main inventory, all three commits and captured composition. Pins match
the earlier studies. Measurement starts at Enter in the populated picker and
ends at the first readable overview, not an expanded-source view. Artifacts:
`/tmp/prui-membership-selection-ea_xdlfz`.

The candidate's first-execution populated-list startup was 1,253 ms and remains
in the observations; subsequent list observations were 532–669 ms. Startup work
was not changed, and these observations do not establish a startup improvement.
The complete verification script passed (installer fixtures, vet, race tests,
terminal journeys and build); configured lint reported zero issues. The overall
10x startup/first-selection goal remains active and unproven.

## Eighteenth iteration: tokenize only through displayed hunks

Current-stage profiling after `10ab5b6` still put #11 main inventory at roughly
563–569 ms. Three disposable, private-home CPU-profiled opens captured all 26
files, three commits and composition. Of 3.21 seconds of sampled in-process CPU
across those opens, 1.05 seconds was attributed to syntax tokenization, largely
Chroma/regexp execution. This excludes package initialization and child-process
CPU; it is not one third of elapsed network-inclusive opening. Artifacts:
`/tmp/prui-current-critical-profile-o2b_mhiy` and
`/tmp/prui-cpu-capture-txzqbmuk`.

Inventory previously tokenized each complete file before projecting categories
onto displayed patch hunks. It now supplies the same full pinned input to the
same lexer and stops iterating after the last needed line on each side. This
preserves multiline state and lookahead rather than truncating the input.
Whole-input UTF-8/byte checks, consumed-token raw-offset validation, per-prefix
span/time bounds and full source-byte charging remain. A valid requested prefix
can now be highlighted even when unused tail spans would exhaust the full
capture limit. Readiness describes captured patch highlighting; it does not
claim that the unused tail was tokenized. Full-source navigation keeps its
ordinary full capture, including EOF/offset validation.

Cache identities now include the requested line extent. A prefix cannot satisfy
a full request; a full result can supply a copied prefix. Retention remains
8 MiB / 128 entries, with no raw source or failures retained. Every comparison
still applies its own byte/span/time budgets before lookup. Tests compare
requested lines with full captures across Go, TypeScript, Python, Rust, HTML
embedding and YAML; cover multiline comments/strings, every displayed hunk,
last-line boundaries, full-byte charging, prefix/full cache isolation and
concurrency, cancellation, oversized/invalid input, span/time limits, unsafe
range fallback and the original YAML normalized-token offset rejection.

Three ten-operation lexer trials on a 1,001-line Go file, no cross-call cache:

| Requested extent | Median | Allocated bytes/op |
| --- | ---: | ---: |
| Full | 31.920 ms | 16,538,224 |
| Through line 10 | 0.381 ms | 371,577 |
| Through line 100 | 3.186 ms | 1,833,678 |

These are approximately 84x / 10x stage improvements, not whole-open claims.
Log: `/tmp/prui-syntax-prefix-bench.txt`.

An added complete cold-open fixture changes a constant near the start of 50
distinct approximately 40 KiB Go files in a 1,000-file tree. Both revisions
require complete main inventory, history and composition, and ready old/new
highlighting for every changed file in both main and commit inventories. The
identical benchmark file ran against an exact `10ab5b6` archive and the candidate,
three one-operation trials each, without concurrent builds/tests:

| Complete cold open | Prior | Prefix capture |
| --- | ---: | ---: |
| Median | 7,157.965 ms | 1,487.490 ms |
| Median allocated bytes/op | 3,233,383,488 | 155,337,328 |
| Displayed main-inventory syntax spans | 1,800 | 1,800 |

This is about 4.8x faster, with roughly 21x fewer allocated bytes. Counts stay at
36 `cat-file`, 44 `diff`, 3 `diff-tree` and 5 `ls-tree` calls per operation; this
change removes unnecessary lexical work. Logs:
`/tmp/prui-prefix-cold-before.txt` and `/tmp/prui-prefix-cold-after.txt`.

Three interleaved release-equivalent live first-selection pairs per PR used
fresh private homes, in-memory credentials and complete capture assertions:

| PR / trial | Prior `10ab5b6` | Prefix capture |
| --- | ---: | ---: |
| #19 / 1 | 2,016.76 ms | 1,982.39 ms |
| #19 / 2 | 2,100.65 ms | 1,932.41 ms |
| #19 / 3 | 1,865.37 ms | 2,035.06 ms |
| #19 median | 2,016.76 ms | 1,982.39 ms |
| #11 / 1 | 1,967.88 ms | 1,766.69 ms |
| #11 / 2 | 1,851.32 ms | 1,732.65 ms |
| #11 / 3 | 1,852.04 ms | 1,866.88 ms |
| #11 median | 1,852.04 ms | 1,766.69 ms |

Observed reductions are about 1.7% / 4.6%, small relative to network variability;
#19 is effectively neutral. Every paired decoded snapshot was identical,
including displayed syntax, with complete inventories, three commits and
composition. Timing is Enter in the populated picker to readable overview,
not expanded-source rendering. The larger synthetic win is not generalized to
these real PRs. Artifacts: `/tmp/prui-prefix-selection-d6buprf2`. The new binary's
first list startup was 1,205 ms and remains recorded; no startup win is claimed.
The complete verification script passed (installer fixtures, vet, race tests,
terminal journeys and build), and configured lint reported zero issues.
The overall 10x startup/first-selection goal remains active and unproven.

## Nineteenth iteration: refill diff reservations after consumption

The early-hunk large-file fixture still used 44 individual diff processes.
`diffReservation` deliberately budgets eight output lines per source record,
plus fixed overhead, to cover hunk headers and duplicated materialized file
headers. Two approximately 1,002-line inputs reserve about 16,000 lines per
file, so the initial 100,000-line budget admitted only six of 50 files. The old
preparation ran once, leaving all later files on the individual path even after
the first six had consumed very little actual output.

Diff preparation now refills only once the preceding window has been consumed
in original file order. Each new window uses the actual remaining materialized
bytes/lines and the unchanged worst-case reservation function. It never reads
past an earlier unknown budget consumer. Prepared results, including errors,
remain keyed by blob pair across windows, preventing failed prepared jobs from
being reissued or falling back. Retained raw results use owned, exact-capacity
copies and a separate cap equal to the configured comparison content limit;
malformed patches cannot grow retention indefinitely by consuming no final
inventory budget. Oversized prepared results become explicit limit failures.
Whole-comparison changed-path collision checks are cached once and include
already-consumed paths, preserving safe file/directory replacement handling.

Tests compare multi-window real-Git capture byte-for-byte with serial inventory,
require refilling without individual fallbacks, and check tight byte/line limits,
checkout immutability, retained-data bounds, reused backend buffers, oversized
results, failure reuse, cancellation, unknown consumers and path collisions
across window boundaries. Existing mixed-change and source-budget tests pass.

Three one-operation complete cold opens per fixture, identical commands against
an exact `eae6c02` archive and candidate, without concurrent builds/tests:

| Fixture | Prior median | Refilled median |
| --- | ---: | ---: |
| Early hunks in 50 large files | 1,507.218 ms | 1,163.560 ms |
| Packed ten commits / 100 files | 611.260 ms | 603.357 ms |

The large-file reduction is about 22.8%; the packed control is essentially
neutral. Large-file Git counts changed from 44 `diff` + 3 `diff-tree` to zero
`diff` + 11 `diff-tree`, saving 36 processes. The other counts stay at 36
`cat-file` and 5 `ls-tree`. Both revisions retain complete source, commits and
composition, ready old/new highlighting for all 50 files in main and commit
inventories, and 1,800 displayed main-inventory spans. Packed-control call counts
are unchanged. Logs: `/tmp/prui-refill-before.txt` and
`/tmp/prui-refill-after.txt`.

The first live driver attempt stopped before selecting any PR: a newly opened
Dependabot #111 moved #19 away from the first row, invalidating the old fixed
title/index assumption. The saved frame showed a correctly loaded list, not an
application startup failure. No timings from that stopped attempt were accepted.
The corrected driver waits for the requested PR number, derives its row from
the rendered list and still verifies the selected snapshot identity. Three
interleaved release-equivalent pairs per PR, with fresh private homes:

| PR / trial | Prior `eae6c02` | Refilled |
| --- | ---: | ---: |
| #19 / 1 | 2,065.73 ms | 2,083.02 ms |
| #19 / 2 | 1,983.14 ms | 2,002.04 ms |
| #19 / 3 | 2,083.44 ms | 1,899.77 ms |
| #19 median | 2,065.73 ms | 2,002.04 ms |
| #11 / 1 | 1,733.97 ms | 1,700.01 ms |
| #11 / 2 | 1,767.14 ms | 1,751.55 ms |
| #11 / 3 | 1,700.55 ms | 1,834.86 ms |
| #11 median | 1,733.97 ms | 1,751.55 ms |

The roughly 3.1% improvement / 1.0% slowdown are effectively neutral within
network variability. Every paired decoded snapshot was identical, with complete
main inventory, three commits and captured composition. Pins match the preceding
studies. Timing remains Enter on the target PR to readable overview, not expanded
source. The larger local-fixture benefit is not generalized to these PRs.
Artifacts: `/tmp/prui-refill-number-selection-lwlceowz`; the stopped driver frame
is under `/tmp/prui-refill-selection-e34mntwd`. Credentials stayed in memory.
The candidate's first list startup was 1,321 ms and remains recorded; no startup
improvement is claimed.

The complete verification script passed, including installer fixtures, vet, race
tests, terminal journeys and build. Configured lint reported zero issues. The
overall 10x startup/first-selection goal remains active and unproven.

## Twentieth iteration: readable captured source while post-save freshness runs

Inspection showed that new browser captures waited for post-save `review.Refresh`
before returning. The TUI refresh already handles unchecked frozen snapshots,
but skips sessions already marked current, so this was not a duplicated check.
Cached browser opens already displayed source with truthful unknown freshness
and checked it in the background.

New interactive PR-list/new-tab captures can now use that same pending flow.
All pinned source, commit membership/history/composition, evidence, predecessor
generation checks and durable snapshot creation finish before the readable
frame. The final commit-capture revision guard is unchanged. The post-save check
is then owned by the existing TUI refresher, and the first frame explicitly says
`Freshness unknown`. Only a successful matching check can persist current;
failure/cancellation stays failed/unknown. Existing identity/generation guards,
changed-revision handling and write preflights remain. Fresh captures in direct/
CLI opening and Inbox keep synchronous post-save freshness, and no periodic
polling is added.

Tests require a complete captured inventory/history/composition to return before
a deliberately blocked post-save check, verify actual current-browser wiring,
visible unknown status, successful/failed status persistence without losing an
intervening read mark, cancellation remaining unknown and unchanged synchronous
direct/Inbox behavior. Additional synthetic writer tests reject unavailable or
moved-head metadata while the initial readable session is unchecked.

The live driver now records both first-readable time and time until the stored
session confirms `current`. It waits for that confirmation before quitting every
trial, preventing a cancelled/skipped background check from masquerading as less
work. Confirmation is observed through brief read-only SQLite queries, with up
to approximately 20 ms polling granularity plus connection overhead. Source
snapshot equality is checked after confirmation. Three interleaved release-
equivalent pairs per PR, fresh private homes and credentials held only in memory:

| PR / trial | Prior readable | Candidate readable | Prior confirmed | Candidate confirmed |
| --- | ---: | ---: | ---: | ---: |
| #19 / 1 | 1,850.05 ms | 1,582.15 ms | 1,877.04 ms | 1,970.63 ms |
| #19 / 2 | 2,066.11 ms | 1,715.36 ms | 2,067.18 ms | 2,081.74 ms |
| #19 / 3 | 2,033.56 ms | 1,481.60 ms | 2,034.81 ms | 1,865.09 ms |
| #19 median | 2,033.56 ms | 1,582.15 ms | 2,034.81 ms | 1,970.63 ms |
| #11 / 1 | 1,984.45 ms | 1,417.99 ms | 1,985.86 ms | 1,733.64 ms |
| #11 / 2 | 1,767.87 ms | 1,382.74 ms | 1,768.92 ms | 1,649.30 ms |
| #11 / 3 | 1,768.80 ms | 1,500.84 ms | 1,769.90 ms | 1,801.66 ms |
| #11 median | 1,768.80 ms | 1,417.99 ms | 1,769.90 ms | 1,733.64 ms |

Readable complete source appears about 22.2% / 19.8% earlier, improving every
pair. Confirmed-current time remains effectively neutral within network
variability; this is an earlier readable surface, not a claim that all checking
finishes that much faster. Every paired decoded frozen snapshot was identical,
with complete inventory, three commits and captured composition. The sampled
first candidate frames explicitly show unknown freshness, and every trial
eventually confirmed current before exit. Timing is Enter on the selected PR to
overview/confirmed status; it does not measure expanded-source rendering.
Artifacts: `/tmp/prui-browser-freshness-selection-c0ferqp1`. The candidate's first
list startup was 1,289 ms and remains recorded; startup work was not changed.
The full verification script and lint passed; the additional write-guard test
also passed independently with the race detector. The overall 10x startup/open
goal remains active and unproven, including time to finish required checks.

## Twenty-first iteration: isolate current startup costs

Rechecked startup independently at runtime revision `442553d`, with no concurrent
builds, tests or other timing jobs. Seven ordinary fresh-process `--version`
launches of the existing release-equivalent binary took 22.28, 19.71, 20.79,
19.79, 19.73, 19.90 and 20.46 ms (median 19.90 ms). These are warm-filesystem
process launches, **not** offline review frames or populated-list startup, and
do not establish a startup improvement against older measurements.

Seven separate `GODEBUG=inittrace=1` launches ended package initialization at
approximately 14 ms. Median reported initializer times:

| Package | Initializer time |
| --- | ---: |
| `modernc.org/libc/honnef.co/go/netdb` | 2.7 ms |
| `github.com/alecthomas/chroma/v2/styles` | 2.7 ms |
| `github.com/alecthomas/chroma/v2/lexers` | 2.6 ms |

Trace timestamps are rounded and tracing itself adds output overhead. The three
initializers account for about 8 ms combined in these samples. Inspection of the
pinned dependency sources confirms Chroma eagerly constructs the exported lexer
registry and parses all embedded styles. Glamour also imports Chroma styles, so
changing only the application's syntax matcher would not isolate that work.
Deferring these dependency initializers would require a broader dependency or
process-boundary change; there is no measured user-visible benefit here large
enough to justify such a change for the first-selection target. No dependency
fork, reduced lexer/style support or runtime change was retained.

The existing `BenchmarkOfflineStartupComponents`, three trials of three
iterations each, separated application work from process initialization and PTY
I/O. Setup, source capture and schema creation remain outside each timer:

| Component | One-file median | 1,000-file median |
| --- | ---: | ---: |
| Open and close writable store | 0.54 ms | 0.53 ms |
| Open and close read-only store | 0.27 ms | 0.28 ms |
| Load and validate saved snapshot | 0.23 ms | 24.16 ms |
| Load, construct model and render first frame | 1.45 ms | 27.49 ms |

The rows are separate measurements, not additive stages. The last row includes
snapshot loading and retains the fixture's complete source/history/composition
assertions and saved read marks. Large saved snapshots merit profiling before
any storage optimization, but they do not explain first-time selection without
a predecessor snapshot. All benchmark trials passed. This evidence-only
iteration changes no application behavior and makes no speedup claim. The 10x
startup/first-selection goal remains active, including fully checked open time.
Diagnostic artifacts: `/tmp/prui-startup-init-summary.json` and
`/tmp/prui-startup-components-current.txt`.

## Twenty-second iteration: locate remaining first-open work

Profiled three fresh private-store, authenticated plain opens each of PRs #19
and #11 using runtime `442553d`. These exercise complete cold capture, storage
and synchronous final freshness, not the browser's earlier readable frame.
Current median recorded stages:

| Stage | PR #19 | PR #11 |
| --- | ---: | ---: |
| Isolated view setup | 397.98 ms | 405.39 ms |
| Main inventory | 79.79 ms | 394.64 ms |
| Evidence capture | 276.89 ms | 297.87 ms |
| Sum of metadata reads within raw capture | 825.21 ms | 674.97 ms |

These durations overlap and must not be added to derive total latency. Metadata
includes initial pinning and capture-end verification; the additional post-save
freshness read is outside the raw-capture timing record. All six complete plain
opens succeeded, with overall times between 1.72 and 1.93 seconds. Artifacts:
`/tmp/prui-head-critical-profile-6kjm92dd`.

A temporary local-only test rebuilt PR #11's main inventory from that captured
fixture and compared the complete result with `reflect.DeepEqual`. It retained
the normal runner's parallel capability and recorded only command names and
durations. Nine Git subprocesses consumed about 90 ms in a 380 ms inventory;
CPU samples attributed 630 of the 640 ms sampled under inventory building to
syntax capture across three runs. Those CPU samples exclude child-process CPU
and are not a wall-time percentage. Isolated view setup separately measured
408.76 ms across five operations; its CPU samples included substantial object
linking and pack preflight work. Optimizing setup alone can be hidden behind the
concurrent initial metadata request.

Prototyped concurrent OLD/NEW syntax capture for two nonempty inputs at least
4 KiB, on multiple processors with over two seconds of syntax budget left.
Both tasks were joined and the budget charged their summed durations, retaining
the existing serial path near exhaustion. Five prototype trials and five
unchanged control trials all reproduced the complete saved inventory exactly:

| Local inventory trial | Control | Prototype |
| --- | ---: | ---: |
| 1 | 384.77 ms | 347.04 ms |
| 2 | 377.61 ms | 341.06 ms |
| 3 | 382.50 ms | 346.12 ms |
| 4 | 378.63 ms | 343.04 ms |
| 5 | 379.81 ms | 349.25 ms |
| Median | 379.81 ms | 346.12 ms |

This saves 33.69 ms in the local stage (8.9%), before considering overlap or
whole-open latency. It does not justify additional concurrent syntax-budget
semantics at present. Reverted the prototype and removed the temporary profiling
tests; no runtime change was retained. Logs: `/tmp/prui-inventory-pair-control.txt`
and `/tmp/prui-inventory-pair-prototype.txt`.

Separately probed the next transport hypothesis: the fixed GraphQL PR metadata
fields `number`, `body`, `baseRefName`, `baseRefOid`, `headRefOid`,
`baseRepository.nameWithOwner` and `headRepository.nameWithOwner` against the
current REST response. Five interleaved pairs used fresh HTTPS connections and
the same in-memory credential. All normalized metadata fields were identical.
REST times were 390.20, 363.83, 422.19, 428.82 and 324.80 ms; GraphQL times were
253.30, 331.80, 337.22, 277.60 and 353.37 ms. Medians were 390.20 / 331.80 ms,
about 15% lower for GraphQL; response sizes were 18,071 / 1,912 bytes.
This diagnostic omits per-request CLI token resolution and connection reuse, so
it does not establish an application speedup. Next, evaluate an ordinary-open
prototype before deciding whether a transport change is worthwhile, including
authentication/permission compatibility, deleted forks, field validation,
bounded errors and unchanged revision checks. No freshness request may simply
be removed. Timings only: `/tmp/prui-metadata-query-probe.json`.

## Twenty-third iteration: narrow metadata reads with retained revision checks

The production HTTP capture client now queries the fixed canonical PR metadata
fields through GraphQL instead of retrieving the full REST PR representation.
Repository and PR identity are typed JSON variables; the query contains no
reviewed content or mutation. GitHub documents ref OIDs and names as available
even for deleted refs, while repository fields are nullable and remain required
by this application ([PullRequest schema](https://docs.github.com/en/graphql/reference/pulls#pullrequest)).
Every initial pin, capture-end check, post-save check and independent write
preflight still invokes fresh metadata retrieval. No request/check was removed,
cached or combined with commit membership.

Both response formats use the same metadata validator. GraphQL errors, partial
or missing fields, deleted forks, unexpected identities, invalid pins and invalid
text fail closed. Fresh CLI credentials are still resolved for each HTTP read.
Custom runners and unavailable token-export compatibility retain their existing
REST CLI request and decoder; this compatibility branch runs before HTTP
dispatch. An attempted HTTP/GraphQL request never retries through REST. The
existing shared operation deadline, response/header/connection limits, redirect
policy and CLI-only writes remain in effect.

First tested a separate release-equivalent prototype against runtime `442553d`,
then tested the final implementation independently with the same driver. Each
series had three interleaved pairs per PR, fresh private stores, credentials only
in memory and no concurrent builds, tests or profiling. The driver waits for
confirmed-current status before quitting and compares complete decoded source.
Prototype medians:

| PR | Prior readable | GraphQL readable | Prior confirmed | GraphQL confirmed |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,463.75 ms | 1,333.06 ms | 1,881.41 ms | 1,598.28 ms |
| #11 | 1,432.21 ms | 1,449.77 ms | 1,784.07 ms | 1,731.54 ms |

Final implementation, including every trial:

| PR / trial | Prior readable | GraphQL readable | Prior confirmed | GraphQL confirmed |
| --- | ---: | ---: | ---: | ---: |
| #19 / 1 | 1,680.40 ms | 1,431.82 ms | 2,048.55 ms | 1,747.37 ms |
| #19 / 2 | 1,533.76 ms | 1,397.79 ms | 1,868.68 ms | 1,714.43 ms |
| #19 / 3 | 1,764.62 ms | 1,266.30 ms | 2,180.77 ms | 1,578.81 ms |
| #19 median | 1,680.40 ms | 1,397.79 ms | 2,048.55 ms | 1,714.43 ms |
| #11 / 1 | 1,432.62 ms | 1,333.54 ms | 1,699.95 ms | 1,576.91 ms |
| #11 / 2 | 1,366.39 ms | 1,516.38 ms | 1,650.65 ms | 1,960.07 ms |
| #11 / 3 | 1,402.54 ms | 1,916.14 ms | 1,684.06 ms | 2,117.76 ms |
| #11 median | 1,402.54 ms | 1,516.38 ms | 1,684.06 ms | 1,960.07 ms |

PR #19 improved in every pair across both series: the final readable median was
16.8% earlier and confirmed-current median 16.3% earlier. PR #11 was mixed: its
prototype was essentially neutral, while the final series regressed about 8.1%
readable / 16.4% confirmed. Those slower observations remain included. All twelve
paired snapshots across both series were identical, with complete inventory,
three commits and captured composition. First candidate list startups were
1,243 / 1,174 ms and remain recorded; list startup work was unchanged.
Artifacts: `/tmp/prui-metadata-graphql-selection-flfjqsls` and
`/tmp/prui-metadata-graphql-final-selection-r9ddqp03`.

Investigated the PR #11 regression with three interleaved plain-open pairs and
stage timings, retaining all observations. Median raw-capture metadata time fell
702.68 to 620.98 ms; view setup was 389.39 / 392.92 ms and inventory 384.23 /
382.03 ms. Total plain-open times were 1,771.78 / 1,763.91 ms (effectively
neutral), with one slower GraphQL pair. These diagnostic runs indicate lower
metadata cost with otherwise similar local work, but do not overturn the mixed
interactive result or establish a PR #11 improvement. Artifacts:
`/tmp/prui-graphql-confirm-profile-oe9cqdj0`.

Retain the smaller fixed read for its repeatable PR #19 benefit while treating
PR #11 and broader network performance as unresolved. The overall 10x
startup/first-selection goal remains active and unproven. New focused race tests
cover fixed request identity, credential changes, required fields, partial
errors, response limits, authentication compatibility, cancellation/deadlines,
interrupted-body cleanup and no retry after dispatch. The existing local TLS
test compares GraphQL/REST metadata and verifies connection reuse with commit
membership reads. The complete verification script passed, including installer
fixtures, vet, race tests, terminal journeys and build. Configured lint reported
zero issues. No human terminal usability claim is made by these automated checks.

## Twenty-fourth iteration: recheck early view preparation after GraphQL

Revisited the discarded early-view prototype with runtime `efff754`, because
smaller metadata reads might expose more local setup latency. The disposable
prototype starts one isolated local object view while the current-repository PR
list loads, consumes it once and joins/cleans it on exit. It makes no additional
GitHub request. Three interleaved release-equivalent pairs per PR, fresh stores,
full source capture and confirmed freshness:

| PR | Prior readable | Prepared view readable | Prior confirmed | Prepared view confirmed |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,264.79 ms | 1,215.81 ms | 1,465.53 ms | 1,399.04 ms |
| #11 | 1,367.19 ms | 1,284.79 ms | 1,549.70 ms | 1,467.84 ms |

All six paired snapshots were identical and complete. The approximately 4–6%
selection gain remains small relative to speculative filesystem work and the
ownership/lifetime machinery a production version would require. Do not retain
the context-value prototype or introduce a prepared-view API for this result.
First candidate list startup was 1,233 ms and remains recorded; no startup win is
claimed. Artifacts: `/tmp/prui-graphql-prewarm-selection-7jsnpckw`.

## Twenty-fifth iteration: prune impossible Go lexer rules

Chroma's pinned Go lexer has one stateless rule sequence and tries many
impossible regexes before ordinary identifiers/functions. Build a private
character dispatcher from that exact sequence. For ASCII leading characters,
conservative regex first sets select a subsequence in the original order;
the original regexes, groups and emitters still decide every actual match.
Unicode and vertical tab use the complete original sequence, accounting for
the Go/.NET whitespace-class difference. Assertions only over-include candidates.
The complete input and existing offset, byte, span, cancellation and time checks
are unchanged. Initialization is lazy and inside the existing token deadline.

Enable this only for the canonical Go lexer with the reviewed pattern checksum,
single stateless root and matching flags. Other languages, custom lexers and
unreviewed dependency changes retain the original implementation. Do not modify
the global registry, original rules or configuration. Construct and initialize
the private compiled table before publication, then share each root compiled
rule across its candidate lists. The optimized lexer compiles 32 regexes instead
of one pool per character class; the original Chroma lexer remains available
independently for fallback and Markdown rendering. This is a bounded additional
lexer, not a claim that total process retained memory was measured unchanged.

The first prototype dispatched through an extra zero-width regex on every token
and raised allocated bytes in the full microbenchmark from 16.54 to 27.93 MB.
Replaced that with next-state selection after each match and shared compiled
rules. Final medians of three ten-operation `BenchmarkTokenizeThrough` trials,
compared with an unchanged `efff754` archive, without concurrent timing work:

| Go capture extent, 1,001-line fixture | Prior | Final | Prior allocated bytes | Final allocated bytes |
| --- | ---: | ---: | ---: | ---: |
| Full file | 31.677 ms | 14.031 ms | 16,537,745 | 16,538,624 |
| First 10 lines | 0.380 ms | 0.235 ms | 371,577 | 372,332 |
| First 100 lines | 3.247 ms | 1.426 ms | 1,833,691 | 1,834,436 |

The full/100-line stages are about 2.26x / 2.28x faster, with essentially
unchanged allocation volume. This is a lexer-stage result. Logs:
`/tmp/prui-go-dispatch-micro-before.txt`,
`/tmp/prui-go-dispatch-micro-after.txt` (rejected allocation-heavy version), and
`/tmp/prui-go-dispatch-micro-final.txt`.

Replayed the same saved PR #11 main-inventory fixture in the unchanged archive
and final code, with no network or concurrent timing work. Three control times
were 393.19, 390.16 and 390.95 ms; final times were 262.78, 254.30 and 255.33 ms.
The median fell from 390.95 to 255.33 ms (34.7%). Each replay compared the entire
inventory with the saved reference using `reflect.DeepEqual`, including syntax,
statuses and classification. View setup was outside this timer; this is not a
complete-open result. The temporary diagnostic test stayed in disposable
archives. Logs: `/tmp/prui-go-dispatch-inventory-before.txt` and
`/tmp/prui-go-dispatch-inventory-after.txt`.

An initial separate prototype improved PR #11's readable median from 1,399.69
to 1,283.48 ms and confirmed-current from 1,633.20 to 1,517.40 ms; PR #19 was
essentially neutral. Then tested the final shared-regex/state-transition version
independently. Same fresh private-store, complete-source, interleaved driver:

| PR / trial | Prior readable | Final readable | Prior confirmed | Final confirmed |
| --- | ---: | ---: | ---: | ---: |
| #19 / 1 | 1,399.17 ms | 1,365.10 ms | 1,628.02 ms | 1,578.68 ms |
| #19 / 2 | 1,217.13 ms | 1,348.93 ms | 1,449.55 ms | 1,539.98 ms |
| #19 / 3 | 1,350.33 ms | 1,398.25 ms | 1,564.58 ms | 1,678.91 ms |
| #19 median | 1,350.33 ms | 1,365.10 ms | 1,564.58 ms | 1,578.68 ms |
| #11 / 1 | 1,433.80 ms | 1,233.42 ms | 1,647.37 ms | 1,516.07 ms |
| #11 / 2 | 1,349.74 ms | 1,216.73 ms | 1,583.90 ms | 1,579.96 ms |
| #11 / 3 | 1,549.68 ms | 1,233.49 ms | 1,806.71 ms | 1,444.09 ms |
| #11 median | 1,433.80 ms | 1,233.42 ms | 1,647.37 ms | 1,516.07 ms |

Final PR #11 medians improved about 14.0% readable and 8.0% confirmed, with every
pair improving; PR #19 remained essentially neutral within network variation.
All twelve paired decoded snapshots across both series were identical, with
complete inventory, three commits and captured composition. First candidate list
startups were 1,132 / 2,112 ms and remain included. Startup work was not changed,
and there is no startup improvement claim. Artifacts:
`/tmp/prui-go-dispatch-selection-qnaj9efp` and
`/tmp/prui-go-dispatch-final-selection-buyjso62`.

New differential tests compare complete token types and values, including all
ASCII leading characters, Unicode, VT, malformed source, templates, numeric
forms, nested lexing and randomized inputs. Tests also cover conservative first
sets, dependency/configuration fallback, original-registry isolation, compiled
rule sharing and concurrent capture. The final differential fuzz run completed
507,351 executions without a mismatch (`/tmp/prui-go-dispatch-fuzz-final.txt`).
The complete verification script passed, including installer fixtures, vet,
race tests, terminal journeys and build. Configured lint reported zero issues.
The overall 10x startup/first-selection goal remains active and unproven.

## Twenty-sixth iteration: saved-source decoding and rendering cadence

Profiled the 1,000-file offline first-frame component at `254dc20`: 27.91 ms/op
across 100 operations. In-process samples placed most load work under source JSON
decoding; package initialization, terminal I/O and fixture setup are not part of
that benchmark timer. The original streaming decoder copies an already-owned
SQLite payload into its input buffer before decoding it.

First tried standard-library `json.Unmarshal` with the existing canonical byte
comparison. Three ten-operation trials reduced allocations substantially but
left time effectively unchanged: load/validation 24.36 to 24.01 ms and model/
first-frame construction 27.65 to 27.37 ms. Reverted this experiment. Logs:
`/tmp/prui-source-decode-startup-before.txt` and
`/tmp/prui-source-decode-startup-after.txt`.

The retained adapter uses pinned `github.com/segmentio/encoding v0.5.4` with
`github.com/segmentio/asm v1.2.1`. Only frozen source decoding changes; encoding,
canonical-byte comparison, source digests, indexed identity/reference checks,
commit/incremental validation and SQLite transactions remain unchanged.
Unknown fields are rejected, exact field-name matching is required, trailing
data is rejected, and copy optimizations are disabled. Decoded values remain
independent of the input buffer. Parser errors are generic because the new
library's raw diagnostics can contain source fragments. A failing synthetic
privacy test caught that exposure before the generic error was added.
The library documents its copy flags and compatibility approach in its
[official source](https://github.com/segmentio/encoding/tree/v0.5.4/json).

Warm in-process medians, three ten-operation trials of the existing complete
synthetic startup fixtures:

| 1,000-file component | Prior | Fast decoder | Prior allocated bytes | Fast decoder allocated bytes |
| --- | ---: | ---: | ---: | ---: |
| Load and validate | 24.363 ms | 14.335 ms | 22,960,564 | 18,659,000 |
| Load, model and first frame | 27.648 ms | 18.187 ms | 23,337,896 | 19,705,593 |

That stage gain alone did not improve actual fresh-process PTY startup: eleven
interleaved pairs measured 60.63 / 60.19 ms (neutral). All launches, including
first executions of 223.69 / 742.68 ms, remain recorded. Source and all 1,000 read
marks were unchanged. The first harness attempt correctly failed store ownership
validation because the database copy omitted `.sqlite-owner`; fixed the harness
to copy the existing marker with private permissions, without weakening storage
validation. Artifacts: `/tmp/prui-fastjson-offline-startup-alsb7zve`.

Then separated decoder and rendering cadence in a four-variant experiment.
Bubble Tea defaults to a maximum 60 FPS; the candidate uses its supported 120 FPS
cap for both interactive program entry points. This changes presentation cadence,
not model loading, capture completeness, source validation or action consent.
All variants use release-equivalent builds and the same copied private stores.
Every launch is a new process; this is warm-filesystem startup, not a disk-cold
or first-install guarantee. No concurrent timing jobs, builds or tests:

| Saved review / trials per variant | Std decoder, 60 FPS | Std decoder, 120 FPS | Fast decoder, 60 FPS | Fast decoder, 120 FPS |
| --- | ---: | ---: | ---: | ---: |
| Complete 1,000-file synthetic review, nine | 59.27 ms | 60.20 ms | 59.90 ms | 51.71 ms |
| Complete one-file PR #19, seven | 42.97 ms | 35.41 ms | 43.26 ms | 35.04 ms |

The combined candidate improves these medians by about 12.7% / 18.4%. The
factorial result matters: rendering cadence alone benefits the small review,
while the large fixture needs both changes to show an earlier readable frame.
All source payloads and read marks stayed unchanged. First large-fixture launches
were 231.56, 653.49, 165.24 and 632.45 ms respectively and remain in the samples.
Artifacts: `/tmp/prui-startup-decoder-fps-s06m1t17` and
`/tmp/prui-startup-small-decoder-fps-a687k_z9`.

Also checked the final implementation against `254dc20` in three interleaved
first-selection pairs per PR, fresh stores, waiting for confirmed freshness:

| PR | Prior readable | Candidate readable | Prior confirmed | Candidate confirmed |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,382.89 ms | 1,334.24 ms | 1,596.58 ms | 1,573.98 ms |
| #11 | 1,301.40 ms | 1,334.92 ms | 1,516.96 ms | 1,616.60 ms |

Results are mixed, including slower PR #11 samples; do not claim a first-selection
improvement from this change. All six paired snapshots were identical, with
complete inventories, three commits and captured composition. The first
candidate list startup was 1,354 ms and remains recorded. Artifacts:
`/tmp/prui-decoder-render-selection-pk56q5es`.

New tests preserve the original strict streaming decoder as a differential
oracle. They cover nested unknown fields, duplicate/escaped keys, alternate
encodings, malformed UTF-8, version/digest errors, binary source, input-buffer
ownership, privacy and plain-schema assumptions. The final isolated structured
fuzz run compared arbitrary input and generated canonical snapshots with varied
paths, patches and descriptions: 24,330 executions without a difference. A prior
run concurrent with audit/build work made only 31 executions; its low throughput
was investigated with seeds and a bounded isolated replay, and is not used as
the confidence result. Logs: `/tmp/prui-source-decode-final-fuzz.txt`.

Reviewed the two new modules, their MIT licenses, pinned checksums, default copy
behavior and architecture-specific implementation. `go mod verify` passed.
`govulncheck v1.8.0` source/symbol scanning reported no called-symbol findings and
no advisory for either new module. It reported the existing module-only
`GO-2026-5932` for unchanged `golang.org/x/crypto v0.57.0`; no dependency upgrade
or unrelated remediation was performed. Audit: `/tmp/prui-source-fastjson-audit.json`.
This is point-in-time advisory evidence, not proof that unsafe parsing code is
defect-free. All four release targets built with CGO disabled: Darwin/Linux,
amd64/arm64. Configured lint reported zero issues. The first full gate, concurrent
with cross-compilation, hit the existing TypeScript span-limit test's ten-second
deadline and reported timeout. The focused test passed unchanged in isolation;
the complete gate then passed without competing builds, including installer
fixtures, vet, race tests, terminal journeys and build. No deadline, assertion
or resource limit was changed. The overall 10x startup/first-selection goal remains active and
unproven, and human terminal usability remains unverified.

## Twenty-seventh iteration: start membership after initial metadata

The selected PR's commit-membership request previously waited for isolated Git
view setup and ancestry validation, even though it needs only the initial
validated metadata. A private review-layer adapter now starts that request as
soon as the initial metadata response arrives, for runners advertising parallel
use. Local pinning continues independently. Inventory and all commit-parent,
composition and final metadata checks still finish before source capture returns.
The final metadata read waits for membership to join.

The preparation belongs to the review context, rather than pin input setup's
shorter-lived context. Its existing 55-second work and 60-second total deadlines
now include the remaining setup/ancestry work as well as inventory; consuming it
never restarts those deadlines. Exhaustion retains explicit unavailable states.
A pin failure cancels and joins preparation. If fetch discovers different source
pins, the old request is canceled and joined before preparing membership for the
returned revision. Same-pin failures are consumed once, without an automatic
retry. Unsupported clients and runners without parallel capability retain their
existing capture path. No public source API or resource limit changed.

A disposable prototype was compared with `a56dadd` using alternating release
binaries, fresh private HOME/storage directories and three paired first
selections per PR. PR #19's readable median changed from 1,358.69 to 1,192.09 ms
(12.3%), and fully checked from 1,599.06 to 1,429.01 ms (10.6%). PR #11 was
essentially unchanged: 1,375.46 to 1,358.14 ms readable, and 1,583.15 to
1,591.75 ms checked. All six paired decoded snapshots matched, with complete
inventory, three commits and captured composition. The first candidate PR-list
startup was 1,102.81 ms and remains included. Artifacts:
`/tmp/prui-early-membership-selection-sjx_wwit`.

The final production implementation was measured again against `a56dadd` with
the same alternating, fresh-profile procedure (three pairs per PR):

| PR | Prior readable median | Earlier membership | Prior checked median | Earlier membership |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,324.97 ms | 1,110.10 ms | 1,532.73 ms | 1,324.80 ms |
| #11 | 1,317.55 ms | 1,325.50 ms | 1,516.62 ms | 1,557.62 ms |

PR #19 improved by 16.2% readable and 13.6% fully checked; every paired sample
improved. PR #11 remained mixed, including slower samples: readable median was
0.6% slower and checked median 2.7% slower. This is a small networked sample,
not evidence of a universal improvement. All six final paired snapshots matched,
again with complete inventory, three commits and captured composition. The first
candidate PR-list startup was 1,169.93 ms; it remains recorded. This change does
not optimize startup. Final artifacts:
`/tmp/prui-membership-final-selection-svmp6uzj`.

The ordering regression test first failed on the previous implementation: local
ancestry validation waited for membership that could only start after pinning.
New tests also cover pin failure cleanup/cancellation, a failed membership read
without retry, and revision movement during pin/fetch. The latter blocks old
membership until canceled, then verifies it joins before replacement begins.
The final freshness-read fixture rejects a check that precedes membership join.

The complete verification script passed: installer fixtures, formatting, vet,
race tests, terminal journeys and build. Configured lint reported zero issues.
The overall 10x startup/first-selection goal remains active and unproven.
Human terminal usability remains unverified.

## Twenty-eighth iteration: avoid building discarded analysis requests

Reprofiled runtime `5d554fd` with three fresh private-store plain opens per PR.
All six saved snapshots had complete inventory, three commits and captured
composition. PR #11 median view setup was 446.14 ms, inventory 302.39 ms,
evidence 377.58 ms, and the sum of initial/capture-end metadata reads 544.12 ms.
These stages overlap; their sum is not total opening latency. Artifacts:
`/tmp/prui-membership-critical-profile-q_3eb_1o`.

Additional disposable instrumentation measured PR #11 commit capture, including
its final metadata read, at 420.09–525.95 ms. Evidence was 334.89–359.79 ms and
finished before commit capture. About 100 ms remained after the commit stage
before raw opening returned. Separate local derivations from the same frozen
snapshot took 60.75–61.37 ms with no analyzer configured. Inspection found that
Go evaluated `guide.InputFrom` before `guide.Analyze` could return its immediate
nil-analyzer fallback: ordinary opens assembled, privacy-scanned, copied and
hashed a request that no provider would consume. Diagnostic artifacts:
`/tmp/prui-membership-detail-profile-ng3bg9zx` and
`/tmp/prui-capture-profile-1ou7qwyw`.

Raw opening and frozen guide derivation now construct request material only
when the analyzer interface is non-nil, matching `guide.Analyze`'s existing
condition. They still use that function's original fallback. Enabled analysis
keeps the identical `InputFrom` privacy, budget, provenance and digest boundary.
Supporting-evidence retrieval retains its own filtering, and frozen raw source,
commit capture, cleanup and every metadata check are unchanged. The existing
derivation test now also compares the entire disabled-analysis result with the
original frozen snapshot plus its expected fallback and predecessor reference.

A new benchmark derives guides from a complete synthetic frozen comparison
with 100 supporting excerpts. Fixture creation/capture are outside its timer.
Three identical ten-operation trials before/after the change:

| Disabled-analysis derivation | Prior median | Candidate median |
| --- | ---: | ---: |
| Time | 29.809 ms | 0.000479 ms |
| Allocated bytes/op | 60,300 | 240 |
| Allocations/op | 338 | 1 |

This measures eliminated optional work, not a whole-opening or 10x goal result.

A disposable release candidate was compared with the current production binary
in three alternating first-selection pairs per PR, using fresh private stores
and waiting for confirmed-current status before exit:

| PR | Prior readable median | Candidate | Prior checked median | Candidate |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,175.24 ms | 1,124.05 ms | 1,398.71 ms | 1,462.04 ms |
| #11 | 1,343.08 ms | 1,258.31 ms | 1,541.29 ms | 1,466.29 ms |

Every readable pair improved, with median reductions of 4.4% / 6.3%. Fully
checked timing remains mixed: #19's median was 4.5% slower, #11's 4.9% faster.
All six paired decoded snapshots were identical and complete, including all
three commits and captured composition. The first candidate populated-list
startup was 1,166.95 ms and remains recorded; startup was not optimized.
Artifacts: `/tmp/prui-no-analysis-input-selection-bu62eylq`.

The final implementation was then built and measured independently with the
same procedure (three pairs per PR):

| PR | Prior readable median | Final | Prior checked median | Final |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,216.08 ms | 1,083.59 ms | 1,406.98 ms | 1,291.17 ms |
| #11 | 1,351.05 ms | 1,267.43 ms | 1,533.56 ms | 1,459.67 ms |

Final readable medians improved 10.9% / 6.2%, and checked medians 8.2% / 4.8%.
Every final readable pair improved; one #11 checked sample was slower. Keep the
prototype's mixed checked results alongside these observations rather than
claiming uniform network latency gains. Across both series all twelve paired
snapshots matched, with complete inventories, three commits and captured
composition. The first final candidate list startup was 1,255.97 ms and remains
included. Final artifacts: `/tmp/prui-no-analysis-final-selection-st1tf55y`.

The complete verification script passed (installer fixtures, formatting, vet,
race tests, terminal journeys and build); configured lint reported zero issues.
The overall 10x startup/first-selection goal remains active and unproven.
Human terminal usability remains unverified.

## Twenty-ninth iteration: reject a privacy prefilter without an opening win

Runtime `fa311ad` scanned the larger PR's frozen 256 KiB of supporting evidence
in a median 41.50 ms (five local trials). Isolated view teardown was 32.49 ms;
setup was 441.25 ms. A disposable privacy prefilter kept the existing expressions
as the decision and skipped them only when required assignment/material prefixes
were absent. Pattern fingerprints and Unicode-fold guards selected the original
path for unreviewed semantics. Assignment, material, binary-priority and Unicode
cases matched an independent copy of the original policy; 225,287 raw/structured
fuzz executions found no difference. Evidence scanning fell to about 0.97 ms.
Microbenchmarks also showed large ordinary-text improvements, while a long-space
positive was slightly slower and a short credential-material sample increased
by a few microseconds. These are stage results, not opening results.

Three alternating first-selection pairs per PR, fresh private stores, complete
captures, and a wait for confirmed-current status produced:

| PR | Prior readable median | Prototype | Prior checked median | Prototype |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,116.63 ms | 1,191.89 ms | 1,341.16 ms | 1,516.97 ms |
| #11 | 1,283.66 ms | 1,294.16 ms | 1,591.65 ms | 1,541.45 ms |

All six paired snapshots matched. The scanner stage win did not establish a
first-selection win: #19 was slower and #11 essentially neutral. Keep the
production privacy policy unchanged rather than adding manual matching
assumptions for a hidden stage. The first candidate list startup was 1,250.35 ms
and remains recorded. Prototype and probe sources are outside the worktree;
none of its runtime or test code was retained. Artifacts:
`/tmp/prui-privacy-prefilter-4u53_hck` and
`/tmp/prui-privacy-prefilter-selection-j4tlu2o3`.

## Thirtieth iteration: bounded parallel pack resource preflight

A five-view CPU/timing probe attributed substantial setup time to both local
zlib resource checks and Git's independent pack verifier. Median setup was
447.84 ms, Git verification 198.16 ms, and teardown 31.61 ms. In-process samples
included roughly 110 ms/view under the inflater; child-process CPU is excluded
from those samples. The independent verifier remains required.

The resource parser now validates headers and delta references and reserves all
declared inflation bytes before starting at most four workers (also bounded by
GOMAXPROCS and record count). Workers reuse separate inflater state and check
immutable, disjoint compressed spans. Delta result sizes use one atomic shared
96 MiB eligibility allowance, rather than one allowance per worker. All charges
are nonnegative, so successful valid-pack eligibility has the same total as the
original serial parser. Corrupt streams, exhausted aggregate allowance or parent
cancellation stop and join every worker before Git verification or fallback.

Checksums, exact zlib boundaries, declared sizes, per-record overflow probes,
1 MiB object bounds, 64-level delta depth/cycle checks and thin-pack rejection
remain. The 10,000-object and 16 MiB retained-input ceilings are unchanged.
Auxiliary memory grows by bounded plan entries and up to four inflater states;
no claim of unchanged heap usage is made. Git still independently verifies the
private pack/index with `--threads=1`; only that verification can authorize
omitting duplicate loose links. Original-store immutability, fetch isolation,
resource-failure handling and ordinary unsupported-layout mirroring remain.

The first prototype measured median local setup at 322.96 ms, versus the prior
447.84 ms (27.9% lower). Git's verifier was 188.22 ms, and teardown 30.10 ms.
Probes: `/tmp/prui-pack-parallel-ohup6pdl` (including its CPU profile).
Three alternating fresh-profile first-selection pairs per PR then measured:

| PR | Prior readable median | Prototype | Prior checked median | Prototype |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,150.29 ms | 1,033.29 ms | 1,408.24 ms | 1,327.32 ms |
| #11 | 1,242.18 ms | 1,142.01 ms | 1,491.70 ms | 1,365.98 ms |

Readable medians improved 10.2% / 8.1%, checked medians 5.7% / 8.4%.
One #19 pair was slower; all #11 pairs improved. All six paired decoded source
snapshots matched, with complete inventory, three commits and captured
composition. The first candidate list startup was 1,279.75 ms and remains in
the observations; startup was not optimized. Artifacts:
`/tmp/prui-pack-parallel-selection-8xk_qh2m`.

New tests compare serial/parallel eligibility, shared-budget exact boundaries,
corrupt streams and cancellation, alongside the existing real REF/OFS delta,
forged-index, immutable-source, failure and limit tests. The original serial
parser is retained only as a differential test oracle. Fuzzing repairs outer
checksums and includes a multi-object seed so it can reach framing/parallel
logic. The prototype completed 178,844 comparisons; the final implementation
completed 143,618 comparisons with no eligibility or plan difference.

A portable resource-only benchmark with 4,096 tiny records also checks worker
setup overhead. Three 30-operation trials measured median time 2.768 to
1.915 ms (30.8% lower), allocated bytes/op 2,050,062 to 2,320,299 (13.2% higher),
and allocations/op 16,526 to 16,570. It does not exercise Git identity
verification or complete opening; its deliberately synthetic OIDs remain
ineligible for source omission without that separate verification.

The final release build was then measured independently with three alternating
fresh-profile pairs per PR:

| PR | Prior readable median | Final | Prior checked median | Final |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,250.11 ms | 1,175.04 ms | 1,499.66 ms | 1,491.84 ms |
| #11 | 1,333.48 ms | 1,250.57 ms | 1,547.85 ms | 1,506.69 ms |

Final readable medians improved 6.0% / 6.2%; checked medians improved only
0.5% / 2.7%. One final #11 readable pair was slower. Keep these smaller gains
alongside the prototype, including every observation. All twelve paired
snapshots across both series were identical, complete, and included three
commits plus captured composition. The first final candidate list startup was
1,159.73 ms and remains recorded. Final artifacts:
`/tmp/prui-pack-parallel-final-selection-9co7hhhk`.

The complete verification script passed (installer fixtures, formatting, vet,
race tests, terminal journeys and build). The new tiny-record benchmark passed
separately; configured lint reported zero issues. No limits, assertions or
independent-verifier settings were weakened. The overall 10x startup and
first-selection goal remains active and unproven. Human terminal usability
remains unverified.

## Thirty-first iteration: rebaseline and share the PR-list connection

Rebuilt original `78cb664` with the same CGO-disabled, trimmed release flags and
Go 1.26.9 toolchain as `405c105`. Three alternating fresh-profile pairs per PR
confirmed the original-to-current ratios in the summary above. Fully checked
medians were 6,167.40 to 1,361.79 ms for #19 and 9,650.77 to 1,466.75 ms for #11.
Every paired source snapshot matched and contained complete inventory, three
commits and captured composition. Existing authentication stayed in memory;
there were no remote writes. Artifacts:
`/tmp/prui-original-current-selection-tmft45gl`.

Separately launched both releases offline from independent SQLite backups of
identical complete one-file and 1,000-mark fixtures. Ownership markers and
private modes were retained, provider credentials removed, and source digests
plus every progress row compared before/after. All 44 runs preserved source and
marks. Startup remains only 1.25x / 1.77x faster, rather than 10x. Artifacts:
`/tmp/prui-original-current-offline-dz42buyo`.

Revisited shared-list transport and local view preparation because the capture
critical path has changed substantially since the earlier rejected prototypes.
A balanced four-variant experiment used four runs per PR and rotated each
variant through every order position. All used release flags, fresh private
profiles and the same complete-source/freshness requirements. Instrumented
variants reported successful one-time consumption of all 16 prepared views;
the suspected checkout-path mismatch was not observed. A context-carried
preparation remains an experiment-only interface.

| PR / variant | List startup median | Readable median | Checked median | Median list + selection |
| --- | ---: | ---: | ---: | ---: |
| #19 baseline | 705.75 ms | 1,291.47 ms | 1,536.38 ms | 2,039.49 ms |
| #19 shared list | 575.39 ms | 1,133.37 ms | 1,353.40 ms | 2,029.35 ms |
| #19 prepared view | 554.74 ms | 1,108.70 ms | 1,389.62 ms | 1,676.44 ms |
| #19 combined | 693.50 ms | 1,062.03 ms | 1,282.21 ms | 1,756.18 ms |
| #11 baseline | 559.95 ms | 1,301.00 ms | 1,495.41 ms | 1,828.74 ms |
| #11 shared list | 526.85 ms | 1,192.41 ms | 1,500.05 ms | 1,806.69 ms |
| #11 prepared view | 592.44 ms | 1,234.34 ms | 1,482.74 ms | 1,811.57 ms |
| #11 combined | 548.44 ms | 1,158.98 ms | 1,400.84 ms | 1,727.90 ms |

The final column is the median of each run's list-plus-selection sum, excluding
navigation keystrokes; it is not the sum of the two medians. Shared-list readable
medians improved 12.2% / 8.3%, with mixed individual and fully checked results.
The combined readable medians improved 17.8% / 10.9%. All 24 variant-to-baseline
snapshot comparisons matched, with complete inventory, three commits and
captured composition. First list/prepared/combined startups were 1,467.32 /
1,278.09 / 1,232.57 ms and remain included. Artifacts:
`/tmp/prui-list-view-factorial-zosox49h`.

Connection reuse is implemented first as a separately verified change. The fixed
list GraphQL query, variables and decoder are unchanged. Production lists now
use the existing bounded read client, allowing its TLS connection to serve
subsequent pinning/membership reads. Each request still resolves fresh CLI
credentials; responses are not cached. Custom runners retain the original CLI
request. Unavailable token export may fall back before HTTP dispatch; attempted
HTTP reads never retry via CLI. The same operation deadline, 1 MiB decompressed
body cap, header/connection bounds and redirect policy remain. Writes stay on
the existing explicit CLI path. No prepared-view state or extra local object
copying before selection is included in this change.

The TLS regression test first failed on the prior implementation: only the
three capture requests reached the shared connection, while the list bypassed
it. It now compares identical normalized CLI/HTTP lists and verifies one
connection carries the list, two metadata reads and commit membership. Added
coverage checks exact query/variables, fresh credentials/responses, invalid
identity refusal, authentication compatibility, cancellation, a deadline that
includes token export, bounded errors and no fallback after dispatch. Existing
list-field validation and all other read/write boundaries remain covered.

The final production implementation was measured against `405c105` in a fresh
series of three alternating first-selection pairs per PR:

| PR | Prior readable median | Final | Prior checked median | Final |
| --- | ---: | ---: | ---: | ---: |
| #19 | 1,208.54 ms | 1,033.01 ms | 1,410.05 ms | 1,282.22 ms |
| #11 | 1,242.50 ms | 1,184.47 ms | 1,477.11 ms | 1,450.13 ms |

Readable medians improved 14.5% / 4.7%; every readable pair improved, although
one #11 difference was only 1.76 ms. Checked medians improved 9.1% / 1.8%, with
mixed individual #11 results. List-startup medians were 677.79 to 617.78 ms for
#19 and 580.04 to 599.69 ms for #11. Median list-plus-selection sums improved
1,824.23 to 1,767.70 ms and 1,822.54 to 1,775.29 ms (about 3.1% / 2.6%). Do not
claim a universal startup improvement or multiply these ratios into the earlier
original-build cohort. All six final paired snapshots matched and contained
complete inventory, three commits and captured composition. The first final
candidate list startup was 1,119.51 ms and remains recorded. Artifacts:
`/tmp/prui-list-shared-final-selection-5byvo25o`.

The complete verification script passed (installer fixtures, formatting, vet,
race tests, terminal journeys and build); configured lint reported zero issues.
The additional gain from early view preparation remains a measured candidate
for a later slice. It needs explicit ownership/lifetime and checkout/runner/limit
binding; the context-value prototype is not included in production. The full
10x startup/first-selection goal remains active and unproven, and human terminal
usability remains unverified.

## Reproduction and validation

```sh
go test ./internal/context -run '^$' -bench BenchmarkRetrieveLargeTree -benchtime=1x -count=3
go test ./cmd/prui -run '^$' -bench BenchmarkPullRequestOpen -benchtime=1x -count=3
go test ./internal/source -run '^$' -bench BenchmarkBlobRead -benchtime=1s -count=3
go test ./internal/source -run '^$' -bench BenchmarkBorrowObjects -benchtime=1x -count=3
go test ./internal/source -run '^$' -bench BenchmarkPackPreflight -benchtime=30x -count=3
go test ./internal/source -run '^$' -fuzz '^FuzzPackPreflight$' -fuzztime=20s -fuzzminimizetime=1s -parallel=4
go test ./internal/syntax -run '^$' -bench BenchmarkLexerSelection -benchtime=100x -count=3
go test ./internal/syntax -run '^$' -bench BenchmarkTokenizeCaptureReuse -benchtime=10x -count=3
go test ./internal/syntax -run '^$' -bench BenchmarkTokenizeThrough -benchtime=10x -count=3
go test ./internal/syntax -run '^$' -fuzz '^FuzzGoDispatchTokens$' -fuzztime=20s -parallel=4
go test ./internal/session -run '^$' -fuzz '^FuzzSourceDecodeEquivalence$' -fuzztime=20s -fuzzminimizetime=1s -parallel=4
go test ./internal/session -run '^$' -bench BenchmarkSQLiteFreshnessProgress -benchtime=1x -count=3
go test ./internal/commits -run '^$' -bench BenchmarkCompositionTreeListing -benchtime=3x -count=3
go test ./cmd/prui -run '^$' -bench BenchmarkOfflineStartupComponents -benchtime=3x -count=3
go test ./internal/review -run '^$' -bench BenchmarkGuideDerivationWithoutAnalyzer -benchtime=10x -count=3
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
   including more commits and missing-object fetches. Earlier matched build
   comparisons demonstrated about 2.56x / 3.46x improvements; newer per-iteration
   results are recorded above and do not establish the overall 10x target.
   Populated PR-list startup has typically been around 0.6 seconds; the earlier
   median offline startup was about 45 ms. Remeasure under matching conditions.
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
5. Current eager package initialization has now been measured separately (see
   the twenty-first iteration). Its largest three initializers total about 8 ms
   under those conditions. Prioritize the first-selection critical path before
   dependency isolation; a startup rewrite needs a measured user-visible benefit.
6. Broaden verified-copy eligibility only with evidence from larger/multiple-pack
   stores. Current unsupported layouts retain ordinary mirroring. Retain all
   integrity, work, cancellation, fetch-barrier and cleanup guarantees.

Continue comparing complete opens after each change. Do not mark the goal
complete based on a favorable fixture, cache hit or isolated stage.
