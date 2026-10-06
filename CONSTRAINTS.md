# Phases 1 To 3 Contract

- Raw source comes only from pinned committed objects, never working files or model output.
- Compare the unique, ancestry-verified merge base to head. Reject ambiguous or unresolved comparisons.
- Account for every raw change and every unit exactly once. Limits produce explicit incomplete/unavailable states, never an empty-success substitute.
- Never execute reviewed code, hooks, filters, textconv, external diff, credential helpers, submodule commands, or repository-selected network helpers.
- Never mutate the checkout, index, refs, configuration, or object files. Temporary isolated storage is tool-owned and removed on completion/cancellation.
- No telemetry or persisted credentials. Source retrieval, opening, resuming, refresh, guide generation, and background work never write reviews. GitHub writes require an explicit reviewer action in the interactive TUI: an immediate line comment, comment action, or confirmed PR review submission. Phases 1 to 3 have no model integration and no source upload; Phase 4 analysis supersedes the no-upload part of this rule under the PR-list behavior below.
- Default limits: 10,000 entries; 50 MiB materialized content; 1 MiB per blob; 100,000 diff lines; 60 seconds per operation; 512 MiB temporary fetch storage.
- Fetch storage is monitored, not an exact network-byte cap. Terminate the process group on exhaustion; polling permits overshoot. No automatic resource-limit retries.
- Verification: `go vet ./...`, `go test -race -count=1 ./...`, `go build ./...`. Synthetic fixtures only; never weaken tests or suppress checks to pass.
- Terminal usability requires human verification. Passing render tests does not prove accessibility.
- Terminal styling is presentation-only: semantic styles wrap already-escaped display lines after scrolling and clipping; optional base foreground/background colors compose trusted styled cells at the final viewport boundary. Styling never edits logical text, wording, labels, ordering, or width, and carries no meaning absent from labels. Explicit base colors may add only blank viewport padding. Removing styles and that presentation-only padding reproduces the uncolored logical render exactly; colorless profiles skip canvas painting and retain previous bytes. Never mutate terminal default colors or interpret raw source as styling.
- Color capability is detected, never probed: honor the terminal profile and `NO_COLOR`/`CLICOLOR`/`CLICOLOR_FORCE`/`TERM=dumb`. Colorless profiles render today's text. Built-in theme selection and validated global `theme.json` overrides affect only the interactive semantic palette and remain subject to that capability handling. The `t` picker may atomically persist only a selected built-in to the app-owned global file; it never changes review/session data and never overwrites invalid configuration. `--plain` output neither reads theme configuration nor emits terminal control sequences.
- Persist frozen actual patches and metadata independently of Git object lifetime; never rewrite a comparison/plan in place or automatically carry completion to a new version.
- Reopening checks freshness explicitly or labels it unknown. Failed checks never imply current revisions; stale sessions remain readable. No hidden polling.
- Store only app-owned local data outside the reviewed checkout, with private permissions, durable SQLite transactions, integrity/reference validation, and transactional generation checks. Preserve corrupt/unsupported records; reject stale writers. SQLite serializes writers; no process-lifetime store lock is held.
- Session deletion transactionally removes its state and only unreferenced payloads, not unrelated data, external service records, backups, or guaranteed forensic traces. Raw stored source is not encrypted and may contain sensitive content.

# Phase 4 Analysis Contract

