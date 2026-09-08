# pr-review

Read-only, pinned GitHub PR review in a local terminal. Deterministic file slices, bounded pinned repository evidence, durable local reading progress, and explicit revision checks. No generated analysis, model configuration, server, or telemetry. Phases 1-3 implemented.

## Run

Building requires Go 1.26.8+. Opening a new comparison requires Git and authenticated GitHub CLI (`gh`); resuming a frozen snapshot does not require Git or its checkout. Missing `gh`/authentication/connectivity leaves resume usable with unknown freshness. Dependencies are pinned in `go.mod`/`go.sum`: Bubble Tea v2.0.9 and Lip Gloss v2.0.2. macOS/Linux only initially.

```sh
go run ./cmd/pr-review open https://github.com/owner/repo/pull/42 --repo /path/to/checkout
go run ./cmd/pr-review open 42 --repo /path/to/checkout --github-repo owner/repo --plain
go run ./cmd/pr-review open 42 --repo /path/to/checkout --github-repo owner/repo --exclude 'secrets/*'
go run ./cmd/pr-review sessions
go run ./cmd/pr-review resume SESSION_ID
go run ./cmd/pr-review resume SESSION_ID --offline --plain
go run ./cmd/pr-review resume SESSION_ID --new --repo /path/to/checkout
go run ./cmd/pr-review delete SESSION_ID
go build -o pr-review ./cmd/pr-review
```

`open`, normal `resume`, and explicit refresh contact GitHub. `sessions`, `delete`, and `resume --offline` do not. Implementation tests use synthetic objects and mocked GitHub only. No source is uploaded to a model or other service. Git fetch sends requested revision IDs and Git protocol negotiation to GitHub; metadata/authentication use the user's trusted `gh` installation.

`--plain` emits escaped text without a pager, color, terminal control sequences, or interaction. It includes the bounded evidence scope report. It is automatic for redirected stdin/stdout or `TERM=dumb`. Exit codes: 0 successful raw inventory, 1 invalid input/open/runtime failure, 2 unavailable review content, 130 canceled loading. Exit 0 is not approval or evidence that you read the review.

## Keyboard

| Key | Action |
| --- | --- |
| `n` / `p` | Next / previous review unit |
| `]` / `[` | Next / previous file slice |
| `ctrl+h` / `ctrl+l` | Focus list / diff |
| `enter` / `esc` | Open selected item / go back |
| Up / down, `j` / `k` | Navigate list or scroll focused diff |
| `J` / `K` | Scroll diff by 5 lines |
| Page Down / Page Up | Page through actual diff |
| `h` / `l`, left / right | Horizontal scrolling; no hidden line truncation in plain output |
| Home | Reset selected unit's scroll |
| `i` | Toggle full unit inventory |
| `e` | Show bounded evidence and included/excluded scope |
| `g` | Show canonical GitHub URL for copying; does not launch a browser |
| `m` | Mark/unmark the selected file slice; saves immediately, including when viewing one of its units |
| `r` | Explicit metadata refresh; no diff recomputation or polling |
| `N` | Start a new comparison with empty progress; retain the old session |
| `s` | Session picker; up/down to select, Enter to resume, Esc to return |
| `?`, `q`, Ctrl+C | Help, quit, cancel loading |

Selection and per-unit vertical offsets survive resizing; navigation positions are not persisted across processes. No mouse capture, so terminal-native text selection remains available. Textual markers and labels are primary; reverse video adds a focused-row cue, and no information depends on color alone. Extremely small terminals clip controls; enlarge or use plain output. The user approved Phase 1 terminal behavior; broad theme/platform/accessibility coverage is not established.

## Sessions And Freshness

Every `open` saves a new frozen session, even with `--plain`. `sessions` lists full IDs, progress, and historical check status. Resume loads stored patches and metadata, not the checkout: branch deletion, garbage collection, or deleting the checkout cannot change the review. Full unchanged source context is not retained in Phase 2 and is explicitly labeled unavailable.

- `unchecked`: no freshness check this opening (`--offline`, or a check interrupted before completion).
- `current`: base/head SHAs and repository identities matched at the last explicit check, not a continuous guarantee.
- `stale`: base/head or repository identity changed. Keep navigating the old snapshot, or press `N` / use `resume ID --new` for a new unreviewed session.
- `check_failed`: metadata/authentication/connectivity failed; freshness unknown, frozen review still available.

Resume checks metadata by default; `--offline` disables all network actions for that invocation. New comparisons use the saved absolute checkout path unless `resume --repo` overrides it. Failed replacement preserves the old snapshot/progress. Every new session or plan version starts unreviewed, even with identical patch bytes; Phase 2 exposes only the deterministic `file-v1` plan. Local completion never means GitHub approval or complete source availability.

Phase 3 evidence is read only from pinned Git tree/blob objects. It prioritizes manifests, documentation, nearby tests, and changed directories within 100 files, 32 KiB per excerpt, 256 KiB retained bytes, and five seconds. Press `e` to inspect retained evidence and omissions. Relationships are observational unless explicitly marked otherwise; AST indexing and history expansion are deferred. Exclusions are local policy, and credential filters are defense in depth rather than secret detection.

## Local Storage And Deletion

The CLI prints storage location; TUI help shows storage and session ID. Defaults:

- macOS: `~/Library/Application Support/pr-review/sessions`
- Linux: `$XDG_DATA_HOME/pr-review/sessions`, or `~/.local/share/pr-review/sessions` when unset/non-absolute

