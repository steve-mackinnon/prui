# Configurable Guide Models with Charm Fantasy — Tasks

Source: [spec](../docs/spec/SPEC-MULTI-PROVIDER-GUIDES.md) and
[implementation plan](multi-provider-guides-plan.md). Check off tasks only
after their acceptance criteria and verification pass. File lists are likely
touch points, not permission to modify another task's owned files in parallel.

## G-01: Prove Fantasy is compatible with the guide request contract

**Description:** Pin one Go-1.26-compatible Fantasy release and run a local
request-capture spike before changing production guide behavior.

**Acceptance criteria:**
- [x] The exact release compiles with the repository toolchain and its dependency diff is reviewed.
- [x] A non-streaming OpenAI Responses object call makes exactly one request with `store:false`, the intended model, and the strict guide schema.
- [x] The SDK accepts an injected HTTP client; its retry and redirect behavior is established in tests without a live provider.

**Verification:** `go test ./internal/guide -run 'Fantasy.*Contract' -count=1`; `go build ./...`.

**Dependencies:** None. **Scope:** Medium, about 3 files.

**Files likely touched:** `go.mod`, `go.sum`, `internal/guide/fantasy_contract_test.go`.

## Checkpoint A: Library gate

- [x] G-01 passes. Record the exact version and any observed limits in the spec before implementation continues.

## G-02: Resolve one global guide selection

**Description:** Add the XDG-style macOS/Linux `config.json` loader and
validation, without touching theme configuration or reading credentials.

**Acceptance criteria:**
- [x] Missing config yields the current OpenAI model and legacy `OPENAI_BASE_URL` origin semantics; present config yields one immutable provider/model selection.
- [x] Invalid, duplicate, or unknown fields and unsafe endpoints fail safely; relative `XDG_CONFIG_HOME` falls back to `~/.config`.
- [x] The selection exposes a sanitized destination, credential-variable name, and deterministic non-secret fingerprint; no key value enters config or provenance.

**Verification:** `go test ./internal/guideconfig -count=1`.

**Dependencies:** G-01 contract. **Scope:** Small, 2 files.

**Files likely touched:** `internal/guideconfig/config.go`, `internal/guideconfig/config_test.go`.

## G-03: Enforce the common outbound transport policy

**Description:** Build a reusable HTTP client for Fantasy provider constructors
that bounds request and response bodies and forbids redirects.

**Acceptance criteria:**
- [x] An oversized request fails before any bytes reach a local server; an oversized response becomes a bounded error.
- [x] Redirects are not followed and cannot forward source or credentials to another host.
- [x] Caller cancellation stops the request; transport errors cannot expose keys, request bodies, or raw provider bodies in durable reasons.

**Verification:** `go test ./internal/guide -run 'Transport|Fantasy.*Limit' -count=1`.

**Dependencies:** G-01. **Scope:** Small, 2 files.

**Files likely touched:** `internal/guide/fantasy_transport.go`, `internal/guide/fantasy_transport_test.go`.

## G-04: Reproduce the existing OpenAI guide flow with Fantasy

**Description:** Implement the shared Fantasy-backed `guide.Analyzer` and the
OpenAI provider constructor, leaving the old adapter as the CLI default until
focused parity tests pass.

**Acceptance criteria:**
- [x] Existing bounded prompt and `prui_guides` schema produce the same validated bundle shape and provenance.
- [x] Local fixtures cover successful output, refusal, incomplete output, malformed schema, timeout/cancel, and sanitized provider failure.
- [x] The current privacy tests still prove withheld paths/content never reach the injected HTTP client; no schema repair or second request occurs.

**Verification:** `go test -race -count=1 ./internal/guide`.

**Dependencies:** G-02, G-03. **Scope:** Medium, up to 4 files.

**Files likely touched:** `internal/guide/fantasy.go`, `internal/guide/fantasy_openai.go`, `internal/guide/fantasy_test.go`, `internal/guide/fantasy_openai_test.go`.

## Checkpoint B: OpenAI parity

- [x] Focused guide tests and `go build ./...` pass with both adapters present.
- [x] Compare captured old/new OpenAI requests for source scope, schema, `store:false`, one-request behavior, and byte limits.

## G-05A: Add the native Anthropic provider constructor

