# Spec: Named TUI Theme Presets

Status: Approved and implemented on 2026-10-02. Automated verification passes;
human terminal visual acceptance remains separate.
Date: 2026-10-02.

## Objective and scope

Give reviewers recognizable Catppuccin, One Dark, Tokyo Night, and Gruvbox
palettes through the existing `--theme` flag and `t` picker. Preserve the calm
workspace: neutral headings, colored changes, and a restrained focus accent.
This extends [Interactive TUI Themes](SPEC-custom-themes.md); existing defaults,
configuration, persistence, and capability contracts continue to apply, with
the additive foreground/background token extension specified below.

The original release added four dark presets; the expanded catalog is described
below. Additional Catppuccin flavors, Nord, and Dracula can follow the same catalog contract.
No remote downloads, dependencies, arbitrary theme files, syntax highlighting,
automatic background detection, search UI, or full-screen live preview.

## Expanded catalog and grouped picker (2026-10-02)

The follow-up request adds eighteen presets, for twenty-six total. Tokyo Night
already exists and is retained once. CLI IDs normalize spacing/underscores;
Monokai corrects the requested spelling “monekai.” `ayu` means Ayu Light, while
`ayu-dark` means its dark variant. Melange uses its dark palette.

| Appearance | Additional IDs |
| --- | --- |
| Dark | `ayu-dark`, `gh-dark`, `horizon`, `material-dark`, `material-deep-ocean`, `melange`, `monokai`, `night-owl`, `poimandres`, `rose-pine`, `vscode-dark`, `wombat` |
| Light | `ayu`, `gh-light`, `gruvbox-light`, `one-light`, `rose-pine-dawn`, `vscode-light` |

Catalog metadata explicitly classifies appearance as Terminal, Dark, or Light.
Preserve all existing IDs, values, overrides, and the original catalog prefix.
Picker display order can differ from catalog order: Terminal stands alone,
followed by Dark and Light sections. Headings are nonselectable, keyboard
navigation follows visual order and skips headings, and mouse hits map only to
selectable entries. The selected group remains identified when its heading
scrolls out of view. Counts refer to selectable themes; compact terminals show
the selected group and theme without losing bounded geometry. Candidate sample
placement and mouse targets share the same grouped viewport geometry.

All named presets paint complete base palettes using the existing thirteen
roles. Description Markdown selects Glamour's light or dark baseline using
catalog appearance, not a name special case or inferred override luminance.
Foreground/background overrides continue to control the final canvas. Existing
syntax behavior remains; no syntax theme system or dependency is introduced.

Verification adds exact requested roster/appearance assertions, all-catalog
CLI/config/capability checks, light Markdown baseline/content/override checks,
and grouped navigation, heading-click, scrolled mouse, resize, preview placement,
and inherited-channel isolation regressions. Palette provenance is recorded in
[theme attribution](../THEME-ATTRIBUTION.md).

## Baseline before the original preset implementation

- `internal/theme/theme.go` defines eleven semantic tokens, four built-ins,
  validated colors, and immutable resolved palettes. `BuiltInNames()` supplies
  picker order and consumers that validate names.
- `internal/theme/config.go` owns validated global JSON and atomic persistence;
  `cmd/prui/theme.go` applies CLI precedence while retaining token overrides.
- `internal/tui/style.go` applies foreground colors to semantic lines. Context
  text inherits terminal foreground. Unfocused selection uses a background;
  focused selection uses bold/reverse. The app does not paint a canvas background.
- `theme_picker.go` renders all names; `picker_geometry.go` clips the modal.
  More names can hide the candidate and controls on short terminals. Its
  minimum modal width can also exceed the available width.
- `description_markdown.go` uses Glamour LightStyle only for `light`, otherwise
  DarkStyle. Theme switches already invalidate description caches per tab.

## Product contract

### Catalog and identity

Keep the first four entries and their existing eleven token values unchanged. Append these entries in
the order below. IDs are exact lowercase kebab-case and remain the persisted
and CLI representation. Show readable display names in the picker; do not add
implicit aliases or accept the misspelling `catpuccin`.

