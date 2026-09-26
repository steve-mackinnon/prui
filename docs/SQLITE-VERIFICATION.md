# SQLite replacement verification

The fresh SQLite implementation replaces session, guide-cache and repository
registry file persistence. No old-cache reader, import, dual-write mode, backend
selector or compatibility `Save` method remains. Existing user caches are left
untouched. Windows implementation and testing are deferred.

## Embedded driver

- Pinned `modernc.org/sqlite v1.59.0`, BSD-3-Clause license; module minimum Go 1.25.
- Runtime `SELECT sqlite_version()` reports SQLite 3.53.4.
- The implementation uses the driver's CGo-free in-process engine, not an external
  executable or database service. Driver documentation:
  [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite@v1.59.0).
- CGo-disabled real create/close/reopen tests pass with `PATH=/nonexistent`.
- macOS executable linkage contains ordinary OS libraries/frameworks and no
  SQLite shared library (`otool -L`).
- `govulncheck@v1.8.0 ./...` reports no vulnerabilities in the reachable graph.
  This is a point-in-time result, not a guarantee about future advisories.

## Verification scope

The macOS arm64 full `scripts/verify.sh` gate passed on Go 1.26.8: formatting,
vet, all race tests, compiled-binary PTY scenarios, and build. GolangCI-Lint
reported zero issues. Synthetic metadata fixtures now contain valid pinned
repository/SHA values; schema validation was not weakened to accept them.

Native Linux arm64 also passed the complete `scripts/verify.sh` gate, including
race and PTY tests, inside official `golang:1.26.8` (image digest
`sha256:6c2a5538f964f1c82f97ad14988bf05de100d922d159d0e398b54c7b0ca0c6c9`).
The disposable container had networking disabled and read-only repository and
module-cache mounts; its build cache lived under container `/tmp`. A separate
CGo-disabled Linux test run passed all storage, session, CLI and TUI packages.
This is native Linux execution in a VM, not a cross-build.

Linux exposed a pre-existing verifier precision defect: a sub-millisecond
process launch rounded to zero. The harness now retains nanoseconds internally;
the public report keeps its millisecond format. A 250 µs regression test and
macOS/Linux verification pass without weakening the positive-duration assertion.

Reproduce the Linux gate with a locally available Go module cache:

```sh
docker run --rm --network none \
  --mount type=bind,src="$PWD",dst=/src,readonly \
  --mount type=bind,src="$(go env GOMODCACHE)",dst=/go/pkg/mod,readonly \
  -w /src -e GOCACHE=/tmp/gocache golang:1.26.8 sh ./scripts/verify.sh
```

Acceptance tests cover:

- 100 raw/guided sessions with different checkout hints share one source row,
  retain independent progress, and allocate a new row when the description changes.
- State-only updates and metadata lists work without decoding corrupt source
  payloads; opening still detects corruption. Query plans use recency indexes.
- Real independent processes compete on one generation: one commits and one
  receives a conflict. Loaded source/progress are read in one transaction.
- A child killed during an uncommitted journaled write leaves the prior state
  intact after writable recovery. Read-only opening refuses recovery.
- SQLite page-limit exhaustion rolls back the whole update. A held reader causes
  a bounded failed commit with an explicit uncertain-outcome error; no retry is
  attempted and the previous committed state remains readable.
- Deletion preserves shared source, derived children, reusable guides and the
  repository registry. An injected SQL failure rolls back deletion.
- Private permissions, ownership, unsafe symlinks, foreign/legacy stores,
  orphan control artifacts, schema versions, concurrent initialization,
  interrupted initialization, and URI-significant Unicode paths.

## Same-machine measurements

MacBook with Apple M2 Max, macOS arm64, Go 1.26.8. Five runs per benchmark.
History fixture: 50 sessions with 256 KiB retained evidence per source. Values
below are medians; these are local measurements, not performance guarantees.

| Metric | File persistence baseline | SQLite |
| --- | ---: | ---: |
| Latest matching history lookup | 2.948 ms/op | 2.263 ms/op |
| Lookup allocated bytes | 2,216,620 B/op | 2,815,623 B/op |
| Lookup allocations | 1,552/op | 513/op |
| State update | not measured | 0.424 ms/op, 5,751 B/op |
| List 50 session summaries | not measured | 0.174 ms/op, 37,575 B/op |
| CGo-disabled app executable | 21,353,346 bytes | 27,609,954 bytes |

Lookup is faster in this sample with fewer allocations, but allocates about 27%
more bytes. SQLite payload copying and canonical re-encoding for integrity checks
still materialize the selected snapshot. This change does not claim lazy loading
of a large opened review. State updates and summary listing avoid that payload
cost. The embedded engine increases the executable by about 6.3 MB (29%).

The baseline was collected before replacing persistence. Benchmarks ran with
`-benchmem -count=5`; performance and integration commands overlapped, so timing
variance should be expected. Correctness checks and structural sharing are the
acceptance requirements; no latency threshold was lowered or introduced.
