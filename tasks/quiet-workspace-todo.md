# Quiet workspace implementation

## 1. Workspace shell — workspace_layout
- [x] Persistent escaped repository/PR/title identity and quiet selected context tabs.
- [x] Single pane divider, explicit focus, correct content budget at narrow/short sizes.
- [x] Existing resize, split-diff, description scrolling and cursor semantics preserved.
Verification: focused workspace/context/description tests. Depends on: none.
Files: model.go, workspace helpers/tests, context/description tests. Scope: medium.

## 2. Quiet styling — quiet_styling
- [x] Neutral secondary chrome and restrained accent, readable additions/removals.
- [x] Existing theme schema/customization and high-contrast preserved.
- [x] Styled/unstyled content and color-independent focus cues remain equivalent.
Verification: theme/style/color focused tests. Depends on: none.
Files: style.go, theme.go, theme tests, color/new styling tests. Scope: medium.

## 3. Compact PR picker — compact_picker
- [x] Compact rows plus selected metadata, explicit checks and safe long titles.
- [x] Selection stays visible through navigation and resize; switcher uses compatible rows.
- [x] Existing filter/order/open/cancel and async semantics unchanged.
Verification: picker tests for long lists, short screens and escaping. Depends on: none.
Files: pr_picker_view.go, picker.go, lifecycle.go picker rendering, picker tests. Scope: medium.

## Checkpoint: initial streams
- [x] Review diffs, resolve shared-file seams, run focused integrated tests.

## 4. Status and contextual shortcuts — root
- [x] Progress, pending count and Review remain discoverable; severe states take priority.
- [x] Local hints match focused context and preserve explicit post/pending composer labels.
- [x] Omitted health detail remains accessible from Health & help.
Verification: state/width/status priority tests. Depends on: 1.
Files: chrome.go, bindings.go, chrome tests, layout integration. Scope: medium.

## 5. File/guide hierarchy polish — quiet_styling and root
- [x] Consistent selection/read markers, quieter file separators and bounded paths.
- [x] Repetitive structural text reduced while guide explanations stay accessible.
- [x] Raw inventory and whole-file marking semantics stay explicit.
Verification: guide/render/target/path tests. Depends on: 1, 2.
Files: guides.go, render.go, relevant tests; model.go integration coordinated with root. Scope: medium.

## Checkpoint: complete UI
- [x] Integrated TUI and theme tests pass; affected baselines reviewed.

## 6. Integration verification — root
- [x] Review/update affected golden screens, without removing behavioral coverage.
- [x] Verify width boundaries, small heights, long content, focus and immutable targets.
- [x] Full verification script and diff checks pass; changed-code lint reports zero issues.
- [ ] Repository-wide lint is clean (11 pre-existing findings remain; no suppressions added).
Verification: ./scripts/verify.sh; golangci-lint run ./...; git diff --check.
Depends on: 1–5. Files: snapshot fixtures and existing integration tests as needed.

## 7. Documentation and acceptance — root
- [x] README and layout specification match implemented behavior.
- [x] Final review finds no source/session/plain behavior changes.
- [x] Terminal walkthrough completed, or clearly recorded as outstanding human acceptance.
Depends on: 6. Files: README.md, docs/spec/SPEC-tui-review-layout.md, this checklist. Scope: small.


## Verification record — 2026-09-25

- `GOCACHE=/tmp/prui-go-cache ./scripts/verify.sh`: PASS, including formatting,
  vet, race tests, compiled-binary PTY tests and build. TUI suite: 28.647 seconds.
- `golangci-lint run --new-from-rev=HEAD ./...`: PASS, 0 new issues.
- `golangci-lint run ./...`: 11 existing findings in theme config and existing
  TUI key/state helpers. No checks or thresholds were disabled.
- `git diff --check`: PASS. Local `prui` binary rebuilt.
- Inspected updated wide/narrow review, guide, picker, description, comment and
  error baselines. Automated viewport tests cover 60/99/100/120/159/160 columns,
  short heights, selected explanations, escaping, and monochrome focus.
- Synthetic Git/PTY verification needed execution outside the restricted
  sandbox. No live GitHub or provider operations were used.
- Corrected stale tests to select inventory explicitly for unit traversal and
  use current Files target-navigation keys. The 50,000-unit viewport fixture
  retains 100 moves and adds explicit selection/height assertions.
- Human terminal usability/accessibility walkthrough remains outstanding;
  automated terminal tests do not establish that acceptance.
