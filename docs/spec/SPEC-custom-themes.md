> Full theme presets update (2026-10-02): [Named TUI Theme Presets](SPEC-theme-presets.md)
> adds four family presets, optional `foreground`/`background` tokens, and
> capability-aware canvas painting. Existing four palettes retain inherited base
> channels. The expanded picker scrolls and offers an isolated candidate sample.
> This supersedes the older fixed four-theme/eleven-token contract below.

> Quiet workspace update (2026-09-25): the approved option A refresh changes
> terminal/light/dark default chrome colors to neutral text with restrained
> focus accents. Earlier promises below of preserving the exact original
> palette are historical; token names, overrides, capability handling and the
> high-contrast palette remain unchanged.

# Spec: Interactive TUI Themes

## Status

Implemented. This document records the approved scope and behavior.

## Objective

Let reviewers choose an intentional visual palette without making a terminal
theme, terminal color capability, or color itself a requirement for using
`prui`.

Today the interactive TUI maps semantic roles to mostly ANSI palette entries;
the user’s terminal determines what ANSI names such as `green` and `red` look
like. A few chrome colors are fixed 256-color indices. This feature preserves
that behavior as the default while offering built-in app-owned palettes and
one small, validated override file.

The feature is for interactive TUI users who need a readable light palette,
consistent review colors across terminals, or a stronger contrast option. It
includes the bounded keyboard theme picker specified below, but does not
introduce remote theme downloads, syntax-theme loading, or arbitrary style
injection.

## Decisions

### Theme selection

- The default is `terminal`, which exactly preserves the current semantic
  palette and therefore follows the terminal's ANSI palette where applicable.
- Built-in themes are `terminal`, `light`, `dark`, and `high-contrast`.
- There is deliberately no `auto` light/dark setting: terminal capability
  detection does not reliably reveal whether the user has a light or dark
  background. `terminal` is the safe automatic behavior.
- Theme selection is process startup configuration only. It is never stored in
  sessions and does not alter a frozen review, source data, progress, or
  comments.
- Pressing `t` in an interactive review opens a keyboard-only theme picker
  modal. Selecting a built-in persists it globally for future launches, but
  never alters review/session data.
- The app reads one optional configuration file at startup. An explicit
  `--theme NAME` CLI flag takes precedence for that invocation. A valid file
  setting takes precedence over the default. The default is `terminal`.

### Configuration location and format

Use a new app-owned configuration directory, never the reviewed checkout:

| Platform | File |
| --- | --- |
| macOS | `~/Library/Application Support/prui/theme.json` |
| Linux with absolute `XDG_CONFIG_HOME` | `$XDG_CONFIG_HOME/prui/theme.json` |
| Linux otherwise | `~/.config/prui/theme.json` |

Use JSON parsed by Go's standard library so the feature adds no dependency.
The file is optional; a missing file is not an error. It contains only theme
selection and token overrides:

```json
{
  "theme": "dark",
  "colors": {
    "selection": "#3b4252",
    "focusedBorder": "#88c0d0",
    "warning": "bright-yellow"
  }
}
```

`theme` must name a built-in theme. Each `colors` key is optional and overrides
the selected built-in token. Accepted values are CSS-style `#RRGGBB` hex,
ANSI color names (`red`, `bright-yellow`, `cyan`, etc.), `default`, and ANSI
palette indices `0` through `255` represented as strings. Invalid JSON,
unknown keys, unknown theme names, or invalid colors do not partially apply:
the app prints one escaped warning to stderr and uses the default `terminal`
theme. A malformed personal configuration must never prevent opening a review.

The configuration is global user preference, never review/session data. The
program reads it at startup and may atomically create or update it only after a
reviewer selects a built-in in the theme picker. The app never migrates,
downloads, or executes themes.

### Theme picker modal

`t` is available on interactive review and picker screens when no text editor,
comment-action menu, loading modal, or destructive confirmation owns input. It
opens a centered, clipped modal containing the four built-in names and a short
description of each. The current resolved base theme is marked with `›`; this
marker, the names, and the footer commands remain sufficient with all color
disabled.

- `j`/`k` and up/down move the candidate. `enter` immediately applies that
  built-in and persists it as the global `theme` selection for future launches.
  `esc` closes without changing the applied or persisted theme. `t` also closes
  the modal without changing it.
