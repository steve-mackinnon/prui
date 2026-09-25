# pr-review

Pinned GitHub PR review in a local terminal, with inline comments and explicit PR review submission in the interactive TUI. Deterministic file slices, bounded pinned repository evidence, durable local reading progress, and explicit revision checks. Saved reviews open immediately while an asynchronous check refreshes the pinned comparison. OpenAI guide generation requires explicit confirmation. No server, no telemetry, no stored credentials. Source retrieval remains read-only; GitHub writes require an explicit reviewer action.

## Run

Building requires Go 1.26.8+. Opening a new comparison requires Git and authenticated GitHub CLI (`gh`); resuming a frozen snapshot does not require Git or its checkout. Missing `gh`/authentication/connectivity leaves resume usable with unknown freshness. Posting an inline comment or submitting a review additionally requires the authenticated GitHub credential to have Pull requests write permission for the repository. Dependencies are pinned in `go.mod`/`go.sum`: Bubble Tea v2.0.9 and Lip Gloss v2.0.2. macOS/Linux only initially.

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

`open`, normal `resume`, explicit refresh, and listing a repository’s PRs contact GitHub. `sessions`, `delete`, and `resume --offline` do not. Implementation tests use synthetic objects, mocked GitHub, and a local test provider endpoint only. Re-selecting a PR from the interactive PR list or switcher shows a validated local snapshot first, then checks the current pinned revision in the background. An unchanged revision reuses its source inventory, evidence, and reading progress without Git fetching or rebuilding resources; a changed revision opens a new comparison. A successful guide already cached for the pinned comparison can be reused, but generation requires explicit confirmation with `g`. Direct `open`, `resume`, `--plain`, offline mode, and `verify` never start guide generation. Git fetch sends requested revision IDs and Git protocol negotiation to GitHub; metadata/authentication use the user's trusted `gh` installation.

`eval-guides SESSION_ID` reads an existing local session through a read-only store handle and emits a JSON report. It never opens GitHub, reads provider credentials, or creates guides; a session without stored generated guides reports `not_available`. Reports always include structural validation, and include named synthetic-corpus checks only when the frozen inventory matches a curated synthetic fixture.

`verify` is a manual acceptance utility for agents. Both `verify` and `open` must be launched from the root of the target local checkout; neither accepts a checkout path flag. It opens the requested PR through the production read-only source path, drives a fixed offline terminal journey (navigate, mark, quit, resume), and prints one JSON verdict. `--artifacts` must name a new directory outside the checkout; it receives `report.json`, the bounded terminal transcript, and fixed-size SVG screen captures that an agent can attach to chat. Opening allows 60 seconds by default; pass `--open-timeout DURATION` for a host that needs a different bound. On an open failure, the report includes the fixed `open` stage name and locally measured elapsed milliseconds without process output. The verifier creates a private temporary session store and never executes reviewed code or generates guides. Its timing report separately records GitHub metadata, pin-and-inventory work, and offline terminal startup. It requires Python 3 for the standard-library PTY harness and is deliberately excluded from CI.

`--plain` emits escaped text without a pager, color, terminal control sequences, interaction, or GitHub comment capability. It includes the guide block, its analysis scope, and the bounded evidence scope report. It is automatic for redirected stdin/stdout or `TERM=dumb`, and stays uncolored even when the terminal supports color or `CLICOLOR_FORCE` is set. Exit codes: 0 successful raw inventory, 1 invalid input/open/runtime failure, 2 unavailable review content, 130 canceled loading. Analysis being unavailable never changes the exit status. Exit 0 is not approval or evidence that you read the review.

## Review Guides And Automatic PR-List Generation

Guides group the frozen review units into functional chunks, each with a title, description, and ordered sections that reference specific units. They are an interpretation layer: `Slices` and `UnitFiles` are unchanged, reading progress stays file-slice based, and the raw inventory remains the complete source view. Every unit appears in guide navigation exactly once; anything the model did not group lands in a synthesized `Ungrouped changes` guide.

