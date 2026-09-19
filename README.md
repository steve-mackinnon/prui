# pr-review

Read-only, pinned GitHub PR review in a local terminal. Deterministic file slices, bounded pinned repository evidence, durable local reading progress, and explicit revision checks. Optional, opt-in OpenAI analysis groups the change into functional review guides. No server, no telemetry, no stored credentials. Phases 1-4 implemented.

## Run

Building requires Go 1.26.8+. Opening a new comparison requires Git and authenticated GitHub CLI (`gh`); resuming a frozen snapshot does not require Git or its checkout. Missing `gh`/authentication/connectivity leaves resume usable with unknown freshness. Dependencies are pinned in `go.mod`/`go.sum`: Bubble Tea v2.0.9 and Lip Gloss v2.0.2. macOS/Linux only initially.

```sh
cd /path/to/checkout # open and verify must run at the repository root
pr-review # browse this checkout's open GitHub PRs, then select one to review
pr-review open https://github.com/owner/repo/pull/42
pr-review open 42 --github-repo owner/repo --plain
go run ./cmd/pr-review prs owner/repo --plain
go run ./cmd/pr-review prs # browse remembered repositories, then select a PR
go run ./cmd/pr-review sessions
go run ./cmd/pr-review resume SESSION_ID
go run ./cmd/pr-review resume SESSION_ID --offline --plain
go run ./cmd/pr-review resume SESSION_ID --new --repo /path/to/checkout
go run ./cmd/pr-review eval-guides SESSION_ID
pr-review verify https://github.com/owner/repo/pull/42 --artifacts /tmp/pr-review-artifacts
go run ./cmd/pr-review delete SESSION_ID
go build -o pr-review ./cmd/pr-review
```

The no-argument interactive launcher reads the checkout's standard GitHub.com
`origin` remote locally, then lists its open pull requests. It accepts ordinary
HTTPS/SSH GitHub origins and never runs Git in the checkout to resolve them. If
the checkout has no supported `origin`, use `prs owner/repo` or `open` instead.

`open`, normal `resume`, explicit refresh, and listing a repository’s PRs contact GitHub. `sessions`, `delete`, and `resume --offline` do not. Implementation tests use synthetic objects, mocked GitHub, and a local test provider endpoint only. Source is uploaded to OpenAI only after confirming the interactive guide consent screen. Opening, resuming, and browsing do not request guide analysis automatically. Git fetch sends requested revision IDs and Git protocol negotiation to GitHub; metadata/authentication use the user's trusted `gh` installation.

`eval-guides SESSION_ID` reads an existing local session through a read-only store handle and emits a JSON report. It never opens GitHub, reads provider credentials, or creates guides; a session without stored generated guides reports `not_available`. Reports always include structural validation, and include named synthetic-corpus checks only when the frozen inventory matches a curated synthetic fixture.

`verify` is a manual acceptance utility for agents. Both `verify` and `open` must be launched from the root of the target local checkout; neither accepts a checkout path flag. It opens the requested PR through the production read-only source path, drives a fixed offline terminal journey (navigate, mark, quit, resume), and prints one JSON verdict. `--artifacts` must name a new directory outside the checkout; it receives `report.json`, the bounded terminal transcript, and fixed-size SVG screen captures that an agent can attach to chat. Opening allows 60 seconds by default; pass `--open-timeout DURATION` for a host that needs a different bound. On an open failure, the report includes the fixed `open` stage name and locally measured elapsed milliseconds without process output. The verifier creates a private temporary session store and never executes reviewed code or generates guides. Its timing report separately records GitHub metadata, pin-and-inventory work, and offline terminal startup. It requires Python 3 for the standard-library PTY harness and is deliberately excluded from CI.

`--plain` emits escaped text without a pager, color, terminal control sequences, or interaction. It includes the guide block, its analysis scope, and the bounded evidence scope report. It is automatic for redirected stdin/stdout or `TERM=dumb`, and stays uncolored even when the terminal supports color or `CLICOLOR_FORCE` is set. Exit codes: 0 successful raw inventory, 1 invalid input/open/runtime failure, 2 unavailable review content, 130 canceled loading. Analysis being unavailable never changes the exit status. Exit 0 is not approval or evidence that you read the review.

## Review Guides And The Source-Upload Opt-In

Guides group the frozen review units into functional chunks, each with a title, description, and ordered sections that reference specific units. They are an interpretation layer: `Slices` and `UnitFiles` are unchanged, reading progress stays file-slice based, and the raw inventory remains the complete source view. Every unit appears in guide navigation exactly once; anything the model did not group lands in a synthesized `Ungrouped changes` guide. Guides are separate from the provider plan behind `a` / `v` / `o`, which owns unit assignment and ordering; a guide never reassigns a unit.