- The modal does not offer individual token edits. Any valid `colors` overrides
  from `theme.json` remain layered over the candidate base theme, so a user can
  compare bases while retaining their personal token tuning.
- A picker selection writes only the `theme` field. It preserves every valid
  `colors` override and formats the resulting JSON deterministically. An
  explicit `--theme` still wins for the current invocation, but a picker choice
  updates the global preference used at the next launch.
- The write creates the app configuration directory with private permissions
  when absent, writes a private temporary sibling, fsyncs it, atomically
  replaces `theme.json`, and syncs the directory. A write error leaves the
  active theme unchanged, shows an escaped non-modal error, and leaves the
  previous configuration intact.
- A malformed or unknown-key configuration remains read-only: the picker can
  preview themes, but `enter` refuses to overwrite the file and directs the
  reviewer to repair it. This prevents a UI preference action from destroying
  manually maintained configuration.
- The picker must fit tiny terminals using the app's existing modal clipping
  behavior. It must never cover an active text editor or consume keys while an
  editor/action confirmation is active.

### Semantic token contract

Themes set semantic display roles rather than raw line classes or arbitrary
ANSI sequences:

| Token | Current role |
| --- | --- |
| `title` | titles, tabs, healthy footer |
| `fileHeader` | Git file metadata/header lines |
| `hunk` | hunk locations |
| `added` | added diff lines |
| `removed` | removed diff lines |
| `metadata` | metadata cards and diagnostics |
| `warning` | warnings and warning footer |
| `unavailable` | unavailable/error cards |
| `selection` | unfocused selected-row background |
| `focusedBorder` | focused pane outline |
| `border` | unfocused pane outline |

`selection` controls only the background color. Selection retains bold and the
textual `\u203a` marker; focused selection retains reverse video. Themes cannot
remove, change, or add text attributes, labels, glyphs, dimensions, borders, or
keyboard behavior. This keeps color presentation-only and prevents a config
from changing layout or meaning.

Built-ins are fully specified using explicit true-color values, except
`terminal`, which retains the existing ANSI and 256-color mapping. `light` and
`dark` prioritize clear addition/removal/hunk differentiation against their
respective backgrounds. `high-contrast` uses strong foreground/background
separation and avoids red-versus-green as the only differentiator; the existing
textual labels and markers remain authoritative in every theme.

### Capability and plain-output behavior

- Theme selection never enables color. Existing terminal-profile behavior
  remains authoritative: `NO_COLOR`, `TERM=dumb`, non-TTY output, and `--plain`
  suppress colors exactly as today; `CLICOLOR_FORCE` follows the existing
  capability rules.
- Color profiles continue to downsample true-color theme values to ANSI-256 or
  ANSI-16 terminals. No probe is performed.
- `--plain` never reads the theme configuration or palette and emits no terminal
  control bytes. It remains byte-identical regardless of theme selection.
- Stripping styles from an interactive render produces the same unstyled text
  for every built-in and valid override configuration.

### CLI behavior

Add `--theme NAME` to interactive entry points (`current`, `open`, `resume`,
and `prs`). `--theme` is accepted for plain invocations solely for consistent
argument handling, but has no visible effect. `verify` remains fixed to its
existing deterministic terminal setup and rejects `--theme`; its screenshots
must not be affected by a developer's local theme configuration.

`--theme` validation follows the same failure behavior as an invalid config:
print an escaped, actionable error and exit 1 before opening storage, Git, or
GitHub. The error names the invalid value and lists the built-ins; it never
prints user-controlled configuration bytes.

## Tech Stack

- Go 1.26.8+
- Bubble Tea v2.0.9 and Lip Gloss v2.0.2
- Existing `colorprofile` capability downsampling
- Go standard-library `encoding/json` for configuration parsing

## Commands

```sh
go test ./internal/tui -run 'Test(Theme|Color|Style|Snapshot)' -count=1
go test ./cmd/prui -run 'Test(Options|Theme)' -count=1
go test -race -count=1 ./...
go vet ./...
go build ./...
git diff --check
```

## Project Structure

