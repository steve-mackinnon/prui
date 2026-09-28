# Implementation Plan: Interactive TUI Themes

## Overview

Implement the approved `docs/spec/SPEC-custom-themes.md` as five small, ordered slices:
a pure semantic theme domain, safe global configuration, per-model palette
application, CLI startup wiring, and a persistent keyboard-only picker. The
implementation must retain the existing terminal-profile and plain-output
contracts exactly.

The detailed work checklist is [THEME-TASKS.md](THEME-TASKS.md).

## Architecture Decisions

- Define an internal, typed theme domain with a fixed semantic-token set. It
  owns built-ins, validation, color parsing, config decoding, and atomic
  persistence; rendering only consumes a fully resolved palette.
- Replace the mutable package-global presentation palette with a palette owned
  by each `tui.Model`. This permits a picker to change one live program without
  leaking state between models/tests and preserves the existing pure plain
  renderer.
- Keep `terminal` as a compatibility palette that reproduces today's ANSI and
  indexed color choices. Other built-ins use explicit true-color values and let
  the established Color Profile writer downsample them.
- Parse theme configuration before TUI startup. A malformed file is a
  non-fatal diagnostic and uses `terminal`; an invalid CLI name is fatal before
  store, Git, or GitHub setup. `verify` does not load personal theme config.
- `t` selection atomically persists only the built-in name, preserving valid
  token overrides. The TUI updates its model only after persistence succeeds.

## Dependency Graph

```text
semantic tokens + built-ins + parser
             |
             +-- config discovery / validation / atomic persistence
             |              |
             |              +-- CLI selection and startup resolution
             |                             |
             +-- model-owned palette -------+-- keyboard theme picker
                                                |
                                                +-- README, constraints, full verification
```

## Implementation Order

1. Establish the pure theme model and tests first, including compatibility with
   the current `terminal` palette.
2. Add config discovery and durable selection persistence behind that model.
3. Make style application model-owned and prove all existing color/plain
   invariants across built-ins.
4. Wire validated CLI/config selection into interactive startup while isolating
   `verify` and plain behavior.
5. Add the `t` picker, then document and run complete regression checks.

Tasks 1 and 2 can be developed in parallel only after agreeing on the typed
theme/config boundary. Tasks 3–5 are sequential because they share the active
model and command startup paths.

## Verification Checkpoints

- After Tasks 1–2: pure theme/config tests, race check for the new package, and
  manual inspection of generated config fixture permissions/replacement paths.
- After Tasks 3–4: focused TUI/CLI tests plus the existing color-profile matrix
  proves unchanged text and plain bytes.
- After Task 5: full `go vet ./...`, `go test -race -count=1 ./...`, `go build
  ./...`, `git diff --check`, and manual true-color/ANSI-16/NO_COLOR review.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| A shared global palette leaks picker state across models/tests | High | Resolve a complete immutable palette and attach it to each `tui.Model`. |
| A picker write truncates custom token overrides or malformed user config | High | Decode/validate whole file before an atomic replacement; reject malformed/unknown-key files without writing. |
| A new theme causes text/width differences in low-capability terminals | High | Reuse the profile writer and assert ANSI-stripped output equals the existing unstyled render at every profile. |
| Local config changes verification artifacts | Medium | Keep `verify` on a deterministic compatibility palette and bypass config resolution. |
| `t` conflicts with editing/modal input | Medium | Route picker activation only after existing higher-priority editor, action-menu, confirmation, and loading handling. |
| JSON formatting makes hand-maintained config surprising | Low | Preserve valid overrides and document canonical formatting on picker persistence. |

## Open Questions

None. JSON remains the approved dependency-free format; theme persistence is
global and triggered only by a successful picker selection.
