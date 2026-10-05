# Diff style composition tasks

- [x] Reproduce both screenshot color bugs with failing rendered-cell tests.
- [x] Implement shared source/structural foreground separation and cell emphasis.
- [x] Expand regression coverage across styles, capabilities and geometry.
- [x] Update syntax documentation and review the final change.
- [x] Pass focused and full verification; record terminal QA limitations.

## Verification record — 2026-10-05

- Both screenshot regressions failed before the fix and pass afterward.
- `go test ./internal/tui ./internal/syntax -count=1` passed.
- `GOCACHE=/private/tmp/prui-diff-style-go-cache ./scripts/verify.sh` passed
  outside the sandbox: formatting, vet, all race tests, build, and the existing
  shipped-executable PTY smoke tests. Sandbox attempts failed because macOS Git
  temporary-directory lookup and local test-server sockets were restricted;
  no checks were skipped or weakened.
- Additional search/syntax-boundary and active-match tests passed under
  `go test -race ./internal/tui -run TestDiffStyle -count=1` after the full gate.
- `git diff --check` passed; text/geometry screen goldens did not change.
- Self-review covered foreground inheritance, marker separation, fragment reset
  handling, source/semantic row classes, search and selection precedence,
  hyperlinks, immutable coordinates, and bounded visible-cell work.
- Human live-terminal readability/accessibility QA remains pending. Automated
  cell assertions and PTY smoke tests are not a substitute for that check.
