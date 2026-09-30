# Shared Diff Rendering tasks

Spec: [Shared Diff Rendering](../docs/spec/SPEC-shared-diff-rendering.md).
Plan: [Implementation plan](shared-diff-rendering-plan.md).

- [ ] DR-01 — Share immutable text-hunk rendering. Owner: renderer agent.
  - Accept: one raw hunk walk supplies both main and commit rows; typed line
    coordinates stay separate from text; output and exact targets preserved.
  - Files: render.go, commits.go (renderer only), optional style.go, new tests.
  - Verify: renderer/target/split tests and existing TUI snapshots; focused suite.
  - Depends: spec. Scope: medium.
- [ ] DR-02 — Keep the commit editor visible on resize. Owner: viewport agent.
  - Accept: failing reproduction becomes green for fitting/tall editors;
    terminal width/height changes preserve the active cursor and main scroll.
  - Files: model.go (resize only), new commit editor resize tests.
  - Verify: focused resize/editor model tests. Depends: spec. Scope: small.
- [ ] DR-03 — Retain commit source rows while editing. Owner: cache agent.
  - Accept: only selected source retained; draft/blink/refresh/cancel stay fresh;
    allocation growth avoids reparsing all patch rows on each editor update.
  - Files: commits.go (cache/rows), optional commit cache helper, new tests.
  - Verify: allocation regression, overlay lifecycle tests, TUI suite.
  - Depends: DR-01. Scope: medium.
- [ ] Checkpoint — renderer, cache, resize compose with existing comment rules.
- [ ] DR-04 — Share pane-body assembly. Owner: frame agent.
  - Accept: both views use the same narrow/wide body helper; existing labels,
    border classes, selection, clipping, and state remain unchanged.
  - Files: model.go (review frame), commits.go (view), new pane helper/tests.
  - Verify: screen/mouse/theme tests and complete TUI suite.
  - Depends: DR-02, DR-03. Scope: medium.
- [ ] DR-05 — Review and verify integrated changes. Owner: coordinator/reviewer.
  - Accept: resolve concrete review findings, document evidence, no unrelated
    changes; spec index links to this work.
  - Files: spec index and these task documents; targeted fixes if needed.
  - Verify: ./scripts/verify.sh and git diff --check.
  - Depends: DR-01–04. Scope: small plus verification.