Selecting a PR from the interactive PR list or switcher displays the latest validated saved review immediately when one exists, with freshness shown as unknown until an asynchronous GitHub check finishes. The check reuses a matching frozen comparison without Git fetching or resource rebuilding; a changed comparison is pinned normally and replaces the displayed review when ready. A previously generated guide for the same repository, PR number, base SHA, and head SHA is reused locally. Press `g` in an interactive review and confirm to generate a guide; Escape cancels an in-flight request. Direct `open`, `resume`, `--plain`, offline mode, and `verify` do not generate guides. `OPENAI_API_KEY` is never written to a snapshot, log, or error string. The default model is `gpt-5.6-terra`; `OPENAI_BASE_URL` may override the provider endpoint. There are no `--send-source-to-openai`, `--model`, or `--exclude` CLI flags. `resume --offline` rejects guide generation and all GitHub operations before accessing clients or credentials; stored guides remain readable.

A completed guide generation creates a new derived session with empty reading progress, retaining the original frozen snapshot and its progress. Escape cancels an in-flight request; cancellation keeps the current session and does not save a derived session. Missing credentials or invalid provider configuration leave the current session intact.

What leaves the machine is only the assembled request package: pinned patches and pinned-tree evidence that pass the privacy policy a second time at the upload boundary, within 400 units, 32 KiB per unit, 512 KiB total, and 60 seconds. Excluded paths, credential-like content, and budget-exhausted units are withheld and reported as analysis scope; withheld path names are not sent either. The request is one non-streaming HTTPS `POST /v1/responses` with `store: false` and a strict `pr_review_guides` JSON schema. Redirects are not followed, so the bearer credential cannot reach another host. Provider transcripts and credentials are never stored; the snapshot keeps the guides, provider, model, prompt version, schema name, input digest, evidence IDs, limits, and withheld ledger.

Failure is cheap and explicit. A transport error, non-2xx status, refusal, deadline, oversize payload, or unusable structured output produces an `analysis_unavailable` bundle with a stated reason, a durable session, and the unchanged deterministic file plan. Only a structurally valid generated bundle is persisted as a reusable local cache entry; unavailable bundles never suppress a later retry. Retrying cannot modify a stored snapshot; a later successful attempt is a new session. Generated text is model interpretation of the bounded input, not source truth, approval, security findings, or complete architectural documentation, and it can be wrong about anything it was not shown.

The left pane opens on Files. Its right pane shows every changed file in order, with all of each file's hunks together. Scroll through the whole PR or press `j`/`k` to jump between file boundaries. Press `F` for Files or `G` for Guide. When a session has generated guides, Guide shows each guide, section, and covered file portion, with the selected guide's combined diff in the right pane. Enter on a section or file jumps to its position in that diff. A file appears under every section that owns part of it. `n` walks guide rows, `]`/`[` jump between guides, and `tab` expands or collapses the selected guide or section. `i` lists every raw unit. `m` marks the whole file slice of the selected unit. A fallback, absent, or empty bundle leaves Files available.

Reviews opened in one process keep independent in-memory reading positions, hierarchy expansion, pane focus, scroll offsets, notices, and errors. `ctrl+p` (or `p` where control-key reporting is unreliable) opens a keyboard-only PR switcher over the current review. It lists already-open reviews first, then open PRs for the active repository; typing filters, Enter switches or starts a new read-only pinned review, and Escape leaves the current review unchanged. The overlay never takes a permanent column or changes plain output.

The interactive review uses a compact workspace: an identity header, a mode and
selection header, the file/guide rail and detail pane, and a one-line health
status. The status line keeps `R Submit review` and the pending count visible
alongside local reading progress, inventory, guides, freshness, and unavailable
content. Narrow terminals abbreviate the action and retain the highest-severity
state. `?` opens **Health & help**, which groups shortcuts under Navigate,
Review, Views, Diagnostics, and App.

Diff detail opens in unified layout. Press `S` to prefer side-by-side source
rows for the active review tab; that preference stays in memory only and is not
shared with another open PR. At fewer than 160 terminal columns, the TUI keeps
the preference but automatically renders unified detail and says
`side-by-side needs 160 columns`; split detail resumes after resizing to 160
columns. Plain output is always unified.

## Keyboard

