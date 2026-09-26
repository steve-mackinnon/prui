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
pr-review open https://github.com/owner/repo/pull/42
```

The launcher recognizes standard GitHub HTTPS/SSH `origin` remotes. For a
checkout without a supported origin, use an explicit PR URL. Fetching and
reviewing do not switch branches or modify the checkout.

## Everyday commands

```sh
pr-review open 42 --github-repo owner/repo --plain
pr-review prs owner/repo                        # print up to 100 open PRs
pr-review prs                                  # browse remembered repositories
pr-review sessions
pr-review resume SESSION_ID
pr-review resume SESSION_ID --offline --plain
pr-review resume SESSION_ID --new --repo /path/to/checkout
pr-review delete SESSION_ID                     # delete that local session immediately
```

`open` saves a snapshot, including in plain mode. Resume reads the saved source
without requiring Git or the original checkout. Normal resume checks GitHub;
`--offline` disables all network operations. A changed PR needs a new comparison;
old snapshots remain readable. Cached PR reopening preserves reading progress.

All commands accept `--store /path/to/private-directory` for a separate store.
Use `pr-review --help` for the full command syntax. `--plain` is automatic when
input or output is redirected or `TERM=dumb`; it emits escaped, uncolored text
without interactive actions. Exit codes are `0` for complete inventory, `1` for
errors, `2` for unavailable review content, and `130` for canceled loading.
Local reading progress and exit success do not constitute GitHub approval.

## Review in the terminal

| Key | Action |
| --- | --- |
| `j` / `k` | Move between files or guide rows |
| `h` / `l` | Focus the list / diff pane |
| Up / down, `J` / `K` | Scroll diff by one / five lines |
| `F` / `G` | Select Files / Guide |
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
`R` submits Comment, Approve, or Request changes, with the summary and pending
comments, after a separate confirmation. Posting requires GitHub Pull requests
write permission. Pending comments and review summaries are memory-only and are
lost on exit or replacement with a new comparison. Check GitHub before retrying
an uncertain write.

Comments can be refreshed with `c`; replies, reactions, and deletion are available
from a selected comment's action menu. The Commits tab (`3`) is currently a
placeholder. See the [user reference](docs/REFERENCE.md) for all controls, theme
configuration, and current limitations.

## Optional AI guides

Set `OPENAI_API_KEY` in your environment, press `g`, and confirm to send bounded
pinned patches and repository evidence to the Responses API. The current default
model is `gpt-5.6-terra`. `OPENAI_BASE_URL` overrides the destination; only use an
endpoint you trust with both the source and API key.

Opening, resuming, and switching PRs never generate guides automatically. Cached
guides can be reused without uploading. A completed generation saves a derived
session with fresh reading progress and retains the original session. Escape
cancels generation. Raw diffs remain available if analysis fails.

Uploads have size and time limits and apply a heuristic credential filter.
**The filter is not a secret scanner and can miss credentials.** Use guides only
with source you are authorized to share. Requests set `store: false`; credentials
and full provider transcripts are not saved locally. Generated guidance is an
interpretation, not a completeness or security verdict.

## Storage and safety

Source comes from pinned Git objects, comparing the verified merge base to the
PR head. Reviewed code, hooks, filters, and external diffs are not executed.
Missing objects are fetched into private temporary storage. Git, `gh`, and your
executable search path are trusted.

Snapshots contain raw source and may contain secrets already present in the PR.
They are permission-restricted, **not encrypted**, and retained until deleted:

- macOS: `~/Library/Application Support/pr-review/sessions`
- Linux: `$XDG_DATA_HOME/pr-review/sessions`, or `~/.local/share/pr-review/sessions`

Only one process can write a store at a time. `delete` removes the named session,
but not separately cached guides, remembered repositories, GitHub data, or backups.

Large or unavailable blobs remain visibly incomplete; the default per-blob limit
is 1 MiB. PR browsing and comment overlays are bounded to 100 items. See the
[reference](docs/REFERENCE.md#limits-and-incomplete-states) for the full limits and
[pre-release audit](docs/AUDIT.md) for known issues.

## Development

```sh
./scripts/verify.sh       # formatting, vet, race tests, PTY tests, build
golangci-lint run ./...   # use v2.13.2, as pinned in CI
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

Tests also require Python 3. They use synthetic repositories, fake GitHub clients,
and local HTTP servers; live credentials are unnecessary. CI defines macOS and
Linux verification jobs. See [TESTING.md](TESTING.md) for the test workflow and
[CONSTRAINTS.md](CONSTRAINTS.md) for the correctness and safety contract.

| Package | Responsibility |
| --- | --- |
| `cmd/pr-review` | CLI wiring, application actions, online/write guards |
| `internal/source` | Isolated Git access, pinned comparisons, GitHub transport |
| `internal/inventory` | Deterministic file and hunk inventory |
| `internal/context`, `internal/privacy`, `internal/guide` | Bounded evidence, upload filtering, optional guides |
| `internal/session`, `internal/review` | Frozen snapshots, reading progress, review lifecycle |
| `internal/tui`, `internal/theme` | Bubble Tea interface and appearance |
| `internal/verify`, `internal/guideeval` | Terminal acceptance artifacts and local guide evaluation |

For changes, add a focused regression test, preserve the source/privacy
invariants, and run all three checks. Manual live acceptance uses `pr-review verify`;
its requirements are in the [reference](docs/REFERENCE.md#acceptance-and-guide-evaluation).

## License

A license has not been selected or included yet. This is an outstanding step
before the planned open-source release.
