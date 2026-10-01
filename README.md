# prui

Review GitHub pull requests in your terminal, with frozen diffs, durable reading
progress, inline comments, and explicit review submission. Optional AI guides
organize changes into a reading order. No server or telemetry.

## Get started

Requires macOS or Linux, Git, and an authenticated
[GitHub CLI](https://cli.github.com/). GitHub.com repositories only.

Download the archive for your OS and architecture from
[GitHub Releases](https://github.com/steve-mackinnon/prui/releases). `darwin` means
macOS; choose `arm64` for Apple Silicon or `amd64` for Intel. Extract it and place
`prui` in a directory on `PATH`. Each release includes `checksums.txt` for verifying
downloads with `shasum -a 256` (macOS) or `sha256sum` (Linux).

Run `prui --version` to identify the installed version and commit. See the
[changelog](CHANGELOG.md) for changes and [release guide](docs/RELEASING.md) for
versioning and publishing instructions.

To build from source, install Go **1.26.8+**, then from this repository's root:

```sh
go install ./cmd/prui
```

Ensure your Go binary directory (`go env GOBIN`, or `$(go env GOPATH)/bin` when
unset) is on `PATH`. Then open a PR from the **root of the checkout you want to
review**:

```sh
gh auth login
cd /path/to/your/checkout
prui                                      # browse this checkout's open PRs
```

The launcher recognizes standard GitHub HTTPS/SSH `origin` remotes. For a
checkout without a supported origin, use `prui open <PR-URL>`. Select a PR
from the list and press `ctrl+p` to switch PRs or `s` to browse saved sessions.
Reopening a cached PR preserves reading progress. Fetching and reviewing do not
switch branches or modify the checkout.

Use `prui --help` for command syntax and the
[CLI reference](docs/REFERENCE.md#command-line) for less common tasks such as
plain output, offline review, and deletion. `--plain` is automatic when input
or output is redirected or `TERM=dumb`; it emits escaped, uncolored text without
interactive actions. Exit codes are `0` for complete inventory, `1` for errors,
`2` for unavailable review content, and `130` for canceled loading.
Local reading progress and exit success do not constitute GitHub approval.

## Review in the terminal

| Key | Action |
| --- | --- |
| `j` / `k` | Move between files or guide rows |
| `h` / `l` | Focus the list / diff pane |
| Up / down, `J` / `K` | Scroll diff by one / five lines |
| `F` / `G` | Select Files / Guide |
| `]` / `[` | Widen / narrow the left pane |
| `}` / `{` | Next / previous guide or file slice |
| `S` | Toggle side-by-side diff (requires 160 columns) |
| `1` / `2` / `3` / `4` | Show Description / Files / Guide / Commits |
| `v` / `V` | Cycle forward / backward through review tabs |
| `m` | Mark or unmark the selected file as read; saves immediately |
| `ctrl+p` | Switch PRs (except inside an editor) |
| `enter` | Focus diff, then edit a comment on the selected line |
| `R` | Open the review submission form |
| `g` | Open the guide modal to choose a provider and model, then confirm source upload |
| `r` / `N` | Check freshness / start a new comparison |
| `t` | Choose and save a theme |
| `?` / `q` | Health & help / quit |

The Guide pane shows the current guide’s overview, subsection copy, and files.
Its title and position stay at the top while you browse. Scroll past the end
of a guide’s diff to continue to the next guide, or use `}` / `{` to jump
between guides.

**Inside a line editor, Enter posts immediately.** Ctrl+P instead queues a local
pending comment; Shift+Enter adds a newline and Escape discards the editor.
`R` shows Comment, Approve, and Request changes as a three-choice radio list
through selection and confirmation. It submits the comment and pending line
comments after a separate confirmation. Posting requires GitHub Pull requests
write permission. Pending comments and review summaries are memory-only and are
lost on exit or replacement with a new comparison. Check GitHub before retrying
an uncertain write.

Comments can be refreshed with `c`; replies, reactions, and deletion are available
from a selected comment's action menu. The Commits tab (`4`) lists captured PR
commits on the left and the selected commit's first-parent diff on the right.
Use `j`/`k` to select a commit, `l`/Enter to focus its diff, and `n`/`p` to move
between commits from either pane. Captured material also works offline; legacy
sessions and capture limits are labeled. See the [user reference](docs/REFERENCE.md) for all controls, theme
configuration, and current limitations.

### Mouse selection and panel resizing

Click a file, guide row, inventory unit, picker item, diff source cell, or
comment card to select it. Selecting a file scrolls its diff into view; the file
header highlights the selection and follows the cursor while navigating the diff.
Click the visible view tabs to switch views.
Selection does not activate an item: Enter still opens a PR, applies a theme,
or opens the selected comment editor/menu. Existing keyboard controls remain
available. In split diffs, click the old or new source cell to select that
cell's exact comment target; old context cells are not comment targets.

At terminal widths of 100 columns or more, drag the separator between the
navigation list and diff horizontally. The navigation list stays at least
18 columns wide, with at least 40 columns for detail. Mouse dragging and the
`[` / `]` keys share the same per-review width, remembered for this run and restored after
returning from a narrow terminal. The inner old/new diff divider stays equal.

Mouse input is disabled during loading, editors, and confirmation forms.
Wheel scrolling and double-click activation are not supported. Mouse capture
may affect native terminal text selection; use your terminal's selection
modifier (often Shift, depending on the terminal or multiplexer) to copy text.

## Optional AI guides

Press `g` in a review to open a modal over the current review. It always shows
the provider, editable model ID, and recipient before upload. If the displayed
choice is right, press Enter to confirm; `g`, then Enter is the quick path.
Tab moves between the confirmation action, provider, and model. Use the arrow
keys on the provider row and type or delete on the model row. Ctrl+A clears the
model ID for replacement. Escape closes the modal without changing the review
or saved choice.

Providers with a configured, nonempty key in the current environment are
selectable. OpenAI, Anthropic, and Google use `OPENAI_API_KEY`,
`ANTHROPIC_API_KEY`, and `GEMINI_API_KEY` by default. An OpenAI-compatible
provider requires an endpoint in your guide configuration; an explicitly
configured keyless loopback endpoint is also supported. With no guide
configuration file, OpenAI's default model is `gpt-5.6-terra`, and
`OPENAI_BASE_URL` can override its destination. Use only an endpoint you trust
with the source and API key.

To bring your own model, create `~/.config/prui/config.json` on macOS or
Linux. If `XDG_CONFIG_HOME` is an absolute path, use
`$XDG_CONFIG_HOME/prui/config.json` instead. For example:

```json
{
  "guide": {
    "provider": "anthropic",
    "model": "claude-sonnet-4-20250514"
  }
}
```

Set `ANTHROPIC_API_KEY` for that example. Supported providers are `openai`,
`anthropic`, `google`, and `openai-compatible`; Google uses `GEMINI_API_KEY`.
Choose a model ID supported by the selected provider. A custom endpoint needs
`base_url` and usually `api_key_env`, the *name* of the environment variable
holding its key. The file never contains the key itself. See the
[configuration reference](docs/REFERENCE.md#guide-model-configuration) for
examples and endpoint rules.

Repository evidence can include excerpts from unchanged files elsewhere in the
pinned repository, not just files in the PR. See [what the guide request
sends](docs/REFERENCE.md#guide-request-contents) for the selection rules and
limits.

The last confirmed provider and model are remembered across launches in the
private app-owned `guide-last.json` beside `config.json`. This preference
contains no key or source. Opening, resuming, and switching PRs never generate
guides automatically. A cached guide is reused only when its provider, model,
endpoint, prompt, and schema match the current selection. A completed generation
saves a derived session with fresh reading progress and retains the original
session. Generation has a five-minute timeout; Escape cancels it sooner.
Raw diffs remain available if analysis fails.

Uploads have size and time limits and apply a heuristic credential filter.
**The filter is not a secret scanner and can miss credentials.** Use guides only
with source you are authorized to share. The confirmation shows the selected
provider, model, and recipient. OpenAI Responses requests set `store: false`;
other providers have their own retention policies. Credentials and full provider
transcripts are not saved locally. Generated guidance is an interpretation,
not a completeness or security verdict.

## Storage and safety

Source comes from pinned Git objects, comparing the verified merge base to the
PR head. Reviewed code, hooks, filters, and external diffs are not executed.
Missing objects are fetched into private temporary storage. Git, `gh`, and your
executable search path are trusted.

Snapshots contain raw source and may contain secrets already present in the PR.
They are permission-restricted, **not encrypted**, and retained until deleted:

- macOS: `~/Library/Application Support/prui/storage`
- Linux: `$XDG_DATA_HOME/prui/storage`, or `~/.local/share/prui/storage`

SQLite runs inside the application; no SQLite installation or database service is
needed. Independent app processes can open the store, with transactional writes
and generation checks protecting progress. `delete` removes the named session
and unreferenced source data, but preserves reusable guides, remembered
repositories, GitHub data, and backups.

This is a fresh storage format. Old file caches are neither read nor imported.
Windows support is deferred; use a local filesystem on macOS or Linux.

Large or unavailable blobs remain visibly incomplete; the default per-blob limit
is 1 MiB. PR browsing and comment overlays are bounded to 100 items. See the
[reference](docs/REFERENCE.md#limits-and-incomplete-states) for the full limits and
[pre-release audit](docs/AUDIT.md) for known issues.

## Development

```sh
./scripts/verify.sh       # formatting, vet, race tests, PTY tests, build
golangci-lint run ./...   # use v2.13.2, as pinned in CI
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
go run github.com/zricethezav/gitleaks/v8@v8.30.1 git --redact --no-banner --log-opts='--all' .
```

Tests also require Python 3. They use synthetic repositories, fake GitHub clients,
and local HTTP servers; live credentials are unnecessary. CI defines macOS and
Linux verification jobs. See [TESTING.md](TESTING.md) for the test workflow and
[CONSTRAINTS.md](CONSTRAINTS.md) for the correctness and safety contract.

See the [architecture guide](docs/ARCHITECTURE.md) for the package map, data
flow, and persistence boundaries.
The [spec archive](docs/spec/README.md) preserves feature requirements and
design decisions; check current code and user documentation for present behavior.

The secret scan examines all local Git refs. Its three fingerprint exceptions
identify historical synthetic test fixtures, not entire files or directories.
CI fetches complete history for that check. Dependabot tracks Go module and
GitHub Action updates weekly; CI action references are pinned to commit SHAs.

For changes, add a focused regression test, preserve the source/privacy
invariants, and run all four checks. Manual live acceptance uses `prui verify`;
its requirements are in the [reference](docs/REFERENCE.md#acceptance-and-guide-evaluation).

## License

Licensed under the [MIT License](LICENSE).
