# Automated verification

## Text selection and clipboard feedback

`go test ./internal/tui -run 'TextSelection|ClipboardToast|Copy|URL' -count=1`
checks release-only copying, reverse multiline selection, pane clipping, wide
and combining graphemes, gesture cancellation, URL click/drag routing, and
generation-safe toast dismissal. `TestPTYSmoke` also drives a drag through real
SGR mouse input, checks the emitted OSC 52 payload, and waits for the toast to
disappear. These checks verify terminal output; actual clipboard acceptance and
subjective selection visibility still require human terminal verification.

## Conversation Markdown and HTML

`go test ./internal/tui -run 'ConversationMarkdown|OverviewRenders|OverviewConversationMarkdown' -count=1`
covers hidden bot metadata, HTML conversion and tables, collapsed/nested/malformed
details, image labels, referenced links, literal code, raw/decoded controls,
unsafe destinations, Unicode wrapping, bounded caches, theme/resize changes and
Enter/Escape activity identity. Screen fixtures include the screenshot-like
Vercel comment in wide, narrow and expanded views. These are synthetic offline
checks; live GitHub rendering and subjective terminal readability require human QA.

## Diff scrolling performance

Run `go test ./internal/tui -run '^$' -bench BenchmarkFilesScrollTransition -benchmem -count=3`
to compare j/k movement within a file and across a file boundary. The fixture
contains 100 files and 1,000 text hunks, with warmed source caches and no comment
overlays. Both cases execute key handling and View; terminal output is excluded.

On an Apple M2 Max with Go 1.26.8 (2026-09-30), bypassing overlay reconstruction
when comments, pending drafts, and the composer are absent produced these median
boundary-crossing results across three runs:

| Layout | Before | After | Allocated bytes per key, before → after |
| --- | --- | --- | --- |
| Unified, 120 columns | 4.29 ms | 0.59 ms | 17.98 MB → 0.56 MB |
| Split, 180 columns | 12.02 ms | 0.76 ms | 46.02 MB → 0.44 MB |

Measurements used the same benchmark command with CPU and heap profiles enabled
for both versions. Interior and crossing costs were similar; no distinct
file-selection stall was reproduced. Over 96% of baseline allocated bytes were
attributed to detail/sideBySideDetail and rowTargets. The optimization is retained;
the unchanged-stream allocation test guards against rebuilding cached file rows.
Existing cache tests verify rendering does not mutate source and overlays stay
current. Reviews with overlays still rebuild the stream, and navigation still
scans rows; terminal rendering and held-key cadence require interactive checking.

## Repository verification

Run the same gate as CI from the repository root:

```sh
./scripts/verify.sh
```

Run the expanded static-analysis gate locally with:

```sh
golangci-lint run ./...
```

Check dependencies against the Go vulnerability database with the same pinned
tool as CI:

```sh
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

This check needs network access to the public vulnerability database. It analyzes
the pinned dependency graph and fails on identified reachable vulnerabilities;
it does not upload repository source. CI runs it as a separate `Vulnerabilities`
job alongside lint and platform verification.

CI installs the pinned GolangCI-Lint release and runs this check separately
from the platform verification matrix. Its configuration deliberately focuses
on correctness and bug detection; the full all-linters audit is not a merge
gate because it mixes incompatible style rules and arbitrary complexity caps.

The secret scan examines all local Git refs. Its three fingerprint exceptions
identify historical synthetic test fixtures, not entire files or directories.
CI fetches complete history for that check. Dependabot tracks Go module and
GitHub Action updates weekly; CI action references are pinned to commit SHAs.

Requirements: macOS or Linux, the Go version declared in `go.mod`, Git, and
Python 3. The gate checks Go formatting without rewriting files, runs `go vet`,
runs every Go test with the race detector and a five-minute package timeout,
then builds all packages. The Go suite includes the compiled-binary terminal
tests; missing terminal-test prerequisites are failures, not silent skips.

Tests use disposable Git repositories and private session stores, fake GitHub
responses, and local HTTP provider endpoints. No live GitHub/provider credentials
are needed. Local process execution, PTYs, and loopback listeners must be allowed;
a sandbox that denies those operations cannot run the full integration suite.
Never run scripts from a reviewed repository as test fixtures.

`prui verify` is intentionally not a test-suite command: it opens a real
PR through the user's authenticated GitHub CLI. Its journey engine is covered
with a synthetic child executable and standard-library PTY fixture; run a live
verification manually only when an agent has been given a specific PR URL and
checkout.

The GitHub Actions workflow runs this gate on Linux and macOS for pull requests
and pushes to `main`. Repository maintainers can require both `Verify` matrix
checks in branch protection; adding the workflow alone does not configure that
remote setting.

## Where a new test belongs

| Change | Test to add |
| --- | --- |
| Source, inventory, privacy, guides, persistence | Focused behavior test in the owning package, using real local implementations where practical |
| Navigation, focus, back, layout | TUI key/message scenario; assert selected content and navigation outcomes |
| Initialization, asynchronous actions, cancellation | Real Bubble Tea program journey using the driver in `internal/tui/program_test.go` |
| Screen wording or arrangement | Small fixed-size baseline in `internal/tui/testdata/screens`, plus relevant visibility/width assertions |
| CLI wiring, terminal input, resize, exit, restart | Compiled-binary PTY smoke scenario under `cmd/prui` |

For each feature, cover its successful journey plus relevant failure and
cancellation paths. For a bug, reproduce it with a failing test before changing
production code. Preserve existing tests and source/privacy invariants. Coverage
reports help locate missing branches; a high percentage does not establish that
constructors, commands, and screens compose correctly.

Useful focused commands:

```sh
go test -race ./internal/tui -run '^TestProgram' -count=1
go test ./internal/tui -run '^TestScreenSnapshots$' -count=1
go test -race ./cmd/prui -run 'Test.*PTY' -count=1
go test -race ./internal/session ./internal/review -count=1
go test -race -coverprofile=/tmp/prui-coverage.out ./...
go tool cover -html=/tmp/prui-coverage.out
```

## Program and scenario tests

The real-program driver starts `tea.NewProgram` with fixed dimensions and no
terminal renderer. It still executes `Init`, the framework's command dispatch,
asynchronous results, `Update`, and quitting. Screen observations are copied on
the event-loop goroutine, avoiding races from reading a live model in a test.
Wait for an expected observation using a deadline; do not insert sleeps and hope
an operation has finished. Failures report the last observed screen.

Use channels to hold a fake operation pending, fail it, or release it. Assert
that cancellation retains the current review and allows retry. Verify durable
progress through a reopened session, not only a changed in-memory field. Keep
lower-level scenario tests for exhaustive navigation/layout cases; reserve real
program and PTY tests for important composed journeys.

## Screen baselines

Ordinary test runs only compare. For an intentional screen change:

```sh
go test ./internal/tui -run '^TestScreenSnapshots$' -args -update-golden
git diff -- internal/tui/testdata/screens
go test ./internal/tui -run '^TestScreenSnapshots$' -count=1
```

Review the baseline diff as part of the change. Do not automatically regenerate
baselines in CI. Fixtures use fixed content and dimensions, strip styling, and
trim invisible right-edge padding so
that timestamps, random session IDs, paths, and color capability cannot cause
spurious changes. Existing color tests independently verify styling, escaping,
and the equality of styled and unstyled content. Width and selected-row checks
still run while updating: a clipped selection cannot be blessed into a baseline.

The PTY harness reconstructs cursor movement and incremental redraws for its
ASCII fixtures instead of searching an ANSI-stripped transcript. Unsupported
screen operations fail explicitly. It checks restored terminal modes, cursor
visibility, and exit from the alternate screen; the kernel's transient `PENDIN`
indicator is excluded from configured-mode comparison.

## Scope of confidence

The layered suite replaces routine manual regression checks for covered flows.
It does not prove usability, screen-reader behavior, or compatibility with every
terminal/theme. Human accessibility and usability assessment remains useful for
substantial interaction changes. Linux CI results are only established after the
workflow runs on Linux; a successful local macOS run is not a Linux result.

CI configuration follows the official [setup-go documentation](https://github.com/actions/setup-go)
and [checkout documentation](https://github.com/actions/checkout). Go is selected
from `go.mod`, and checkout credentials are not persisted in the working tree.

## PR loading benchmarks

`go test ./internal/session -run '^$' -bench BenchmarkLatestComparisonHistory -benchmem`
measures reopening against 50 saved snapshots with 256 KiB of evidence each.
`go test ./internal/source -run '^$' -bench BenchmarkBlobRead`
measures bounded blob reads using an isolated local Git fixture. Neither benchmark
contacts GitHub. Compare results on the same machine; these measure individual
loading stages, not total interactive startup or network time.

## SQLite persistence verification

Persistence tests use real disposable SQLite databases. They cover canonical
source sharing, generation conflicts, ordered progress, metadata-only listing,
reference-aware deletion, private ownership, read-only behavior, and failure
rollback. Process tests exercise concurrent updates and interrupted transactions.
They never open or import the user's previous file cache.

Run the embedded-driver proof without CGo:

```sh
CGO_ENABLED=0 go test -count=1 ./internal/session/storage
CGO_ENABLED=0 go build -o /tmp/prui-sqlite ./cmd/prui
go test ./internal/session -run '^$' -bench 'Benchmark(LatestComparisonHistory|SQLite)' -benchmem -count=5
```

For release evidence, execute storage tests and the full verification gate on
both macOS and Linux; a cross-build alone does not establish runtime behavior.
The driver is embedded and requires no user-installed SQLite tools or library.
Windows support and verification remain deferred. See
[SQLite verification evidence](docs/SQLITE-VERIFICATION.md) for measured results.

## Durable review drafts

`go test -race ./internal/session ./internal/tui ./internal/source -run 'TestDraft|TestReplyDraft|TestReplyAttempt|TestHistoricalDraft|TestProgramDraft' -count=1`
covers durable editor/summary/reply recovery, a child process exiting without
cleanup after saving write intent, same-comparison sharing and deletion, stale
writers, changed pins, historical provenance, associated root anchors, and
bounded remote outcome matching. The real Bubble Tea journey saves, quits, and
reopens a summary. All GitHub history is synthetic; no fixture contacts GitHub.
Malformed, null, capped, or failed history cannot enable retry. Manual terminal
usability and GitHub's delayed in-flight request behavior remain outside this
synthetic verification.


## Diff text search

Run `go test ./internal/tui -run TestDiffSearch -count=1` for matching,
current-section scope, source navigation, stale-result rejection, result limits,
wrapping/split/resize behavior, escaping, colorless output, and mouse ownership.
`TestPTYSmoke` also drives `/`, query editing, Escape-to-results, `j/k`, and result activation through the
compiled binary in an offline synthetic review, asserting no GitHub invocation.

Run `go test ./internal/tui -run '^$' -bench BenchmarkDiffSearch -benchmem -count=3`
for a 100,020-line fixture. On Apple M2 Max / Go 1.26.8, warm absent-query scans
measured 5.36–5.49 ms mean and 5.93–5.95 ms p95 (2026-10-04), with 497 B/op.
The capped 10,000-result popover measured 0.67–0.76 ms per render; only nearby
snippets are formatted. These exclude terminal output and are observations, not
portable CI thresholds. Deterministic tests enforce cancellation and result caps.

Human terminal review should check light/dark highlight contrast and keyboard/
mouse discoverability. Automated color-cell assertions and PTY checks do not
replace that subjective usability review.

Issue #30 focused synthetic/offline regression coverage:
`go test ./internal/tui ./internal/inventory ./internal/session ./internal/review ./cmd/prui`.
New tests cover explicit full-source consent, immutable blobs despite changed
working files, offline restart/deletion/integrity, AI-input equivalence, source
coverage, insertion/deletion middle/EOF coordinates across display modes,
whitespace progress/search reveal and loaded unresolved-thread navigation.
Human terminal QA remains unverified; screen/PTY checks do not establish it.

Durable general integration: `go test ./internal/tui ./internal/session -run
'Test.*GeneralDraft' -count=1` covers real key/dispatch/restart reconciliation,
abrupt child exit preserving combined general/range/suggestion/reply/review work,
old payloads, bounds, CAS refusal, original-comparison isolation, privacy and
cleanup. Existing combined thread/navigation/incremental regressions remain.

Issue #36 focused provider/cache/UI regressions:
`go test ./internal/source ./internal/issuecontext ./internal/session ./internal/tui ./cmd/prui -run 'TestIssueContext|TestLinear' -count=1`.
Fixtures exercise requested users/teams, aggregate and individual decisions,
labels, authoritative issue links, null/missing/partial GraphQL connections,
real configured Linear request/auth construction with fake transport/credentials,
strict workspace/link validation, cancellation/redirects/bounds, optional malformed
records, global authorization, independent cache restart/integrity/read-only and
latest-writer behavior, explicit refresh vs local open, offline refusal, stale
retention, async result ownership, escaping and narrow viewports. Tests never
read live providers. Human terminal QA remains unverified.

Restacked #36 combined coverage uses actual GH normalization and Linear request
handling with synthetic runner/transport, real private SQLite offline restart,
PR identity separation, captured workspace/auth provenance after account/config
changes, denied/partial-provider failures, and byte-equivalent frozen source.
`TestIssueContextUpdatePrivateAttemptsSourceAndLayoutIntegration` drives actual
I/r/Escape dispatch with real pinned full-source capture and durable v3/v4
private attempts; asserts unchanged layout/cursors/pins/payload generations,
canonical targets and read-only source-row identity. Foreign PR/session/workspace
records are rejected. Main's Alt+R/C/F/slash/Alt+C controls remain covered.

## Word-level highlighting (#37)

`go test ./internal/tui -run 'WordDiff|Syntax|Wrap|Navigation|Whitespace|Search|Anchor' -count=1`
checks changed-token fixtures (Go, Python, JSON and prose), Unicode/combining and
wide characters, tabs, wrapping/panning, unified/split styling, raw text and
canonical comment targets, ambiguous/many-to-many fallback, invalid UTF-8 and
patch/line/token/work limits. Source/navigation/search regression tests accompany
the focused word tests. These are synthetic renderer checks; live human terminal
behavior and accessibility have not been verified for this release.

Restacked word-diff integration: `go test ./internal/tui -run '^TestWordDiff' -count=1`
includes a real Git capture saved in SQLite, deletion of the source repository,
and offline recovery of combined v3 suggestion/v4 general drafts. It checks
immutable dispatched comment/general bytes alongside line/range/file targets,
raw suggestion Before/replacement text, Unicode multi-line emphasis, both layouts,
wrap/pan/resize, the Alt+C collapse handler, full OLD/NEW source and search IDs.
Exact token-work boundaries and oversized replacement blocks also retain line-diff
fallback. Renderer fixtures remain synthetic terminal evidence, not human QA.

`TestIssueContextUpdatePrivateAttemptsSourceAndLayoutIntegration` now also uses
Unicode changed-word source. Actual I/r/Escape open/refresh/cancel and late results
preserve word spans, raw patch/source text, canonical old/new targets and search
IDs alongside prepared suggestion payload bytes, immutable review/general
attempts, SQLite draft generations, layouts and read-only full-source cursors.

## Large diffs with comment overlays

`go test ./internal/tui -run 'TestDiffOverlay|TestWrappedDiffViewport' -count=1`
checks warm scrolling allocations and compares cached output with the original
attachment semantics in unified and split layouts. Fixtures cover range/file
comments, replies, multiple pending comments, editor changes/caret/error,
resizing, line numbers, removal, unanchored comments and snapshot replacement.

`go test ./internal/tui -run '^$' -bench '^BenchmarkLargeJSONScroll$' -benchmem -benchtime=5x -count=3`
measures warm keypress + render cycles for synthetic 10,000/100,000-line JSON
patches with and without one comment. On Apple M2 Max / Go 1.26.8 (2026-10-10),
the 100,000-line comment case improved from 318–322 ms and ~506 MB/op to
1.13–1.14 ms and ~133 KB/op. Without comments it remained ~1 ms.
The fix retains wrapped source and composed rows, compares only freshly rendered
small overlay blocks, and indexes overlays by endpoint. It avoids full-stream
wrapping and comment scans during unchanged navigation. These measurements
exclude cold loading and terminal output; they are observations, not portable
CI time thresholds. The allocation-budget test guards against source-sized
work returning. The reviewer confirmed that the fix resolved scrolling lag on
the reported large JSON PR; broader terminal/accessibility QA remains unverified.
