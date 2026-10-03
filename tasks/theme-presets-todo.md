# Full TUI Themes: Implementation Checklist

Approved for implementation on 2026-10-02. Scoped domain, canvas, and picker
subagents implemented their slices; root integrated Markdown, profiles, and
regression coverage. Human terminal visual acceptance remains a release check.

## 1. Canvas composition verification spike

- [x] Prove a central composition helper can fill absent foreground/background
  channels through nested resets, semantic spans, selection, wide runes, and blanks.
- [x] Verify ordinary cell styling respects profiles and emits no terminal-default
  OSC colors; compare logical text separately from canvas padding.
- [x] Select the simplest approach using installed dependencies and document
  evidence before integrating into Model.View.

Depends on: plan review. Scope: small.
Files: new `internal/tui/theme_canvas.go`, `theme_canvas_test.go`.
Verify: `go test ./internal/tui -run TestThemeCanvas -count=1`.

## Checkpoint: Rendering feasibility

- [x] Cell-level assertions prove resets restore base colors and explicit styles win.
- [x] Colorless output is unchanged; no dependency or terminal-settings workaround.

## 2. Backward-compatible base tokens

- [x] Add `foreground`/`background`; old presets inherit both with `default`.
- [x] Accept optional overrides, validate safely, and preserve their provenance
  across resolve/load/save without migrating existing files.
- [x] Assert old eleven token values remain unchanged and unsafe inputs fail.

Depends on: 1. Scope: medium.
Files: `internal/theme/theme.go`, `theme_test.go`, `config_test.go`.
Verify: `go test ./internal/theme -count=1`.

## 3. Named catalog and four full palettes

- [x] Add ordered metadata and four stable IDs with all thirteen token values.
- [x] Record pinned upstream provenance/attribution and exact palette assertions.
- [x] Keep returned metadata isolated and name/resolve/config definitions consistent.

Depends on: 2. Scope: medium.
Files: `internal/theme/theme.go`, new `presets.go`, `presets_test.go`,
`theme_test.go`, attribution file if required.
Verify: `go test ./internal/theme -count=1`; `go build ./...`.

## 4. Integrate the final interactive canvas

- [x] Apply base colors after clipping/modal composition in Model.View;
  cover every screen and blank viewport cell while preserving semantic styles.
- [x] Preserve inherited/colorless paths and logical content geometry; repaint
  after resize, theme switch, and return to terminal defaults.
- [x] Exercise selections, composers, help, loading, errors, and one-channel
  `default` overrides without adding terminal probes or OSC setters.

Depends on: 3. Scope: medium.
Files: `internal/tui/model.go`, `theme_canvas.go`, `theme_canvas_test.go`,
`style.go` if required, `color_test.go`.
Verify: `go test ./internal/tui -run 'Test(ThemeCanvas|Color)' -count=1`.

## Checkpoint: Full canvas

- [x] New presets paint all regions; inherited old presets retain baseline bytes.
- [x] Resize/switch clears stale colors; terminal profiles preserve logical content.

## 5. Markdown background integration

- [x] Render using resolved base colors and a local Glamour style copy that
  avoids conflicting block backgrounds; retain existing syntax foregrounds.
- [x] Preserve content/wrapping/links/escaping and existing theme-change cache
  invalidation across tabs, including background/foreground overrides.
- [x] Test headings, paragraphs, tables, inline/fenced code, empty lines, and
  switches between inherited and explicit base channels.

Depends on: 4. Scope: medium.
Files: `internal/tui/description_markdown.go`, `description_markdown_test.go`,
`model.go`, `description_view_test.go`.
Verify: `go test ./internal/tui -run TestDescription -count=1`.

## 6. Bounded scrolling theme picker

- [x] Use catalog labels; show active/candidate markers and candidate position.
- [x] Keep selection and controls visible through the specified dimension matrix;
  use compact fallback when needed and reach every preset.
- [x] Retain navigation, cancel, save-before-apply, failure, and CLI lock behavior.

Depends on: 5. Scope: medium.
Files: `internal/tui/theme_picker.go`, `picker_geometry.go`,
`theme_picker_test.go`, `picker_geometry_test.go` (new if necessary).
Verify: `go test ./internal/tui -run 'Test(ThemePicker|Picker)' -count=1`.