- Guide analysis requires explicit `g` selection and confirmation in the interactive review. `g` opens a modal over the review with the provider, editable model ID, sanitized recipient, and upload warning visible; Enter confirms the displayed valid selection and Escape closes the modal without changing the review. PR-list selection may reuse an existing generated guide but never starts generation. Direct `open`, `resume`, `--plain`, offline mode, and `verify` never request analysis automatically. No CLI analysis/model/exclusion flags are accepted. `resume --offline` rejects every network operation before client or credential access; frozen sessions and stored guides remain readable. This rule covers `internal/guide` only; the `internal/analysis` provider path has its own consent contract and no CLI entry point.
- A guide cache lookup is keyed by normalized repository, PR number, base SHA, and head SHA, and reuse additionally requires the selected provider, model, endpoint, prompt version, and schema to match a non-secret bundle fingerprint. A legacy guide without that fingerprint is eligible only under the unchanged default OpenAI selection. It may return only a locally stored, structurally valid successful generated bundle; a cache hit never contacts the provider. Unavailable, failed, corrupt, mismatched, or invalid bundles are never cache hits and do not suppress a future PR-list retry. Cache storage remains app-owned, local, private, atomic, and outside the reviewed checkout.
- Only the assembled `guide.Input` may leave the machine: pinned patches and pinned-tree evidence that pass the privacy policy a second time at the upload boundary. Excluded paths, credential-like content, and budget-exhausted units are withheld, and withheld path names are not sent either.
- Each confirmed guide action makes at most one non-streaming model request with structured `prui_guides` output, a request body limited to 2 MiB, and a response body limited to 4 MiB. OpenAI uses Responses with `store: false`; no retention claim is made for the other providers. Redirects are not followed, so a credential cannot reach another host. Plaintext HTTP endpoints are rejected except explicit loopback endpoints. No automatic provider fallback, schema repair, or text-mode retry may send source in another request.
- Guide configuration comes from the user's global XDG `config.json`, never from the reviewed checkout. The absence of that file preserves OpenAI, `gpt-6.1-sol`, `OPENAI_API_KEY`, and the optional legacy `OPENAI_BASE_URL` origin override. The modal offers native OpenAI, Anthropic, and Google providers with a nonempty value in their designated key environment variable; OpenAI-compatible requires a configured endpoint and key, except an explicitly keyless loopback endpoint. A private app-owned `guide-last.json` beside `config.json` remembers only the last confirmed provider/model pair across launches. A valid, usable remembered pair takes precedence over the configured initial choice; neither file may contain a credential value or reviewed source. The same confirmed selection controls consent, request, provenance, and cache matching. Invalid configuration stops before upload. Credentials are never persisted, logged, or placed in an error, reason, or rendered string. Provider text is sanitized and truncated before it becomes durable session content.
- PR-list analysis is a cancellable action after the selected comparison is loaded and pinned; `g` can manually retry it in an interactive review. Missing credentials or invalid client configuration preserve the current session and report an error. Transport errors, non-2xx status, refusal, provider deadline, oversize payload, or unusable structured output produce an `analysis_unavailable` bundle in a derived session. User cancellation preserves the current session without creating a derived session. Analysis availability does not affect raw inventory completeness or exit status.
- Generated guides are immutable snapshot data with provider, model, prompt version, schema name, input digest, evidence IDs, limits, the withheld ledger, and a non-secret selection fingerprint. Every completed analysis attempt creates a new derived session with empty progress; it never rewrites a stored bundle or transfers the original reading progress.
- Guides never own files. Sections reference validated unit IDs; `UnitFiles` remains the sole authority for reading progress, and every unit appears in guide navigation exactly once via the synthesized ungrouped guide.
- Model output is interpretation, not source truth, approval, security findings, or complete architectural documentation. The CLI uses the model ID confirmed in the guide modal, with `gpt-6.1-sol` as the OpenAI default when no guide configuration file exists; model IDs have no local allowlist.
- Provider tests use a local `httptest` endpoint and a fake credential only. No test may contact a real provider or use a live key.

# Phase 5 Line Comment Contract

