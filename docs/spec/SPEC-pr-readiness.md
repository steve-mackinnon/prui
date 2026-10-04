# PR readiness (#33)

`C` in an open PR review opens a read-only readiness screen from Description,
Files, Guide or Commits. Each opening requests remote evidence. `r`, `c`,
or `ctrl+r` refresh only readiness. Escape returns to pinned code. Arrow keys,
j/k, PageUp/PageDown, u/d, Home/End and the mouse wheel scroll the wrapped view.
The footer adapts to narrow terminals. State has text labels independent of color.
Human terminal accessibility/usability QA is unverified.

## Remote evidence and boundaries

`source.ReadinessReader.ReadReadiness(ctx, Identity)` is a separate read-only
service. `Readiness` carries exact observed head/base SHAs, base branch, observation
time, per-check SHA, individual status/conclusion/app identity/detail URL,
required-review count, current reviewer decisions with original commit SHA,
authoritative GitHub review decision, mergeability/merge state, PR state/draft,
viewer repository permission when available, completeness flags and explicit
unknowns/policies/blockers. It never changes the pinned comparison to match a live
head. The screen prints both heads and labels differences. Old-head approvals are
shown as historical decisions; they are not counted as current approvals locally.
GitHub's review decision remains authoritative for dismissal, required approvers,
and branch policy. Comment-only reviews do not erase a prior reviewer decision;
a later dismissal does. Pending unpublished reviews are excluded.

The reader observes PR metadata before and after retrieval. A mismatched graph
revision, changed head/base/base branch/state/draft or failed final metadata read
invalidates head verification. This is observed evidence, not an atomic GitHub
transaction. Requirements, reviews and check states can change even on a fixed
head. No background polling or automatic retry is performed. A refresh explicitly
marks retained prior evidence stale until the new generation succeeds. Late,
cancelled, wrong-session and wrong-identity results cannot replace newer evidence.
Snapshots are tab-local memory only. Replacement/close cancels pending reads.
Offline refusal happens before client/credential access. Offline code/guide/session
reading is preserved; remote readiness is unavailable. Plain output does not fetch
or render readiness. No SQLite schema, progress, drafts, guide cache or source
material is changed. Readiness is never assembled into AI guide upload material.

## Requirements and completeness

Read both classic commit statuses (latest value per context, newest first) and
check runs (`filter=latest`) at the observed head; aggregate rollups never prove
readiness. List each current run independently. App-constrained contexts must match
the GitHub App ID; a classic status cannot fulfill an app-constrained check. An
unconstrained context can match either API, but every matching current result must
succeed. Missing required contexts receive explicit unknown rows. Only `success`
is success. Pending runs are pending; failure/error/timeouts/action-required/startup
failure are failures. Skipped, cancelled, neutral and unknown conclusions remain
unknown. Optional failures do not become required blockers.

Classic branch protection and effective branch rules are combined. The branch
rules endpoint includes inherited organization rules. Missing permissions, unknown
branch protection, malformed/omitted critical policy fields, unavailable effective
rules or unsupported rule types preserve unknown requirements. Known required
contexts stay required; unmatched contexts stay unknown until *all* requirement
sources are available. They never become optional as a fallback. Required counts
combine using the maximum across classic and inherited policies. Strict/up-to-date,
code-owner, last-push, thread-resolution, signatures, linear-history, locked branches, push restrictions and unsupported
rules remain visible policies requiring GitHub inspection; this version does not
claim to evaluate those policies locally. It may conservatively show unknown or
policy blockers even when GitHub would permit a merge.

Each statuses/checks/reviews/rules connection is limited to five pages of 100.
Exactly filling page five remains incomplete, since absence of a sixth page is
not proved. Check run total counts must agree with retrieved pages; missing or
changing totals remain incomplete. Duplicate check identities across pages,
wrong-head rows, malformed rows or a failed connection never mean empty success.
Each subprocess response is capped at 1 MiB and the entire read has a 60-second
context deadline. At most 25 subprocess requests are issued (two metadata reads,
branch/protection, 20 list pages, one graph query; protection is conditional).
Provider errors do not expose stderr or credential values. Rich output is linked
using bounded HTTPS URLs without credentials or controls; third-party check URLs
are displayed but never fetched. PR/check/review links can be copied into a browser.

## Future merge preflight contract (#34)

`Readiness.Ready(expectedHead)` requires exact head agreement, verified metadata,
complete lists, known requirements, OPEN/non-draft state, MERGEABLE/CLEAN GitHub
facts, all required checks strictly successful, required reviews APPROVED, and no
unknowns or policy blockers. Optional greens do not substitute for required checks.
It reports only conservative evidence, never permission/authorization to merge.
The screen says "No observed merge blockers at the verified live head" and prints
viewer permission separately; it does not offer a merge action.

A future explicit writer must fetch a *new* snapshot immediately before its write,
verify the intended expected head and authorization, and use GitHub's SHA-bound
merge request. A snapshot from this screen/cache must never authorize a later
write, even if the SHA is unchanged. #33 has no mutation endpoint or write test.

## Primary API evidence

- [Check runs](https://docs.github.com/en/rest/checks/runs): per-reference
  check runs, pagination, app identity, head SHA and provider detail links.
- [Commit statuses](https://docs.github.com/en/rest/commits/statuses): statuses
  are reverse chronological; statuses require separate pagination from checks.
- [Branch protection](https://docs.github.com/en/rest/branches/branch-protection):
  required contexts/app IDs, required review counts and additional policy controls.
- [Effective branch rules](https://docs.github.com/en/rest/repos/rules#get-rules-for-a-branch):
  effective rules include rulesets defined at repository and organization levels.
- [PR reviews](https://docs.github.com/en/rest/pulls/reviews): submitted state,
  author and commit identity; pending reviews are not published decisions.
- [GraphQL PullRequest](https://docs.github.com/en/graphql/reference/objects#pullrequest):
  head/base OIDs, reviewDecision, mergeable, mergeStateStatus, state and isDraft.

All automated fixtures are synthetic. Live provider/GitHub calls and mutations are
not used by the tests. Exact tested/reviewed SHA and verification evidence are
recorded on the stacked PR after the final implementation commit.
