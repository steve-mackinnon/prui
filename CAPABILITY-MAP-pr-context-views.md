# Capability Map: PR Context Views

The review workspace will expose first-class, PR-scoped context views without
changing source review, progress, or comment semantics.

| Module id | Responsibility | Depends on |
|---|---|---|
| pr-view-tabs | Stable interactive `Changes`, `Description`, and `Commits` view selection, including per-open-review state | — |
| pr-description | Safely capture, freeze, store, and render the PR description | pr-view-tabs |
| pr-commits | Safely capture, freeze, store, and render the PR commit list | pr-view-tabs |

Build order: `pr-view-tabs` → `pr-description` → `pr-commits`.

The user approved this map on 2026-09-20. The shared tab mechanism ships first;
Description is the first populated context view. Commits deliberately stays a
separate module so pagination, ordering, and its rendering model can be tested
without coupling it to description behavior.
