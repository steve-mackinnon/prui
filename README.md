# pr-review

Phase 1: read-only, pinned GitHub PR review in a local terminal. Deterministic file slices and actual Git unified diffs; no generated analysis, progress storage, model configuration, server, or telemetry.

## Run

Requires Go 1.26.8+, Git, and authenticated GitHub CLI (`gh`). Dependencies are pinned in `go.mod`/`go.sum`: Bubble Tea v2.0.9 and Lip Gloss v2.0.2. macOS/Linux only initially.

```sh
go run ./cmd/pr-review open https://github.com/owner/repo/pull/42 --repo /path/to/checkout
go run ./cmd/pr-review open 42 --repo /path/to/checkout --github-repo owner/repo --plain
go build -o pr-review ./cmd/pr-review
```

These commands contact GitHub when you run them. Implementation tests use synthetic objects and mocked GitHub only. No source is uploaded to a model or other service. Git fetch sends requested revision IDs and Git protocol negotiation to GitHub; metadata/authentication use the user's trusted `gh` installation.

`--plain` emits escaped text without a pager, color, terminal control sequences, or interaction. It is automatic for redirected stdin/stdout or `TERM=dumb`. Exit codes: 0 successful raw inventory, 1 invalid input/open/runtime failure, 2 unavailable review content, 130 canceled loading. Exit 0 is not approval or evidence that you read the review.

## Keyboard

| Key | Action |
| --- | --- |
| `n` / `p` | Next / previous review unit |
| `]` / `[` | Next / previous file slice |
| `tab` / `enter` | Switch focus; switch visible pane below 100 columns |
| Up / down | Navigate list or scroll focused diff |
| `j` / `k`, Page Down / Page Up, Space | Scroll / page actual diff |
| `h` / `l`, left / right | Horizontal scrolling; no hidden line truncation in plain output |
| Home | Reset selected unit's scroll |
| `i` | Toggle full unit inventory |
| `g` | Show canonical GitHub URL for copying; does not launch a browser |
| `?`, `q`, Ctrl+C | Help, quit, cancel loading |

Selection and per-unit vertical offsets survive resizing. No mouse capture, so terminal-native text selection remains available. The UI uses textual focus, kind, and warning labels rather than relying on color. Extremely small terminals clip controls; enlarge or use plain output. Terminal readability, themes, text selection, and assistive technology still need human assessment.

## Source Truth And Safety

- GitHub metadata pins base/head repository identities and 40-character SHA-1 commit IDs. Compare the single verified merge base to head, not base tip to head. Ambiguous histories fail visibly.
- Git runs in private tool-owned bare storage, with sanitized environment/configuration. Local object files are hard-linked into an isolated borrowed view, never rewritten. Hooks, repository config, replacement refs, promisor markers, and alternates are not imported. Worktree Git pointers are supported; bare checkout inputs and object-directory symlinks are rejected. Cross-filesystem hard-link failures are explicit, not copied silently.
- Missing ancestry triggers visible cancellable HTTPS fetches of the exact pinned base/head into the private object store, without borrowed reachability, tags, submodules, maintenance, or checkout. Metadata is read again after fetch; one changed-pin retry is allowed, then failure. Authentication/resource failures do not retry automatically.
- Fetch credentials come from `gh auth token`, then an environment-only, host-scoped Git HTTP authorization header. No token in argv, diagnostics, or files. Redirects and credential helpers are disabled. `gh`, Git, their installed runtime helpers, and the user's executable search path are trusted; this is not a sandbox against a compromised Git binary or local same-user attacker.
- Raw tree metadata is NUL-delimited with full object IDs. Path fields and patch artifacts use byte arrays (lossless base64 in JSON). Display escapes controls, invalid UTF-8, and backslashes; raw stored bytes remain unchanged. Symlinks are blob content, never dereferenced. Submodule pointers show OIDs without reading submodule repositories.
- Every file has a separate metadata unit. Text units contain real Git blob-to-blob hunks: Git's headers name object IDs and use blob mode, while the adjacent file metadata is authoritative for paths, modes, additions, deletions, and renames. No authored/reconstructed patch lines. Binary content uses explicit cards (NUL in the first 8 KiB); Git LFS pointers are reviewed as committed text, not downloaded.
- No local progress is persisted in Phase 1. Temporary metadata/object storage is removed on normal completion or cancellation. SIGKILL, machine failure, or power loss can leave private `pr-review-*` directories under the OS temporary directory; deletion is not guaranteed forensic erasure.

## Limits And Incomplete States

Defaults: 10,000 changed entries, 50 MiB retained patch content, 1 MiB per blob, 100,000 retained diff lines, 60 seconds per Git/network operation, and 512 MiB fetched object storage. Fixed Myers diff, three context lines, no indent heuristic, 50% rename detection with 10,000 candidate limit. Settings/limits participate in inventory identity.

Entry/raw-record limits or missing comparison trees fail the comparison rather than offer a misleading partial file list. Oversized or unavailable blobs/patches remain explicit unavailable units; inventory is labeled INCOMPLETE. Binary/gitlink cards represent known non-text changes, not absent entries. Zero changes is an explicit empty comparison. Inventory completeness is not analysis completeness or reading progress.

Process stdout/stderr are bounded; overrun, timeout, or cancellation kills the process group. Fetch storage is sampled every 10 ms plus checked after each fetch. It can overshoot between samples or during filesystem scans, and counts stored bytes rather than network download bytes. Borrowed hard-linked objects are not counted as downloaded storage. No exact download cap or total process-memory bound is claimed.

## Verification

```sh
go vet ./... && go test -race -count=1 ./... && go build ./...
go test ./internal/source ./internal/inventory -run 'Test(Pinned|Safety|Inventory)'
go test ./internal/tui -run TestRawReview
```

Tests construct disposable trusted Git fixtures, including byte-only paths that macOS cannot materialize. They never execute reviewed scripts or use live credentials. See `CONSTRAINTS.md` for the correctness contract. Phase 2+ (resume, evidence retrieval, models, editable contextual slices) is intentionally absent.
