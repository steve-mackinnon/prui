# Capability Map: PR Context Views

The review workspace will expose first-class, PR-scoped context views without
changing source review, progress, or comment semantics.

| Module id | Responsibility | Depends on |
|---|---|---|
| pr-view-tabs | Stable interactive `Diff`, `Description`, and `Commits` view selection in one active review | — |
| pr-description | Safely capture, freeze, store, and render the PR description | pr-view-tabs |
| pr-commits | Safely capture, freeze, store, and browse PR commits with a selected-commit diff | pr-view-tabs |

Build order: `pr-view-tabs` → `pr-description` → `pr-commits`.

The user approved this map on 2026-09-20. The shared tab mechanism ships first;
Description is the first populated context view. Commits deliberately stays a
separate module so pagination, ordering, and its rendering model can be tested
without coupling it to description behavior.

The 2026-09-30 revision of `SPEC-pr-commits.md` expands this module from
a list-only view to a two-pane commit browser at the user's request. It keeps
the existing module boundary and was approved for implementation that day.
