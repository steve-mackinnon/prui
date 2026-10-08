# Changelog

User-visible changes are recorded here before each release. See
[Releasing](docs/RELEASING.md) for the versioning policy and release procedure.

## [Unreleased]

### Added

- Install or update the latest release with a single README command that detects
  macOS/Linux and ARM64/x86-64, verifies the checksum, and installs to `~/.local/bin`.

### Changed

- Richer diff syntax highlighting distinguishes functions, types, Rust macros,
  constants, attributes, and built-ins where supported by the language lexer.
  Symbol colors can be configured independently of the rest of the interface.

## [0.3.0]

### Added

- Recover unsent inline comments, replies, pending reviews, and general PR
  conversation drafts privately, including offline recovery and reconciliation
  after uncertain submissions.
- Edit published comments and resolve or reopen review threads with explicit
  GitHub actions; compose range and file comments alongside line comments.
- Compare changes since a previous saved review and carry reading progress only
  when captured source proves the file unchanged.
- Inspect live checks, required reviews, and merge policy with `Alt+R`; use `l`
  there for separately confirmed merge, auto-merge, queue, draft/ready, and
  close/reopen actions.
- Browse a cached cross-repository review inbox with `prui inbox`, with explicit
  refresh, filters, local activity tracking, and offline access.
- Read requested reviewers and linked GitHub or authorized Linear issue context
  with `I`, independently of frozen code and AI guides.
- Group files by category, collapse generated files, find changed paths, and
  retain split/unified layouts and pane widths across runs.

- Select the next or previous search match with `n` or `N` while results are focused, or jump between retained matches directly from the diff.

- Copy the selected comment or discussion URL to the clipboard with `y`.

- Expand context with E in Files or Guide, fetching the selected file’s pinned source on demand and reusing it in the open tab; the footer advertises E. Ctrl+D restores the compact diff.

- Open HTTP and HTTPS links from Description and code views with a mouse click; underline links on hover.

- Show each pull request’s target branch in the PR list, selected preview, and opened Description view.

- Filter Files and Guide by selected commits with `C`, showing one net diff while retaining the dedicated Commits browser.

- Find code text with `/` in Files and the current Guide section, with results
  grouped by file, direct match navigation, and highlights in unified and split diffs.

- Compose right-side suggested replacements with Before/After previews, recover private suggestion drafts, and apply eligible published suggestions through a separately confirmed, expected-head GitHub commit without touching the checkout.

- Highlight diff syntax using full pinned-file context, with theme-aware colors and saved token spans for offline reviews.

- Add full foreground/background theming with 22 named presets, bringing the catalog to 26 themes, and group the theme picker into Dark and Light sections with candidate previews.

### Changed

- Make added and removed diff rows stand out with stronger, full-width green/red backgrounds, including blank lines and files without syntax highlighting.

- Simplify the Generate guide dialog with a centered header, distinct field labels and values, and grouped consent and keyboard help.

- Default OpenAI reading guides to `gpt-6.1-sol`; configured and remembered model choices still take precedence.

- Show only the active guide, keep its copy expanded, pin its position tracker, and continue diff navigation across guides.

- Promote Guide to a top-level tab alongside Description, Files, and Commits, with number shortcuts 1–4.

- Wrap diff lines at word boundaries in unified and side-by-side views; use left/right to scroll long tokens.

- Replace the working indicator with a single-cell breathing-dot animation beside the loading label.

- Emphasize changed words in paired diff lines with bold and underline.

### Fixed

- Move through Files diff headers and metadata one display row at a time with j/k.

- Restore J/K scrolling of the source diff in Commits, including while the commit list is focused.

- Prevent a crash when opening a discussion comment's original line before browsing Commits.

- Submit all pending comments with a Comment review without adding a summary.

- Render discussion Markdown and embedded HTML as readable prose, hide bot
  metadata, and expand collapsed details in place in the Overview feed.
- Separate descriptions and discussions with bordered cards, keep a visible
  page cursor while scrolling, and compose comments and replies inside the feed.

- Wrap long inline comment and reply drafts within the editor, keeping the caret visible without changing submitted text.

- Preserve syntax colors and source text when underlining changed words in diffs.

- Keep visible search results at the diff's left margin instead of horizontally clipping the surrounding code; pan only for matches beyond the viewport.

- Reveal the original target line when returning from a file comment to a normal comment, including tall restored drafts.

- Keep the comment editor visible when Tab or Shift+Tab changes the comment type.

- Keep typing `q` in inline comments, suggestions, file and commit comments, and replies from triggering quit.

- Recover general PR drafts privately with immutable uncertain-write evidence; comparison resets preserve old work separately.

- Show the commit filter as a modal over the current review, retaining the underlying Files/Guide workspace.

- Preserve changed-line backgrounds across syntax-color resets, preventing partially highlighted diff rows.

- Keep the review footer at the bottom of the Description page, including short and empty descriptions.

- Make guide-dialog arrows navigate controls and switch the configured provider/model pair from any control.

- Keep the Guide selection in sync when scrolling or navigating the diff.

- Open the Guide tab at the first section instead of following the current file selection.

- Scroll 15 lines with `d`/`u` and keep cursor and line numbers aligned in Files,
  Guide, and Commits.
- Preserve quotes and backslashes in displayed source, highlight expanded and
  full pinned source, and retain historical review threads on unchanged code.

### Compatibility

- Requires Git and an authenticated GitHub CLI (`gh`); binaries remain available
  for macOS and Linux on ARM64 and AMD64.
- Saved draft payloads now include range, suggestion, and general conversation
  state. Earlier binaries reject and preserve newer unsupported draft records;
  reopen them with this release to recover the work. Back up the private local
  data directory before downgrading. No manual data migration is required.
- Guide is now a top-level tab and tab shortcuts are `1`–`4`. Existing configured
  guide models still take precedence over the new default.
- `v0.2.0` was tagged but did not publish because its changelog release section
  was missing. This release includes the changes since published `v0.1.0`.

## [0.1.0]

First public release of prui, a terminal interface for reviewing GitHub pull requests.

### Added

- Browse pull requests and review frozen diffs, individual commits, and PR descriptions.
- Save reading progress locally and reopen captured sessions offline.
- Post inline comments, manage discussions, and explicitly submit GitHub reviews.
- Generate optional AI reading guides after confirming the provider and source upload.
- Choose terminal themes and side-by-side diff views; use plain output for scripts.
- Download macOS and Linux binaries for ARM64 and AMD64, with SHA-256 checksums.
- Identify installed builds with `prui --version`.

### Compatibility

- Requires Git and an authenticated GitHub CLI (`gh`); supports GitHub.com repositories.
- The 0.x series is under active development. Minor releases may include documented
  breaking changes; patch releases preserve compatibility.

- Add opt-in pinned full OLD/NEW source, expanded unchanged context, local full-file
  search, whitespace presentation and loaded unresolved-thread navigation (#30).
