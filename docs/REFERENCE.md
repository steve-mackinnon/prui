# User reference

See the [README](../README.md) for installation and a short introduction. This reference covers interaction, storage, and source limits.

## Command line

Launching `prui` from a repository root opens its PR list. These commands
cover cases that need a specific target or non-interactive output:

| Command | Use |
| --- | --- |
| `prui --version` | Print the build version and commit without opening a checkout or session. |
| `prui open <PR-URL>` | Open a specific PR, including from a checkout without a supported `origin` remote. |
| `prui open 42 --github-repo owner/repo --plain` | Print a review for a script or non-interactive terminal. |
| `prui prs owner/repo` | Print up to 100 open PRs for a repository. |
| `prui prs` | Browse remembered repositories interactively when outside a checkout. |
| `prui sessions` | List saved session IDs, progress, and last freshness status. |
| `prui resume <id> --offline --plain` | Read a saved snapshot without GitHub or the original checkout. |
| `prui resume <id> --new --repo <checkout>` | Start a new comparison using another checkout; keep the old snapshot. |
| `prui delete <id>` | Immediately delete a local session. |

In the TUI, `s` opens saved sessions and `N` starts a new comparison. Direct
`resume <id>` remains available when you already know the ID. Opening a PR in
plain mode saves a snapshot. Normal resume checks GitHub; `--offline` disables
network operations. `--plain` is also selected automatically for redirected
input or output and `TERM=dumb`. It emits escaped, uncolored text without
interactive actions. Exit codes are `0` for complete inventory, `1` for errors,
`2` for unavailable review content, and `130` for canceled loading.
Local reading progress and exit success do not constitute GitHub approval.
Run `prui --help` for the full syntax.

### Pull request list

Pull request rows show the author, check status, and your latest GitHub review:
👁 for a submitted review, ○ for no review, and ✎ for a draft. The selected row
shows whether you approved, requested changes, commented, or had a review
dismissed. This records your latest review, even if new commits have arrived
since it.

## Optional review guides

Guides group the frozen review units into functional chunks, each with a title, description, and ordered sections that reference specific units. They are an interpretation layer: `Slices` and `UnitFiles` are unchanged, reading progress stays file-slice based, and the raw inventory remains the complete source view. Each unit is assigned to a guide exactly once; anything the model did not group lands in a synthesized `Ungrouped changes` guide.

Selecting a PR from the interactive PR list or switcher displays the latest validated saved review immediately when one exists, with freshness shown as unknown until an asynchronous GitHub check finishes. The check reuses a matching frozen comparison without Git fetching or resource rebuilding; a changed comparison is pinned normally and replaces the displayed review when ready. A previously generated guide for the same repository, PR number, base SHA, and head SHA is reused locally only when its guide selection also matches. Press `g` in an interactive review to open the guide modal over that review. Provider, editable model ID, sanitized recipient, and source-upload warning are visible together. The last confirmed valid pair is preselected, so `g` then Enter confirms it; Tab or up/down selects a control, left/right changes the configured provider and its model from any control, and typing or deleting in Model edits the model ID. With only one usable provider, the modal says so instead of offering a provider switch. Escape closes the modal or cancels an in-flight request. Direct `open`, `resume`, `--plain`, offline mode, and `verify` do not generate guides. API keys are never written to a snapshot, log, or error string. With no guide configuration file, OpenAI's default model is `gpt-6.1-sol`, and `OPENAI_BASE_URL` may override its endpoint. There are no guide-generation model or exclusion CLI flags. `resume --offline` rejects guide generation and all GitHub operations before accessing clients or credentials; stored guides remain readable.

### Guide model configuration

The global guide configuration is read from `$XDG_CONFIG_HOME/prui/config.json`
when `XDG_CONFIG_HOME` is absolute, or `~/.config/prui/config.json`
otherwise. This rule applies on macOS and Linux. The file is separate from
`theme.json`, session storage, and the reviewed checkout. An absent file
defaults to OpenAI with `gpt-6.1-sol`, `OPENAI_API_KEY`, and the optional
legacy `OPENAI_BASE_URL` origin override. A present file supplies the initial
provider and model and any custom endpoint or key-variable name;
`OPENAI_BASE_URL` no longer affects that configuration. The guide modal can
select another currently usable provider and model for each confirmed request.

```json
{
  "guide": {
    "provider": "google",
    "model": "<Gemini-model-id>",
    "api_key_env": "GEMINI_API_KEY"
  }
}
```

