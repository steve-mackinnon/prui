# Plan: model-directed local search for guides

Design reference; implementation authorized · 2026-09-26 · [Decision memo](../docs/CONTEXT-RETRIEVAL-DECISION.md)

This replaces the earlier lexical/parser/index experiment. It preserves unrelated unfinished work in `plan.md` and `todo.md`. Implementation of the interactive search loop is now authorized; see [execution tasks](search-agent-implementation.md). Live source uploads still require application consent. The evaluation targets below remain unmeasured.

## Experiment and scope

Compare A = today's fixed `context.Retrieve` selection and single guide request with B = a diff-first request whose model can search pinned source and read selected lines before generating the same guide schema. B starts with eligible changed units and a compact tool/scope description, without A's automatically ranked evidence. Both use the same model, final guide instructions, unit caps and unique-source byte budget. Extra tool rounds are an explicit additional cost, not an equal-compute comparison.

Provide two language-independent operations. No AST parsing, language server, definition resolver, embedding model, persistent index, SQLite migration, daemon, repository shell, or reviewed-code execution. Implement B in the explicitly confirmed interactive `g` path, using the existing OpenAI adapter. Open/resume never generate automatically; cached guides remain local, and `verify` and `eval-guides` remain offline evaluation paths. Tests and CI use synthetic objects and a fake provider. Any live guide trial still requires explicit interactive consent.

## Pinned source and lifetime

Git blobs contain the committed text; an ordinary working-directory checkout is unnecessary. For this first experiment, reopen a private `source.NewView` on explicit guide preparation and verify that the **saved** merge-base/head commits and required tree objects are available. Do not call the current pinning routine to obtain newer GitHub tips, follow branches, or fetch automatically. Materialize only bounded eligible blob bytes into process memory, preserving `(repository, commit, raw path, mode, blob OID)` membership. Scan these bytes locally, then close the Git view once this frozen corpus is ready. Keep the corpus alive only for preparation and the guide attempt; release it on success, failure or cancellation.

This avoids retaining a Git view for every idle review tab. If saved objects are unavailable or collected, show that full-tree search is unavailable and offer generation from existing saved patches/evidence under its usual consent. Never silently substitute working files or current revisions. Partial preparation has an explicit coverage/omission ledger; absence from the prepared corpus is not proof of absence from the repository. A later explicit fetch of exact pins is out of scope.

A future product integration may reuse an already-open pinned view, but must preserve the same ownership and cleanup rules. Completed derived sessions save only actually uploaded evidence and guide provenance, not the full corpus or provider transcript. Existing sessions and reading progress stay immutable; cancellation saves no derived session.

## Tool contract

| Tool | Behavior |
| --- | --- |
| `search(pattern, path_glob, revision)` | Literal, case-sensitive text search; nonempty UTF-8 pattern up to 256 bytes, optional app-defined path glob, revision enum `head` or `merge_base`. Return stable path/line order with up to 20 matches and two context lines on either side. |
| `read_lines(path, start, end, revision)` | Exact corpus-member path and positive inclusive line range, at most 200 lines. Return only those pinned lines, subject to remaining byte limits. |

Paths are identifiers within the allowlisted corpus, never filesystem paths to open. Validate glob syntax in-process; no shell expansion or arbitrary Git arguments. Unsupported/non-UTF-8 paths are explicitly unavailable to tools while remaining in raw review inventory. Skip binary, symlink and submodule content. Search literal text in code, tests, manifests and documentation alike; the model can search an identifier, import string or phrase, then read around matches. No `definition` tool or claim of semantic resolution.

Each result identifies revision, commit, blob, path and exact line range. Reuse the existing Evidence representation where possible, with length-delimited IDs covering identity/range/bytes and retrieval version. Attach an application-generated retrieval reason, not an unsanitized model query. Return structured statuses for complete, truncated, unavailable and budget-exhausted results. A no-match result is qualified by corpus coverage. Deduplicate identical evidence IDs for accounting; overlapping distinct excerpts conservatively count separately. Preserve the result sequence only in transient memory.

## Privacy and consent

