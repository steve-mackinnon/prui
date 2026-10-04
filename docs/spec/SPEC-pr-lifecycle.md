# PR lifecycle (#34)

Lifecycle controls are tab-owned live state opened from the readiness page with `l`.
Each action shows repository/PR, expected live head and method, then requires Enter.
Pinned source, session progress, pending reviews and analysis privacy remain unchanged.
Offline and plain modes expose no writers.

Fetch repository merge methods, auto-merge policy, base merge queue, PR state,
viewer capabilities, effective rules and current readiness. Unknown permissions,
unsupported rules and incomplete retrieval remain visible. Supported GitHub policy
is enforced by GitHub; a readiness snapshot is evidence, never atomic authorization.
Immediately before each write, re-read and compare PR ID/head/base/state/draft and
re-evaluate permissions, method, readiness and policy. Intersect repository methods with effective `allowed_merge_methods` and linear
history restrictions. Queue auto-merge remains available when GitHub permits it;
confirmation shows the queue policy method and sends no ignored method override.
Never request admin bypass,
queue jump, branch updates or protection overrides. Merge, auto-merge enable and
queue enqueue include `expectedHeadOid`. Other lifecycle APIs lack an atomic head
condition; label that limitation and retain server authorization.

Write once via GraphQL JSON stdin; never replay a write after transport uncertainty.
Always fetch canonical state after a write attempt. A complete refresh reconciles
observable target state; ambiguous outcomes remain locked until explicitly resolved
by the reviewer outside this attempt. Repeated Enter and stale asynchronous results
cannot duplicate or retarget writes. Successful mutations never rewrite frozen code.

Synthetic transports cover every action, permission/method/rule denial, changed
confirmation head/state/draft, canonical refresh, uncertainty and repeated keys.
Broad gates: vet, full race, build, scripts/verify.sh, lint with zero findings, diff
checks; coordinate shared resource reservation. Human terminal/live mutation QA is
unverified. All real roadmap PR merges belong to the user.

API sources: https://docs.github.com/en/graphql/reference/pulls and
https://docs.github.com/en/graphql/reference/repos .

Known richer policy: queue, linear history, signatures, non-fast-forward and
required deployments, plus validated status-check and pull-request requirements.
Unrecognized rule types remain unavailable. Direct merging retains concrete
check/review failures even when GitHub reports CLEAN; supported richer-policy
fulfillment uses CLEAN only with complete evidence and validated rule shapes.