| Key | Action |
| --- | --- |
| `ctrl+p` / `p` | Open the PR switcher; type to filter, Enter switches/opens, Escape cancels |
| `n` / `p` | Move between comment targets in a focused Files diff; move through files, guide rows, or inventory units in the list |
| `]` / `[` | Next / previous guide, or file slice without guides |
| `F` / `G` | Select Files or Guide in the left pane |
| `S` | Toggle side-by-side detail; unified is the default and narrow terminals fall back below 160 columns |
| `v` / `V` | Next / previous PR context view: Changes, Description, or Commits |
| `tab` | Expand / collapse the selected guide or section |
| `ctrl+h` / `ctrl+l` | Focus list / diff |
| `enter` / `esc` | In the list, focus the selected diff / go back; in a focused diff, open a composer for a target or an action menu for a selected comment / discard local drafts and menus |
| `enter` in a line editor | Post the line comment immediately; `shift+enter` inserts a newline |
| `ctrl+p` in a line editor | Save or update a local pending comment without posting |
| `R` | Open Submit review from Changes, Description, or Commits |
| `c` | Refresh ephemeral inline review comments (online reviews only) |
| `j` / `k` | Jump to next / previous file in Files; navigate guide rows or inventory units elsewhere |
| Up / down | Navigate list or scroll focused diff |
| `J` / `K` | Scroll diff by 5 lines |
| `zz` | Center the focused diff on its selected line |
| `d` / `u`, Page Down / Page Up | Page through actual diff |
| `h` / `l`, left / right | Horizontal scrolling; no hidden line truncation in plain output |
| Home | Reset active diff scroll |
| `i` | Toggle the full unit inventory; every raw unit, unfiltered by guides |
| `e` | Show bounded evidence and included/excluded scope |
| `U` | Show canonical GitHub URL for copying; does not launch a browser |
| `g` | Request guide generation after confirming source upload; Escape cancels an in-flight request |
| `m` | Mark/unmark the whole file slice of the selected unit, including its units under other guide sections; saves immediately |
| `r` | Explicit metadata refresh; no diff recomputation or polling |
| `N` | Start a new comparison with empty progress; retain the old session |
| `s` | Session picker; up/down to select, Enter to resume, Esc to return |
| `t` | Open the theme picker; up/down or `j`/`k` chooses, Enter saves and applies, Esc/`t` cancels |
| Escape during a cancellable action | Cancel the operation and retain the current review |
| `?`, `q`, Ctrl+C | Help, quit, cancel loading |

Selection, expansion, and per-unit vertical offsets survive resizing; navigation positions and expansion state are not persisted across processes, and they are rebuilt from the immutable bundle so navigation cannot drift from the stored guides. No mouse capture, so terminal-native text selection remains available. Textual markers and labels are primary: the selected row is marked `› ` whether or not its pane is focused; a muted background reinforces it when unfocused and reverse video reinforces the focused row. The interactive view additionally colors diff structure — file headers, hunk locations, additions, removals — and unit states such as metadata, binary, gitlink, unavailable, and warning chrome. Color is presentation only: no wording, label, or ordering depends on it, and terminals without color show the same text. Extremely small terminals clip controls; enlarge or use plain output. The user approved Phase 1 terminal behavior; broad theme/platform/accessibility coverage is not established.

Every interactive PR has `Changes`, `Description`, and `Commits` context views. Description is escaped plain text frozen from GitHub when the session opened; it is available after an offline resume and does not refresh when selected. Empty captured descriptions and older sessions that did not capture one are labeled explicitly. Context views are not included in `--plain` output.

## Inline Review Comments

The interactive diff pane marks one selected display line. Press `enter` while that pane is focused to open a Markdown composer only when the line is commentable: added and context lines target the new-file `RIGHT` side, and deleted lines target the old-file `LEFT` side. Headers, hunk markers, binary/metadata/unavailable content, and paths that cannot be sent as valid JSON are not commentable. `j`/`k` move the line cursor through commentable lines; scrolling and the cursor are separate.

