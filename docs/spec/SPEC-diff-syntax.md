# Diff syntax highlighting

Capture Chroma token categories from complete pinned old/new blobs while the
isolated Git view exists. Persist only spans for patch lines, as optional JSON
metadata; never retain additional source text. Existing snapshots remain readable.
Highlighting is presentation-only and does not change inventory identities,
patch bytes, comment anchors, completeness, or plain output.

Use bounded per-file and per-inventory work and metadata. Unsupported languages,
invalid text, and exhausted budgets fall back to normal source foregrounds with
colored diff markers and existing row backgrounds. Legacy
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
its file-header color. Changed rows blend approximately 8% added/removed color
with the theme background (GitHub Dark retains its established 25% fill) and fill
the available row width, including lines with no syntax tokens. Inherited
backgrounds use the theme family’s light/dark baseline for changed rows only. No new theme configuration
keys or native build dependencies are needed.

Source foregrounds and changed-word/search attributes follow the
[diff style composition contract](SPEC-diff-style-composition.md). Token-free
lines and fragments use the same normal foreground as ordinary text between
tokens; missing foreground channels are resolved before row styling. Structural
patch markers retain added/removed colors independently of source colors.
Changed-word and active-search emphasis is applied to trusted rendered cells so
nested SGR resets cannot split an identifier's color or corrupt ANSI sequences.
