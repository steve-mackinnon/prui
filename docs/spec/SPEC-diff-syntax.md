# Diff syntax highlighting

Capture Chroma token categories from complete pinned old/new blobs while the
isolated Git view exists. Persist only spans for patch lines, as optional JSON
metadata; never retain additional source text. Existing snapshots remain readable.
Highlighting is presentation-only and does not change inventory identities,
patch bytes, comment anchors, completeness, or plain output.

Use bounded per-file and per-inventory work and metadata. Unsupported languages,
invalid text, and exhausted budgets fall back to existing diff styling. Legacy
snapshots may highlight independently reconstructed old/new hunk fragments.

Render categories using the active theme, preserve diff markers, and carry spans
through escaping, wrapping, horizontal scrolling, split projection and commit
number columns. Cached source rows stay free of ANSI and theme-dependent data.

Implementation order: capture and token tests; optional persistence; rendering
and geometry tests; documentation and complete verification. Validate multiline
context outside hunks, independent old/new state, offline round trips, terminal
escaping, Unicode, wrapping, selection, and unchanged plain text. Run focused Go
tests, scripts/verify.sh, and git diff --check.

## Implemented limits and palette

Full-file lexing accepts up to 256 KiB and 16,000 spans per side, checks a
100 ms cooperative deadline between tokens, and respects cancellation. Chroma
regexp matches have their own timeout; the cooperative deadline is not a hard
preemption guarantee. Each inventory admits 4 MiB of input, 32,000 retained spans,
and a 500 ms cumulative lexing budget. Empty capture results are explicit so the
renderer does not retry budget-exhausted files as legacy fragments.

Keywords use the theme's focused accent, strings its added color, numbers its
warning color, comments its metadata color, names its title color, and operators
its file-header color. Explicit backgrounds blend with added/removed colors;
inherited terminal backgrounds remain inherited. No new theme configuration
keys or native build dependencies are needed.
