# PR list stack positions and two-line rows

Each list entry reserves two terminal lines: a single unwrapped title using the
available list width, then metadata (stack position when known, author, viewer
review marker, checks, and target branch). The split preview keeps compact check
symbols; the full-width list keeps check labels. Overflow titles retain the
existing selected-title carousel. At one available line, show the title only.
Selection, mouse hit testing and scrolling operate on whole entries. The preview
and footer retain their existing layout and focus behavior.

Number linear stacks base-first (`1/4`, `2/4`, etc.) by matching an open PR's base
branch and base repository against another loaded open PR's head branch and head
repository. Reuse the existing bounded GraphQL list request, adding head branch
and repository fields. No additional requests or changes to review pins occur.
Only loaded open PRs count (the existing list limit is 100). Omit ordinals for
singletons, unknown heads, ambiguous matches, branching components and cycles.
A fork's identically named head branch must not match a base repository branch.
Filtering the switcher retains the complete loaded-list stack positions.

Verify branch normalization and dependency edge cases with synthetic GitHub
responses; verify row widths, title animation, both-line mouse selection,
scrolled selection, resize, previews, and intentional screen baselines.
