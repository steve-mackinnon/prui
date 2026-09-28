# Spec: SQLite Storage — Fresh Start

Status: Implemented and verified on macOS/Linux on 2026-09-26.
Evidence: [SQLite verification](../SQLITE-VERIFICATION.md).
Date: 2026-09-26
Scope id: sqlite-storage

## Objective

Replace the local JSON persistence backend with SQLite for newly created stores.
Preserve the application's frozen-review, reading-progress, guide-provenance,
and offline-access behavior. Reviewers should reopen a PR without accumulating
another copy of its large source payload, and marking a file read should update
only small state records.

This is one backend replacement. Session and guide persistence share one
consistency boundary and release together. Existing file-based caches and
sessions are not imported, converted, or supported by the new backend. The user
confirmed a completely fresh start and that Windows support can come separately;
this change ships on macOS and Linux.

## Assumptions and scope

- No compatibility with existing on-disk JSON stores is required. Do not build
  an importer, legacy reader, migration manifest, backup-copy workflow, or
  JSON/SQLite dual-write mode. New sessions receive new IDs.
- Delete and replace all existing file-based session, guide-cache, and repository
  registry persistence code. SQLite is the only persistence backend for those
  records; no file-backend switch, fallback, compatibility adapter, or obsolete
  read/write helper remains in the delivered implementation.
- Keep the current CLI commands with their default storage location. Writable
  opening of a new empty location initializes SQLite; read-only opening requires
  an existing initialized SQLite store and never creates one.
- Use a fresh default storage directory, separate from the old `sessions`
  directory. Old caches are ignored and left untouched, not automatically deleted.
  An old or unrelated nonempty default directory fails without import or deletion.
- Keep SQLite and platform-specific file operations behind a narrow storage
  boundary so Windows can be added later. Windows paths, ACLs, runtime tests,
  and CI are outside this change.
- Replace the lifetime single-writer restriction with short SQLite write
  transactions and optimistic session-generation checks. Multiple processes may
  open a SQLite store; conflicting state updates still fail visibly.
- No automatic expiry, guide-purge command, encryption, compression, remote
  storage, synchronization, or general database abstraction in this change.
- Preserve current freshness and guide-generation behavior. The audited stale
  description and cross-filesystem Git hard-link defects need separate fixes.

## Current behavior and constraints

`internal/session/store.go` owns storage policy and the public session types.
`review.Session` aliases `session.Record`; CLI and TUI callers use concrete
`*session.Store` handles. Preserve those domain types where practical.

Current storage uses a version-1 `.format` marker, a lifetime `.lock`, per-session
`snapshot.json` and `state.json`, `guides/<digest>.json`, and `repositories.json`.
Snapshots are immutable; state saves check a snapshot digest and generation.
The existing `Save` reads and serializes large snapshots even for progress-only
updates. `LatestComparison` orders state records but must load candidate source
payloads; `List` retains every decoded snapshot. Cached opening creates a full
new raw snapshot and may create a second guided copy.

`CONSTRAINTS.md` remains authoritative until implementation explicitly updates
its storage mechanics to this approved specification. Preserve pinned source,
privacy, no-network offline behavior, corruption visibility, and stale-writer
rejection. The approved replacement for its lifetime lock is transaction-level
writer exclusion plus generation checks; that is an intentional contract change.

## Tech stack and storage configuration

