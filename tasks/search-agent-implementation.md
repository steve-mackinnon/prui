# Implementation tasks: search-based guide agent

Authorized 2026-09-26. Use the existing OpenAI Responses client and current model configuration; no additional SDK, parsers, persistent index, or live-provider trial. This execution checklist supersedes the harness-only scope of context-retrieval-plan.md for the interactive `g` path. Existing raw review and saved-evidence fallback remain available.

## Shared boundaries

- Prepare a bounded corpus from saved merge-base/head Git objects through a private source view, never current refs or working files; no automatic fetch. Close view after preparation; corpus lives only for consent/generation.
- Expose language-independent search and line reads. Filter whole blobs before excerpts; recheck outputs and propagate privacy decisions to all changed units/rename aliases.
- Preview eligible files and their text; allow exclusions before confirming incremental source disclosure. Freeze prepared input and scope for the attempt. Offline checks precede credentials and network access.
- Existing OpenAI adapter executes a maximum of four Responses requests, eight tool calls, 60-second whole-loop deadline; retain strict final guide validation and upload limits. Provider-independent tool data lives outside transport code.
- Persist only uploaded evidence and final guide provenance in a derived session. No full corpus, transcript, credentials, or progress transfer. Cancellation preserves original session.

## Tasks and ownership

1. [x] Pinned corpus/search/read engine with synthetic tests — corpus agent; owns internal/context/search*.go.
2. [x] Bounded Responses tool loop and fake-provider tests — loop agent; owns internal/guide/search*.go and necessary OpenAI adapter changes.
3. [x] Preparation, inspection, exclusion and consent UI — UI agent; owns internal/tui preparation/consent changes and tests.
4. [x] Shared preparation/input contract, exact-pin application wiring and derived session evidence — integrating agent; owns guide preparation/input/analyze changes, cmd/review integration and tests.
5. [x] Update contracts/reference and implementation status; independent review, full vet/race/build checks — integrating agent with follow-up review delegation.

Tasks 1–3 run independently against the agreed contract; task 4 integrates them. After integration verify end-to-end fake-provider search/read/final flow, excluded source, unavailable objects, limits, cancellation, offline refusal and immutable snapshots. Run go vet ./..., go test -race -count=1 ./..., go build ./..., git diff --check. Use fake credentials/local endpoints only. Human terminal usability and empirical live-model guide quality remain separately reported limitations.

## Validation and remaining evaluation

Completed: `go vet ./...`, `go test -race -count=1 ./...`, `go build ./...`, and `git diff --check`. A final focused race test also verifies that the evidence view retains omitted-file and incomplete-search warnings. All provider tests use loopback servers and fake credentials. Independent review covered approval binding, privacy/rename closure, cancellation, stale-tab handling, immutable source and durable scope reporting.

Original session source remains identical; submitted search excerpts live in the derived guide bundle and successful local cache entry. No production indexing service, parser, new model SDK, or persistent corpus was added. The corpus and provider continuation exist only in memory for one attempt.

Human terminal usability/accessibility and empirical model quality, latency, token cost and search recall are not established by these tests. The proposed comparative evaluation in context-retrieval-plan.md remains future work, not a measured benefit or required live upload in this implementation task.
