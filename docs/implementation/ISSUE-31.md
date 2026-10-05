# Issue 31: suggested changes

Originally stacked on issue 29 (`5dd00d1`, PR #45 on #41). Restacked onto
reviewed PR #45 `ebc8d93ba5ac6e0581c3b98a320c5c54c3bd39fb`, then actual
main merge `bdedc5476e0e837c6409445f5c6c396e16b62934`, retaining only issue 31
commits. Subsequently restacked onto main `7f64f19ffa26ac5bd5d032c3ff668d4221fb6f84`
with merged published-comment/thread actions (PR #44), preserving both contracts,
keyboard routes, status/preview rendering and unsent-work shutdown protection.
Combined regressions verify uncertain published edits cannot alter private draft
generations, queued range/file targets or immutable suggestion attempts; restart
retains the exact attempted payload and performs no extra writes. A suggestion
application alone now keeps its frozen comparison during freshness refresh.
Also integrated main `f0a3dea37f58fccd96cf6e51b8f2b80b82412b9c` with PR #46
pinned-source navigation and local search. Combined keyboard tests prove full
OLD/NEW and expanded-only context search hits stay read-only even after a
canonical target was selected; canonical searched patch rows retain exact
suggestion source/coordinates. Search and navigation keys cannot consume
replacement-editor input or application confirmation/discard actions.
Also integrated main `d7597f4c4c94a3bd3fb3d21f08790d31b9413fa5` with PR #47
incremental review and comparison-reset maintenance. Reset clears only active
suggestion application/confirmation/scroll state before loading the new
comparison’s independent drafts. Prepared and uncertain original applications,
range/file drafts and exact payloads remain unchanged in their private record
and recover on reopening the original comparison. The regression failed before
the narrow fix and also verifies parent search cancellation/navigation reset and
read-only incremental controls without remote writes.
The restack preserves the canonical conversation writer and complete
conversation docs, the shared themed modal surface, and the read-only selected
commit net-diff restriction. Combined tests exercise conversation refresh and
general-editor isolation with ranged suggestions, real legacy v1 → range v2 →
replacement v3 store restart, and suggestion modal theme/confirmation semantics. The immutable comparison,
raw old/new context coordinates, private draft payload version 2 targets (suggestion editor/apply payloads use version 3), and explicit
remote-write boundaries remain authoritative.

## API decision and evidence (2026-10-04)

GitHub's documented [review-comment REST endpoints](https://docs.github.com/en/rest/pulls/comments)
create/read/update/delete comments and replies; they do not expose an apply
suggestion operation. The documented [commit mutation](https://docs.github.com/en/graphql/reference/commits#createcommitonbranch)
creates a commit and advances the named branch, with required `expectedHeadOid`.
That documented mechanism is used instead of undocumented web endpoints.
It applies the replacement as a remote commit; it does not claim to set GitHub's
native suggestion-applied UI metadata or resolve the review thread.

GitHub [documents write access for applying suggestions](https://docs.github.com/en/pull-requests/how-tos/review-pull-requests/incorporating-feedback-in-your-pull-request).
The implementation requires canonical `permissions.push` on the actual head
repository, including forks. Upstream maintainer permission alone does not grant
this mutation's head-repository write access; that case is visibly unsupported.
GitHub still enforces credential scopes, rules and branch protections at mutation.

## Compose and preview

In a focused PR diff, Ctrl+S captures the selected right-side line/range and opens
a replacement editor initialized from raw captured source. Ctrl+V and Ctrl+O
retain issue 29's range/side controls. Replacement input is distinct from Markdown.
Enter posts generated suggestion Markdown; Ctrl+P queues it. Empty replacement
means deletion. Fences grow when replacement source contains backticks.

Existing and draft suggestions show Before/After, escaped for terminals. Pending
suggestions reopen as replacement editors. Replacement, original text, full target
coordinates and mode are persisted privately and recover offline. New historical,
left-side and file-level suggestions are unsupported; existing historical replies
remain supported. Offset/multiple suggestion blocks are explicitly unsupported.

## Apply and uncertainty

Enter on an existing inline comment opens its actions; Ctrl+A prepares application.
Preparation refreshes the exact canonical comment ID/body/current anchor, PR pins,
head repository/ref, permission and committed file. Only ordinary mode 100644 UTF-8
text files at most 512 KiB are eligible. Executable files, symlinks, binaries and
unavailable/oversized source are refused. Selected source must match the exact
captured range; no search or automatic reanchoring is used.

The apply modal displays repository, branch, path/range and Before/After. Enter
opens confirmation; a second Enter commits. Before dispatch the private draft
stores the entire exact GraphQL mutation bytes, expected head, prepared full file,
comment body and source coordinates. Confirmation repeats canonical comment,
PR/ref/permission/source checks and sends the retained payload on stdin exactly
once. `expectedHeadOid` protects concurrent pushes/applications after preflight.
Canonical returned commit/ref OIDs and URL are validated. The frozen snapshot
stays unchanged; the reviewer explicitly opens a new comparison.

Failed/canceled/interrupted attempts retain their exact payload and block retry.
Ctrl+R reads remote state only. A changed head with an exact whole-file result is
conservatively treated as already applied, requiring explicit draft discard.
Changed content, missing source, offline state, and unchanged heads retain the
uncertainty block: force-push-back and delayed requests make an unchanged head
insufficient proof of absence. There is no idempotency guarantee or auto retry.
Ctrl+D explicitly discards retained work. The same uncertainty state recovers
after restart; later replacement edits never change the attempted payload.

## Verification

All regressions use synthetic fixtures and fake Runner/client boundaries. No live
GitHub/provider write or suggestion application is performed. Tests cover selected
ranges, context/source provenance, nested fences/deletions/EOF, replacement recovery,
confirmation, canonical edit/delete, pushes, fork permissions, unsupported modes,
conflicts, response validation and conservative outcome reconciliation.
Human terminal QA is unverified. Final full gates and exact-SHA independent review
are recorded in the stacked PR.
