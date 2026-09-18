# Automated verification

Run the same gate as CI from the repository root:

```sh
./scripts/verify.sh
```

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

`pr-review verify` is intentionally not a test-suite command: it opens a real
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
| CLI wiring, terminal input, resize, exit, restart | Compiled-binary PTY smoke scenario under `cmd/pr-review` |

For each feature, cover its successful journey plus relevant failure and
cancellation paths. For a bug, reproduce it with a failing test before changing
production code. Preserve existing tests and source/privacy invariants. Coverage
reports help locate missing branches; a high percentage does not establish that
constructors, commands, and screens compose correctly.

Useful focused commands:

```sh
go test -race ./internal/tui -run '^TestProgram' -count=1
go test ./internal/tui -run '^TestScreenSnapshots$' -count=1
go test -race ./cmd/pr-review -run 'Test.*PTY' -count=1
go test -race ./internal/session ./internal/review -count=1
go test -race -coverprofile=/tmp/pr-review-coverage.out ./...
go tool cover -html=/tmp/pr-review-coverage.out
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
