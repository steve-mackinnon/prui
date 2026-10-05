# Diff style composition implementation plan

Approved spec: [consistent diff style composition](../docs/spec/SPEC-diff-style-composition.md).
User authorized implementation on 2026-10-05.

1. Reproduce both screenshots with complete rendered-cell assertions.
2. Separate structural patch markers from source at the shared presentation
   boundary. Resolve missing source foreground channels before row styling.
   Use background-only source row classes so intentionally inherited source
   foregrounds remain inherited; preserve existing semantic status colors.
3. Apply word/active-search emphasis to trusted rendered cells, avoiding nested
   underline wrappers that process SGR bytes as individual runes. Search match
   fragments choose their existing contrasting base foreground before syntax
   overrides; fill only absent match backgrounds.
4. Exercise themes/capabilities, word/search/selection composition, geometry,
   prefix handling, links, escaping, unchanged cached rows and review identities.
5. Run focused tests, the full verify script, review the diff and record terminal
   QA limits. Update the syntax fallback documentation.

Dependencies are sequential. No delegation, additional dependencies, capture
changes or external writes. Cell transforms operate only on clipped visible text.