Prepare the corpus locally before any model call. Apply user-owned path exclusions before reading, scan each complete eligible blob before returning any excerpt, and withhold credential-like or binary content. Propagate privacy decisions to changed units, metadata and both names/aliases of renames before constructing the initial diff request. Changed files that cannot be fully checked are explicitly withheld in the experimental arm. Preserve the existing default policy and second outbound check; secret detection remains heuristic and exclusions cannot guarantee other files never mention the same information.

The preparation view shows recipient, model, frozen pins, eligible paths and full scrollable text, included diff units, byte limits, local omissions and file exclusion controls. State clearly: confirmation authorizes this attempt to upload selected excerpts from **any approved file**, through multiple requests, within the displayed limits. It does not mean the entire corpus is uploaded. Per-file exclusions cover associated changed units and rename aliases. Exclusions are memory-only for this attempt.

Bind confirmation to a digest of the filtered corpus membership/content, initial input, policy, pins, endpoint, model, prompt/tool versions and limits. Edits require a new preview and confirmation; after sending starts, freeze this configuration. The model cannot widen scope. All tool outputs, filenames, status text and follow-up requests pass the outbound boundary; omit denied paths from provider-visible errors and counts. Maintain a local inspectable ledger of uploaded excerpts. Replayed history contains only approved material. Cancellation stops future uploads but cannot retract bytes already sent.

This explicitly supersedes two rules **for the experimental path only**: the single Responses call, and upload restricted to the initially assembled `guide.Input`. The replacement contract is an initial approved input plus bounded, policy-checked tool results from an approved frozen corpus. Document this exception before enabling the path; preserve offline refusal before credentials/client access and the current redirect, credential and non-retention-of-transcripts rules. `store:false` remains required on every call and is not a promise of zero provider retention.

## Budgets and request loop

Proposed starting limits, to be measured rather than silently expanded:

- Corpus preparation: 10,000 tree entries across both revisions, 1 MiB/blob, 50 MiB retained materialized text across both revisions, five seconds; explicit partial scope on exhaustion. Track metadata and process memory separately. Nothing persists between attempts.
- Existing changed-unit limits: 400 units, 32 KiB/unit. Initial units plus distinct uploaded evidence stay within 512 KiB; tool evidence additionally has a 256 KiB cap. At most 16 KiB per tool result including metadata. Include envelope overhead in transport accounting separately.
- At most eight tool calls total, at most three tool-result batches, and at most four provider requests including the final answer. Enforce limits even when the model requests many calls together. Bound each local operation to one second and total tool execution to five seconds.
- Sixty seconds for the automatic generation loop, without resetting on each request. Local preparation and user inspection precede that timer and are reported separately. Retain 2 MiB/request and 4 MiB/response caps; cap cumulative transport at 8 MiB outbound and 16 MiB inbound, including replay. Report tokens and dollar cost where usage/pricing are available; bytes do not bound model output tokens, so configure a supported per-response output-token limit as well.

Use the Responses tool-calling flow with application-executed tools. Hold required continuation items and tool-call IDs in memory, replaying only validated approved history with `store:false`. Reject unknown tools and malformed arguments; never treat source text as instructions. On the final permitted request disable tools and request the strict guide schema. If a tool budget runs out, allow a final bounded answer acknowledging incomplete evidence if time permits; invalid output, provider failure or deadline follows the existing unavailable-bundle policy. User cancellation preserves the original session. Providers without tool support fail visibly; no unannounced larger request or retry.

Keep structural validation, synthesized ungrouped units, file ownership and progress unchanged. Distinguish generated interpretation from source facts. Record retrieval/tool version, evidence IDs, input/provenance digest and limits in the result; cache successful guides with their uploaded evidence ledger and retrieval version. Existing historical fixed-selection guides remain readable and reusable without any provider call.

## Ordered implementation tasks