Use Go 1.26.8, `database/sql`, and a pinned CGo-free SQLite driver. Proposed driver:
`modernc.org/sqlite`; v1.59.0 is a reviewed documentation candidate, not an approved
installation or a claim that it is the newest or vulnerability-free release.
Before pinning, verify its exact Go/toolchain and target-platform compatibility,
license, bundled SQLite version, transitive dependencies, and vulnerability scan.
No runtime `sqlite3` executable, server, ORM, or C compiler requirement.
The driver documents its CGo-free implementation in its [official package
reference](https://pkg.go.dev/modernc.org/sqlite).

Proposed initial connection policy:

- Database file: `<store>/store.sqlite3`, owned by this application.
- Rollback journal (`journal_mode=DELETE`), `synchronous=EXTRA`, foreign keys on,
  bounded busy waiting (5 seconds), and cancellable database calls. Verify the
  effective settings on every new connection rather than assuming defaults.
- Start with one database connection per Store handle. Independent processes
  may have their own handles. Close result sets before another operation needs
  that connection; do not hold a transaction over UI, Git, or provider work.
- Serialize each logical mutation in one short write transaction. On any error,
  roll back and return a typed conflict/busy/storage error; do not retry an
  operation with uncertain commit outcome as if it definitely failed.
- Read-only opens use `mode=ro`, never `immutable=1` against a live database, and
  perform no initialization, schema upgrades, repair, or persistent PRAGMA writes.
  If crash recovery requires a write, return a clear instruction to reopen with
  a writable command.
- Set and validate an application identifier and a separate database-schema
  version. Payload format versions and guide prompt versions remain distinct.
  Unknown newer schemas fail closed without changing the database.

Rollback journaling is an initial simplicity choice, not a performance claim.
SQLite permits multiple readers and one writer; transactions must remain short
([transaction documentation](https://www.sqlite.org/lang_transaction.html)).
`EXTRA` is chosen to retain durable commits in DELETE journal mode according to
SQLite's [synchronous setting documentation](https://www.sqlite.org/pragma.html#pragma_synchronous).
WAL can be assessed later if measured contention justifies its lifecycle cost.

## Data model and invariants

The table names below define responsibilities; exact DDL and indexes belong in
the subsequent implementation plan. Prefer explicit SQL and a small schema.

| Entity | Contents and rules |
| --- | --- |
| `snapshots` | Content-addressed immutable source payload: inventory, bounded context, slices, unit ownership, and frozen description. Excludes session checkout hints, guide bundles, and parent-session links. Stores payload format, digest, indexed comparison identity, and completeness metadata. |
| `snapshot_files` | Small file-ID/ordinal membership records for a snapshot, allowing progress validation without reading patch blobs. Unique per snapshot/file. |
| `guide_bundles` | Immutable content-addressed bundle payloads. Session-attached unavailable results remain valid historical records but are never reusable cache hits. |
| `sessions` | Stable newly generated session ID, snapshot reference, optional bundle reference, immutable checkout hint and derived-from provenance, freshness, timestamps, generation, and logical snapshot reference. |
| `progress` | Reviewed file IDs and their preserved order per session. Unique membership; validated against that session's snapshot. |
| `guide_cache` | Normalized repository, PR, base/head SHAs, inventory identity, current prompt version, and a bundle reference. Only successful validated bundles qualify. |
| `repositories` | Normalized repository, remembered checkout, and durable listing order. Paths remain machine-specific hints. |

Rules:

1. Preserve patch/path bytes, including invalid UTF-8 paths, nil-versus-empty
   description semantics, metadata, provenance, and progress order.
   JSON blobs can retain the existing byte-slice encoding. No lossy conversion
   of paths to SQL text or database-wide collation rules for source paths.
2. Hash a versioned canonical source payload for deduplication; equal commit
   SHAs alone do not establish equal inventory, limits, context, or description.
   Identical raw and guided reopenings share one source payload even when their
   session IDs, checkout hints, or guide links differ.
3. Reconstruct the existing logical Snapshot on load. Session-visible immutable
   fields cannot be changed through a progress save. Define SnapshotReference
   from a versioned canonical representation of the immutable logical snapshot;
   it remains stable across subsequent loads and state updates.
4. Keep domain validation and digest checks at create/load boundaries.
   Foreign keys and SQLite integrity checks do not replace inventory validation.
   Compare indexed identity fields with the validated payload before cache reuse.
5. `DerivedFrom` is historical provenance, not a cascading ownership link.
   Creating a derived session validates its live parent. Later deletion of a
   parent cannot erase or make its child unreadable.
6. A progress/freshness update atomically compares expected generation and
   immutable reference, validates file membership, applies state, and increments
   generation. Exactly one of two stale competing updates succeeds. Numeric
   range/overflow errors must be explicit, never silently truncate values.
7. A guide cache hit requires the full comparison identity, compatible inventory
   and prompt, valid bundle checksum, and existing structural validation. No
   network call occurs. Older prompt bundles remain readable in saved sessions.
8. Database corruption and I/O failures are visible storage errors; invalid
   individual guide artifacts are misses. No corrupt database is silently
   replaced with an empty store. Preserve corrupt session rows for diagnosis.
9. Deleting a session removes its progress and live references transactionally.
   Reclaim only source/bundle rows with no session or cache references. Guide
   cache entries and other sessions remain intact.
   This is logical deletion, not a promise to erase freed pages or backups;
   shrinking the database is not automatic or performed on each deletion.

## API and lookup behavior

Provide Create/Load, guide-cache, registry, path, close, and read-only operations
through SQLite at the session boundary. Preserve application-level semantics,
not the existing filesystem implementation or persistence API where it obstructs
the replacement. SQL connection/transaction types must not leak into the TUI or
application orchestration.

Introduce a narrow state-only update method and route progress/freshness callers
through it. The existing mutable `Record` API cannot both detect arbitrary
snapshot edits and avoid serializing that snapshot. Replace `Save(*Record)` with
the explicit state-only contract and update all callers and behavioral tests in
this work. Do not retain a compatibility Save method. The replacement must make
immutable snapshot updates impossible through the state API, validate the stored
reference and generation, and validate progress membership transactionally.

Style sketch, not committed implementation:

```go
type StateUpdate struct {
    ReviewedSliceIDs []string
    RevisionStatus  RevisionStatus
}

func (s *Store) UpdateState(ctx context.Context, id string,
    expectedGeneration uint64, snapshotReference string,
    update StateUpdate) (State, error)
```

Use Go formatting, typed errors, parameterized SQL, explicit rollback paths, and
no source-bearing debug logs. Apply returned state to UI-owned objects only
following successful commit; workers continue returning results to the event loop.

Add a metadata-summary listing path for session picker/CLI consumers. Fetch
patches only when a session opens. Lookup indexes cover normalized repository,
PR, revision pair, and session recency with a deterministic session-ID tie break.
Candidate validation skips invalid records but never falls back to scanning
unrelated large payloads. Ordinary listing is metadata-only; deep corruption is
reported on load, while already known invalid records remain visible in lists.
Document this change from eager full validation rather than claiming unchanged
validation timing. Session summaries are not proof of source integrity.

## Fresh-store initialization and lifecycle

The default paths are:

- macOS: `~/Library/Application Support/prui/storage/store.sqlite3`.
- Linux: `$XDG_DATA_HOME/prui/storage/store.sqlite3` when XDG_DATA_HOME is
  absolute; otherwise `~/.local/share/prui/storage/store.sqlite3`.

These defaults deliberately use `storage` instead of the former `sessions`
subdirectory. There is no scan, fallback read, or transfer from the old location.
The CLI reports its active storage location as it does today.

Writable startup recognizes only a new empty app-owned location or an existing
SQLite store with the expected application identifier and supported schema.
A legacy `.format`/JSON store, unrelated nonempty directory, or foreign database
is rejected unchanged. Do not reuse a legacy ownership marker to authorize
SQLite initialization in a populated directory.

Initialize the schema and its application/version metadata transactionally.
Concurrent first opens must converge on one initialized database, never observe
partial tables, overwrite another creator's database, or discard committed data.
Test interrupted initialization explicitly: complete only an identifiable,
empty app-owned initialization attempt; reject ambiguous artifacts unchanged.
A corrupt or missing database in an established store is an error, not an excuse
to silently start over. The exact initialization ownership protocol belongs in
the implementation plan; it must not require a lifetime application writer lock.

After creation, SQLite is the sole live source. No legacy persistence code is
retained for on-disk compatibility. Existing file-store tests should be replaced
with equivalent SQLite behavioral tests; import/old-format round-trip tests are
not requirements for this fresh-start backend.

Read-only opening never creates a missing directory/database or initializes a
schema. SQLite crash recovery that requires writes must be performed by an
explicit writable open. Future SQLite schema upgrades, when needed, must be
versioned and transactional; there is no historical SQLite schema to migrate in
this first version. Unsupported newer schema versions are rejected unchanged.

## Portability and privacy

- Use the fresh macOS/Linux paths above; resolve a valid Linux XDG data path
  without unnecessarily requiring HOME. Windows
  path selection will be specified in the separate Windows effort.
- Keep data outside the checkout and in private local storage. Protect database,
  journal and any initialization artifacts. Retain 0700/0600 checks and
  reject unsafe symlinks without following external targets. Isolate these Unix
  policies from SQL logic; do not claim protections against malicious same-user
  edits or assume Unix mode bits will enforce privacy on Windows.
- SQL storage tests must run on actual macOS and Linux. Keep the narrow database
  engine independent of Git subprocess code and its Unix dependencies, providing
  a testable boundary for the future Windows port.
- Windows compilation, data paths, ACL/reparse handling, CI/runtime tests, Git
  environment/process handling, terminal support, and case/drive-aware checkout
  containment are follow-up work. Cross-filesystem Git object copying is also
  separate from this storage replacement. A portable SQLite dependency alone is
  not evidence that the application supports Windows.
- No network/shared filesystem guarantee, telemetry, persisted credentials,
  provider transcripts, comment drafts, or encryption is introduced.

## Project structure

Proposed ownership, subject to sizing in the implementation plan:

```text
internal/session/store.go                 domain-facing Store and validation
internal/session/types.go                 existing public record types, if extracted
internal/session/state.go                 state-only updates and lookup integration
internal/session/storage/                 independent SQL engine, schema, platform I/O
internal/session/storage/*_test.go         native portable persistence tests
internal/session/*_test.go                 domain and persistence behavior tests
internal/review/lifecycle.go               state-only save callers
cmd/prui/{main,lifecycle}.go          startup reporting and summary listing
internal/tui/                             summary picker/state-save callers as needed
CONSTRAINTS.md, TESTING.md, docs/REFERENCE.md  approved contract and fresh-store guidance
.github/workflows/verify.yml              existing macOS/Linux verification matrix
```

No broad domain-model rewrite or framework solely to swap backends. Remove the
superseded JSON persistence implementation once its callers use SQLite; retain
useful domain validation, not an unused compatibility backend.

The removal includes per-session snapshot/state file readers and writers,
`guides/*.json` cache I/O, `repositories.json` I/O, legacy `.format` handling,
the lifetime `.lock` writer mechanism, and persistence-only atomic-file and
directory-sync helpers. Replace their integrity/concurrency responsibilities
with SQLite transactions and domain checks. Remove obsolete fixtures, tests,
and documentation tied solely to that backend, while retaining equivalent
behavioral coverage for the replacement. Shared helpers with other live users
must be kept with those owners rather than deleted indiscriminately.

JSON encoding inside SQLite payload columns is permitted; it is not a separate
file cache. This change concerns session/guide/registry persistence. Independent
theme configuration, temporary Git source acquisition, and in-memory rendering
caches are outside this backend replacement.

## Testing strategy and success criteria

Use Go's existing testing framework, temporary private stores, synthetic records,
real SQLite files, and subprocess crash tests. Do not open the user's real store.

| ID | Acceptance evidence |
| --- | --- |
| S1 | New SQLite stores round-trip sessions, progress, guide bundles, repository order, and invalid-byte paths after process restart. |
| S2 | Fresh-store fixtures cover nil/empty descriptions, unavailable and old-prompt guides, deleted parents, corruption, and unsupported SQLite schemas. Selecting a legacy or unrelated store rejects it unchanged. |
| S3 | Concurrent first opens and interrupted initialization never expose partial schema or overwrite committed data. Crashes during later writes leave either the previous committed state or the complete new state. No tests require importing old stores. |
| S4 | Two independent processes can open a new-format store; competing updates with one expected generation yield exactly one success. Busy timeout, cancellation, disk-full, and commit errors are bounded and visible. |
| S5 | State-only saving reads/writes no snapshot blob and offers no immutable-field updates. All former Save callers use the new contract; no compatibility Save or old file-persistence code remains. Invalid file IDs cannot enter progress. |
| S6 | Reopen the same PR 100 times, including raw/guided sessions and differing checkout hints: one identical source payload is stored, session IDs/progress remain independent, and changed source/description/context creates a distinct payload. |
| S7 | Lookup excludes unrelated PR payloads by indexed query. Listing decodes zero source blobs. Add query-plan and payload-read assertions, and compare existing history benchmarks on the same machine. |
| S8 | Guide hits/misses retain current comparison/prompt/validation rules and never contact a provider. Guide cache rows survive session deletion. |
| S9 | Deleting one of several sharing sessions preserves the rest; deleting the last reference reclaims only unreferenced content. |
| S10 | Read-only SQLite opens create/change no persistent artifacts, cannot mutate, and require a writable recovery open for a hot journal. |
| S11 | Private permissions and symlink rejection are verified natively on macOS/Linux, including spaces, Unicode, and URI-significant path characters. Do not interpolate filesystem paths into unescaped SQLite URIs. |
| S12 | Existing offline/resume/guide/CLI/TUI/PTY behavior and new storage tests pass on macOS and Linux. Windows implementation and verification are explicitly deferred. |

Measure allocated memory, bytes read, logical source-blob counts, and elapsed
time for large snapshots/history. Avoid arbitrary timing thresholds; structural
requirements S5-S7 are mandatory and benchmark regressions must be explained.
Use the existing 128 MiB payload bound; enforce bounds before allocating blobs
and preserve all lower source/context limits. Tests must include a multi-record
history and insufficient-space failure without silent eviction.

## Commands

Existing full application checks (macOS/Linux):

```sh
go vet ./...
go test -race -count=1 -timeout=5m ./...
go build ./...
./scripts/verify.sh
golangci-lint run ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
git diff --check
```

Focused commands after the proposed package exists:

```sh
go test -race -count=1 ./internal/session/storage ./internal/session ./internal/review ./cmd/prui
go test ./internal/session -run '^$' -bench BenchmarkLatestComparisonHistory -benchmem -count=5
CGO_ENABLED=0 go test -count=1 ./internal/session/storage
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/prui-linux ./cmd/prui
```

Cross-compilation is a build check, not a native runtime test.
No implementation or dependency checks are claimed to have run for this draft.

## Boundaries

- Always: validate new/read records, preserve domain validation and generation
  conflicts, use transactions, keep SQL behind Store, and run required checks
  without lowering the quality bar. Replace file-format-specific tests with
  equivalent SQLite behavior checks rather than preserving obsolete formats.
- Always: document changed locking/listing semantics, the fresh default location,
  and the absence of legacy data import when SQLite becomes the default.
- Always: delete the superseded filesystem backend and its obsolete support
  code/tests/docs as part of the replacement, with equivalent SQLite behavioral
  coverage. Removing that code is explicitly authorized by the user.
- Ask first: expand to Windows implementation/testing, automatic deletion/retention,
  encryption, remote stores, existing-data import, or a materially
  different public workflow. Approval of this specification covers the stated
  schema/backend/dependency changes; routine implementation choices within
  that scope do not need repeated approval.
- Never: modify the user's live store during development, import or delete old
  filesystem caches automatically, dual-write JSON and SQLite, weaken behavioral
  tests, or claim application-wide Windows support from database-only checks.

## Review decisions and next gate

User-confirmed scope: start completely fresh, delete and replace the existing
file-based persistence code with SQLite, retain no compatibility backend, and
defer Windows support. Proposed remaining decisions: shared immutable
source payloads, state-only updates, short concurrent transactions, DELETE/EXTRA
journaling, and a new default `storage` directory alongside the old `sessions`
location. Pin the driver after compatibility and dependency verification during
implementation planning.

After human review of this spec, follow the planning-and-task-breakdown skill to
add the ordered implementation plan to `tasks/plan.md`; after plan review, add
small acceptance-tested tasks to `tasks/todo.md`, preserving existing sections.
Implementation tasks are recorded in the SQLite sections of `tasks/plan.md` and
`tasks/todo.md`; the user approved the plan and Sol-agent implementation.
