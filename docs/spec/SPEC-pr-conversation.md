# Complete live PR conversation (#27)

The Discussions screen becomes a chronological PR conversation when the GitHub
conversation reader is available. It combines general PR comments, submitted
reviews (including empty-body decisions), and each inline-thread comment/reply.
Events preserve stable identities, authors (or `[deleted]`), and UTC RFC3339
timestamps. Equal timestamps use stable IDs as the ordering tie-breaker; missing
inline timestamps are labeled unavailable. Original inline context remains
accessible with `o`. General activity has no diff anchor and never enters pinned
source, session serialization, guides, or draft storage.

`D` opens conversation; `c` explicitly refreshes. There is no background polling.
Refresh replaces observed events by identity and retains navigation and detail
scroll. Late results are scoped to their originating tab, session and read
generation. Confirmed general creations remain readable until an authoritative
refresh observes their identity; an absent confirmed creation makes the view
partial even if the remote page retrieval otherwise completed.

`n` opens a memory-only general PR comment. `r` on a general-comment detail opens
a new general comment seeded with an @mention. GitHub has no nested reply API for
general comments, so the editor explicitly labels this behavior. Inline replies
remain separate diff comment actions. Enter explicitly posts one request; Escape
discards; Shift+Enter inserts a newline. General comments cannot be queued into a
pending review. Offline/unsupported clients cannot open this editor.

Every write requires online operation and an immediate exact comparison freshness
preflight (repository identities plus base/head SHAs). JSON bodies travel only on
stdin to `repos/{owner}/{repo}/issues/{number}/comments`. Responses and errors are
validated/sanitized. No automatic retry occurs. An uncertain delivery retains the
editable draft and an immutable attempted body plus the previously observed event
IDs. Partial, failed, or stale refreshes cannot authorize retry. A complete,
verified refresh compares new general events to that original attempted body;
a match blocks another post and asks the reviewer to inspect it before starting
another comment. A nonmatch permits an intentional Enter retry. Matching is
conservative because GitHub offers no client idempotency key for this endpoint.

General comments and reviews each fetch at most five pages of 100 records,
sharing a 4 MiB response budget. Every gh response remains bounded to 1 MiB.
The entire composed read uses a 60-second deadline. Existing thread bounds remain
500 threads, 100 replies per thread, 2,000 inline comments and 4 MiB under that
same deadline. Partial retrieval never means no activity. Unavailable and stale
reads retain the prior live snapshot; offline mode performs no network access.

API basis: [GitHub general PR comments](https://docs.github.com/en/rest/issues/comments)
and [submitted reviews](https://docs.github.com/en/rest/pulls/reviews).

Validation uses fake gh responses, synthetic application fixtures, key/message
scenarios and a real Bubble Tea program journey. No test contacts GitHub.
Terminal usability/accessibility still requires human assessment.
