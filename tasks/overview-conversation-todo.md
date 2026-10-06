# Overview conversation tasks

- [x] Overview document, discoverable D shortcut, activity/detail focus, mouse routing.
- [x] Exact current and historical code context; explicit unavailable states.
- [x] Exact comment/reply/range/file navigation to Files, reply actions, and return.
- [x] Preserve editor ownership, drafts, selected identity, and refresh position.
- [x] Focused deterministic tests and real Bubble Tea navigation/reply journey.
- [x] Review final screens, documentation, and complete repository verification.

Verification: `./scripts/verify.sh` passed (vet, full race suite including PTY smoke,
and build). After the final reply-attribution and filter-focus fixes, affected
Overview, mouse, filter, screen, and recovery tests also passed with `-race`.
The freshness-guard mutation was detected by the extended-target tests.