| ID | Display name | Description | Family variant |
| --- | --- | --- | --- |
| `catppuccin-mocha` | Catppuccin Mocha | Soft pastel accents | Mocha |
| `one-dark` | One Dark | Cool Atom-inspired accents | Original One Dark |
| `tokyo-night` | Tokyo Night | Blue and violet accents | Night, not Storm |
| `gruvbox-dark` | Gruvbox Dark | Warm earthy accents | Medium dark |

Use a small ordered internal definition catalog containing ID, display name,
description, and complete token values. Retain `BuiltInNames()` and `Resolve()`
contracts; return copies of catalog metadata, keeping palette maps private.
Existing four entries may keep their lowercase display names for compatibility.
Resolve and metadata lookup must use the same definitions so name lists,
descriptions, and palettes cannot drift. Do not introduce a plugin registry or
mutable global active theme.

### Palette mapping

These are proposed application mappings, not claims of exact editor-theme
reproduction. Use upstream colors for text/change/accent roles; selection and
secondary chrome are app-specific choices. Record source revision and required
license attribution when the implementation vendors palette values.

| Semantic token | Catppuccin Mocha | One Dark | Tokyo Night | Gruvbox Dark |
| --- | --- | --- | --- | --- |
| `foreground` | `#cdd6f4` | `#abb2bf` | `#c0caf5` | `#ebdbb2` |
| `background` | `#1e1e2e` | `#282c34` | `#1a1b26` | `#282828` |
| `title`, `fileHeader` | `#cdd6f4` | `#abb2bf` | `#c0caf5` | `#ebdbb2` |
| `hunk`, `metadata` | `#a6adc8` | `#abb2bf` | `#a9b1d6` | `#bdae93` |
| `added` | `#a6e3a1` | `#98c379` | `#9ece6a` | `#b8bb26` |
| `removed`, `unavailable` | `#f38ba8` | `#e06c75` | `#f7768e` | `#fb4934` |
| `warning` | `#f9e2af` | `#e5c07b` | `#e0af68` | `#fabd2f` |
| `selection` | `#313244` | `#3e4451` | `#292e42` | `#3c3836` |
| `focusedBorder` | `#89b4fa` | `#61afef` | `#7aa2f7` | `#83a598` |
| `border` | `#6c7086` | `#5c6370` | `#737aa2` | `#928374` |

### Full foreground/background theming

Approved scope: each new preset owns its base foreground and background across
review panes, split/unified diffs, lists, tabs, status/footer, descriptions,
comments/composer, help, confirmations, loading, errors, and blank screen cells.
Use the two base tokens above plus the existing semantic overrides. Separate
pane/modal surface tokens are unnecessary for this release: all use the base
background, with selection retaining its dedicated surface.

Add `foreground` and `background` to the fixed vocabulary (thirteen tokens).
For the existing `terminal`, `light`, `dark`, and `high-contrast` presets, both
new tokens default to `default`, preserving their current appearance. Existing
configuration needs no rewrite. Users may explicitly override either new token
on any preset; `default` means inherit that channel from the terminal. Keep all
existing eleven token values and explicit override provenance unchanged.

Apply base colors in one final presentation/composition boundary after logical
layout, escaping, clipping, and modal placement. Preserve explicit semantic
foregrounds and selection backgrounds; fill missing color channels, including
after embedded style resets. A final outer Lip Gloss wrapper alone is not an
acceptable assumption: prove nested resets, styled/uncolored spans, wide
characters, and blank cells remain correctly painted.

Paint only the app viewport with ordinary cell styling. Do not set
`tea.View.BackgroundColor` or `ForegroundColor`, which change terminal defaults
through OSC sequences; do not mutate terminal settings or request background
colors. Reuse existing Lip Gloss/ANSI/Ultraviolet dependencies where practical.
Any ANSI interpretation at this boundary is limited to trusted renderer output;
raw source is escaped first and never interpreted as styling.

