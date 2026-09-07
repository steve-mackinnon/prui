# Phase 1 Contract

- Raw source comes only from pinned committed objects, never working files or model output.
- Compare the unique, ancestry-verified merge base to head. Reject ambiguous or unresolved comparisons.
- Account for every raw change and every unit exactly once. Limits produce explicit incomplete/unavailable states, never an empty-success substitute.
- Never execute reviewed code, hooks, filters, textconv, external diff, credential helpers, submodule commands, or repository-selected network helpers.
- Never mutate the checkout, index, refs, configuration, or object files. Temporary isolated storage is tool-owned and removed on completion/cancellation.
- No model integration, telemetry, source upload, review writes, or persisted credentials in Phase 1.
- Default limits: 10,000 entries; 50 MiB materialized content; 1 MiB per blob; 100,000 diff lines; 60 seconds per operation; 512 MiB temporary fetch storage.
- Fetch storage is monitored, not an exact network-byte cap. Terminate the process group on exhaustion; polling permits overshoot. No automatic resource-limit retries.
- Verification: `go vet ./...`, `go test -race -count=1 ./...`, `go build ./...`. Synthetic fixtures only; never weaken tests or suppress checks to pass.
- Terminal usability requires human verification. Passing render tests does not prove accessibility.