```text
internal/tui/theme.go          theme tokens, built-ins, validation, palette construction
internal/tui/style.go          semantic line classes and style application
internal/tui/theme_test.go     palette/token, override, and colorless invariants
internal/tui/theme_picker.go   transient picker state, key handling, and modal render
cmd/prui/options.go       --theme parsing and command validation
cmd/prui/theme.go         config discovery/loading and startup warnings
cmd/prui/*_test.go        CLI precedence, malformed config, and verify isolation
README.md                      theme selection, schema, paths, and accessibility behavior
CONSTRAINTS.md                 theme-specific preservation of the color contract
```

## Code Style

Use named semantic tokens and immutable palette values per program instance;
avoid package-global mutable configuration. Theme parsing returns a typed result
and a safe, fixed-format diagnostic. Rendering only receives the resolved
palette.

```go
type Theme struct {
    Name   string
    Colors map[ThemeToken]color.Color
}

func ResolveTheme(selection string, overrides map[ThemeToken]string) (Theme, error) {
    // Validate all input, then construct a complete immutable semantic palette.
}
```

No raw config strings or ANSI escapes reach rendering. All style construction
continues after escaping, clipping, and scroll projection.

## Testing Strategy

- Unit-test every built-in theme resolves every token and each accepted color
  syntax; reject unknown tokens, malformed JSON, partial hex, out-of-range
  indices, and unrecognized names.
- Test precedence: CLI selection over file selection over `terminal` default;
  file overrides apply only after a valid named built-in resolves.
- Test configuration discovery for macOS and Linux/XDG using injected home,
  GOOS/path inputs; never use the real user configuration in tests.
- Re-run the existing color-profile matrix for every built-in theme and an
  override palette. ANSI stripping must reproduce exactly the unstyled render,
  and no rendered line may exceed terminal width.
- Test `NO_COLOR`, `TERM=dumb`, non-TTY, and `--plain` against each theme.
  `--plain` must not read config and must emit no terminal control bytes.
- Test malformed config produces one escaped startup warning and a usable
  `terminal` palette; invalid CLI `--theme` exits before any app side effect.
- Test `t` opens a color-independent modal; navigation, enter, escape, and
  repeated `t` have the specified apply/cancel behavior; enter atomically
  persists only the selected theme while retaining token overrides; errors and
  malformed files leave both the active theme and original file unchanged; and
  it is unavailable while higher-priority input states own keys.
- Snapshot only the `terminal` theme to preserve current deterministic
  baselines; add focused semantic-token assertions for the other built-ins
  rather than color-code-dependent screen snapshots.
- Manually inspect `light`, `dark`, and `high-contrast` in true-color, ANSI-16,
  and `NO_COLOR` terminals before release.

## Boundaries

- Always: preserve current terminal-capability handling, plain-output bytes,
  escaped rendering, clipping/width behavior, and non-color state cues; use
  semantic tokens; test all profiles.
- Ask first: adding a config parser dependency; migrating a user config;
  changing terminal-detection semantics; adding a remote/preset download or
  individual token editor to the picker.
- Never: read a theme from the repository; execute or interpret configuration
  as code/ANSI; weaken `NO_COLOR`/`--plain` behavior; make status, selection,
  or diff meaning depend on a palette.

## Success Criteria

- A reviewer can run `prui --theme light`, `dark`, `high-contrast`, or
  `terminal`; `terminal` remains the default and visually preserves the current
  palette.
- A valid optional `theme.json` can select a built-in and override only named
  semantic colors, with CLI precedence.
- `t` opens an accessible theme picker modal in interactive screens. Selecting
  a built-in atomically persists the global theme selection while preserving
  valid token overrides and never changes review/session state.
- All invalid theme input fails safely and predictably: malformed file warns
  and falls back; invalid CLI argument exits before normal work begins.
- Every color-disabled/profile/plain invariant already guaranteed by the app
  still passes for every theme.
- `verify` and its artifacts are independent of the developer's personal theme
  file.
- Documentation gives exact paths, schema, valid names, precedence, and the
  fact that themes supplement rather than replace terminal accessibility.

## Open Questions

None. The only deliberate product tradeoff is JSON over YAML/TOML to avoid a
new dependency and keep parsing/security behavior small. Revisit only if a
user-facing configuration experience proves too unfriendly.
