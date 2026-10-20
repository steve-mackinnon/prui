# prui

prui is a terminal workspace for reviewing GitHub pull requests. Review a fixed
snapshot without switching branches, save your reading progress between sessions,
and post inline comments or submit a review. Optional AI guides group related
changes into a reading order. No server or telemetry.

For a large PR you review over several sittings, prui keeps the comparison you
started with and remembers which files you read. If the author pushes new commits,
you can keep reading the saved snapshot or explicitly start a new comparison.
Your checkout stays untouched, so you can review while working on another branch.

## Install

Install or update to the latest release on macOS or Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/steve-mackinnon/prui/main/scripts/install.sh | sh
```

Requires `curl` and an authenticated [GitHub CLI](https://cli.github.com/)
(`gh auth login`). The installer detects your platform, verifies the release's
SHA-256 checksum, and installs `prui` to `~/.local/bin` without sudo. If that
directory is not on your `PATH`, run `export PATH="$HOME/.local/bin:$PATH"`
and add it to your shell's startup file for future terminals.

Requires macOS or Linux, Git, and an authenticated
[GitHub CLI](https://cli.github.com/). Supports GitHub.com repositories.
No Go installation or AI API key is needed to use a released binary.

### Download a release

Download your platform's archive and `checksums.txt` from
[GitHub Releases](https://github.com/steve-mackinnon/prui/releases).
`darwin` means macOS; `arm64` means Apple Silicon or Linux ARM64, and `amd64`
means Intel/AMD x86-64.

For example, install the latest release on **Apple Silicon macOS** with `gh`:

```sh
gh auth login
prui_download_dir=$(mktemp -d)
gh release download --repo steve-mackinnon/prui \
  --pattern 'prui_*_darwin_arm64.tar.gz' --pattern checksums.txt \
  --dir "$prui_download_dir"
