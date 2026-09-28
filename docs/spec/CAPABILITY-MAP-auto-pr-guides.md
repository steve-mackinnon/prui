# Capability Map: Automatic PR-List Guides

| Module id | Responsibility | Depends on |
| --- | --- | --- |
| guide-cache | Persist and retrieve validated successful guide bundles for one immutable PR comparison. | — |
| pr-list-guide-loading | Resolve a cached guide or generate one after an interactive PR-list selection, with cancellable loading feedback. | guide-cache |

Build order: `guide-cache` → `pr-list-guide-loading`

The cache identity includes normalized repository, PR number, base SHA, and head
SHA. A head SHA alone is not enough: an unchanged PR head can produce a
different review diff after its base advances.
