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

Install Go **1.26.8+**, then:

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

The PR picker shows the list beside a description and metadata preview on wide
terminals, and stacks them on narrower terminals. Use Tab to focus the preview,
then `j` / `k` or Page Up / Page Down to scroll; Tab returns to the list.
Shift+J / Shift+K scroll the preview from either panel without changing focus. Mouse
clicks focus either panel and the wheel scrolls the panel under the pointer.
Overflowing selected PR titles scroll horizontally, keeping the PR number and
metadata fixed. Usernames and check states use distinct theme accents.
The footer stays at the bottom. Descriptions are captured with the list; `r`
reloads it. Opening a review captures its own frozen description.

1. Select a PR and press Enter. prui captures a fixed comparison and opens Files.
2. Use `j` / `k` to select files and `l` to focus the diff. Scroll with up / down;
   press `m` to mark a file as read. Progress saves immediately.
3. Press `q` to leave and reopen the PR later, or press `s` to select a saved
   session. Reading progress is retained. Use `r` to check freshness and `N` to
   start a new comparison if the PR changed; the old session remains available.
   Only files proved entirely unchanged retain read marks. Press `5` for captured
   changes since the previous review, `n/p` to navigate files, `d` for draft anchor
   outcomes, and `o` to inspect the original snapshot and drafts.
4. In the focused diff, press Enter on a commentable line to compose a comment.
   **Enter in the editor posts immediately.** Ctrl+P queues it for a review,
   Shift+Enter adds a newline, and Escape discards the editor.
5. Press `R` to choose Comment, Approve, or Request changes, add a summary, and
   submit it with your queued comments after a separate confirmation.

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

### Essential controls

| Key | Action |
| --- | --- |
| `j` / `k` | Select files or guide rows |
| `h` / `l` | Focus list / diff |
| Up / down | Scroll the focused diff |
| `m` | Mark or unmark a file as read |
| `/` | Find code text across Files or within the current Guide section |
| `F` | Filter Files by filename or path; Enter keeps the filter, Escape clears it |
| `1` / `2` / `3` / `4` | Show Description / Files / Guide / Commits |
| `v` / `V` | Cycle forward / backward through Description, Files, Guide, and Commits |
| `P` | Switch PRs |
| `ctrl+p` | Inside a line/range editor, queue the comment |
| `ctrl+v` | Start/cancel a range; j/k selects the end |
| `ctrl+o` | Choose old/new side of split or context rows |
| `ctrl+f` | Compose an immediate file comment |
| `D` / `c` | Browse discussions / refresh discussions |
| `C` | Filter the Files/Guide diff by selected commits |
| `R` | Open review submission |
| `g` | Choose an AI guide provider and model, then confirm upload |
| `r` / `N` | Check freshness / start a new comparison with proven unchanged progress |
| `5` | Captured changes since previous review and private draft anchor outcomes |
| `s` | Browse saved sessions |
| `?` / `q` | Health & help / quit |

Newly opened pull requests start on the Description tab. Use `2` to switch to Files. The file list shows filenames beside muted directories. Click **Filter (F)** in the file header or press `F` to filter filenames and paths (case-insensitive, including previous names for renamed files). Enter returns to navigation with the filter applied; Escape in the list clears it.

Press **/** or click **Find** in the diff header to search saved diff text.
Results are grouped by file; Up/Down selects a match and Enter jumps to it.
Matches are highlighted in the diff. Guide search covers only the current section,
including collapsed children. Escape moves from query editing to results; use
`j/k` or arrows to select a match, `/` to edit again, and Escape again to close.
Search is literal and case-insensitive,
and does not fetch omitted context or full files.

Mouse selection, pane resizing, themes, and side-by-side diffs are also available.
In Files or Guide, select **Commits [C]** to choose commits for one net diff.
**All changes** restores the full PR comparison. Selected commits are read-only;
use All changes to mark files or the Commits tab to discuss an individual commit.
Diff syntax highlighting is captured from the complete pinned old/new files and
saved as token spans for offline review; no extra full-file source is retained.
Older sessions use best-effort hunk highlighting. Unsupported languages and files
beyond highlighting limits retain ordinary diff styling. Colors follow the active
theme, with full-row green/red backgrounds that keep additions and deletions
distinct from unchanged code.
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

Unsent review work is saved privately alongside local session storage, separately
from frozen source and guides. Inline and reply editors, pending comments, review
summaries, and decisions recover when you reopen the same pinned comparison.
The recovery modal shows retained text even offline. Changing the PR head starts
an independent draft; reopen the previous saved session to inspect the old one.

In the quit confirmation, `s` saves and quits, Enter discards drafts in open tabs,
and Escape continues reviewing. Escape inside a comment/reply editor discards
that editor. Use `d` to remove a pending comment or `ctrl+d` in the review/recovery
modal to discard that comparison's drafts. Deleting its last saved session also
deletes its drafts. Storage is private local SQLite, not encrypted or synced to
GitHub.

After a failed, canceled, or interrupted submission, `ctrl+r` checks GitHub before
retry is enabled. An exact remote match asks you to discard the retained draft;
failed or bounded/incomplete history keeps retry blocked. No request is repeated
automatically. Matching uses the original attempted request, not later edits.
Identical earlier submissions conservatively count as matches; delayed requests
still have no GitHub idempotency guarantee.

For complete pinned OLD/NEW source and expanded context, opt in with
`prui open <PR> --cache-full-source` (also available on `current`, `prs`, and
`resume --new`). This stores bounded, private, unencrypted local source without
AI upload. On Files use Ctrl+E for context, Alt+O/N for OLD/NEW, Ctrl+D for diff,
Ctrl+W for whitespace presentation, and / plus F3/Shift+F3 for code search.
Alt+Up/Down open previous/next loaded unresolved thread.
[Storage, offline coverage and read-only coordinate policy](docs/spec/SPEC-code-navigation.md).

Live PR readiness: press `Alt+R` in a review to inspect individual checks, required
reviews and merge policies. Press `r` inside readiness to refresh its remote
evidence independently of pinned code. Unknown, partial and stale data remain
visible; no merge action is performed. See [readiness contract](docs/spec/SPEC-pr-readiness.md).

Suggested changes have a dedicated replacement editor: `ctrl+s` on right-side
lines/ranges shows Before/After, Enter posts, and `ctrl+p` queues. On an existing
inline suggestion, Enter opens actions and `ctrl+a` prepares a separately confirmed
remote commit. Application checks canonical source, comments, permission and expected
head without changing your checkout. See [suggestion limits and uncertainty](docs/REFERENCE.md#suggested-changes).

File organization and layout: `B` toggles category grouping, `Alt+C` collapses or
reveals generated files, and `F` finds every changed path including collapsed
files. Split/unified and pane widths persist across runs; narrow terminals keep
the desired wide layout for later restoration. See
[attribute rules, pinned provenance, defaults and limits](docs/spec/SPEC-file-categories-layout.md).