`provider` must be `openai`, `anthropic`, `google`, or
`openai-compatible`, and `model` must be a nonempty model ID. The native
providers default to `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, and
`GEMINI_API_KEY` respectively; `api_key_env` can name another variable.
Store the key in that environment variable, not in the JSON file. The
configuration is resolved for the interactive session; restart the CLI after
changing it. The provider list includes native providers whose designated key
variables have nonempty values. A compatible provider is selectable only when
configured with its endpoint and key, or with an explicitly keyless loopback
HTTP endpoint. Key values are never displayed or stored. If no provider is
usable, confirmation is disabled.

The last confirmed provider and model are stored as a private, atomic
`guide-last.json` file beside `config.json`. This preference contains only
the provider and model ID; it does not replace endpoint or key-variable
configuration. A usable remembered pair takes precedence when the modal opens.
If it is unavailable, the configured pair is used when available, followed by
another usable provider. Invalid preference data is never overwritten
silently. Editing the model ID requires a nonempty ID without whitespace or
control characters. There is no provider model catalog or local model allowlist.

An OpenAI-compatible endpoint needs an explicit API prefix, usually including
`/v1`, and a key variable:

```json
{
  "guide": {
    "provider": "openai-compatible",
    "model": "<endpoint-model-id>",
    "base_url": "https://models.example/v1",
    "api_key_env": "MY_MODEL_API_KEY"
  }
}
```

For an explicit loopback HTTP endpoint, `api_key_env` may be omitted when
the endpoint needs no credential. OpenAI also accepts `base_url`; an origin
gets `/v1` appended, while a path already supplied is retained. Anthropic
and Google use their fixed provider endpoints. Remote URLs must use HTTPS;
redirects are refused. Invalid configuration stops a guide request before
source or credentials are sent. A compatible endpoint must support the
schema-tool request used for structured guide output; there is no text-mode
fallback or automatic provider switch.

A completed guide generation creates a new derived session with empty reading progress, retaining the original frozen snapshot and its progress. Escape cancels an in-flight request; cancellation keeps the current session and does not save a derived session. Missing credentials or invalid provider configuration leave the current session intact.

### Guide request contents

The request contains a comparison ID and eligible review units from the frozen
PR inventory. Each included unit carries its ID, file path, kind, and patch
text, including added and removed lines. File metadata units can carry a path
without patch text. Units withheld by the privacy policy or request budgets
are not sent; the provider sees only counts and generic reasons for withheld
units, not their paths.

The request can also contain excerpts from files **outside the PR diff**. When
the session opens, the app examines the pinned head tree and the merge-base
versions of deleted files. Manifests, documentation, and tests in the same
directories as changed files rank highest. Other files in those directories
share the next rank with manifests, documentation, and tests elsewhere; other
source files follow. This ranking covers the repository trees, so unchanged
files elsewhere in the repository can be selected. Evidence records a path, source
tree, kind, and line range locally; the provider receives the path, kind, line
range, and excerpt text. A blob larger than the per-file limit is omitted, not
partially read. The evidence is frozen with the session, so guide generation
does not read the live checkout or uncommitted work.

Evidence collection is budgeted at 100 retained files, 32 KiB per blob,
256 KiB of retained excerpts, and five seconds. The final guide request keeps
at most 400 units, 32 KiB per unit, and 512 KiB of patches plus evidence,
with a five-minute analysis deadline. The current consent screen describes these
categories and the recipient; it does not list the exact paths in the assembled request.
There is currently no user-configurable path exclusion for guide uploads.

Only the assembled request package leaves the machine. The privacy policy is
applied again at that boundary. An excluded file's metadata, patches, and
associated evidence are withheld, including both names of a rename. Built-in
exclusions cover credential-like filenames (such as `.env` and `.pem` files),
binary content, and recognized credential-like text. Budget limits can omit
individual units while retaining other units from the same file. The credential
filter recognizes common quoted keys and key material, but is heuristic and
can miss secrets; confirm uploads only for source you are authorized to share.

The selected endpoint receives both source and its credential, if one is
configured, so use only a trusted endpoint. Guide generation makes at most
one non-streaming model request with structured `prui_guides` output.
OpenAI uses Responses with `store: false`; other providers use their native
structured-output protocols, and no shared retention policy is promised.
Requests are limited to 2 MiB and responses to 4 MiB. Redirects are not
followed. Provider transcripts and credentials are never stored; the snapshot
keeps the guides, provider, model, prompt version, schema name, input digest,
evidence IDs, limits, withheld ledger, and a non-secret selection fingerprint.
Omission ledgers stay local. A legacy guide without a fingerprint is eligible
for automatic reuse only under the unchanged default OpenAI selection.

Failure is cheap and explicit. A transport error, non-2xx status, refusal, deadline, oversize payload, or unusable structured output produces an `analysis_unavailable` bundle with a stated reason, a durable session, and the unchanged deterministic file plan. Only a structurally valid generated bundle is persisted as a reusable local cache entry; unavailable bundles never suppress a later retry. Retrying cannot modify a stored snapshot; a later successful attempt is a new session. Generated text is model interpretation of the bounded input, not source truth, approval, security findings, or complete architectural documentation, and it can be wrong about anything it was not shown.

Reviews open on the Files tab. The right pane shows every changed file in order, with all of each file's hunks together. Scroll through the whole PR or press `j`/`k` to jump between file boundaries. Press `F` for Files or `G` for Guide. When a session has generated guides, Guide shows each guide, section, and covered file portion, with the selected guide's combined diff in the right pane. Selecting a section or file with `j`/`k` scrolls to its position in that diff; Enter also focuses the diff. Selecting a guide restores its saved scroll position. A file appears under every section that owns part of it. `n` walks guide rows, `}`/`{` jump between guides, and `tab` expands or collapses the selected guide or section. `]`/`[` widen or narrow the left pane by two columns. `i` lists every raw unit. `m` marks the whole file slice of the selected unit. A fallback, absent, or empty bundle leaves Files available.

Reviews opened in one process keep independent in-memory reading positions, hierarchy expansion, pane focus, scroll offsets, notices, and errors. The first tab, `PRs [P]`, opens the PR list by click or uppercase `P`. It opens a PR switcher over the current review. It lists already-open reviews first, then open PRs for the active repository; typing filters, Enter switches or starts a new read-only pinned review, and Escape leaves the current review unchanged. The overlay never takes a permanent column or changes plain output.

The interactive review uses a quiet workspace: a persistent repository/PR
identity (and title when available from the PR browser), understated context
tabs, and a file/guide rail separated from the detail by one divider. The
selected row, active context tab, and pane focus remain explicit without color.
Below 100 columns, the focused pane uses the full width.

The status line keeps `R Submit review` and the pending count visible alongside
local reading progress and freshness. Warnings take priority over routine
health information. At heights of 10 rows or more, a second footer line shows
shortcuts for the current pane, context view, or comment editor; shorter
terminals use one footer row. `?` opens **Health & help**, with complete review
health and shortcuts grouped under Navigate, Review, Views, Diagnostics, and App.
Use arrows or j/k to scroll help, Page Up/Down to page, and Escape to return.
The editor explicitly labels Enter as **post now** and Ctrl+P as **save pending**;
these actions keep their existing behavior.

PR browsing and switching use compact rows. The selected PR's available author,
opened date, last contributor, and checks appear below the list when space
permits. Short terminals prioritize visible choices and selection.

Diff detail opens in unified layout. Press `S` to prefer side-by-side source
rows for the active review tab; that preference stays in memory only and is not
shared with another open PR. At fewer than 160 terminal columns, the TUI keeps
the preference but automatically renders unified detail and says
`side-by-side needs 160 columns`; split detail resumes after resizing to 160
columns. Plain output is always unified.

## Keyboard

| Key | Action |
| --- | --- |
| `P` | Open the PR switcher; type to filter, Enter switches/opens, Escape cancels |
| `n` / `p` | Move between comment targets in a focused Files diff; move through files, guide rows, or inventory units in the list |
| `}` / `{` | Next / previous guide, or file slice without guides |
| `]` / `[` | Widen / narrow the left file and guide pane by two columns |
| `G` | Select the Guide tab |
| `C` | Open the commit filter in Files/Guide |
| `S` | Toggle side-by-side detail; unified is the default and narrow terminals fall back below 160 columns |
| `/` | Find code text in Files or the current Guide section |
| `F` | Filter Files by filename or path; Enter keeps the filter, Escape clears it |
| `1` / `2` / `3` / `4` | Select Description / Files / Guide / Commits |
| `v` / `V` | Next / previous PR context view: Description, Files, Guide, or Commits |
| `tab` | Expand / collapse the selected guide or section |
| `h` / `l`, `ctrl+h` / `ctrl+l` | Focus list / diff |
| `enter` / `esc` | In the list, focus the selected diff / go back; in a focused diff, open a composer for a target or an action menu for a selected comment / discard local drafts and menus |
| `enter` in a line editor | Post the line comment immediately; `shift+enter` inserts a newline |
| `ctrl+p` in a line editor | Save or update a local pending comment without posting |
| `R` | Open Submit review from any review tab |
| `c` | Refresh ephemeral review discussions and inline comments (online reviews only) |
| `D` | Open the PR-wide Discussions list from any context view |
| `j` / `k` | Jump to next / previous file in Files; navigate guide rows or inventory units elsewhere |
| Up / down | Navigate list or scroll focused diff |
| `J` / `K` | Scroll diff by 5 lines |
| `zz` | Center the focused diff on its selected line |
| `d` / `u`, Page Down / Page Up | Page through actual diff |
| Left / right | Horizontal scrolling; no hidden line truncation in plain output |
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

Selection, expansion, and diff scroll offsets survive resizing; navigation positions and expansion state are not persisted across processes, and they are rebuilt from the immutable bundle so navigation cannot drift from the stored guides. Mouse capture supports selection and divider dragging; use your terminal selection modifier for native text copying. Textual markers and labels are primary: the selected row is marked `› ` whether or not its pane is focused; a muted background reinforces it when unfocused and reverse video reinforces the focused row. The interactive view additionally colors diff structure — file headers, hunk locations, additions, removals — and unit states such as metadata, binary, gitlink, unavailable, and warning chrome. Color is presentation only: no wording, label, or ordering depends on it, and terminals without color show the same text. Extremely small terminals clip controls; enlarge or use plain output. Broad terminal/platform/accessibility coverage is not established.

Every interactive PR has `Description` (`1`), `Files` (`2`), `Guide` (`3`), and `Commits` (`4`) context views, in that order. Reviews open on Files; `F` opens the Files filename filter; `G` selects Guide. Guide has its own top-level tab and requires explicit generation with `g` when no guide is available. Description is display-only GitHub-flavored Markdown frozen from GitHub when the session opened; it is available after an offline resume and does not refresh when selected. Raw HTML remains literal text, links are not activated, and images never load. Empty captured descriptions and older sessions that did not capture one are labeled explicitly. Context views are not included in `--plain` output.

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
Wheel input scrolls diff and description panes, moves selection in navigation
lists, and navigates discussions. Double-click activation is not supported. Mouse capture
may affect native terminal text selection; use your terminal's selection
modifier (often Shift, depending on the terminal or multiplexer) to copy text.

### Filtering the diff by commits

The **Commits [C]** control in Files and Guide opens a checkbox modal over the
current review. Use
up/down or j/k to navigate and Space or Enter to toggle a commit. Select
**All changes** to restore the original full-PR comparison. Escape or C closes
the picker and retains the selection. Selecting no commits shows an empty view;
selecting every commit in a complete captured list restores All changes.

Selected commits contribute to one net diff, which drives both the file list
and code pane. Repeated edits combine and cancelling changes disappear. Changes
apply in history order starting from the first selected commit's parent. Skipped
commits do not contribute changes; a selection that depends on a skipped change
can conflict. Conflicts or missing frozen source are explained instead of showing
a misleading partial result. Merge changes use the first parent.

Filtering uses frozen local data and leaves the checkout untouched. Older sessions
without composition source keep the full comparison and dedicated Commits browser.
Incomplete commit capture is labeled; selecting all captured entries does not
silently include uncaptured commits. Filter selection lasts for this process and
is independent of the Commits tab's selected commit.

Filtered reading does not mark files or compose line comments. Existing pending
review drafts are preserved. Editing a pending comment from the review form
returns to All changes so its original target and editor are visible. Return to
All changes to mark or comment on the PR
diff, or use the dedicated Commits tab for commit discussions. Guide text remains
an interpretation of the full PR; filtering does not generate a new guide, and
unmatched filtered files remain accessible in a separate group.

### Commit browsing

`4` opens a two-pane commit browser: the captured PR commit list on the left,
and all changed files in the selected commit on the right. Selecting a row
immediately changes the diff. Rows show subject, author, and abbreviated SHA in
GitHub's returned order. The first returned commit is initially selected.

| Input | Commit list focused | Commit diff focused |
| --- | --- | --- |
| `j`/`k`, Down/Up | Next/previous commit | Scroll one line |
| `n`/`p` | Next/previous commit | Next/previous commit |
| PageDown/PageUp, `d`/`u` | Page through commits | Page through diff |
| Home/End | First/last commit | Start/end of diff |
| `l`, Ctrl+L, Enter | Focus diff | Enter composes on a supported comment target |
| `h`, Ctrl+H, Escape | Stay in list | Return to list |
| `[`/`]` | Resize commit list at 100+ columns | Same |

Click commits to select them; wheel over the list moves selection and wheel
over the diff scrolls it. Below 100 columns the focused pane fills the body;
Enter/`l` opens detail and Escape/`h` returns to the list. Selection clamps at
the ends. Revisiting a commit restores its reading offset during this run;
switching context views preserves commit state and Files/Guide reading state independently.

Each unified commit diff compares against its first parent. Merge commits are
labeled as first-parent comparisons, root commits compare against the empty
tree, and empty commits are labeled. These are individual changes, including
changes later reverted in the PR. Online discussions appear below exactly
matching original commit lines, with textual outdated/resolved status and thread
counts in the rail. Commit rows do not mark files or alter pending PR drafts.
Enter can compose an immediate comment on a captured head-commit line that also
exists in the frozen PR diff. Older commits support added and context lines (RIGHT side) in complete, regular
single-parent diffs of added or modified files. Historical deletions, renames,
copies, root commits and merge commits remain non-commentable. GitHub can mark a
comment outdated immediately if later commits changed its line. These commit composers
cannot queue a comment with `ctrl+p`; `R` still opens the head-based PR review
form. `S` changes layout in Files and Guide.

Commit metadata and bounded patches are frozen during opening and stored with
the session; selection and offline resume make no Git/GitHub calls. The first
100 commits are captured, with a visible notice when more may exist. Capture
has an aggregate 50 MiB material / 100,000-line / 60-second budget plus the
existing blob and storage bounds. Missing or limited diffs are labeled while
the main PR review remains usable. Older sessions display that commits were not
captured; they do not backfill on resume.

## Inline review comments

The interactive diff pane marks one selected display line. Press `enter` while that pane is focused to open a Markdown composer only when the line is commentable: added and context lines target the new-file `RIGHT` side, and deleted lines target the old-file `LEFT` side. Headers, hunk markers, binary/metadata/unavailable content, and paths that cannot be sent as valid JSON are not commentable. In Files, `n`/`p` move the line cursor through commentable lines; scrolling and the cursor are separate.

Press `enter` on a selected commentable diff line to open an inline editor immediately beneath it. The bordered editor keeps the draft visually separate from code and shows a blinking caret at the edit point. Type Markdown normally; `enter` posts one comment immediately, `ctrl+p` adds it to the local pending review, `shift+enter` adds a newline, and `esc` discards the editor. Opening a line with an existing pending comment reopens that draft. Up to 100 pending comments appear under their target lines and in the Submit review screen; they are memory-only and are lost when the app exits. A failed immediate submission keeps the editor and target for an intentional retry.

`R` opens **Submit review** from any PR context view. The form shows Comment, Approve, and Request changes as radio choices; use `j`/`k` or up/down to select one. All three choices and the selected state remain visible through confirmation, including on short terminals. Press Tab or Enter to move to the comment box below. Comment and Request changes require text; Approve allows an empty comment. Tab again to browse pending line comments: Enter edits the selected draft at its diff target, and `d` removes it. Enter in the comment box opens a confirmation showing the review decision and pending count; a second Enter sends the review and all pending comments to GitHub in one request. Escape backs out without writing. A failed submission leaves the comment and pending drafts in memory for inspection; because a network failure can have an uncertain outcome, check GitHub before retrying. Successful submission clears the local queue and refreshes the inline comment overlay.

Online reviews load bounded review discussions through GitHub GraphQL; `c`
refreshes them. `D` opens the PR-wide list, including outdated, file-level,
multiline, and unplaceable discussions. Select with j/k or arrows, Enter reads
replies and a historical snippet, `o` views the captured original commit when
its context is available, and Escape returns. Thread detail displays a validated
GitHub permalink. Missing capture is labeled without assuming a force-push.

GitHub supplies separate outdated and resolved flags: outdated never implies
resolved. Current-line comments render in the main diff only when live PR pins
match the frozen comparison and path/side/line match exactly. Original anchors
render on their own commits. Comments with unavailable context remain in `D`.
Limits are 500 threads, 2,000 comments, 100 comments per thread, 4 MiB aggregate
response material and 60 seconds per refresh; partial results and counts are
labeled. A successful post is inserted immediately from GitHub's canonical
response. An uncertain posting outcome retains the draft and directs you to
refresh before retrying; `ctrl+r` refreshes while keeping an active draft. Writes
never retry automatically. Discussions and drafts
remain memory-only, absent from sessions, guides and plain output. Offline
reviews neither fetch nor post them; browsing captured commits stays offline.

Comment boxes participate in focused-diff `j`/`k` navigation, are visibly marked, and are kept in view. `enter` on one opens a local action menu: `r` opens a separate rune-aware reply box indented beneath that message, `a` opens the finite GitHub reaction picker (`1`–`8` choose its displayed reaction), and `d` is shown only after the authenticated viewer identity matches the displayed comment author. GitHub replies can target only a thread’s top-level comment, so `r` on an existing reply automatically uses that root and renders the canonical response in its thread. Reaction totals are compact emoji chips (`👍`, `👎`, `😄`, `😕`, `❤️`, `🎉`, `🚀`, `👀`) in the message box’s bottom border; an explicit non-UTF-8 locale uses the original GitHub token labels instead. Deletion requires a second `enter` confirmation. `esc` always closes the menu or draft without a write. Reply/reaction/deletion requests are preflighted against the frozen PR and update only that tab’s memory-only overlay from the canonical GitHub response; a failed reply retains its draft for retry.

Immediately before posting a comment or submitting a review, prui re-reads GitHub metadata and requires the repository identities plus base and head SHAs to equal the frozen comparison. If they differ, it makes no write and asks you to open a new comparison. A successful request uses the frozen head SHA and line targets; GitHub can still mark a comment outdated if the pull request advances after that preflight. Writes are unavailable in `--plain` and `resume --offline`, and no open, resume, refresh, guide action, or background task can submit one.

## Color

The interactive TUI supports twenty-six built-in themes. The selector shows
Terminal separately, followed by Dark and Light groups:

| Theme ID | Appearance |
| --- | --- |
| `terminal` (default) | Inherited terminal colors with ANSI accents |
| `light` | Neutral accents for a light terminal |
| `dark` | Neutral accents for a dark terminal |
| `high-contrast` | Strong semantic accents |
| `catppuccin-mocha` | Soft pastels on deep blue |
| `one-dark` | Cool Atom-inspired colors on charcoal |
| `tokyo-night` | Blue and violet on a dark night background |
| `gruvbox-dark` | Warm earthy colors on dark gray |
| `ayu` | Ayu Light with warm orange accents |
| `ayu-dark` | Ayu Dark |
| `gh-dark` | GitHub Dark |
| `gh-light` | GitHub Light |
| `gruvbox-light` | Warm Gruvbox Light |
| `horizon` | Horizon's warm dark palette |
| `material-dark` | Material Dark |
| `material-deep-ocean` | Material Deep Ocean |
| `melange` | Melange's warm dark palette |
| `monokai` | Classic Monokai |
| `night-owl` | Night Owl |
| `one-light` | One Light |
| `poimandres` | Poimandres |
| `rose-pine` | Rosé Pine |
| `rose-pine-dawn` | Rosé Pine Dawn |
| `vscode-dark` | VS Code Dark |
| `vscode-light` | VS Code Light |
| `wombat` | Wombat |

The named family presets paint the entire app viewport with their base
foreground/background, including blank space, panes, editors, and modals. The
original four themes retain inherited base colors and their existing accents.
Selections and semantic change colors remain distinct. Theme colors affect app
cells only; they do not change terminal default colors.

Choose a theme for one invocation with `--theme NAME` on `prui`, `current`,
`open`, `resume`, or `prs`; an invalid name exits before normal work begins.
For example, `prui --theme catppuccin-mocha`. The flag is accepted with `--plain`
but has no visible effect. `verify` does not accept `--theme` and never reads
personal theme configuration.

The optional global configuration file is `theme.json`:

- macOS: `~/Library/Application Support/prui/theme.json`
- Linux: `$XDG_CONFIG_HOME/prui/theme.json` when `XDG_CONFIG_HOME` is
  absolute; otherwise `~/.config/prui/theme.json`

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

Supported tokens are `foreground`, `background`, `title`, `fileHeader`, `hunk`, `added`, `removed`,
`metadata`, `warning`, `unavailable`, `selection`, `focusedBorder`, and
`border`. Values must be `#RRGGBB`, `default`, an ANSI name (such as `red` or
`bright-yellow`), or an ANSI palette index from `0` through `255`, written as
a string. The explicit `--theme` name wins over the file's `theme` value, and
valid `colors` overrides remain applied. Invalid JSON, duplicate or unknown
keys, invalid names, and invalid color values are ignored as a whole: the app
warns safely and uses the terminal palette (or a valid explicit `--theme`).
Such a file is never overwritten. `foreground` and `background` are optional
base-channel overrides; `default` inherits that channel from the terminal.
Existing configuration files need no migration. Older prui versions reject new theme IDs and the
two new keys, so omit them when sharing configuration with those versions.

