# Shared Diff Rendering tasks

Spec: [Shared Diff Rendering](../docs/spec/SPEC-shared-diff-rendering.md).
Plan: [Implementation plan](shared-diff-rendering-plan.md).

- [x] DR-01 — Share immutable text-hunk rendering. Owner: renderer agent.
  - Accept: one raw hunk walk supplies both main and commit rows; typed line
    coordinates stay separate from text; output and exact targets preserved.
  - Files: render.go, commits.go (renderer only), optional style.go, new tests.
  - Verify: renderer/target/split tests and existing TUI snapshots; focused suite.
  - Depends: spec. Scope: medium.
- [x] DR-02 — Keep the commit editor visible on resize. Owner: viewport agent.
  - Accept: failing reproduction becomes green for fitting/tall editors;
    terminal width/height changes preserve the active cursor and main scroll.
  - Files: model.go (resize only), new commit editor resize tests.
  - Verify: focused resize/editor model tests. Depends: spec. Scope: small.
- [x] DR-03 — Retain commit source rows while editing. Owner: cache agent.
  - Accept: only selected source retained; draft/blink/refresh/cancel stay fresh;
    allocation growth avoids reparsing all patch rows on each editor update.
  - Files: commits.go (cache/rows), optional commit cache helper, new tests.
  - Verify: allocation regression, overlay lifecycle tests, TUI suite.
  - Depends: DR-01. Scope: medium.
- [x] Checkpoint — renderer, cache, resize compose with existing comment rules.
- [x] DR-04 — Share pane-body assembly. Owner: viewport/frame agent.
  - Accept: both views use the same narrow/wide body helper; existing labels,
    border classes, selection, clipping, and state remain unchanged.
  - Files: model.go (review frame), commits.go (view), new pane helper/tests.
  - Verify: screen/mouse/theme tests and complete TUI suite.
  - Depends: DR-02, DR-03. Scope: medium.
- [x] DR-05 — Review and verify integrated changes. Owner: coordinator/reviewer.
  - Accept: resolve concrete review findings, document evidence, no unrelated
    changes; spec index links to this work.
  - Files: spec index and these task documents; targeted fixes if needed.
  - Verify: ./scripts/verify.sh and git diff --check.
  - Depends: DR-01–04. Scope: small plus verification.

## Implementation evidence — 2026-09-30

- Renderer agent implemented shared textHunkLines with typed source coordinates
  and display-time commit numbering. Exact anchor and split-projection tests pass.
- Viewport agent reproduced offscreen short/tall editors after shrinking the
  terminal, then fixed resize visibility. The same agent extracted paneBodyRow;
  existing screen goldens are unchanged.
- Cache agent measured editor redraw allocations at 210 / 15,084 for 30 / 3,000
  lines before the fix, and 20 / 20 afterward. Tests cover retained immutable
  source, snapshot replacement, refreshed discussions, and fresh/cancelled editors.
- Combined go test ./internal/tui ./internal/commits passed. Independent review
  found no actionable issues; focused renderer, cache, resize, commit, snapshot,
  and side-by-side tests passed during review.
- Coordinator removed the superseded target-construction wrapper while retaining
  its path-validation assertion against the shared predicate.
- Full gate initially hit sandbox restrictions: macOS Git confstr warnings
  contaminated fixture SHA output and localhost HTTP fixtures could not bind.
  The same gate passed outside the sandbox; no tests were weakened.
- GOCACHE=/tmp/review-go-cache ./scripts/verify.sh passed: formatting, go vet,
  complete race-enabled tests (including program/PTY coverage), and build.
  git diff --check passed. All tasks and the integration checkpoint are complete.