- Phase 5's initial GitHub write is one reviewer-initiated, line-anchored pull-request comment from the interactive TUI. Later review submission and comment actions extend this boundary as described below. There are no automatic comments, timeline comments, or retries. The private draft contract below supersedes the initial memory-only draft behavior.
- A comment target is derived from immutable raw patch bytes: additions and context use the new path, `RIGHT`, and new-file line; deletions use the old path, `LEFT`, and old-file line. Headers, hunk markers, no-newline markers, non-text units, unavailable units, and invalid UTF-8 paths are never commentable. Terminal escaping, styling, cursor position, and scroll offsets never determine the target.
- The editor is transient, tab-owned inline state. `enter` focuses the diff from the list and, only in a focused diff on a commentable line, opens it directly below that line. Within it, `enter` explicitly submits, `shift+enter` adds a newline, `backspace`/`delete` remove the preceding/following rune, and `esc` discards. Unsent comment text is durably saved under the private draft contract below; it is never logged, rendered in errors, or placed in process arguments.
- Read-only comment overlays are bounded, per-tab, and ephemeral. They load only online and on explicit `c` refresh; every rendered item must exactly match the frozen head SHA, path, side, and line. Remote bodies, logins, and paths are escaped; comments never enter sessions, logs, or plain output.
- Every submit first reads current GitHub metadata and requires base repository, head repository, base SHA, and head SHA to exactly match the frozen comparison. Any mismatch or failed preflight makes no write and directs the reviewer to open a new comparison. The request retains the frozen head SHA; it never retargets a moved pull request and does not retry after the preflight. A post-preflight race may leave GitHub to mark a successfully created comment outdated.
- Submission validates the repository, pull number, frozen 40-character head SHA, UTF-8 path/body, positive line, and `LEFT`/`RIGHT` side. It sends JSON only through `gh api --method POST --input -` stdin to GitHub's pull-request review-comment endpoint; the authenticated credential must have Pull requests write permission. Responses and raw command diagnostics are untrusted and are not stored or rendered.
- `--plain`, redirected/non-interactive operation, and `resume --offline` never expose or invoke comment posting. Offline refusal occurs before GitHub client or credential access. Cancellation makes no claim about whether an in-flight GitHub write completed.
- Synthetic tests prove target mapping, keyboard/composer behavior, exact stdin request construction without body arguments, validation, offline refusal, preflight mismatch without a commenter call, and success/error/cancel behavior. No test contacts GitHub or uses a live credential.

# Phase 5a Inline Comment Action Contract

- Anchored overlay comments are transient diff selections, distinct from line targets. `enter` on a target opens only a new-comment editor; `enter` on a comment opens only a local action menu. Escape never writes.
- Viewer identity, replies, deletion, and reactions use narrow bounded GitHub interfaces. Reply bodies are UTF-8 and JSON stdin only; stable comment IDs, repository/PR identities, and a finite documented reaction set are validated before invoking `gh`.
- Reaction rendering is emoji-first only for UTF-8 locales, with the bounded GitHub reaction tokens as the fallback for explicit non-UTF-8 locales. Emoji glyph/font support is not probed, and reaction meaning never depends on color.
- Every action remains online-only and freshness-preflighted. Delete additionally fetches the authenticated viewer and is permitted only when that login exactly equals the loaded comment author, then requires explicit final confirmation.
- Action results carry origin tab, stable comment ID, and generation. Stale results and results for another tab are ignored. Canonical replies and reactions, menus, viewer identity, and deletion state are overlay-only: never sessions, logs, or plain output. Unsent reply bodies and their frozen target/root identity follow the private draft contract.

# Pull Request Review Submission Contract

- `R` is a persistent, visible action in every interactive PR context view. It opens a tab-owned modal over the current review with Comment, Approve, and Request changes, a Markdown summary, pending comment list, and a separate confirmation step. Comment and Request changes require a nonblank summary; Approve may omit one. Escape closes the modal without discarding its draft or changing the underlying review position; tiny terminals use a compact layout. Discarding unsent drafts on quit also requires a modal confirmation.
- In the inline editor, `enter` still posts one comment immediately. `ctrl+p` queues or updates a local pending comment without a network request. Pending comments are displayed at their immutable diff target and in the review form; the reviewer can edit or remove them before submission. Pending bodies and the summary follow the private draft contract and survive exit; a new comparison has independent drafts.
- Review submission sends one JSON stdin request through `gh api` to the pull-request reviews endpoint with the frozen head SHA, event, summary, and validated line targets. No draft text appears in process arguments, logs, immutable sessions, guides, or plain output. A successful request clears the local queue and refreshes the read-only inline overlay.
- Before the write, the application checks current repository identities and base/head SHAs against the frozen comparison. Mismatch, offline mode, invalid input, or unavailable GitHub capability makes no write. There is no automatic retry after an uncertain outcome; a failed request retains local drafts for inspection.
- Queued comments and asynchronous results belong to their originating review tab. Switching tabs cannot retarget a request. A new comparison displays its own drafts; old drafts remain recoverable with the old saved comparison and are never retargeted. Source retrieval, guide generation, and progress updates cannot submit a review.


# Commit Discussion Extension

