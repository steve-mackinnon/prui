# Implementation Plan: Named Themes with Full Canvas Colors

Status: Approved and implemented on 2026-10-02. Automated verification is
recorded in the checklist; human terminal visual acceptance remains separate.
Syntax themes are excluded.

Contract: [SPEC-theme-presets.md](../docs/spec/SPEC-theme-presets.md).
Checklist: [theme-presets-todo.md](theme-presets-todo.md).
Feature-specific files preserve unrelated unfinished `plan.md`/`todo.md` work.

## Architecture

Retain the validated theme domain, per-model styles, atomic persistence, and
color-profile writer. Extend the fixed vocabulary with base foreground and
background; existing presets inherit both by default. An ordered catalog owns
four new complete palettes and picker metadata.

Apply base channels centrally to the final interactive canvas, preserving
explicit semantic styles and selection surfaces. Do not change terminal default
colors through View color fields/OSC. Existing layout/content/cache calculations
remain independent of canvas padding. Colorless/default paths keep old bytes.
Reuse existing dependencies; first prove that the composition approach handles
nested ANSI resets and blank/wide cells before integrating it across screens.

Markdown keeps its existing syntax foreground behavior. Adapt a per-render copy
of the Glamour style so block backgrounds inherit the app canvas; pass resolved
base colors and retain all-tab theme cache invalidation. No syntax theme system.

## Dependency graph and ordered slices

```text
canvas feasibility spike -> base tokens/config -> catalog/presets
           |                       |                   |
           +-----------------------+-> final canvas ---+
                                           |
                                Markdown background integration
                                           |
                             bounded picker -> candidate sample
                                           |
                           integration / documentation / terminal QA
```

1. Verify a reset-safe, capability-aware canvas composition approach with a
   synthetic rendering fixture. Reject a naive outer wrapper if resets leak.
2. Add the two base tokens and backward-compatible config validation.
3. Add the ordered catalog and four named palettes with base colors.
4. Integrate canvas painting centrally in View, including resize/theme changes.
5. Integrate description backgrounds and resolved-color cache invalidation.
6. Make the enlarged theme picker scroll and fit small terminals.
7. Add a candidate sample showing the palette's base and semantic colors.
8. Verify CLI/config/persistence and renderer-level terminal capabilities.
9. Update references and complete full automated and human visual checks.

Each task is a focused slice with regression coverage; work is sequential
because the rendering contract is shared. No parallel agent work is needed.

## Checkpoints

- After 1: prove visible-cell backgrounds/foregrounds, nested resets, clipping,
  and color-profile behavior without terminal-default mutation. Revisit the
  rendering approach if this fails before expanding scope.
- After 2–4: old theme baselines stay stable; new presets paint all screens,
  blank cells, and selections; switching/resize clears stale colors.
- After 5–7: Markdown has no foreign block surfaces; candidate sample is isolated;
  picker dimensions and override handling pass focused tests.
- After 8–9: full gates pass; record human visual acceptance separately.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Nested ANSI resets expose terminal colors | Visible-cell fixture first; apply defaults at final composition boundary. |
| Padding changes layout or plain bytes | Keep logical text separate; paint only interactive color-capable canvas. |
| Terminal default setters bypass profile handling | Use cell colors, leave View color fields unset; capture program output. |
| Markdown emits conflicting block backgrounds | Local style adaptation and background-specific regression fixtures. |
| Cached description retains previous override | Use resolved base palette and retain all-tab cache invalidation. |
| Preview changes active palette/config | Separate candidate resolution and saver/model-state assertions. |
| Low-color terminals collapse contrast | Downsample through existing writer and inspect true-color/256/16 profiles. |
| New token config surprises older binaries | No automatic rewrite; document older versions reject new optional keys. |

## Deferred work

Additional variants/families, separate pane/modal surfaces, diff backgrounds,
live full-screen preview, syntax theme selection, and terminal background
detection. No new dependencies are planned.

## Approved follow-up: expanded catalog and Dark/Light grouping

1. Catalog owner adds eighteen palettes with explicit appearance metadata,
   upstream attribution, and exact roster/palette validation; original eight
   entries remain unchanged. Provide `Theme.IsLight()` for Markdown.
2. Picker owner projects catalog entries into Terminal/Dark/Light visual rows,
   skips headings during navigation, and shares geometry with mouse targets and
   the independent sample overlay. Root integrates the sample bounds helper.
3. Root selects the Markdown baseline from appearance; integration owner extends
   CLI/config and light Markdown tests. Existing all-theme capability loops cover
   the expanded catalog automatically.
4. Root updates references, performs independent review, and runs full vet/race/
   build/lint checks. Reinstall from the worktree for the user's ongoing testing.

Owners have disjoint files; palette and picker contracts are established before
consumer integration. Catalog source values and picker geometry are reviewed
before final verification. User authorized this extension directly.