Press `t` in an interactive review or navigation picker to open a
theme picker. It is unavailable while another modal owns input.
The list scrolls to keep the candidate visible and shows its position and group.
Arrows and `j`/`k` follow the displayed order, skipping Dark/Light headings;
heading clicks do not select or save a theme. At
60 columns by 18 rows or larger, a synthetic sample previews candidate base
colors, changes, warnings, and selections using your overrides. Browsing does
not apply or save a candidate; small terminals retain a compact candidate view.
Use arrows or `j`/`k` to choose and Enter to save and apply a built-in; Escape
or `t` cancels. A successful choice atomically updates only the global
`theme` value, retaining valid configured color overrides. It does not change
review or session data. A failed save keeps the active palette unchanged.

PR-description Markdown inherits the app background and ordinary foreground
when base colors are explicit. The preset’s Dark/Light classification selects
the corresponding existing Markdown baseline. Its Markdown and syntax foreground
colors remain; these presets do not introduce syntax themes.

Color is still detected once at startup from the terminal and process
environment. `NO_COLOR` and `TERM=dumb` disable it, `CLICOLOR_FORCE` follows
the existing capability rules, and redirected output is never colored.
Lower-capability terminals downsample to 256 or 16 colors rather than losing
text. Themes supplement these accessibility safeguards: color is never the
only cue, and the picker retains textual names, markers, and controls even
when color is disabled.

