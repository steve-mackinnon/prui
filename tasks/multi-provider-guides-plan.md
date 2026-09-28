# Implementation Plan: Configurable Guide Models with Charm Fantasy

Spec: [SPEC-MULTI-PROVIDER-GUIDES.md](../docs/spec/SPEC-MULTI-PROVIDER-GUIDES.md).
Task checklist: [multi-provider-guides-todo.md](multi-provider-guides-todo.md).
These task-specific files preserve the unrelated unfinished work in
`tasks/plan.md` and `tasks/todo.md`.

## Overview

Let a reviewer select one guide provider and model in an XDG-style global
`config.json`, while keeping credentials in environment variables and the
existing `g` plus consent boundary. Charm Fantasy handles provider protocols;
the application continues to own bounded input, request transport limits,
validation, consent, cache semantics, and immutable sessions.

## Architecture decisions

- Pin a Go-1.26-compatible Fantasy release only after a local fixture proves
  exact OpenAI Responses parity. Keep the current OpenAI adapter available
  until the replacement passes the same safety tests.
- Resolve one immutable, secret-free selection for the interactive invocation.
  `internal/guideconfig` owns file paths, parsing, endpoint normalization,
  model identity, and credential-variable names. The command layer reads the
  actual credential only after confirmed `g` and the offline check.
- Keep `guide.Analyzer` as the stable application interface. A shared Fantasy
  analyzer owns the prompt, schema, and result mapping; provider constructors
  stay small and separate. The transport wrapper enforces byte limits and
  redirect policy for every SDK request.
- Reuse the existing one-slot-per-comparison guide cache. An optional selection
  fingerprint in the bundle gates automatic reuse without a SQLite migration.
  Saved sessions remain readable when selection changes.
- No automatic fallback to another provider, permissive object mode, agent
  loop, or retry. Provider capabilities are proved with local request fixtures.

## Dependency graph

```text
G-01 Fantasy feasibility and pinned dependency
  ├─ G-02 global config resolver ───────────┬─ G-07 consent view
  ├─ G-03 bounded HTTP transport ─┐         ├─ G-08 cache reuse gate
  └───────────────────────────────┴─ G-04 OpenAI Fantasy parity
                                    ├─ G-05A Anthropic
                                    ├─ G-05B Google
                                    └─ G-06 custom compatible endpoint
G-02 + G-04 + G-05A + G-05B + G-06 + G-07 + G-08 ── G-09 CLI integration
G-09 ── G-10 documentation ── G-11 old-adapter cleanup ── G-12 final verification
```

G-01 is a stop/go checkpoint. G-02 and G-03 can start independently after its
contract is fixed. G-05A, G-05B, and G-06 can be delegated concurrently after G-04
defines the provider-constructor seam; they must own disjoint files. G-07 and
G-08 can also run independently once G-02 fixes the selection type. G-09 and
G-10 are integration steps with one owner for shared wiring and documentation.
Do not run agents concurrently on `go.mod`, `go.sum`, `cmd/prui/wiring.go`,
`internal/guide/fantasy.go`, or `CONSTRAINTS.md`.

## Delivery checkpoints

1. **Library gate after G-01:** exact version, Go compatibility, no retry,
   `store:false`, strict schema, and client injection are observed locally.
   If any mandatory behavior is unavailable, update the spec and plan before
   implementation; retain the current OpenAI adapter meanwhile.
2. **Core gate after G-04:** the OpenAI Fantasy path passes focused request,
   privacy, response, cancellation, and error tests while the old path remains
   the CLI default.
3. **Feature gate after G-09:** each configured provider has one tested
   interactive generation path; consent and cache selection match the request;
   no-config OpenAI behavior remains usable.
4. **Release gate after G-12:** full project verification, code review, and
   documentation agree. No live provider or GitHub call is part of automated
   verification; a human terminal walkthrough remains an explicit acceptance
   item.

## Risks and mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Fantasy v0.41.3 has large transitive dependencies or an incompatible API | High | Pin exact version and compile in G-01 before building on it. |
| SDK retries, redirects, or request expansion send source unexpectedly | High | Capture outbound requests through a bounded client; assert count, size, destination and headers for each provider. |
| Structured output differs among models/endpoints | High | Preserve local `guide.Validate`; use explicit provider mode and fail closed on unsupported models. |
| Existing cached guide ignores new provider selection | High | Compare a non-secret selection fingerprint before reuse; test switching selection and legacy bundles. |
| Config discovery changes theme or offline behavior | Medium | Use a separate `config.json`; keep `theme.json` and offline boundary unchanged. |
| Several agents edit shared files concurrently | Medium | Establish types first, assign disjoint provider files, integrate centrally, and serialize shared-file edits. |

## Verification commands

Focused commands are listed with each task. Final gate:

```sh
go vet ./...
go test -race -count=1 ./...
go build ./...
./scripts/verify.sh
git diff --check
```

Use only synthetic input, fake credentials, and local HTTP servers in tests.
No release or live acceptance action is implied by completing this plan.

## Open questions

None block task breakdown. G-01 must determine whether the pinned Fantasy
release satisfies every required transport and OpenAI request contract. A
failed gate triggers a spec/plan revision rather than an unreviewed relaxation.
