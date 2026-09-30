# Implementation Plan: Commit Discussions

Spec: [SPEC-pr-commit-discussions.md](../docs/spec/SPEC-pr-commit-discussions.md).
Checklist: [commit-discussions-todo.md](commit-discussions-todo.md).
Status: Drafted 2026-09-30; implementation has not started.

Use feature-specific files following the commit-browser convention. Existing
`tasks/plan.md` and `tasks/todo.md` contain unrelated work and remain intact.
The user requested specification and planning together; this package is the
concrete result for review, not approval to implement or make external writes.

## Dependency Order and Slices

1. CD-01 establishes API contracts and historical posting eligibility evidence.
2. CD-02–04 retain bounded historical discussions and expose a read-only overlay.
3. CD-05–07 show exact commit/main-diff placement and navigation with shared counts.
4. CD-08–10 enable supported historical comment creation after the write gate.
5. CD-11–12 complete lifecycle journeys, contract amendments, and verification.

Build the read path first so comments cannot disappear when writes arrive.
Maintain existing functionality at each increment; avoid a source-interface
cutover that temporarily removes current-head comments. Transitional adapters
are acceptable but must be removed before completion.

## Architecture Decisions

- Shared ephemeral discussion overlay; no session or database changes.
- Read records have optional anchors; write targets remain strictly validated.
- GraphQL thread state provides outdated/resolved truth; existing REST writes
  retain GH authentication and stdin JSON boundaries.
- Original SHA determines commit-thread ownership. Exact current anchor determines
  main-diff placement. Unplaceable records stay in a PR-wide overlay.
- Immediate historical posting, with provenance distinct from head-based writes;
  no historical entry can enter pending PR review submission.
- Full refresh is bounded and generation-scoped; no navigation fetch or polling.

## Checkpoints

A (CD-02–04): historical/outdated/unplaceable discussions readable, complete and
partial results distinguishable, existing comments/actions still work.

B (CD-05–07): exact inline placement, counts and jump/return work in wide/narrow
layouts with no main review/progress mutation or per-commit request.

C (CD-08–10): write eligibility evidence recorded; supported SHA targets validated;
confirmed outdated creation remains visible; unknown outcomes never auto-retry.

D (CD-11–12): focused and full gates pass, documentation reflects actual support,
and human terminal evidence is recorded separately from automated results.

## Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| First-parent coordinates differ from GitHub PR anchor semantics | Investigate first; unsupported cases remain read-only; no silent retarget |
| Two pagination levels hide replies or old threads | Explicit nested cursors and total budgets; visible incomplete states |
| Immediately outdated creation is mistaken for failure | Normalize readable write responses independently of writable anchors |
| Force-pushed history absent from capped snapshot | Honest uncaptured label, snippet/link fallback, no guessed membership |
| Live threads placed onto stale frozen diff | Metadata verification and freshness-qualified exact matching |
| Late refresh contaminates replacement review | Identity plus generation gating and cancellation |
| Historical drafts enter head review batch | Distinct provenance, composer guard, submission validation |
| Contract docs contradict new behavior | Narrow explicit amendments in CD-12; do not weaken unrelated rules |

## Open Technical Gates

CD-01 resolves API field contracts, keybinding conflicts, safe link-launch reuse,
and supported historical target cases. Live comment experiments require separate
explicit authorization; synthetic verification does not claim live interoperability.
No new dependencies, SQL migration, or automatic source recapture are planned.