Styling runs after source escaping, layout, and clipping. Logical content and
width remain unchanged; explicit base colors may add only blank viewport
padding. Colorless output skips canvas painting and preserves existing logical
bytes. Colored lines never exceed the terminal width, horizontal scrolling
never splits an escape sequence, and `--plain` contains no terminal controls.
Palette sources and licenses are recorded in [theme attribution](THEME-ATTRIBUTION.md).

## Sessions And Freshness

Every `open` saves a new frozen session, even with `--plain`. Reopening a cached comparison can copy its reading progress; a newly pinned comparison starts unreviewed. `sessions` lists full IDs, progress, and historical check status. Resume loads stored patches and metadata, not the checkout: branch deletion, garbage collection, or deleting the checkout cannot change the review. Full unchanged source is not retained; only patches and bounded pinned-tree evidence are available offline.

- `unchecked`: no freshness check this opening (`--offline`, or a check interrupted before completion).
- `current`: base/head SHAs and repository identities matched at the last explicit check, not a continuous guarantee.
- `stale`: base/head or repository identity changed. Keep navigating the old snapshot, or press `N` / use `resume ID --new` for a new unreviewed session.
- `check_failed`: metadata/authentication/connectivity failed; freshness unknown, frozen review still available.

Resume checks metadata by default; `--offline` disables all network actions for that invocation. New comparisons use the saved absolute checkout path unless `resume --repo` overrides it. Failed replacement preserves the old snapshot/progress. A new comparison or newly generated guide starts unreviewed, even with identical patch bytes. Reopening a cached PR preserves reading progress, including when it reuses a cached guide. Local completion never means GitHub approval or complete source availability.

