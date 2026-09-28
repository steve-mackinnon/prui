# Capability Map: Pull Request Review Comments

| Module id | Responsibility | Depends on |
| --- | --- | --- |
| comment-contract | Typed, validated request and subprocess-input boundary for one GitHub review comment | — |
| diff-line-targeting | Preserve diff-line provenance and let a reviewer select one commentable line | comment-contract |
| comment-composer | Compose, cancel, and submit one Markdown comment from the selected target | comment-contract, diff-line-targeting |
| comment-delivery | Wire the explicit GitHub write, mandatory freshness preflight, documentation, and end-to-end regressions | comment-contract, comment-composer |

Build order: `comment-contract` → (`diff-line-targeting`) → `comment-composer` → `comment-delivery`.

`comment-contract` is deliberately separate from the existing read-only
`source.GitHub` interface. This prevents comment creation from becoming an
implicit capability of normal metadata and pinning operations.