- The commit-discussions capability supersedes Phase 5's current-head-only read
  overlay limit: bounded GraphQL review threads retain original/current anchors,
  authoritative outdated/resolved status and fallback snippets/links. All data
  remain memory-only, escaped and absent from sessions, logs, guides and plain
  output. Offline refusal and explicit refresh/no-polling rules still apply.
- Current anchors may render only after metadata before/after retrieval matches
  the frozen PR pins, and only at an exact raw diff target. Original anchors may
  render only at their own captured commit SHA/path/side/line; unplaceable threads
  remain discoverable. Outdated never implies resolved; uncaptured never implies
  removed. Partial retrieval cannot imply complete counts or no discussions.
- Commit composers post immediately and cannot enter the head-based pending
  review queue. Delivery validates captured SHA membership/raw patch provenance
  plus full freshness. Captured head targets must also match the frozen PR
  diff. Historical RIGHT additions/context require a complete captured
  single-parent A/M diff with unchanged path and exact raw membership, backed by
  recorded live API evidence. Historical LEFT/rename/copy/root/merge targets remain
  disabled.
  No silent retarget, automatic retry, or additional permissions. Unsent historical commit comments follow the private draft contract.
- A confirmed creation stays readable when its current anchor is missing.
  Uncertain delivery retains the draft and requests refresh before intentional
  retry. Existing main-diff actions retain their author/freshness protections.

# Complete PR Conversation Contract

- Overview and discussion detail render bodies through the local Markdown/HTML
  display adapter. HTML comments and unused reference definitions stay hidden;
  details show summaries and expand in place in Overview. Preserve code literals
  and neutralize controls, decoded entities and unsafe link destinations. Never
  execute HTML or fetch images. Presentation cannot rewrite raw bodies, drafts,
  suggestions, submitted payloads or source targets. The tab-owned render cache
  is bounded to 128 entries and 4 MiB, including raw bodies and rendered lines.

- Issue #27 supersedes Phase 5's restriction against timeline comments only for
  an explicit general PR comment or @mention reply in the interactive conversation
  editor. Local general drafts follow the private draft contract; pending review semantics remain separate.
- Conversation is ephemeral live data: general comments, submitted decisions and
  bodies, and individual inline activity retain actor, timestamp and stable ID.
  Never place general events into pinned source, sessions, guides or draft storage.
- General writes use JSON stdin only, an exact freshness preflight and one explicit
  Enter action. Escape never writes; offline refusal precedes client access.
  General replies are labeled new @mention PR comments, distinct from inline replies.
- Retrieval is paginated and bounded; partial, stale, unavailable and offline states
  remain visible. Refresh preserves selection/detail navigation, deduplicates by
  stable identity and retains confirmed creations until observed remotely.
- Unknown delivery retains editable text and the immutable attempted body/observed
  identities. Only complete verified refresh can reconcile; an observed matching
  attempt blocks retry. No automatic retries or write-on-refresh behavior.

# Private Review Draft Contract (issue #26)

- Inline editors (including captured historical commit editors), reply editors, pending line comments, review decisions, and summaries are saved on each changed interactive event in an optional mutable `review_drafts` SQLite table. The table is independent of immutable snapshots and guides, keyed by normalized repository, PR number, base/head SHAs, and base/head repository identities. It uses the existing private app-owned storage, durable SQLite writes, bounded UTF-8 payloads (1 MiB per comparison), a versioned payload, and compare-and-swap generations. Corrupt/unsupported data and stale writers are rejected without overwriting stored work. Failed saves retain the in-memory text and block submissions.
- Recovery makes no network write and displays a local recovery modal, including reply text without a remote overlay. Switching PR tabs preserves drafts. Opening a changed comparison never copies or retargets old drafts; reopen its saved comparison to recover the old work. Guide generation and source snapshots never receive drafts.
- Before dispatching any persisted comment, reply, or review submission, save an immutable copy of the exact attempted request. Restart, cancellation, and failed responses retain that record. Editing and resubmission remain blocked until an explicit `ctrl+r` outcome check succeeds. The check uses only read-only GitHub endpoints and compares authenticated author, exact body, frozen anchor/root parent or review event, head SHA, and pending comments. It examines at most ten pages of 100 records per history; malformed, failed, or capped history never proves absence. A matching request blocks retry and asks for explicit discard; identical prior submissions conservatively count as matches. No match permits a later explicit submit with the normal freshness preflight; reconciliation never retries automatically. GitHub does not provide an idempotency guarantee for delayed in-flight writes.
- `esc` in an inline/reply editor explicitly discards that editor; `d` removes the selected pending comment, and `ctrl+d` in the review/recovery modal discards that comparison's drafts. In the quit confirmation, `s` saves and quits, Enter discards drafts in open tabs and quits, and Escape resumes. Interrupt/crash preserves the last committed draft and attempted request. Deleting the last saved session for a comparison also deletes its drafts; deleting one of several sessions for it retains them. Local deletion is not remote deletion, encryption, or guaranteed forensic erasure.
- Frozen stale-head drafts remain inspectable offline. Existing write preflights continue to reject changed PR pins and historical source without captured provenance. `--plain`, guide inputs, logs, and source snapshot payloads exclude draft text. Human terminal usability remains unverified by render tests alone.

