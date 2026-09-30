# Implementation Plan: Pull Request Commit Browser

Implements `docs/spec/SPEC-pr-commits.md`, approved for implementation by the user
on 2026-09-30. Existing incomplete plans remain intact. This feature's checklist
is `tasks/commit-browser-todo.md`.

## Approach

Define the shared immutable `internal/commits` bundle first. Source/diff capture,
session persistence, and TUI behavior then proceed in separate owned packages
against that contract. The coordinator integrates capture into review opening,
including retry/cancellation, before validating the complete application.

Dependency order: shared types → capture/storage/UI → review integration →
offline/program regressions → documentation, independent review, full gate.

Agent ownership: capture owns source/inventory/commits; persistence owns session;
UI owns tui; coordinator owns review integration, documentation, and final checks.
Shared files/types require coordination before modification. Each task uses
focused failing tests and verified increments; integration tests establish the
end-to-end path rather than only testing each implementation in isolation.

## Risks

- Optional capture must not erase the main diff: unavailable data remains explicit.
- Frozen membership must match the PR revision: retry the entire opening once.
- Stored material must be bounded: aggregate capture and encoded payload limits.
- Commit rendering must not inherit PR comment targets or progress controls.
- Canonical JSON compatibility must preserve old source bytes and digests.

No dependencies, SQL migrations, network writes, or checkout mutations planned.
