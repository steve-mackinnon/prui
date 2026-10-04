# Diff text search tasks

- [x] Match frozen source and current-section scope.
  - Files: diff_search.go, diff_search_test.go, style.go, render.go.
  - Acceptance: literal Unicode-aware occurrences, stable source identity, bounded results, no structural-row matches.
  - Verify: focused matcher/scope tests.
- [x] Integrate query input, lifecycle, and exact result activation.
  - Files: diff_search.go, model.go, picker.go, diff_search_test.go.
  - Acceptance: modal owns input, stale results ignored, exact source navigation without writes.
  - Verify: model and navigation regressions.
- [x] Render grouped popover and inline highlights.
  - Files: diff_search_view.go, syntax.go, render.go, mouse.go, diff_search_test.go.
  - Acceptance: grouped snippets, clipping-safe highlights, mouse ownership, narrow/split support.
  - Verify: render and mouse tests.
- [x] Verify and document.
  - Acceptance: full repository gate, lint, benchmark, help/reference and feature tests updated.
  - Verify: ./scripts/verify.sh; golangci-lint run ./...; git diff --check.


## Verification record — 2026-10-04

- Focused matcher/model/render/mouse tests passed, including section isolation,
  stale results, capped results, original source cache immutability, split-side
  targeting, and resize reveal of a match deep within a wrapped line.
- `./scripts/verify.sh`: formatting, vet, full race suite, compiled-binary PTY
  journeys, and build passed.
- `golangci-lint run ./...`: zero issues.
- `git diff --check`: passed.
- Benchmarks and machine details are recorded in `TESTING.md`.
- Existing snapshot changes are limited to the Find header; three new snapshots
  cover the search popover in wide, narrow, and split views.
- Final code review checked source provenance, escaping, modal ownership, bounded
  query work, stale generations, and exact source-side navigation.
- Real-terminal subjective contrast/discoverability remains a human acceptance
  check; automated theme-cell assertions and offline PTY interaction passed.