# Range and File Comment Contract (issue #29)

- The line-target rule above extends to same-side contiguous ranges within one raw captured hunk. Persist and submit both start/end coordinates; never flatten a range. Reject absent, cross-hunk, cross-file, cross-side, changed-pin, and unsupported historical anchors explicitly.
- File targets use a captured changed path and `subject_type:file`, with no line/side coordinates. Captured non-text files are eligible; unavailable files and unsafe paths are rejected. The new path is used except for deletion, which uses the old path.
- Immediate range/file creation uses the documented review-comment endpoint. Queued ranges use the documented batch review fields. File targets cannot be queued through the documented batch endpoint: preserve the draft and show an explicit unsupported error, never omit a target or invent a line.
- New private drafts use payload version 2, so old binaries reject rather than flatten extended targets. Version 1 remains readable. Editors, pending targets, and immutable attempts retain all coordinates during edit, refresh, restart, and offline recovery. Remote reconciliation compares complete current/original target shapes.

# Published Comment and Thread Action Contract

- Issue #28 adds only explicit published-comment edits and review-thread
  resolve/reopen actions in the inline and discussion views. General PR comments
  may be edited; submitted review decisions/bodies are distinct events. Actions
  use stable remote identities, never inferred code coordinates or outdated state.
- Editing re-reads comment membership and authenticated authorship. Thread actions
  re-read GitHub's `viewerCanResolve`/`viewerCanUnresolve` permissions and known
  resolution state. Missing permissions or identities deny the action. Metadata
  before and after retrieval must match the frozen repository/base/head pins.
  GitHub remains authoritative for write authorization.
- JSON stdin carries mutation data. Offline refusal precedes client/credential
  access. Canonical successful responses preserve identities and update all local
  affected views without changing captured anchors, parent IDs or timestamps.
- Failure retains edits in process memory. Unknown outcomes freeze attempted
  body/desired resolution and block submission until explicit complete verified
  refresh establishes that identity's current state. A matching attempt blocks
  retry; a mismatch permits a separate intentional action. Missing identities
  remain uncertain. Refresh and background work never retry mutations.
- Read generations invalidate pre-attempt/pre-success results. Subsequent verified
  reads may show another client's edits, reopens or deletions. Partial refreshes
  retain only omitted confirmed/selected identities, deduplicate threads/comments,
  and label them stale. Retained comments never become current code overlays or
  editable identities, and never override authoritative incoming siblings.

# Pinned Complete Source Navigation Extension

- Issue #30 permits optional complete OLD/NEW changed-file source in immutable
  local snapshots only after explicit `--cache-full-source` process consent.
  Default opening remains patch-only; consent is not persisted across launches.
- Capture uses pinned inventory OIDs in the isolated object view, never working
  files or filters. Limits are 1 MiB/blob, 4 MiB distinct text and 100,000 lines,
  with a 60-second maximum capture deadline and no cache-specific fetch/retry.
  Invalid UTF-8, NUL/binary, gitlinks, absent capture and exhausted limits remain
  visibly unavailable. Empty/absent sides are not missing-source failures.
- Stored source is private local unencrypted data and may include sensitive
  unchanged content. It extends local storage only, never AI input or permission
  to write externally. Existing immutable SQLite integrity/deletion rules apply.