**Description:** Wire Fantasy's native Anthropic provider to the shared
analyzer and bounded transport.

**Acceptance criteria:**
- [x] The constructor uses the configured model and the credential supplied after confirmed generation.
- [x] Local fixture responses map to valid guide candidates; refusal, incomplete output, and unsupported schema behavior remain unavailable.
- [x] It makes at most one bounded request, and no raw SDK/provider error reaches a stored reason.

**Verification:** `go test ./internal/guide -run 'FantasyAnthropic' -count=1`.

**Dependencies:** G-04. **Scope:** Small, 2 files.

**Files likely touched:** `internal/guide/fantasy_anthropic.go`, `internal/guide/fantasy_anthropic_test.go`.

## G-05B: Add the native Google provider constructor

**Description:** Wire Fantasy's native Google provider to the shared analyzer
and bounded transport.

**Acceptance criteria:**
- [x] The constructor uses the configured model and the credential supplied after confirmed generation.
- [x] Local fixture responses map to valid guide candidates; refusal, incomplete output, and unsupported schema behavior remain unavailable.
- [x] It makes at most one bounded request, and no raw SDK/provider error reaches a stored reason.

**Verification:** `go test ./internal/guide -run 'FantasyGoogle' -count=1`.

**Dependencies:** G-04. **Scope:** Small, 2 files.

**Files likely touched:** `internal/guide/fantasy_google.go`, `internal/guide/fantasy_google_test.go`.

## G-06: Add the custom OpenAI-compatible endpoint

**Description:** Use Fantasy's compatible provider with a configured base URL
and explicit schema-tool object mode, without permissive text fallback.

**Acceptance criteria:**
- [x] The endpoint receives exactly the configured model and one schema-constrained request; remote HTTP and URL credentials are rejected before client construction.
- [x] A loopback endpoint can operate without an API key, while a configured key is sent only to its selected endpoint.
- [x] Unsupported tools/object output is reported as unavailable, with no second provider, text-mode, or retry attempt.

**Verification:** `go test ./internal/guide -run 'FantasyCompat' -count=1`.

**Dependencies:** G-04. **Scope:** Small, 2 files.

**Files likely touched:** `internal/guide/fantasy_compat.go`, `internal/guide/fantasy_compat_test.go`.

## G-07: Make guide consent selection-aware

**Description:** Show the exact selected provider, model, and sanitized
recipient before upload, with retention wording appropriate to that provider.

**Acceptance criteria:**
- [x] Consent displays provider/model/origin and never a key, URL credentials, query, or path.
- [x] The `store:false` statement appears only when that request field is sent; cancellation makes no analyzer call.
- [x] Existing source-scope and credential-filter warnings remain visible in the TUI.

**Verification:** `go test ./internal/tui -run 'GuideConsent' -count=1`.

**Dependencies:** G-02. **Scope:** Small, 2 files.

**Files likely touched:** `internal/tui/guide_consent.go`, `internal/tui/guide_consent_test.go`.

## G-08: Gate guide-cache reuse on the selected model

**Description:** Add non-secret selection provenance and refuse automatic cache
reuse when provider/model/endpoint/schema selection differs.

**Acceptance criteria:**
- [x] Existing sessions and legacy guide bundles remain readable; no SQLite schema migration occurs.
- [x] A cache hit requires the current fingerprint; legacy bundles match only the unchanged OpenAI default without endpoint override.
- [x] PR-list reopening cannot display a saved or cached guide from another selection; explicit resume still reads that saved session. A new successful guide can replace the existing comparison cache slot.

**Verification:** `go test ./internal/session ./cmd/prui -run 'GuideCache|CachedGuide' -count=1`.

**Dependencies:** G-02. **Scope:** Medium, up to 5 files.

**Files likely touched:** `internal/guide/guide.go`, `internal/session/sqlite_guides.go`, `internal/session/sqlite_guides_test.go`, `cmd/prui/lifecycle.go`, `cmd/prui/lifecycle_test.go`.

## G-09: Wire selection through the interactive guide action

**Description:** Connect the one resolved selection to consent, cache lookup,
and analyzer construction in `cmd/prui`; make the offline guard precede
credential access.

