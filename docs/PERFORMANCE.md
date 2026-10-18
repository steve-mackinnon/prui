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

## Reproduction and validation

```sh
go test ./internal/context -run '^$' -bench BenchmarkRetrieveLargeTree -benchtime=1x -count=3
go test ./cmd/prui -run '^$' -bench BenchmarkPullRequestOpen -benchtime=1x -count=3
go test ./internal/source -run '^$' -bench BenchmarkBlobRead -benchtime=1s -count=3
go test ./internal/source -run '^$' -bench BenchmarkBorrowObjects -benchtime=1x -count=3
go test ./internal/syntax -run '^$' -bench BenchmarkLexerSelection -benchtime=100x -count=3
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

1. Extend live first-selection measurements beyond PR 109, including a larger
   PR and the interactive first readable frame. The authenticated nine-file PR
   remains only 2.08x faster. Current synthetic offline startup is under 50 ms;
   no-argument PR browsing can have different network costs.
2. Profile evidence retrieval, per-commit work and network sequencing after the
   object-view improvement. Preserve pinning, freshness and completeness checks;
   identify duplicate acquisition and process/connection overhead before editing.
   Reuse authenticated benchmark setup without saving credentials.
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