When at least one base channel is explicit, paint/pad the interactive canvas
through its width and height. Geometry calculations and content caches remain
based on unpadded logical content; canvas padding is solely presentation. The
terminal/default-channel path preserves previous content bytes. Switching back
to an inherited palette or resizing must repaint cleared regions without stale
colors. Explicit selection backgrounds win over the base; focused selection
retains bold/reverse, reversing the resolved foreground/background pair. No
new diff backgrounds or focus behavior are introduced.

Capability handling remains unchanged. Colorless/NoTTY output skips canvas
painting and preserves the preexisting logical render exactly. A background
must never be used as a reason to force colors. Use the current profile writer
for true-color conversion/downsampling; inspect renderer output, not only
`View().Content`, for capability and reset behavior.

### Markdown without syntax themes

No new syntax palettes, Chroma theme selection, or language-color mapping.
Retain Glamour's current LightStyle/DarkStyle choice and existing syntax behavior.
Integrate description output with the shared base canvas: ordinary text and
otherwise unstyled whitespace receive base colors, and theme-managed Markdown
block backgrounds cannot leave foreign-colored rectangles. Adapt a local copy
of the existing Glamour style to inherit the app background; do not mutate its
shared style definitions. Existing explicit Markdown/syntax foreground colors
may remain, and are not advertised as exact family colors.

Pass the resolved theme (rather than only its name) where needed for background
integration, honoring base overrides. Keep current theme-change invalidation for
all description caches; caching must not reuse output from different resolved
base colors. Preserve content, wrapping, links, source escaping, and all
existing terminal safety rules.

### Picker behavior

- Retain `t`, `esc`, `j/k`, arrows, and Enter; clamp at either end without wrap.
- Open at the active theme. Keep distinct textual active/candidate markers.
- Render a height-bounded scrolling list. Selected candidate is always visible;
  show its position as `5/8` and retain apply/save/cancel hints.
- Compute geometry from available width/height, never the complete catalog
  height. Reuse the existing picker viewport helper where practical.
- At widths below 36 or heights below 8, use a compact candidate view. At
  height one show candidate plus position; at height two or more add controls.
  Clamp to available columns, including long names and wide text.
- Add a candidate sample when width is at least 60 and height at least 18:
  `Title`, `@@ hunk @@`, `+ added`, `- removed`, `warning`, and focused/unfocused
  border/selection samples styled with the candidate plus existing overrides,
  including a rectangular sample of its foreground and background.
  The sample is transient, uses synthetic text, and never changes active Model
  styles, writes configuration, or changes the review behind the modal.
- Hide the sample before sacrificing list rows or control hints on resize.
  Colorless samples retain words, signs, and focus attributes.
- Enter follows the existing save-before-apply path. A locked CLI selection
  saves for next launch; failure retains active theme and original config.
  Esc/`t` cancels without a write. Navigation alone never applies or saves.

### CLI and configuration

All existing interactive command forms accept new IDs, for example
`go run ./cmd/prui --theme catppuccin-mocha`. Existing validated token overrides
still win over base colors, including the two new optional keys. This is an
additive token extension in the existing JSON format, without migration; malformed files
retain the current safe fallback/read-only behavior. Invalid CLI names fail
before source/store/network setup. Plain output and `verify` remain independent
of personal theme configuration. Update help's enumerated names if necessary.

## Go TUI inspiration and primary sources