Additional evidence is read only from pinned Git tree/blob objects. It prioritizes manifests, documentation, nearby tests, and changed directories within 100 files, 32 KiB per excerpt, 256 KiB retained bytes, and five seconds. Press `e` to inspect retained evidence and omissions. Relationships are observational unless explicitly marked otherwise; AST indexing and history expansion are deferred. Exclusions are local policy, and credential filters are defense in depth rather than secret detection.

## Local Storage And Deletion

The CLI prints storage location; TUI help shows storage and session ID. Defaults:

- macOS: `~/Library/Application Support/prui/storage`
- Linux: `$XDG_DATA_HOME/prui/storage`, or `~/.local/share/prui/storage` when unset/non-absolute

Storage must be outside the reviewed checkout. Old file caches and unrelated nonempty directories are rejected without import or deletion. Symlink roots and unsafe database/control files are rejected. Directories use 0700 and files 0600. Use a local filesystem with working locks and sync semantics; Windows support is deferred.

The application embeds SQLite in process through a CGo-free Go driver. Users do not need a SQLite executable, shared library, or database service. `store.sqlite3` contains immutable, content-addressed source and guide payloads, independent session state, ordered progress, reusable guide references, and remembered repositories. `.sqlite-owner` records storage ownership and initialization phase; `.sqlite-init.lock` protects only initialization. Do not remove these control files. SQLite manages short transaction locks and its rollback journal; several app processes can open the store at once. Operations have bounded waits and report contention rather than silently losing updates.