**Acceptance criteria:**
- [x] No-config users retain the OpenAI workflow and `OPENAI_BASE_URL` behavior; a configured provider/model drives the exact confirmed request.
- [x] Invalid config or missing credential leaves the current review intact and sends nothing; offline mode reads no provider key or client.
- [x] PR-list opens may reuse only a matching cached guide, while `open`, `resume`, `--plain`, and `verify` never auto-generate.

**Verification:** `go test -race -count=1 ./cmd/prui ./internal/tui`; `go build ./...`.

**Dependencies:** G-04, G-05A, G-05B, G-06, G-07, G-08. **Scope:** Medium, up to 4 files.

**Files likely touched:** `cmd/prui/wiring.go`, `cmd/prui/wiring_test.go`, `cmd/prui/guide_consent_test.go`, `cmd/prui/lifecycle_test.go`.

## Checkpoint C: Complete configured path

- [x] Local end-to-end fixtures prove each provider's configured `g` action, consent, cache behavior, cancellation, and raw-review fallback.
- [x] Focused guide, config, command, session, and TUI tests pass without a live service.

## G-10: Update the user and project contracts

**Description:** Document the shipped config and provider behavior and revise
the OpenAI-only project contract.

**Acceptance criteria:**
- [x] `CONSTRAINTS.md`, README, and reference show XDG config paths, provider/model selection, credential sources, consent, and limits accurately.
- [x] The spec and architecture docs reflect the final implementation and any tested Fantasy limitations.
- [x] Source-upload, offline, cache, and one-request guarantees remain explicit in the revised contract.

**Verification:** `rg -n 'OPENAI_BASE_URL|OPENAI_API_KEY|Responses|config.json' README.md CONSTRAINTS.md docs`; `git diff --check`.

**Dependencies:** G-09. **Scope:** Medium, 5 files.

**Files likely touched:** `README.md`, `CONSTRAINTS.md`, `docs/REFERENCE.md`, `docs/ARCHITECTURE.md`, `docs/spec/SPEC-MULTI-PROVIDER-GUIDES.md`.

## G-11: Retire the old OpenAI transport after parity

**Description:** Remove or consolidate the old protocol implementation only
after the new path is the CLI default and its behavioral coverage is retained.

**Acceptance criteria:**
- [x] No production wiring selects the old transport.
- [x] Existing privacy, failure, and schema tests remain covered by the Fantasy path before old tests are removed.
- [x] Prompt/schema definitions have one source of truth and focused guide tests remain green.

**Verification:** `go test -race -count=1 ./internal/guide ./cmd/prui`; `rg -n 'NewOpenAI|OpenAIOptions' internal cmd`.

**Dependencies:** G-10. **Scope:** Small, 2 files.

**Files likely touched:** `internal/guide/openai.go`, `internal/guide/openai_test.go`.

## G-12: Verify and review the complete change

**Description:** Run repository gates, inspect the final diff and real terminal
consent flow, and resolve any remaining regression before completion.

**Acceptance criteria:**
- [x] All spec acceptance criteria have synthetic test evidence; no tests use live keys or providers.
- [x] Full format, vet, race, build, PTY, and changed-code lint gates pass; any pre-existing lint findings are distinguished from new ones.
- [ ] A human terminal walkthrough confirms readable consent on narrow and normal widths; code review finds no bypass of privacy, offline, or cache contracts.

**Verification:** `./scripts/verify.sh`; `golangci-lint run --new-from-rev=HEAD ./...`; `git diff --check`; manual terminal walkthrough.

**Dependencies:** G-11. **Scope:** Integration and verification; edit only files needed to fix a concrete finding.

**Files likely touched:** None unless verification identifies a specific defect;
record any repair and its focused test in this checklist.

Verification on the main-based PR branch: `go vet ./...`, `go build ./...`,
changed-code `golangci-lint run --new-from-rev=origin/main ./...`,
`git diff --check`, and `go test -race -count=1 ./...` all pass. Focused TUI
consent and all four interactive provider fixtures pass. A human terminal
walkthrough remains pending.

## Checkpoint D: Ready for release review

- [ ] G-01 through G-12 are complete with evidence recorded here.
- [x] Documentation, config examples, consent text, and provider requests agree.
- [x] No unrelated historical task checklist was changed.
