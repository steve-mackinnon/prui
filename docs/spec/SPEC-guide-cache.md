# Spec: Guide Cache

## Objective

Avoid repeating an OpenAI guide request when a reviewer reopens the same
immutable pull-request comparison from the interactive PR list. A cache hit
returns a locally stored, structurally valid generated guide; it never contacts
the provider. The user sees a guide for the pinned comparison without changing
raw inventory, source safety, or reading-progress semantics.

Only `guide.Generated` bundles are reusable. An `analysis_unavailable` bundle
remains in its originating derived session for transparency but is not cached;
a later PR-list open may retry it.

## Tech Stack

Go 1.26.8. Reuse `internal/session` for private, atomic local storage,
`internal/guide.Validate` for structural validation, and the immutable metadata
already retained in `inventory.Comparison.Metadata`.

## Commands

```sh
go test ./internal/session ./internal/guide -count=1
go test -race ./internal/session ./internal/guide -count=1
go vet ./...
go test -race -count=1 ./...
go build ./...
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/session/store.go          private cache lookup/write API and validation
internal/session/store_test.go     durable cache hit/miss/corruption coverage
internal/guide/*.go                existing guide validation and provenance
```

## Code Style

Keep cache identity explicit and pass typed comparison metadata rather than
constructing ad-hoc path strings at callers:

```go
type GuideCacheKey struct {
    Repository string
    Number     int
    BaseSHA    string
    HeadSHA    string
}

func (s *Store) LoadGeneratedGuide(key GuideCacheKey, inv inventory.Inventory) (*guide.Bundle, error)
```

The storage package hashes a canonical key for the on-disk name, uses existing
private permissions and atomic replacement helpers, and validates a read bundle
against the current inventory before returning it.

## Testing Strategy

- Unit-test cache key normalization and distinction between base/head pairs.
- Test durable write then reopen through a new store instance.
- Test that only generated bundles are written and that unavailable bundles do
  not suppress a later generation.
- Test corrupt, symlinked, mismatched, or inventory-invalid artifacts fail
  closed and are never applied as guides.
- Use generated local fixtures only; no provider or GitHub access.

## Boundaries

- Always: store cache data outside the checkout with private permissions,
  atomic writes, integrity checks, and structural validation.
- Always: key the full immutable comparison (repository, PR number, base SHA,
  head SHA), not only a session ID or head SHA.
- Ask first: change the existing session schema/format, add a dependency, or
  expose cache-management CLI commands.
- Never: persist credentials, provider transcripts, unavailable results as a
  reusable cache hit, or source outside the existing bounded guide bundle.

## Success Criteria

- A valid generated guide persists on disk and can be loaded after restart.
- The same repository/PR/base/head comparison is a cache hit with no provider
  invocation.
- A changed base or head is a cache miss.
- Invalid or unavailable artifacts never become a guide and do not prevent a
  retry.

## Open Questions

None. Prompt/model changes do not invalidate a generated guide in this scope;
the persisted bundle continues to disclose its provenance.
