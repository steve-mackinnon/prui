# Phases 1 To 3 Contract

- Raw source comes only from pinned committed objects, never working files or model output.
- Compare the unique, ancestry-verified merge base to head. Reject ambiguous or unresolved comparisons.
- Account for every raw change and every unit exactly once. Limits produce explicit incomplete/unavailable states, never an empty-success substitute.
- Never execute reviewed code, hooks, filters, textconv, external diff, credential helpers, submodule commands, or repository-selected network helpers.
- Never mutate the checkout, index, refs, configuration, or object files. Temporary isolated storage is tool-owned and removed on completion/cancellation.
- No telemetry, review writes, or persisted credentials. Phases 1 to 3 have no model integration and no source upload; Phase 4 analysis supersedes the no-upload part of this rule under the opt-in below.
- Default limits: 10,000 entries; 50 MiB materialized content; 1 MiB per blob; 100,000 diff lines; 60 seconds per operation; 512 MiB temporary fetch storage.
- Fetch storage is monitored, not an exact network-byte cap. Terminate the process group on exhaustion; polling permits overshoot. No automatic resource-limit retries.
- Verification: `go vet ./...`, `go test -race -count=1 ./...`, `go build ./...`. Synthetic fixtures only; never weaken tests or suppress checks to pass.
- Terminal usability requires human verification. Passing render tests does not prove accessibility.
- Terminal styling is presentation-only: it wraps already-escaped whole display lines after scrolling and clipping, never edits text, wording, labels, ordering, or width, and carries no meaning that the labels do not already carry. Removing every style must reproduce the uncolored render exactly.
- Color capability is detected, never probed or configured: honor the terminal profile and `NO_COLOR`/`CLICOLOR`/`CLICOLOR_FORCE`/`TERM=dumb`. Colorless profiles render today's text. `--plain` output is never colored and contains no terminal control sequences.
- Persist frozen actual patches and metadata independently of Git object lifetime; never rewrite a comparison/plan in place or automatically carry completion to a new version.
- Reopening checks freshness explicitly or labels it unknown. Failed checks never imply current revisions; stale sessions remain readable. No hidden polling.
- Store only app-owned local data outside the reviewed checkout, with private permissions, atomic replacement, integrity/reference validation, and a single-writer lock. Preserve corrupt/unsupported records; reject stale writers.
- Session deletion removes its final and interrupted-write artifacts, not unrelated files, external service records, backups, or guaranteed forensic traces. Raw stored source is not encrypted and may contain sensitive content.

# Phase 4 Analysis Contract

- Guide analysis uploads source only when the invocation passes `--send-source-to-openai` on `open`. The flag is the acknowledgement; there is no separate enable switch, no config file, and no default-on behavior. `resume`, `sessions`, and `delete` never construct an analyzer or contact a provider. This rule covers `internal/guide` only; the `internal/analysis` provider path has its own consent contract and no CLI entry point.
- Only the assembled `guide.Input` may leave the machine: pinned patches and pinned-tree evidence that pass the privacy policy a second time at the upload boundary. Excluded paths, credential-like content, and budget-exhausted units are withheld, and withheld path names are not sent either.
- Requests are one non-streaming HTTPS Responses call with `store: false`, a strict `pr_review_guides` JSON schema, a bounded request body, and a bounded response body. Redirects are not followed, so the bearer credential cannot reach another host. Plaintext HTTP endpoints are rejected except loopback for tests.
- The credential comes from `OPENAI_API_KEY` and is never persisted, logged, or placed in an error, reason, or rendered string. Provider text is sanitized and truncated before it becomes durable session content.
- Analysis never fails `open`. A missing analyzer, transport error, non-2xx status, refusal, deadline, oversize payload, or unusable structured output produces an `analysis_unavailable` bundle with a stated reason. Exit status still derives only from raw inventory completeness.
- Generated guides are immutable snapshot data with provider, model, prompt version, schema name, input digest, evidence IDs, limits, and the withheld ledger. Retrying analysis creates a new session; it never rewrites a stored bundle.
- Guides never own files. Sections reference validated unit IDs; `UnitFiles` remains the sole authority for reading progress, and every unit appears in guide navigation exactly once via the synthesized ungrouped guide.
- Model output is interpretation, not source truth, approval, security findings, or complete architectural documentation. Model ids are passed through to the provider rather than checked against a local allowlist.
- Provider tests use a local `httptest` endpoint and a fake credential only. No test may contact a real provider or use a live key.
