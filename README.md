# prui

A terminal workspace for reviewing GitHub pull requests without switching branches.
Read a pinned snapshot, save your progress, and post comments or submit a review.
Optional AI guides group related changes into a reading order. No server or telemetry.

## Install

Requires macOS or Linux, Git, `curl`, and an authenticated
[GitHub CLI](https://cli.github.com/) (`gh auth login`). Supports GitHub.com repositories.

```sh
curl -fsSL https://raw.githubusercontent.com/steve-mackinnon/prui/main/scripts/install.sh | sh
```

The installer verifies the release checksum and installs to `~/.local/bin` without
sudo. If needed, add `export PATH="$HOME/.local/bin:$PATH"` to your shell's startup
file and run it in your current terminal. Run the installer again to update.
You can also download a binary from [Releases](https://github.com/steve-mackinnon/prui/releases).
No Go installation or AI API key is needed for a released binary.

## Your first review

From the root of the checkout you want to review:

```sh
prui                                    # browse open PRs
prui open https://github.com/owner/repo/pull/42  # open a specific PR
```

1. Select a PR with `j`/`k` or Down/Up and press Enter. New reviews open on
   **Overview**; press `2` or click **Files** to read the diff.
2. Use Tab to switch between the file list and diff. In the diff, `j`/`k` moves
   the line cursor; Down/Up scrolls. Press `m` to mark a file as read.
3. Press Enter on a commentable line to compose. **Enter in the editor posts
   immediately; Ctrl+P queues a local pending line comment.** Shift+Enter adds a
   newline; Escape discards the editor.
4. Press `R` or choose **Submit review** to send a Comment, Approve, or Request
   changes review with your queued comments after confirmation.

Reading progress saves immediately. **Space → Saved reviews** reopens a saved
comparison. Press **r** or choose **Actions → Refresh PR** to update source,
discussions, and comments. Unchanged code keeps your position and drafts; changed
code opens the latest comparison while preserving the old review and its drafts
in Saved reviews. Workspace `c` and `N` are aliases for Refresh PR. Marking files
as read does not submit a GitHub review.

## Keys

Keys below apply while navigating; text editors use their own controls.

| Key | Action |
| --- | --- |
| `j` / `k` | Next / previous list item or diff cursor line |
| Down / Up | Move list selection or scroll the diff |
| `h` / `l`, Tab / Shift+Tab | Focus list / diff; Tab switches panes |
| `J` / `K` | Scroll diff down / up 5 lines |
| `d` / `u` | Scroll down / up 15 lines |
| `zz` | Center diff on its cursor |
| Enter / Escape | Activate selection or compose a comment / go back |
| `1` / `2` / `3` / `4` | Overview / Files / Guide / Commits |
| Space / Ctrl+K | Open Actions for review tools, settings, and saved reviews |
| `m` / `R` | Mark file read / submit review |
| `r` | Refresh PR source, discussions, and comments |
| `/` / `F` | Search diff text / filter file paths |
| `?` | Help and review health; `s` inside help shows all shortcuts |
| `q` / Ctrl+C | Quit |

Mouse clicks and scrolling work too. See the [full keyboard reference](docs/REFERENCE.md#keyboard)
for ranges, suggestions, discussions, and other controls, or `prui --help` for commands.
For offline reading, use `prui sessions`, then `prui resume <id> --offline`.

## Optional AI guides

Set `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, or `GEMINI_API_KEY`, then press `g` to
choose a provider and model and confirm generation. Guides never generate
automatically. See [model and endpoint configuration](docs/REFERENCE.md#guide-model-configuration)
for custom providers and local endpoints.

**Generation sends eligible source to the selected provider**, potentially including
excerpts from unchanged files. The credential filter can miss secrets; use only
source you are authorized to share and an endpoint you trust. Guides can be wrong.
See [request contents and privacy limits](docs/REFERENCE.md#guide-request-contents).

## Storage and safety

Your checkout stays untouched, and reviewed code is not executed. Saved snapshots
contain source and private drafts; they are permission-restricted, **not encrypted**.
They remain until deleted with `prui delete <id>`; reusable caches can survive deletion.
See [storage and deletion](docs/REFERENCE.md#local-storage-and-deletion) and
[source limits](docs/REFERENCE.md#limits-and-incomplete-states).

## Development

Requires Go **1.26.9+**. Clone the repository and run `go install ./cmd/prui` to
build from source. Run `./scripts/verify.sh` for formatting, vet, race tests, PTY
tests, and build checks. See [TESTING.md](TESTING.md#repository-verification) for all
verification gates, [CONSTRAINTS.md](CONSTRAINTS.md) for the quality contract,
and the [architecture guide](docs/ARCHITECTURE.md) for the package map.
Release instructions are in [RELEASING.md](docs/RELEASING.md).

[Changelog](CHANGELOG.md) · [MIT License](LICENSE)