Progress updates compare the expected generation and immutable snapshot reference, validate reviewed file membership, and commit state atomically without reading or rewriting source payloads. Listing sessions reads summary metadata; opening a session validates its payloads and references. Source and guide payloads are each limited to 128 MiB. Corrupt records and unsupported formats fail visibly and remain untouched. SQLite uses DELETE journaling, EXTRA synchronization, and foreign-key enforcement.

Stored raw patches may contain sensitive source, including secrets already present in the PR. Storage is permission-restricted, not encrypted; checksums detect corruption, not malicious same-user rewriting. No credentials from `gh`, raw provider transcripts, or telemetry are stored. Sessions remain until explicitly deleted. Reopening a PR uses indexed recency/comparison queries and validates matching candidates newest-first. Opening a large review still materializes its source payload.

`delete ID` atomically removes that session and its progress, reclaiming source and guide payloads only when no session or reusable guide cache references them. Derived sessions remain readable after their parent is deleted. Independent cached guides and repository registrations survive. Deletion has no confirmation prompt and does not guarantee forensic erasure or immediate database-file shrinkage; no automatic VACUUM runs. GitHub state, provider records and backups are unaffected. Normal source-fetch temporary storage is cleaned independently; a kill/power loss before a session exists can leave unassociated `prui-*` directories in the OS temp directory.