Press `enter` on a selected commentable diff line to open an inline editor immediately beneath it. The bordered editor keeps the draft visually separate from code and shows a blinking caret at the edit point. Type Markdown normally; `enter` posts one comment immediately, `ctrl+p` adds it to the local pending review, `shift+enter` adds a newline, and `esc` discards the editor. Opening a line with an existing pending comment reopens that draft. Up to 100 pending comments appear under their target lines and in the Submit review screen; they are memory-only and are lost when the app exits. A failed immediate submission keeps the editor and target for an intentional retry.

`R` opens **Submit review** from any PR context view. The form shows Comment, Approve, and Request changes as radio choices; use `j`/`k` or up/down to select one. Press Tab or Enter to move to the comment box below. Comment and Request changes require text; Approve allows an empty comment. Tab again to browse pending line comments: Enter edits the selected draft at its diff target, and `d` removes it. Enter in the comment box opens a confirmation showing the review decision and pending count; a second Enter sends the review and all pending comments to GitHub in one request. Escape backs out without writing. A failed submission leaves the comment and pending drafts in memory for inspection; because a network failure can have an uncertain outcome, check GitHub before retrying. Successful submission clears the local queue and refreshes the inline comment overlay.

Online reviews load a bounded, read-only overlay of review comments and `c` refreshes it. Comments render only when their frozen head SHA, path, side, and line exactly match a diff target; remote author and body text are escaped. A successful post is inserted immediately from GitHub’s canonical response. Remote comments are never stored in sessions or plain output; offline reviews neither fetch nor post them.

Comment boxes participate in focused-diff `j`/`k` navigation, are visibly marked, and are kept in view. `enter` on one opens a local action menu: `r` opens a separate rune-aware reply box indented beneath that message, `a` opens the finite GitHub reaction picker (`1`–`8` choose its displayed reaction), and `d` is shown only after the authenticated viewer identity matches the displayed comment author. GitHub replies can target only a thread’s top-level comment, so `r` on an existing reply automatically uses that root and renders the canonical response in its thread. Reaction totals are compact emoji chips (`👍`, `👎`, `😄`, `😕`, `❤️`, `🎉`, `🚀`, `👀`) in the message box’s bottom border; an explicit non-UTF-8 locale uses the original GitHub token labels instead. Deletion requires a second `enter` confirmation. `esc` always closes the menu or draft without a write. Reply/reaction/deletion requests are preflighted against the frozen PR and update only that tab’s memory-only overlay from the canonical GitHub response; a failed reply retains its draft for retry.

Immediately before posting a comment or submitting a review, pr-review re-reads GitHub metadata and requires the repository identities plus base and head SHAs to equal the frozen comparison. If they differ, it makes no write and asks you to open a new comparison. A successful request uses the frozen head SHA and line targets; GitHub can still mark a comment outdated if the pull request advances after that preflight. Writes are unavailable in `--plain` and `resume --offline`, and no open, resume, refresh, guide action, or background task can submit one.

## Color

The interactive TUI supports four built-in semantic palettes: `terminal` (the
default), `light`, `dark`, and `high-contrast`. `terminal` preserves the
existing ANSI/256-color mapping, so it follows the terminal's palette. Choose
a palette for one invocation with `--theme NAME` on `pr-review`, `open`,
`resume`, or `prs`; an invalid name exits before normal work begins. The flag
is accepted with `--plain` but has no visible effect. `verify` does not accept
`--theme` and never reads personal theme configuration.

The optional global configuration file is `theme.json`:

- macOS: `~/Library/Application Support/pr-review/theme.json`
- Linux: `$XDG_CONFIG_HOME/pr-review/theme.json` when `XDG_CONFIG_HOME` is
  absolute; otherwise `~/.config/pr-review/theme.json`

It may select a built-in and override the fixed semantic color tokens:

```json
{
  "theme": "dark",
  "colors": {
    "selection": "#30363d",
    "focusedBorder": "bright-cyan",
    "warning": "214"
  }
}
```

Supported tokens are `title`, `fileHeader`, `hunk`, `added`, `removed`,
`metadata`, `warning`, `unavailable`, `selection`, `focusedBorder`, and
`border`. Values must be `#RRGGBB`, `default`, an ANSI name (such as `red` or
`bright-yellow`), or an ANSI palette index from `0` through `255`, written as
a string. The explicit `--theme` name wins over the file's `theme` value, and
valid `colors` overrides remain applied. Invalid JSON, duplicate or unknown
keys, invalid names, and invalid color values are ignored as a whole: the app
warns safely and uses the terminal palette (or a valid explicit `--theme`).
Such a file is never overwritten.