(
  cd "$prui_download_dir" || exit 1
  grep 'darwin_arm64.tar.gz$' checksums.txt | shasum -a 256 -c - &&
  tar -xzf prui_*_darwin_arm64.tar.gz &&
  mkdir -p "$HOME/.local/bin" &&
  install -m 755 prui "$HOME/.local/bin/prui"
)
export PATH="$HOME/.local/bin:$PATH"
prui --version
```

For Intel macOS, replace `darwin_arm64` with `darwin_amd64`. For Linux, use
`linux_amd64` or `linux_arm64` and replace `shasum -a 256 -c -` with
`sha256sum -c -`. Add `export PATH="$HOME/.local/bin:$PATH"` to your shell's
startup file to keep the command available in new terminals. The downloaded
archive remains in the temporary directory for inspection.

### Build from source

Install Go **1.26.9+**, then:

```sh
git clone https://github.com/steve-mackinnon/prui.git
cd prui
mkdir -p "$HOME/.local/bin"
GOBIN="$HOME/.local/bin" go install ./cmd/prui
export PATH="$HOME/.local/bin:$PATH"
prui --version
```

Local source builds report `dev` and `unknown` for version and commit.
See the [changelog](CHANGELOG.md) for release changes.

## Your first review

From the **root of the checkout you want to review**:

```sh
gh auth login                         # if you have not authenticated already
cd /path/to/your/checkout
prui                                  # browse this checkout's open PRs
# Or open a specific PR from the same checkout:
prui open https://github.com/owner/repo/pull/42
```

1. Select a PR with the arrows and press Enter. The Overview shows its description
   and discussion. Click **Files** to read the changes.
2. Use **Up/Down** to move within the focused pane and **Tab** to switch between
   the file list and diff. **Enter** activates the selection; **Escape** goes back.
   You can also click items and scroll over either pane.
3. Press **Space** or **Ctrl+K** to open **Actions**. Press the displayed
   hotkey to choose a tool, or select with arrows and Enter. This is where you find marking files as
   read, discussions, saved reviews, display options, and optional AI guides.
4. In the focused diff, press **Enter** on a commentable line to compose a comment.
   **Enter in the editor posts immediately. Ctrl+P queues a local pending comment**
   for your review. Shift+Enter adds a newline; Escape discards the editor. The
   editor's Tab/Shift+Tab selects Comment, Suggestion, or File comment where supported.
5. Choose **Submit review** above or from Actions. Choose Comment, Approve, or
   Request changes, add a summary, and confirm to send it with queued comments.
   For Comment, the summary is optional when you have queued comments.

Press **?** for a short getting-started guide and review health. Press **s** inside
help to reveal the full shortcut reference. Advanced shortcuts remain available;
you can learn them as you need them.

Reading progress saves immediately. **Actions → Saved reviews** reopens a saved
comparison. If the PR changes, use **Refresh GitHub metadata** and **New comparison** in
Actions; the old comparison and its drafts remain available, and only files
proved entirely unchanged retain read marks. **Changes since previous review**
shows captured changes and draft outcomes. AI guide generation opens a provider
and model chooser and requires confirmation before uploading eligible source.

Use `ctrl+v` on a raw diff line to set a range start, `j/k` to select its end,
and Enter to compose. Both endpoints must be on the same side and in one captured
hunk. `ctrl+o` chooses the old/new side of a paired split row or an unchanged
context row before starting the range. The range anchors remain visible during selection and editing.
Use `ctrl+f` to compose a comment on the selected changed file, including binary
and gitlink files with captured metadata. File comments post immediately with
Enter; GitHub's documented batch review endpoint does not support them, so
`ctrl+p` rejects them while preserving the editor. Historical ranges and file
comments are explicitly unsupported; use the pinned PR Files view.

Posting requires GitHub Pull requests write permission. Pending comments, editors,
and review summaries persist privately with their original comparison. New
comparisons never copy or silently remap drafts. An uncertain attempted request
requires an explicit outcome check in the original saved comparison before retry.
See [incremental review provenance and storage](docs/implementation/issue-32-review-progress.md).
Marking files as read does not submit a GitHub review.

The launcher recognizes standard GitHub HTTPS/SSH `origin` remotes. A checkout
without a supported origin can still use `prui open <PR-URL>`.
For offline reading, list session IDs with `prui sessions`, then run
`prui resume <id> --offline`.

### Everyday controls

| Key | Action |
| --- | --- |
| Up / Down | Move in the focused list or diff |
| Tab / Shift+Tab | Switch list and diff panes |
| Enter | Activate a selection; in the diff, compose a comment or open comment actions |
| Space / Ctrl+K | Open Actions; use its hotkeys, or arrows and Enter |
| Escape | Go back or close the current menu |
| ? | Getting started and health; s inside help shows all shortcuts |

Choose views with the labeled tabs above the review. The Actions menu includes
secondary views, display settings, guide expansion, progress tools, and saved
reviews. It opens from an idle review; editors, searches, filters, and other
modals keep their own keyboard controls. See the [full keyboard reference](docs/REFERENCE.md#keyboard)
for optional accelerators.

Overview shows the captured PR description followed by live Discussions. Press
`D` (or click **Discussions [D]**) to jump past a long description. In Discussions,
Up/Down moves the cursor, Tab/Shift+Tab jumps between activity, Enter expands
or collapses a card in place, and
`c` explicitly refreshes. Inline cards show captured code before the comment;
outdated or unplaceable comments retain labeled historical context. General
comments and review decisions appear in chronological order alongside inline
activity. `n` opens a new PR comment box at the bottom of the feed and scrolls to it.
`r` opens a reply editor beneath a selected comment: inline comments reply to
their code thread, while PR comments create a new message mentioning the author.
Enter posts; Escape cancels in place.

Press `y` to copy the selected discussion item's URL, or a selected inline
comment's URL in Files/Guide or its action menu. Clipboard writes use
OSC 52 and require a terminal that allows it. The shortcut leaves text editors
unchanged; items without a URL show an unavailable notice.

All text fields accept your terminal's paste shortcut or menu action, including
comments, replies, suggestions, review summaries, search and filters, the PR
switcher, and the guide model field. **Ctrl+V** also requests clipboard text in
terminals that support OSC 52 clipboard reads. In the diff with no text field
active, Ctrl+V still starts or cancels a line range. Multiline editors paste at
the cursor; single-line fields reject multiline text and retain their input
limits. Pasting never submits a comment or review.

Discussion bodies render Markdown with real paragraph breaks, lists, code,
tables and clickable link labels. Embedded HTML is converted to readable text;
images use their labels, and hidden bot metadata is omitted. Expand collapsed
details with Enter; Escape collapses the selected card. Bordered backgrounds
separate the description and discussion cards, and highlight the card under the cursor.
Code examples stay literal, and rendering never changes the text used to edit
or post a comment. PR descriptions retain their existing literal HTML policy.

Use **Open in Files [f]** on a placeable inline activity to select its exact
comment or reply in Files, then Enter to open the existing reply/thread actions.
Escape closes inner editors first and then returns to the originating Overview
activity, restoring the previous Files filter and reading position. **View
original commit [o]** is available when its original anchor is captured. Missing,
stale, or mismatched anchors show why Files navigation is unavailable. Older
threads also stay inline in Files when captured original/head source at the same
path proves the RIGHT-side code unchanged, including uniquely matched shifted
lines and ranges. These cards are labeled **Earlier commit** and open their
original discussion for actions; GitHub's outdated/resolved status is preserved.
Changed or ambiguous code and unavailable source remain in historical context.
Discussion refresh and posting retain the existing freshness and delivery checks;
view switching does not fetch or post.

The PR list uses two lines per entry: the title above author, review, checks, and target branch. Linear stacks show base-first positions such as `1/4` and `2/4`, based on branch dependencies among the loaded open PRs.

Newly opened pull requests start on the Overview tab. Click **Files** (or press `2`) to read the diff. The file list shows filenames beside muted directories. Click **Filter (F)** in the file header or press `F` to filter filenames and paths (case-insensitive, including previous names for renamed files). Enter returns to navigation with the filter applied; Escape in the list clears it.

Press **/** or click **Find** in the diff header to search saved diff text.
Results are grouped by file; Up/Down selects a match and Enter jumps to it.
Matches are highlighted in the diff. Guide search covers only the current section,
including collapsed children. Escape moves from query editing to results; use
`n/N`, `j/k`, or arrows to select a match, `/` to edit again, and Escape again to close.
With search results retained, `n`/`N` in the diff jumps to the next/previous match.
Search is literal and case-insensitive,
and does not fetch omitted context or full files.

Drag-select text in a content pane to copy it automatically on release. A small
`copied to clipboard` toast appears at the bottom for two seconds (requires a
terminal that permits OSC 52 clipboard writes). Single-click selects;
double-click opens a PR or list item, focuses review detail, or opens a diff
comment editor. Page and form controls support mouse clicks, and the wheel
scrolls content and navigates lists. Pane resizing, themes, and side-by-side
diffs are also available.
In Files or Guide, select **Commits [C]** to choose commits for one net diff.
**All changes** restores the full PR comparison. Selected commits are read-only;
use All changes to mark files or the Commits tab to discuss an individual commit.
Diff syntax highlighting is captured from the complete pinned old/new files and
saved as token spans for offline review; no extra full-file source is retained.
Syntax capture has a 5-second comparison budget and a 1-second limit per file side.
Capture failures are labeled beside the affected diff and in the review footer;
`?` Health & help shows the file and reason. These diagnostics survive offline
resume. Unsupported file types receive an informational label. Existing empty
captures with no recorded reason are labeled as such. Open a new comparison to
retry capture; reopening a saved review preserves its metadata.

Older sessions use best-effort hunk highlighting. Unsupported languages and files
beyond highlighting limits use the normal source foreground. Colors follow the
active theme; colored `+`/`-` markers and full-row green/red backgrounds keep
additions and deletions distinct. Ordinary source stays the same color whether
or not a line has syntax tokens, and changed-word emphasis preserves token colors.
Functions, types, macros, constants, attributes, and built-ins use separate token
categories where Chroma recognizes them, with configurable symbol colors. Rust
function calls and many custom type uses remain plain because its lexer does not
classify them. Saved reviews retain the categories captured when they were opened.
See the [user reference](docs/REFERENCE.md) for all controls and limitations,
or run `prui --help` for command syntax.

## Optional AI guides

Guides group related changes into sections with an overview and a reading order.
Press `g` to choose OpenAI, Anthropic, Google, or a configured OpenAI-compatible
endpoint. The modal shows the provider, editable model ID, and recipient;
Enter confirms the upload and Escape cancels. Set `OPENAI_API_KEY`,
`ANTHROPIC_API_KEY`, or `GEMINI_API_KEY` for the corresponding native provider.
With no guide configuration file, OpenAI defaults to `gpt-6.1-sol`. A usable
remembered provider/model choice takes precedence; you can change it in the modal.
See [model and endpoint configuration](docs/REFERENCE.md#guide-model-configuration)
for custom providers and local endpoints.

Opening, resuming, and switching PRs never generate guides automatically.
Completed generation creates a derived session with fresh reading progress and
retains the original session. Raw diffs remain available if generation fails.

**Guide requests send source to the selected provider**, including eligible
patches and potentially excerpts from unchanged files in the pinned repository.
The credential filter is heuristic and can miss secrets. Use only source you
are authorized to share and an endpoint you trust with both source and its API
key. OpenAI Responses requests set `store: false`; other providers have their
own retention policies. Credentials and full provider transcripts are not saved
locally. Guides can be wrong and are not a completeness or security verdict.
See [request contents and privacy limits](docs/REFERENCE.md#guide-request-contents).

## Storage and safety

Reviews use pinned Git objects, comparing the verified merge base to the PR head.
Reviewed code, hooks, filters, and external diffs are not executed. Git, `gh`,
and your executable search path are trusted.

Saved snapshots contain raw source and may contain secrets already present in
the PR. They are permission-restricted, **not encrypted**, and retained until
deleted. SQLite is embedded; no database service or SQLite installation is needed.

- macOS: `~/Library/Application Support/prui/storage`
- Linux: `$XDG_DATA_HOME/prui/storage`, or `~/.local/share/prui/storage`

`prui delete <id>` immediately removes a session and unreferenced source data;
reusable guide caches and remembered repositories can survive deletion.
See [storage and deletion](docs/REFERENCE.md#local-storage-and-deletion) for details.

Large or unavailable content is visibly labeled; the default per-blob limit is
1 MiB. PR browsing returns up to 100 PRs. Discussion retrieval is bounded to
500 threads and 2,000 comments, with partial results labeled. See the reference
for [discussion limits](docs/REFERENCE.md#inline-review-comments) and
[source limits](docs/REFERENCE.md#limits-and-incomplete-states).

## Development

For changes, add a focused regression test and preserve the source/privacy
invariants. Run the verification gates:

```sh
./scripts/verify.sh       # formatting, vet, race tests, PTY tests, build
golangci-lint run ./...   # v2.13.2, as pinned in CI
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
go run github.com/zricethezav/gitleaks/v8@v8.30.1 git --redact --no-banner --log-opts='--all' .
```

Tests require Python 3 and use synthetic repositories, fake GitHub clients,
and local HTTP servers. Live credentials are unnecessary. See
[TESTING.md](TESTING.md) for verification and live acceptance,
[CONSTRAINTS.md](CONSTRAINTS.md) for the correctness and safety contract, and the
[architecture guide](docs/ARCHITECTURE.md) for the package map.
The [spec archive](docs/spec/README.md) and [historical audit](docs/AUDIT.md)
preserve design and review context. Maintainers can follow the
[release guide](docs/RELEASING.md) to publish a version.

## License

Licensed under the [MIT License](LICENSE).