1. [ ] **Baseline and fixtures** (small; context tests and new harness fixtures). Freeze 12 synthetic PR histories: four development, eight held-out, covering Go, JavaScript/TypeScript, Python and Markdown. Include distant definitions/tests, aliased imports, ambiguous names, unrelated docs, files over 32 KiB, old/new signature changes, renames, missing objects and secret canaries. Annotate relevant pinned ranges and expected behavioral groups before tuning prompts. Capture A locally; verify exact-pin and dirty-working-tree isolation.
2. [ ] **Pinned corpus and two local tools** (medium; new context search implementation/tests and narrow source helpers). Implement bounded preparation, filtering, literal search and range reads. Test exact bytes/ranges, both revisions, invalid paths/arguments, partial scope, whole-blob credential detection outside returned lines, cancellation and cleanup. Tools work with scripted requests without a model.
3. [ ] **Preparation and consent binding** (medium; experimental preparation view/controller/tests). Inspect full candidate text, exclude files and verify final approved scope. Any configuration change invalidates confirmation. Prove excluded names/content and rename aliases cannot enter an initial input or tool result; verify in a real terminal before product adoption.
4. [ ] **Bounded tool loop** (medium; experimental guide orchestrator/provider adapter/tests). Use fake-provider scenarios for search → read → final guide, multiple calls, bad arguments, source prompt injection, retries requested by the model, budget limits, refusal, timeout and cancellation. Validate exact outbound bodies and replay accounting. Check final schema, unit coverage, immutable derived sessions, and no transcript/credential persistence. Document the experimental contract exception before enabling live execution.
5. [ ] **Evaluate and decide** (small; guide evaluation fixtures/report). Run offline scripted lookup/timing experiments first. Separately authorized manual interactive trials compare A/B on synthetic source; `verify`, CI and `eval-guides` never generate guides or read live keys. If live trials do not occur, label semantic quality and model latency unmeasured. Produce the adoption/defer report; no production indexing work follows automatically.

Dependencies: 1 → 2 → 3 → 4 → 5. After task 2, checkpoint provenance/privacy and local cost. After task 4, checkpoint consent, upload accounting and failure behavior before any live trial. For code implementation run `go vet ./...`, `go test -race -count=1 ./...`, `go build ./...` and `git diff --check`; fake provider endpoints and synthetic repositories only.

## Proposed success measures

These are pilot decision gates, not measured results. Record model/settings, prompt/tool versions, hardware, source size, corpus omissions, tool queries/counts in transient trial observation, and request usage. Persist only aggregate measurements and approved evidence/provenance, not raw provider transcripts. Use at least 30 local timing repetitions on 1,000- and 10,000-entry fixtures within 50 MiB.

| Dimension | Gate |
| --- | --- |
| Evidence relevance | Held-out mean required-range recall improves ≥20 percentage points over A at equal unique-source caps; precision falls no more than 5 points. Report each language and definitions/imports/tests/docs separately. |
| Guide quality | Same model/settings and three runs per arm per held-out PR. Two blinded reviewers rate grouping, reading order, contextual correctness and unsupported claims. B wins ≥6/8 PRs by median rating, with no new material unsupported claim identified by either reviewer. Report disagreements and all attempts, including failures. This is not statistical proof. |
| Latency | Corpus preparation p95 ≤5 s; search/read p95 ≤250 ms on the largest bounded fixture. Whole automatic loop ≤60 s and median ≤1.5× A. Report preparation, local search, model/network and human inspection time separately. |
| Request cost | All attempts obey eight tools/four requests and byte caps; median billed tokens ≤2× A. Report actual replay bytes, tokens and available provider usage costs, including failed attempts. Failure means reconsider limits/design rather than hiding expensive runs. |
| Storage/update cost | No persistent index or update job. Zero full-corpus bytes retained after the attempt; saved evidence within 256 KiB. Report object-view setup, text materialization, tree enumeration, repeated scanning and peak memory/temp disk. Repeated PRs pay preparation again—measure it rather than assuming reuse. |
| Safety and scope | Every excerpt matches its saved pin; zero excluded canaries/names in serialized fixture requests; no pre-consent model requests, working-file reads, reviewed-code execution or automatic fetches. Offline refusal precedes credential access. Cancellation and tab switching cannot retarget or save the attempt. |
| Inspection | Two reviewers can identify a file's revision, inspect it, exclude it, and explain that subsequent tool results may be uploaded from the remaining approved set. The local sent-evidence ledger matches captured fake-provider requests. |

If quality fails, inspect missing/ambiguous searches before adding machinery. If quality succeeds but repeated scans dominate latency, evaluate a local reusable text index. If ambiguous symbol matches cause substantive errors, evaluate targeted parsers. If model round trips dominate, prefer fewer batched searches or fixed local selection. Embeddings and remote corpus services remain deferred.
