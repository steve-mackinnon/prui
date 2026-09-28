# pr-review

Review GitHub pull requests in your terminal, with frozen diffs, durable reading
progress, inline comments, and explicit review submission. Optional AI guides
organize changes into a reading order. No server or telemetry.

## Get started

Requires macOS or Linux, Go **1.26.8+** to build, Git, and an authenticated
[GitHub CLI](https://cli.github.com/). GitHub.com repositories only.

From this repository's root, install the binary:

```sh
go install ./cmd/pr-review
```

Ensure your Go binary directory (`go env GOBIN`, or `$(go env GOPATH)/bin` when
unset) is on `PATH`. Then open a PR from the **root of the checkout you want to
review**:

```sh
gh auth login
cd /path/to/your/checkout
pr-review                                      # browse this checkout's open PRs
```

The launcher recognizes standard GitHub HTTPS/SSH `origin` remotes. For a
checkout without a supported origin, use `pr-review open <PR-URL>`. Select a PR
from the list and press `ctrl+p` to switch PRs or `s` to browse saved sessions.
Reopening a cached PR preserves reading progress. Fetching and reviewing do not
switch branches or modify the checkout.

Use `pr-review --help` for command syntax and the
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
| `1` / `2` | Show Diff / frozen PR Description |
| `m` | Mark or unmark the selected file as read; saves immediately |
| `ctrl+p` | Switch PRs (except inside an editor) |
| `enter` | Focus diff, then edit a comment on the selected line |
| `R` | Open the review submission form |
| `g` | Generate a guide after confirming source upload |
| `r` / `N` | Check freshness / start a new comparison |
| `t` | Choose and save a theme |
| `?` / `q` | Health & help / quit |

**Inside a line editor, Enter posts immediately.** Ctrl+P instead queues a local
pending comment; Shift+Enter adds a newline and Escape discards the editor.
`R` shows Comment, Approve, and Request changes as a three-choice radio list
through selection and confirmation. It submits the comment and pending line
comments after a separate confirmation. Posting requires GitHub Pull requests
write permission. Pending comments and review summaries are memory-only and are
lost on exit or replacement with a new comparison. Check GitHub before retrying
an uncertain write.

Comments can be refreshed with `c`; replies, reactions, and deletion are available
from a selected comment's action menu. The Commits tab (`3`) is currently a
placeholder. See the [user reference](docs/REFERENCE.md) for all controls, theme
configuration, and current limitations.

### Mouse selection and panel resizing

Click a file, guide row, inventory unit, picker item, diff source cell, or
comment card to select it. Click the visible view tabs to switch views.
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

With no guide configuration file, set `OPENAI_API_KEY` in your environment,
press `g`, and confirm to send bounded pinned patches and repository evidence
to OpenAI. The default model is `gpt-5.6-terra`; `OPENAI_BASE_URL` can override
its destination. Use only an endpoint you trust with the source and API key.

To bring your own model, create `~/.config/pr-review/config.json` on macOS or
Linux. If `XDG_CONFIG_HOME` is an absolute path, use
`$XDG_CONFIG_HOME/pr-review/config.json` instead. For example:

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

Opening, resuming, and switching PRs never generate guides automatically. A
cached guide is reused only when its provider, model, endpoint, prompt, and
schema match the current selection. A completed generation saves a derived
session with fresh reading progress and retains the original session. Escape
cancels generation. Raw diffs remain available if analysis fails.

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

- macOS: `~/Library/Application Support/pr-review/storage`
- Linux: `$XDG_DATA_HOME/pr-review/storage`, or `~/.local/share/pr-review/storage`

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

The secret scan examines all local Git refs. Its three fingerprint exceptions
identify historical synthetic test fixtures, not entire files or directories.
CI fetches complete history for that check. Dependabot tracks Go module and
GitHub Action updates weekly; CI action references are pinned to commit SHAs.

For changes, add a focused regression test, preserve the source/privacy
invariants, and run all four checks. Manual live acceptance uses `pr-review verify`;
its requirements are in the [reference](docs/REFERENCE.md#acceptance-and-guide-evaluation).

## License

Licensed under the [MIT License](LICENSE).