- [Lazygit theme configuration](https://github.com/jesseduffield/lazygit/blob/master/docs/Config.md):
  separate active/inactive borders and selected-row treatment. Adopt semantic
  roles while retaining our existing color-independent focus cues.
- [BubbleTint](https://github.com/lrstanley/bubbletint): named palettes and
  runtime tint selection for Go TUIs. Adopt a catalog and candidate sample;
  the current theme domain makes a new library unnecessary for four presets.
- [Catppuccin's Go palette](https://github.com/catppuccin/go/blob/main/mocha.go):
  authoritative Mocha role values; leave room for explicit flavor IDs later.
- [Original One Dark colors](https://github.com/atom/one-dark-syntax/blob/master/styles/colors.less):
  neutral chrome and blue focus, with green/red changes. Convert upstream HSL
  to hex where needed; selection is our UI mapping.
- [Tokyo Night Night](https://github.com/folke/tokyonight.nvim/blob/main/lua/tokyonight/colors/night.lua)
  and [shared Storm palette](https://github.com/folke/tokyonight.nvim/blob/main/lua/tokyonight/colors/storm.lua):
  Night derives shared colors and changes background values.
- [Gruvbox palette](https://github.com/morhetz/gruvbox/blob/master/colors/gruvbox.vim):
  medium dark surfaces, warm neutrals, and bright change colors.

## Engineering conventions and commands

Use Go 1.26.8, Bubble Tea v2, Lip Gloss v2, and standard-library tests already
in `go.mod`. New catalog code belongs in `internal/theme/presets.go` with tests
beside it; picker changes stay in `internal/tui`, CLI tests in `cmd/prui`.
Use typed tokens and validated colors, following existing code:

```go
resolved, err := theme.Resolve("catppuccin-mocha", overrides)
if err != nil {
    return err
}
m.SetTheme(resolved)
```

Format changed Go files with `gofmt -w <changed-files>` during implementation.
Focused checks: `go test ./internal/theme ./internal/tui ./cmd/prui -count=1`.
Final checks: `./scripts/verify.sh`, `golangci-lint run ./...`, and
`git diff --check`. Build: `go build ./...`.

## Testing and acceptance criteria

1. All catalog IDs resolve complete, isolated palettes; metadata order is stable,
   IDs are unique, every token is valid, and exact new token values are asserted.
   Existing palettes and explicit overrides retain their values/provenance.
2. Every new preset is selectable through CLI/config/picker. Tests exercise
   CLI precedence, locked selection, successful persistence, write failure,
   malformed config, and cancellation with synthetic fixtures.
3. Navigation reaches first/last entries, candidate remains visible through
   resize, and no view exceeds width/height. Cover 1-, 2-, 7-, 8-, 12-, 18-,
   and 24-row screens and widths 19, 20, 35, 36, 59, 60, 80, and 120.
4. Candidate samples preserve active palette/review/config; sample colors
   include overrides. Hiding/showing the sample cannot lose the candidate.
5. Existing all-theme capability tests cover TrueColor, ANSI256, ANSI16,
   Ascii/NO_COLOR, and NoTTY. ANSI-stripped styled text equals the unstyled
   equivalent. Plain bytes, clipping, escaped content, and focus cues hold.
6. Canvas tests inspect visible cells: base foreground/background cover every
   region, nested resets restore base colors, and semantic/selection styles win.
   Test empty/short content, wide runes, all screens/modal states, resize,
   theme switches, and one or both base channels overridden to `default`.
   Compare styled/unstyled logical text separately from presentation padding;
   canvas tests assert only additional blank cells, never modified content.
7. Existing palettes with inherited base channels retain baseline bytes.
   Colorless/plain paths do not gain padding or controls. Program-level capture
   verifies profile fallback and absence of theme-related OSC color commands.
8. Markdown receives the app background without foreign block rectangles;
   existing syntax foregrounds remain supported without new syntax themes.
   Base overrides and switches invalidate cached output across review tabs.
9. Human visual QA checks all screens on dark and light host terminals, across
   true-color, ANSI256, ANSI16, and NO_COLOR. Review contrast in the actual
   painted canvas, selections, and Markdown; update proposed mappings when
   necessary before accepting implementation.

## Boundaries and review decisions

- Always preserve source/session data, validated overrides, existing palette
  values, capability handling, and color-independent meaning.
- Ask first before new dependencies, additional tokens beyond the approved two,
  terminal-default mutation, changing focus/reverse semantics, or syntax themes.
- Never download/execute theme content, probe terminal backgrounds, overwrite
  invalid config, or style plain/artifact output from personal preferences.

Full foreground/background scope and implementation plan are accepted. The
canvas spike passed reset, color-channel, Unicode, selection, and hyperlink
regressions using existing dependencies. Human terminal visual QA remains a
separate release acceptance step.

Plan: [Theme presets implementation plan](../../tasks/theme-presets-plan.md).