All commands accept `--store /private/directory`, outside the reviewed checkout. Use a new/empty directory or an existing app-owned store; unrelated nonempty directories and symlink roots are rejected. Directories use 0700, files 0600. One process holds the store's advisory writer lock for its lifetime; another invocation fails visibly until it exits. OS process exit releases the lock; do not delete `.lock` to bypass it. Use a local filesystem with working advisory locks, rename, and fsync semantics.

Each session directory contains immutable `snapshot.json` (comparison, inventory, byte-preserving patches, file plan, checkout hint) and replaceable `state.json` (schema version, snapshot SHA-256 reference, reviewed slice IDs, freshness, generation, timestamp). Patch references are content hashes. Saves use private temporary files, file sync, atomic replacement, and directory sync; generation checks reject outdated updates. JSON files are bounded to 128 MiB each. Corrupt/unsupported records remain untouched and appear as unreadable entries, never silently migrated or replaced.

Stored raw patches may contain sensitive source, including secrets already present in the PR. Storage is local and permission-restricted, not encrypted; checksums detect corruption, not malicious same-user rewriting. No credentials from `gh`, raw provider payloads, or telemetry are stored. Sessions remain until explicitly deleted; listing/loading currently reads snapshots eagerly, so many large sessions can consume substantial memory/time. Large-review lazy loading remains Phase 5 work.

`delete ID` removes that session's snapshot, state, and interrupted `.write-*` files, including unreadable sessions. No confirmation prompt: the explicit command is the deletion request. It never removes another session, GitHub state, provider records, backups, or guaranteed forensic disk traces. Incomplete creation directories remain inspectable/deletable rather than silently erased. Normal source-fetch temporary storage is cleaned independently; a kill/power loss before a session exists can leave unassociated `pr-review-*` directories in the OS temp directory, as in Phase 1.

## Source Truth And Safety

- GitHub metadata pins base/head repository identities and 40-character SHA-1 commit IDs. Compare the single verified merge base to head, not base tip to head. Ambiguous histories fail visibly.
- Git runs in private tool-owned bare storage, with sanitized environment/configuration. Local object files are hard-linked into an isolated borrowed view, never rewritten. Hooks, repository config, replacement refs, promisor markers, and alternates are not imported. Worktree Git pointers are supported; bare checkout inputs and object-directory symlinks are rejected. Cross-filesystem hard-link failures are explicit, not copied silently.
- Missing ancestry triggers visible cancellable HTTPS fetches of the exact pinned base/head into the private object store, without borrowed reachability, tags, submodules, maintenance, or checkout. Metadata is read again after fetch; one changed-pin retry is allowed, then failure. Authentication/resource failures do not retry automatically.
- Fetch credentials come from `gh auth token`, then an environment-only, host-scoped Git HTTP authorization header. No token in argv, diagnostics, or files. Redirects and credential helpers are disabled. `gh`, Git, their installed runtime helpers, and the user's executable search path are trusted; this is not a sandbox against a compromised Git binary or local same-user attacker.
- Raw tree metadata is NUL-delimited with full object IDs. Path fields and patch artifacts use byte arrays (lossless base64 in JSON). Display escapes controls, invalid UTF-8, and backslashes; raw stored bytes remain unchanged. Symlinks are blob content, never dereferenced. Submodule pointers show OIDs without reading submodule repositories.
- Every file has a separate metadata unit. Text units contain real Git blob-to-blob hunks: Git's headers name object IDs and use blob mode, while the adjacent file metadata is authoritative for paths, modes, additions, deletions, and renames. No authored/reconstructed patch lines. Binary content uses explicit cards (NUL in the first 8 KiB); Git LFS pointers are reviewed as committed text, not downloaded.
- Local progress is persisted separately from immutable source content. Temporary metadata/object storage is removed on normal completion or cancellation. SIGKILL, machine failure, or power loss can leave private `pr-review-*` directories under the OS temporary directory; deletion is not guaranteed forensic erasure.

## Limits And Incomplete States

Defaults: 10,000 changed entries, 50 MiB retained patch content, 1 MiB per blob, 100,000 retained diff lines, 60 seconds per Git/network operation, and 512 MiB fetched object storage. Fixed Myers diff, three context lines, no indent heuristic, 50% rename detection with 10,000 candidate limit. Settings/limits participate in inventory identity.

Entry/raw-record limits or missing comparison trees fail the comparison rather than offer a misleading partial file list. Oversized or unavailable blobs/patches remain explicit unavailable units; inventory is labeled INCOMPLETE. Binary/gitlink cards represent known non-text changes, not absent entries. Zero changes is an explicit empty comparison. Inventory completeness is not analysis completeness or reading progress.

Process stdout/stderr are bounded; overrun, timeout, or cancellation kills the process group. Fetch storage is sampled every 10 ms plus checked after each fetch. It can overshoot between samples or during filesystem scans, and counts stored bytes rather than network download bytes. Borrowed hard-linked objects are not counted as downloaded storage. No exact download cap or total process-memory bound is claimed.

## Verification

```sh
go vet ./... && go test -race -count=1 ./... && go build ./...
go test ./internal/source ./internal/inventory -run 'Test(Pinned|Safety|Inventory)'
go test ./internal/tui -run TestRawReview
go test -race ./internal/session ./internal/review ./internal/tui
go test ./cmd/pr-review -run 'Test(Lifecycle|Storage|Options)'
```

Tests construct disposable trusted Git fixtures, including byte-only paths that macOS cannot materialize. Lifecycle tests cover process restart/interruption, deleted checkouts, concurrent writers, stale revisions, offline/auth failures, corruption, permissions, deletion, and keyboard actions. They never execute reviewed scripts or use live credentials. See `CONSTRAINTS.md` for the correctness contract. Phase 3+ (evidence retrieval, models, editable contextual slices) is intentionally absent.