Press `t` in an interactive review or navigation picker to open a
keyboard-only theme picker. It is unavailable while another modal owns input.
Use arrows or `j`/`k` to choose and Enter to save and apply a built-in; Escape
or `t` cancels. A successful choice atomically updates only the global
`theme` value, retaining valid configured color overrides. It does not change
review or session data. A failed save keeps the active palette unchanged.

Color is still detected once at startup from the terminal and process
environment. `NO_COLOR` and `TERM=dumb` disable it, `CLICOLOR_FORCE` follows
the existing capability rules, and redirected output is never colored.
Lower-capability terminals downsample to 256 or 16 colors rather than losing
text. Themes supplement these accessibility safeguards: color is never the
only cue, and the picker retains textual names, markers, and controls even
when color is disabled.

The guarantees, verified by tests, are: styling only wraps whole display lines that were already escaped, so escaping of hostile patch bytes is unchanged; removing every style yields exactly the uncolored render, so no character, label, warning, or line is added, dropped, or reworded by color; colored lines never exceed the terminal width and horizontal scrolling never splits an escape sequence; and `--plain` output is never colored and contains no terminal control sequences, whatever the terminal supports.

## Sessions And Freshness

Every `open` saves a new frozen session, even with `--plain`. `sessions` lists full IDs, progress, and historical check status. Resume loads stored patches and metadata, not the checkout: branch deletion, garbage collection, or deleting the checkout cannot change the review. Full unchanged source context is not retained in Phase 2 and is explicitly labeled unavailable.

- `unchecked`: no freshness check this opening (`--offline`, or a check interrupted before completion).
- `current`: base/head SHAs and repository identities matched at the last explicit check, not a continuous guarantee.
- `stale`: base/head or repository identity changed. Keep navigating the old snapshot, or press `N` / use `resume ID --new` for a new unreviewed session.
- `check_failed`: metadata/authentication/connectivity failed; freshness unknown, frozen review still available.

Resume checks metadata by default; `--offline` disables all network actions for that invocation. New comparisons use the saved absolute checkout path unless `resume --repo` overrides it. Failed replacement preserves the old snapshot/progress. Every new session starts unreviewed, even with identical patch bytes. Guides do not carry completion. Local completion never means GitHub approval or complete source availability.

Phase 3 evidence is read only from pinned Git tree/blob objects. It prioritizes manifests, documentation, nearby tests, and changed directories within 100 files, 32 KiB per excerpt, 256 KiB retained bytes, and five seconds. Press `e` to inspect retained evidence and omissions. Relationships are observational unless explicitly marked otherwise; AST indexing and history expansion are deferred. Exclusions are local policy, and credential filters are defense in depth rather than secret detection.

## Local Storage And Deletion

The CLI prints storage location; TUI help shows storage and session ID. Defaults:

- macOS: `~/Library/Application Support/pr-review/sessions`
- Linux: `$XDG_DATA_HOME/pr-review/sessions`, or `~/.local/share/pr-review/sessions` when unset/non-absolute

All commands accept `--store /private/directory`, outside the reviewed checkout. Use a new/empty directory or an existing app-owned store; unrelated nonempty directories and symlink roots are rejected. Directories use 0700, files 0600. One process holds the store's advisory writer lock for its lifetime; another invocation fails visibly until it exits. OS process exit releases the lock; do not delete `.lock` to bypass it. Use a local filesystem with working advisory locks, rename, and fsync semantics.

Each session directory contains immutable `snapshot.json` (comparison, inventory, byte-preserving patches, file slices, checkout hint) and replaceable `state.json` (schema version, snapshot SHA-256 reference, reviewed slice IDs, freshness, generation, timestamp). Patch references are content hashes. Saves use private temporary files, file sync, atomic replacement, and directory sync; generation checks reject outdated updates. JSON files are bounded to 128 MiB each. Corrupt/unsupported records remain untouched and appear as unreadable entries, never silently migrated or replaced.

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