Analysis is off by default. In an interactive review, press `g` to inspect the upload consent screen, then Enter to confirm or Escape to return without uploading. This works after opening, resuming, or choosing a PR in the browser. `OPENAI_API_KEY` is read only after confirmation and is never written to a snapshot, log, or error string. The default model is `gpt-5.6-terra`; `OPENAI_BASE_URL` may override the provider endpoint. There are no `--send-source-to-openai`, `--model`, or `--exclude` CLI flags. Plain output never starts analysis. `resume --offline` rejects guide generation and all GitHub operations before accessing clients or credentials; stored guides remain readable.

A confirmed request creates a new derived session with empty reading progress, retaining the original frozen snapshot and its progress. Escape cancels an in-flight request; cancellation keeps the current session and does not save a derived session. Missing credentials or invalid provider configuration leave the current session intact.

What leaves the machine is only the assembled request package: pinned patches and pinned-tree evidence that pass the privacy policy a second time at the upload boundary, within 400 units, 32 KiB per unit, 512 KiB total, and 60 seconds. Excluded paths, credential-like content, and budget-exhausted units are withheld and reported as analysis scope; withheld path names are not sent either. The request is one non-streaming HTTPS `POST /v1/responses` with `store: false` and a strict `pr_review_guides` JSON schema. Redirects are not followed, so the bearer credential cannot reach another host. Provider transcripts and credentials are never stored; the snapshot keeps the guides, provider, model, prompt version, schema name, input digest, evidence IDs, limits, and withheld ledger.

Failure is cheap and explicit. A transport error, non-2xx status, refusal, deadline, oversize payload, or unusable structured output produces an `analysis_unavailable` bundle with a stated reason, a durable session, and the unchanged deterministic file plan. Retrying cannot modify a stored snapshot; a later successful attempt is a new session. Generated text is model interpretation of the bounded input, not source truth, approval, security findings, or complete architectural documentation, and it can be wrong about anything it was not shown.

When a session has generated guides, they are the default left pane: guide, then section, then the file portions each section covers, with the selected guide's combined diff in the right pane. Enter on a section or file jumps to its position in that diff. A file appears under every section that owns part of it. `n` walks rows, `]`/`[` jump between guides, `tab` expands or collapses the selected guide or section, Enter focuses its diff, and `G` switches to the deterministic file plan. `i` still lists every raw unit and navigates them one at a time, so the complete source view is never behind an interpretation. Marking is unchanged: `m` marks the whole file slice of the selected unit, including the units that file contributes to other guides, and both the header and footer say so. A fallback, absent, or empty bundle simply has no rows, so those sessions navigate the file plan exactly as before.

Reviews opened in one process keep independent in-memory reading positions, hierarchy expansion, pane focus, scroll offsets, notices, and errors. `ctrl+p` (or `p` where control-key reporting is unreliable) opens a keyboard-only PR switcher over the current review. It lists already-open reviews first, then open PRs for the active repository; typing filters, Enter switches or starts a new read-only pinned review, and Escape leaves the current review unchanged. The overlay never takes a permanent column or changes plain output.

## Keyboard

| Key | Action |
| --- | --- |
| `ctrl+p` / `p` | Open the PR switcher; type to filter, Enter switches/opens, Escape cancels |
| `n` | Next guide row; next unit in the file plan and inventory |
| `]` / `[` | Next / previous guide, or file slice without guides |
| `G` | Switch between the guide hierarchy and the deterministic file plan |
| `tab` | Expand / collapse the selected guide or section |
| `ctrl+h` / `ctrl+l` | Focus list / diff |
| `enter` / `esc` | Open selected item / go back |
| Up / down, `j` / `k` | Navigate list or scroll focused diff |
| `J` / `K` | Scroll diff by 5 lines |
| Page Down / Page Up | Page through actual diff |
| `h` / `l`, left / right | Horizontal scrolling; no hidden line truncation in plain output |
| Home | Reset selected unit's scroll |
| `i` | Toggle the full unit inventory; every raw unit, unfiltered by guides |
| `e` | Show bounded evidence and included/excluded scope |
| `a` | Show the accepted provider plan; advisory claims only |
| `v` / `o` | Move the selected unit to another slice / reorder slices; `enter` confirms, `esc` cancels |
| `u` | Show canonical GitHub URL for copying; does not launch a browser |
| `g` | Inspect guide-upload consent; Enter confirms, Escape cancels |
| `m` | Mark/unmark the whole file slice of the selected unit, including its units under other guide sections; saves immediately |
| `r` | Explicit metadata refresh; no diff recomputation or polling |
| `N` | Start a new comparison with empty progress; retain the old session |
| `s` | Session picker; up/down to select, Enter to resume, Esc to return |
| Escape during a cancellable action | Cancel the operation and retain the current review |
| `?`, `q`, Ctrl+C | Help, quit, cancel loading |