Read-only commands require an existing initialized store and do not initialize, repair, or recover it. If a rollback journal requires recovery, reopen through a writable command first. The previous `sessions` directory is left untouched and is not used by this version.

## Source Truth And Safety

- GitHub metadata pins base/head repository identities and 40-character SHA-1 commit IDs. Compare the single verified merge base to head, not base tip to head. Ambiguous histories fail visibly.
- Git runs in private tool-owned bare storage, with sanitized environment/configuration. Local object files are hard-linked into an isolated borrowed view, never rewritten. Hooks, repository config, replacement refs, promisor markers, and alternates are not imported. Worktree Git pointers are supported; bare checkout inputs and object-directory symlinks are rejected. Cross-filesystem hard-link failures are explicit, not copied silently.
- Missing ancestry triggers visible cancellable HTTPS fetches of the exact pinned base/head into the private object store, without borrowed reachability, tags, submodules, maintenance, or checkout. Metadata is read again after fetch; one changed-pin retry is allowed, then failure. Authentication/resource failures do not retry automatically.
- Fetch credentials come from `gh auth token`, then an environment-only, host-scoped Git HTTP authorization header. No token in argv, diagnostics, or files. Redirects and credential helpers are disabled. `gh`, Git, their installed runtime helpers, and the user's executable search path are trusted; this is not a sandbox against a compromised Git binary or local same-user attacker.
- Raw tree metadata is NUL-delimited with full object IDs. Path fields and patch artifacts use byte arrays (lossless base64 in JSON). Display escapes controls, invalid UTF-8, and backslashes; raw stored bytes remain unchanged. Symlinks are blob content, never dereferenced. Submodule pointers show OIDs without reading submodule repositories.
- Every file has a separate metadata unit. Text units contain real Git blob-to-blob hunks: Git's headers name object IDs and use blob mode, while the adjacent file metadata is authoritative for paths, modes, additions, deletions, and renames. No authored/reconstructed patch lines. Binary content uses explicit cards (NUL in the first 8 KiB); Git LFS pointers are reviewed as committed text, not downloaded.
- Local progress is persisted separately from immutable source content. Temporary metadata/object storage is removed on normal completion or cancellation. SIGKILL, machine failure, or power loss can leave private `prui-*` directories under the OS temporary directory; deletion is not guaranteed forensic erasure.

## Limits And Incomplete States

Defaults: 10,000 changed entries, 50 MiB retained patch content, 1 MiB per blob, 100,000 retained diff lines, 60 seconds per Git/network operation, and 512 MiB fetched object storage. Fixed Myers diff, three context lines, no indent heuristic, 50% rename detection with 10,000 candidate limit. Settings/limits participate in inventory identity.

Entry/raw-record limits or missing comparison trees fail the comparison rather than offer a misleading partial file list. Oversized or unavailable blobs/patches remain explicit unavailable units; inventory is labeled INCOMPLETE. Binary/gitlink cards represent known non-text changes, not absent entries. Zero changes is an explicit empty comparison. Inventory completeness is not analysis completeness or reading progress.

Process stdout/stderr are bounded; overrun, timeout, or cancellation kills the process group. Fetch storage is sampled every 10 ms plus checked after each fetch. It can overshoot between samples or during filesystem scans, and counts stored bytes rather than network download bytes. Borrowed hard-linked objects are not counted as downloaded storage. No exact download cap or total process-memory bound is claimed.

## Acceptance and guide evaluation

From the root of a target checkout, run a live acceptance journey with:

```sh
prui verify https://github.com/owner/repo/pull/42 --artifacts /tmp/prui-artifacts
```

This contacts GitHub through authenticated `gh`, opens a pinned comparison, then
drives an offline terminal journey (navigate, mark, quit, resume). Python 3 is
required. `--artifacts` must name a new directory outside the checkout. It receives
`report.json`, a bounded terminal transcript, and SVG/text screen captures, which
can contain reviewed source. The verifier uses a private temporary session store,
never executes reviewed code, and never generates guides or posts comments.

Opening has a 60-second default timeout; `--open-timeout DURATION` changes it.
`--measure-runs N` collects 1–10 timing observations. The JSON report records
metadata, pin/inventory, and offline startup timing. This live acceptance command
is intentionally excluded from CI; the underlying journey has synthetic tests.

`prui eval-guides SESSION_ID` reads a local session with a read-only store
handle and prints a JSON report. It never contacts GitHub or a provider, reads
provider credentials, or creates guides. Structural validation is always included;
synthetic-corpus checks apply only to matching curated fixtures. A session without
stored generated guides reports `not_available`.