- Context/full-file and whitespace projections never change patches, unit IDs or
  read progress. Only canonical patch rows retain comment targets; added context
  and full-source rows stay read-only. Search is local with visible scope, skipped
  coverage and result limits. Unknown thread resolution never means unresolved.
- See `docs/spec/SPEC-code-navigation.md` for consent, offline recovery,
  invalidation, provenance and coordinate mapping policy.


# Incremental Review Contract (issue #32)

- The predecessor is a specific saved session ID/reference/generation with full repo and base/head pins. Capture a navigable direct committed prior-head/new-head tree diff in isolated object storage; never derive source from checkout contents or refs. Recheck current pins after capture. Explicit unavailable/partial states preserve all original snapshots and drafts. Rewrites and base changes do not imply ancestry or unchanged content.
- Carry whole-file read marks only with complete captured evidence and identical both-side blob IDs/modes/paths/status, diff settings/version, and every raw unit/patch. Partial/unavailable content, renames, equal path/display text alone, and changed old-side base content never prove unchanged. Check predecessor generation/reference inside the initial progress transaction.
- Anchor assessments retain complete old/new range/file shapes and return advisory outcomes only. Exact file identity takes precedence over other paths; equal-content copies do not establish renames. Original draft bodies, replies, summaries, historical targets, and immutable uncertain attempted requests remain separate private mutable data. No source/guide payload or model request includes them. Changes navigation and draft reconciliation never submit, retry, copy drafts, or silently retarget; recreation requires explicit target selection and the existing write confirmation/preflight.
- Persist provenance and captured patches under the existing private SQLite integrity and resource/storage ceilings. Offline snapshots remain readable without Git or network. Deleting an original does not erase the independently captured comparison, and unavailable original drafts are never synthesized. Human terminal QA remains unverified unless actually performed.

# Read-only PR Readiness Contract

- Issue #33 adds ephemeral, exact-head-labelled readiness evidence, independent
  of immutable code, source caches, progress and AI guide inputs. `Alt+R` opens it;
  explicit readiness refresh never refreshes pinned source or writes remotely.
- Unknown protection/rules, permission failures, bounded/partial retrieval,
  skipped/cancelled/unknown required checks and mismatched revisions cannot imply
  readiness. Unavailable requirements never turn unmatched checks into optional.
- `Readiness.Ready(expectedHead)` is conservative evidence only; #34 must perform
  a new exact-head read and authorization preflight before an explicit merge.
- See `docs/spec/SPEC-pr-readiness.md` for controls, limits, API and cache policy.

# Suggested Change Contract

- Ctrl+S captures right-side line/range source from immutable raw hunks and edits replacement text independently of Markdown. Full target coordinates and replacement mode survive private draft payload version 2 targets (suggestion editor/apply payloads use version 3) recovery, including offline recovery.
- Existing and draft supported suggestions show escaped Before/After previews. Unsupported left/file/historical creation, offset/multiple blocks, unavailable source and modes are explicit; existing historical replies remain valid.
- Application requires a separate confirmation showing actual head repository/ref, path/range and replacement. The documented GitHub `createCommitOnBranch` mutation applies one remote commit with `expectedHeadOid`; it never changes the checkout or claims native suggestion UI metadata.
- Refresh canonical comment body/current anchor, complete PR pins, head ref, head-repository write permission, ordinary 100644 file mode and exact selected source before preparation and confirmation. Apply only the retained full-file payload, once, on stdin; concurrent pushes are rejected by expected head.
- Persist exact attempted mutation bytes and provenance privately before dispatch. Uncertain results block retry and require read-only reconciliation; whole-file equality at a changed remote head counts conservatively as applied. An unchanged head does not prove absence (force-push-back/delayed writes). No automatic write/retry, silent reanchoring, credential persistence or source diagnostics.

# Durable general conversation integration