Selection, expansion, and per-unit vertical offsets survive resizing; navigation positions and expansion state are not persisted across processes, and they are rebuilt from the immutable bundle so navigation cannot drift from the stored guides. No mouse capture, so terminal-native text selection remains available. Textual markers and labels are primary: the selected row is marked `> ` when its pane is focused and `· ` when it is not, and reverse video only reinforces the focused row. The interactive view additionally colors diff structure — file headers, hunk locations, additions, removals — and unit states such as metadata, binary, gitlink, unavailable, and warning chrome. Color is presentation only: no wording, label, or ordering depends on it, and terminals without color show the same text. Extremely small terminals clip controls; enlarge or use plain output. The user approved Phase 1 terminal behavior; broad theme/platform/accessibility coverage is not established.

## Color

Color is detected once at startup from the terminal and the process environment; there is no color option, theme, or configuration file. `NO_COLOR` and `TERM=dumb` disable it, `CLICOLOR_FORCE` enables it, and redirected output is never colored. Lower-capability terminals downsample to 256 or 16 colors rather than losing text.

The guarantees, verified by tests, are: styling only wraps whole display lines that were already escaped, so escaping of hostile patch bytes is unchanged; removing every style yields exactly the uncolored render, so no character, label, warning, or line is added, dropped, or reworded by color; colored lines never exceed the terminal width and horizontal scrolling never splits an escape sequence; and `--plain` output is never colored and contains no terminal control sequences, whatever the terminal supports.

## Sessions And Freshness

Every `open` saves a new frozen session, even with `--plain`. `sessions` lists full IDs, progress, and historical check status. Resume loads stored patches and metadata, not the checkout: branch deletion, garbage collection, or deleting the checkout cannot change the review. Full unchanged source context is not retained in Phase 2 and is explicitly labeled unavailable.

- `unchecked`: no freshness check this opening (`--offline`, or a check interrupted before completion).
- `current`: base/head SHAs and repository identities matched at the last explicit check, not a continuous guarantee.
- `stale`: base/head or repository identity changed. Keep navigating the old snapshot, or press `N` / use `resume ID --new` for a new unreviewed session.
- `check_failed`: metadata/authentication/connectivity failed; freshness unknown, frozen review still available.

Resume checks metadata by default; `--offline` disables all network actions for that invocation. New comparisons use the saved absolute checkout path unless `resume --repo` overrides it. Failed replacement preserves the old snapshot/progress. Every new session or plan version starts unreviewed, even with identical patch bytes. The persisted plan is still the deterministic `file-v1` one; guides sit above it and never carry completion. Local completion never means GitHub approval or complete source availability.

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

Run `./scripts/verify.sh` for the complete gate, including compiled-binary terminal tests. It requires Go, Git, and Python 3 on macOS/Linux. See [TESTING.md](TESTING.md) for test layers, screen baseline updates, and the per-feature regression checklist. CI runs the same command on Linux and macOS.

```sh
go vet ./... && go test -race -count=1 ./... && go build ./...
go test ./internal/source ./internal/inventory -run 'Test(Pinned|Safety|Inventory)'
go test ./internal/tui -run 'Test(RawReview|Guide|Plain)'
go test -race ./internal/session ./internal/review ./internal/tui
go test ./cmd/pr-review -run 'Test(Lifecycle|Storage|Options)'
go test ./internal/guide -run 'Test(OpenAI|Analyze|Input|Validate)'
```

Tests construct disposable trusted Git fixtures, including byte-only paths that macOS cannot materialize. Lifecycle tests cover process restart/interruption, deleted checkouts, concurrent writers, stale revisions, offline/auth failures, corruption, permissions, deletion, and keyboard actions. Analysis tests drive a local `httptest` provider endpoint with a fake key and cover valid, malformed, schema-violating, refused, unauthorized, rate-limited, failing, hung, and oversize responses. They never execute reviewed scripts, contact a real provider, or use live credentials. See `CONSTRAINTS.md` for the correctness contract. Guide navigation tests drive a fake analyzer and walk every row, so the hierarchy is proven against real frozen units. Editable contextual slices and guide-level completion are intentionally absent.
