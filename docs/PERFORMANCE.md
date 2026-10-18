# Startup and PR opening performance

## Goal and measurement contract

Target: make startup and PR opening 10 times faster without changing core
behavior. **Active; not achieved.** Compare medians on the same host, build
configuration, fixture and cache state. A stage-level speedup does not establish
an end-to-end speedup. Keep network latency separate from application work.

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
diff subprocesses. Repeated objects occur across the main comparison, commit
diffs, composition and evidence; these formerly launched Git each time.

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

## Reproduction and validation

```sh
go test ./internal/context -run '^$' -bench BenchmarkRetrieveLargeTree -benchtime=1x -count=3
go test ./cmd/prui -run '^$' -bench BenchmarkPullRequestOpen -benchtime=1x -count=3
go test ./internal/source -run '^$' -bench BenchmarkBlobRead -benchtime=1s -count=3
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
caps and retry after failed reads. The repository verification script passed
after the first two optimizations, including vet, race tests, PTY tests and build.

## Next iterations

1. Measure the user's slow startup entry point and representative PR, including
   local startup, remote calls, commit capture, persistence and first usable frame.
   Current synthetic startup is already under 50 ms; no-argument PR browsing
   can have different network costs.
2. Measure bounded Git batch blob reads for distinct-object cold misses. Keep
   the existing runner's cancellation/output bounds and individual blob limits;
   test malformed/missing objects and exact output framing before adopting it.
3. Profile remaining blob-pair diff subprocesses and repeated commit comparisons.
   Consider bounded reuse or concurrency only with deterministic coverage,
   resource limits and byte-identical patches preserved.
4. Profile large cached snapshot decoding/validation and session creation.
   Any storage optimization must keep integrity validation, isolation and stale
   writer rejection. Do not trade these checks for a faster first frame.
5. Measure eager package initialization separately before considering dependency
   isolation. A startup rewrite needs evidence of a user-visible benefit.

Continue comparing complete opens after each change. Do not mark the goal
complete based on a favorable fixture, cache hit or isolated stage.