- General editor payload v4 reads versions 1–3 without losing range targets or suggestion attempts. Earlier binaries reject and preserve v4. Persist only local text, rune cursor, reply event identity, original attempted body, at most 500 previously observed general IDs, and uncertain/matched state. Fetched events remain ephemeral.
- Save posting intent durably before dispatch; interrupted posting recovers uncertain. Editing cannot replace the attempted body. Only complete current-verified bounded conversation refresh reconciles; partial/stale/failed/offline, inline-only, and pre-attempt reads cannot clear uncertainty. No automatic retries.
- Comparison reset clears prepared suggestion and general state, preserving original attempts under their original private key. Incremental navigation labels the original work without rendering its bodies or remapping it. Escape, quit/recovery discard, successful posting and last-session deletion clean up local drafts.

# Read-only Review Inbox Contract

- Issue #35's account inbox is explicit cached/refreshed triage, independent of
  frozen source and review state. No polling, alerts, uploads or remote writes.
- Unknown first capture, missing/null fields, access failures and bounded results
  remain labelled unknown/incomplete. Mark-read is explicit, local and scoped to
  account plus immutable PR identity; it never changes frozen review evidence.
- A cross-repository item opens its own frozen session or a checkout whose current
  repository identity is verified. Never fall back to another repository checkout.
- See `docs/spec/SPEC-review-inbox.md` for filters, controls, bounds and cache policy.

# Reviewer and Issue Context Contract (#36)

- `I` opens independently saved PR-identity context locally. Only `r` within that
  screen authorizes GitHub context refresh and configured Linear reads. No polling,
  automatic context fetch, remote write, guide-input expansion, or AI upload.
- GitHub reviewDecision is authoritative; nil means unknown. Individual decisions
  come from latestOpinionatedReviews. These labels are context, not merge blockers.
  Closing issue references come only from GitHub's structured API connection.
- Linear reads require global secret-free configuration with explicit enablement,
  allowed repositories, workspace, credential env name and auth mode. Credentials
  remain in memory/HTTP headers only, never argv, cache, logs or diagnostics.
  Candidate links in PR description are untrusted; validated provider responses
  supply metadata. No arbitrary URL, redirect, asset, or repository config fetch.
- Requests are bounded: one GitHub GraphQL page per connection, 100 records each,
  1 MiB response, 60-second total refresh; at most 10 Linear issues, 25-second
  optional slice, 1 MiB per response, 2 MiB aggregate accepted response budget.
  Errors and limit exhaustion produce visible partial/unavailable states.
- Context cache is independent of immutable code/guide payloads and progress,
  keyed by normalized repository/PR with capture timestamp, head, checksum,
  validation and transactional latest-capture-wins writes. Offline read never
  touches integration configuration or credentials. Saved context is historical;
  failed refresh keeps it labelled stale. Cache failure never blocks source review.
- Synthetic fixtures/transports only in tests. Human terminal usability remains
  unverified unless explicitly performed.
- Captured Linear workspace/auth mode is non-secret historical provenance,
  never evidence of current viewer authorization. Offline cache access does not
  inspect account configuration or credentials; configuration changes cannot
  relabel foreign PR/workspace data. Missing legacy provenance is explicit.
- Context Update operations preserve private v3/v4 dispatched attempts and
  canonical anchors, frozen source bytes, reading layouts and read-only cursors.
  Cancelled/old-generation/foreign-session/foreign-PR results cannot replace them.


# PR Lifecycle Extension (#34)

- Lifecycle writes require an explicit interactive confirmation showing PR, action,
  method and expected live head. Offline/plain operation never exposes a writer.
- Re-read canonical head/base/state/draft, permissions, readiness and policy before
  each write. Merge/auto-enable/enqueue bind expectedHeadOid; other APIs lack an
  atomic revision guard, which remains visible. No admin bypass, queue jump or
  protection override is requested. GitHub remains the policy authority.
- Direct merge requires complete, consistent checks/reviews and validated policy.
  Richer supported policy may be fulfilled by authoritative CLEAN evidence only
  after rule shapes are validated; unknown/malformed rules remain unavailable.
  Auto/queue may wait under valid policy; immediate readiness is not required.
- Always refresh canonical lifecycle state after an attempted write. Uncertain
  outcomes lock further writes and reconcile through reads, never automatic replay.
  All lifecycle state is ephemeral and tab-owned; frozen snapshots are unchanged.
- Tests use synthetic transports for all writes. Real roadmap PR lifecycle mutation
  and terminal human QA are not part of automated verification.
