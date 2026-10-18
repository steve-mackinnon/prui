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

## Reproduction and validation

```sh
go test ./internal/context -run '^$' -bench BenchmarkRetrieveLargeTree -benchtime=1x -count=3
go test ./cmd/prui -run '^$' -bench BenchmarkPullRequestOpen -benchtime=1x -count=3
go test ./internal/source -run '^$' -bench BenchmarkBlobRead -benchtime=1s -count=3
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

1. Measure the user's slow startup entry point and representative PR, including
   local startup, remote calls, commit capture, persistence and first usable frame.
   Current synthetic startup is already under 50 ms; no-argument PR browsing
   can have different network costs.
2. Reduce process creation for the remaining distinct blob-pair diffs. A batched
   Git strategy must preserve byte-identical patches, committed attribute
   isolation, path handling, partial/failure states and aggregate limits. Existing
   bounded concurrency already removes some waiting; it does not remove forks.
3. Expand full-source sizes/change shapes and measure fresh-process first-time
   selection. The new distinct-basename and ten-commit fixtures show that the
   result is not yet universally 10x. Investigate the per-commit overhead while
   retaining membership, ancestry, completeness and freshness checks.
4. Profile large cached snapshot decoding/validation and session creation.
   Any storage optimization must keep integrity validation, isolation and stale
   writer rejection. Do not trade these checks for a faster first frame.
5. Measure eager package initialization separately before considering dependency
   isolation. A startup rewrite needs evidence of a user-visible benefit.

Continue comparing complete opens after each change. Do not mark the goal
complete based on a favorable fixture, cache hit or isolated stage.