### Complete PR conversation

`D` opens chronological live activity: general PR comments, submitted review
bodies and decisions, and each inline-thread comment/reply. UTC timestamps and
actors are shown; `c` explicitly refreshes while preserving selected activity.
Partial retrieval, stale snapshots, unavailable reads and offline mode are labeled.
General comments and reviews each load at most 500 records, with a combined 4 MiB
response budget and the discussion operation's 60-second deadline.

In conversation, `n` opens a general PR comment; `r` from a general-comment detail
opens a new @mention PR comment. These replies have no inline anchor. Enter posts
immediately after comparison freshness checks; Shift+Enter adds a newline and
Escape discards. Local text, reply context and cursor recover privately at the
original comparison, separately from pending review submissions. An
unknown posting outcome retains the draft and requires Ctrl+R reconciliation before
intentional retry; if the attempted body appears as new activity, another post is
blocked until the reviewer inspects it and starts a new comment. Inline replies
continue through diff comment actions. `o` on inline detail opens original context.
See [the conversation contract](spec/SPEC-pr-conversation.md).


## Find text in diffs

On Files or Guide, press `/` or click **Find (/)** in the diff header.
Files searches all available saved text hunks, including files hidden by the
filename filter. Guide searches only the selected section, including collapsed
children. Select a section or its file row first; a guide heading has no section
scope. Full inventory mode searches the saved comparison.

Type literal text; matching ignores case and includes additions, deletions, and
saved unchanged context. It excludes paths, headings, comments, and guide prose.
The popover groups occurrences by file, with line/side labels and snippets.
Up/Down or PageUp/PageDown selects a result; Enter or a result click reveals it
in the diff without opening a comment. Activating a file hidden by the filename
filter clears that filter and shows a notice. `F` filters filenames.

Escape leaves the query field and focuses results. Use `j/k` or Up/Down to
select results; Enter jumps to the selected match. Press `/` (or click the query)
to edit again, and Escape from results to close while keeping highlights. Tab
also switches between editing and results. There are no Clear/Close buttons;
use Backspace/Delete to edit or empty the query. Query editing supports
Left/Right, Home/End, Backspace/Delete, and single-line paste up to 1,024 UTF-8
bytes. Search owns keyboard input while open; letters such as `j/k` and `q` are
query text only while editing. Result navigation never changes the query.

Search stays local and works offline. It searches saved patches, not entire files
or omitted context. Unavailable/non-text content is labeled as skipped. Results
are capped at 10,000 with an explicit refine-query notice; visible diff matches
remain highlighted. Context lines count once in split view, and activation uses
the valid right-side target. Colorless terminals keep selection and line labels
without changing source text. Queries and results are not persisted.

## Complete pinned source and navigation

`--cache-full-source` is explicit consent to store bounded complete OLD/NEW
changed-file source locally for newly opened comparisons; it is not remembered
or AI-upload consent. On Files, Ctrl+E expands unchanged context, Alt+O/N opens
OLD/NEW source, Ctrl+D restores diff and Ctrl+W hides whitespace-only replacement
runs. / searches that source scope; F3/Shift+F3 step between results.
Additional context/full files are read-only; canonical patches retain comment
anchors and progress. Missing source and limited search coverage remain visible.
Alt+Up/Down open previous/next loaded unresolved-thread detail; unknown resolution
is excluded. See [the complete policy](spec/SPEC-code-navigation.md).

### Live PR readiness

`Alt+R`: open checks/reviews/blockers for the PR. Within the view, `r`/`c`/`ctrl+r`
refresh remote readiness; j/k, arrows, PgUp/PgDn, u/d, Home/End and the mouse wheel
scroll; Escape returns. Both pinned and observed live head SHAs are displayed.
Readiness is ephemeral, unavailable offline and never uploaded in guide material.
See [the read-only readiness contract](spec/SPEC-pr-readiness.md).

## Suggested changes

In a focused PR diff, use `ctrl+s` to edit a replacement for selected right-side
lines (`ctrl+v` selects a range). The editor shows Before/After; Enter posts a
GitHub suggestion, `ctrl+p` queues it, and an empty replacement deletes the lines.
Replacement edits and range anchors recover privately, including offline.

Enter on a published inline suggestion opens comment actions. `ctrl+a` prepares
application and shows the head repository, branch and replacement. Enter opens
confirmation; a second Enter creates a remote commit. GitHub permissions and
expected branch head are checked; the checkout and pinned snapshot stay untouched.
Open a new comparison to review the resulting commit.

Application supports a current right-side anchor on an ordinary non-executable
UTF-8 file up to 512 KiB. Left/file/historical targets, multiple/offset suggestions,
symlinks, executables, unavailable source, stale comments/pins and conflicting text
are refused. Forks require write access to the fork's head repository; upstream
maintainer permission alone is unsupported by this commit mechanism. This creates
a GitHub commit without claiming native suggestion-applied badges/thread resolution.

Uncertain application retains its exact attempted payload privately and blocks
retry. `ctrl+r` reconciles read-only; a matching whole-file result requires explicit
discard. An unchanged head or unrelated later content cannot prove absence and
keeps retry blocked. `ctrl+d` discards the retained apply attempt. Offline recovery
shows retained replacement/source but never applies or checks GitHub.