## 7. Candidate sample with base colors

- [x] Show a synthetic rectangular sample with candidate foreground/background,
  semantic roles, selections, and existing overrides when dimensions permit.
- [x] Use candidate canvas only within the sample; navigation/resize/cancel
  leaves active palette/review/cache/config unchanged.
- [x] Hide sample before sacrificing navigation rows/hints; retain colorless cues.

Depends on: 6. Scope: small.
Files: `internal/tui/theme_picker.go`, `theme_picker_test.go`.
Verify: `go test ./internal/tui -run TestThemePicker -count=1`.

## Checkpoint: Description and picker

- [x] No foreign Markdown background rectangles or stale base-color caches.
- [x] Picker fits; sample shows resolved overrides without applying/saving them.

## 8. Configuration and renderer integration regressions

- [x] Cover CLI selection/precedence, invalid rejection, locked save, safe fallback,
  failed writes, and foreground/background override persistence with new presets.
- [x] Capture actual program/renderer output for TrueColor, ANSI256, ANSI16,
  Ascii/NO_COLOR, and NoTTY; detect OSC color changes and leaked color controls.
- [x] Retain plain-output isolation and old theme baselines; compare logical text
  and canvas geometry separately so padding cannot mask content changes.

Depends on: 7. Scope: medium.
Files: `cmd/prui/theme_test.go`, `main_test.go`, `internal/theme/config_test.go`,
`internal/tui/color_test.go`, new renderer integration test file.
Verify: `go test ./internal/theme ./internal/tui ./cmd/prui -count=1`.

## 9. Documentation and release verification

- [x] Document all IDs and optional base keys, inherited old defaults, full-canvas
  behavior, sample behavior, and unchanged syntax theming. Explain that older
  app versions reject newly added override keys; do not migrate configs.
- [ ] Visually inspect all presets/screens on light and dark host terminals and
  all supported profiles; record human readability acceptance/palette adjustments.
- [x] Full checks pass and final diff matches the approved scope.

Depends on: 8. Scope: medium.
Files: `docs/REFERENCE.md`, `docs/spec/SPEC-custom-themes.md`,
`docs/spec/SPEC-theme-presets.md`, CLI help if needed, this checklist.
Verify: `./scripts/verify.sh`; `golangci-lint run ./...`;
`go build ./...`; `git diff --check`.

## Checkpoint: Complete

- [x] All spec acceptance criteria and automated gates pass.
- [ ] Human terminal visual QA recorded. Syntax themes and terminal-default
  mutation are excluded and verified automatically.

## Verification record

- Focused domain/config/CLI/picker/canvas/Markdown/profile tests passed.
- Actual Bubble Tea renderer capture and output replay passed for TrueColor,
  ANSI256, ANSI16, Ascii, and NoTTY, including resize and returning to inherited
  terminal colors. Canvas waits for capability detection at startup.
- Independent review found candidate inherited channels were filled by the active
  frame; fixed by overlaying candidate cells after frame painting and guarded by
  final-View regression tests. Follow-up review found no required issues.
- Repository verification: `./scripts/verify.sh` (vet, full race tests, build)
  and `golangci-lint run ./...` passed again after the preview and Markdown fixes.
- Sandbox Git fixture object access failed; verification uses the authorized
  unsandboxed execution with synthetic fixtures only. Build/lint caches live in
  `/private/tmp`. No dependencies changed or checks weakened.

## Expanded catalog follow-up

- [x] Add the eighteen requested presets with complete palettes, appearance
  metadata, and provenance; preserve original IDs/colors/order prefix.
- [x] Show Terminal separately and Dark/Light sections with nonselectable
  headings; preserve keyboard/mouse/scroll/resize and sample isolation.
- [x] Use explicit catalog appearance for Markdown light/dark baseline.
- [x] Extend CLI/config, palette, light Markdown, and grouping regressions.
- [x] Update user reference/spec, complete independent review, pass full checks,
  and reinstall this worktree build for testing.

Expanded catalog verification: full vet/race suite/build passed; final palette
contrast changes passed focused race checks and lint (zero issues). Independent
review checked grouping, preview composition, Markdown, and light-palette
readability. Three light accents were darkened with contrast regressions.
Installed updated worktree build to `/Users/steve/go/bin/prui` for user testing.
