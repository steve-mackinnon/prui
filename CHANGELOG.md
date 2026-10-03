# Changelog

User-visible changes are recorded here before each release. See
[Releasing](docs/RELEASING.md) for the versioning policy and release procedure.

## [Unreleased]

### Added

- Add full foreground/background theming with 22 named presets, bringing the catalog to 26 themes, and group the theme picker into Dark and Light sections with candidate previews.

### Fixed

- Keep the Guide selection in sync when scrolling or navigating the diff.

- Open the Guide tab at the first section instead of following the current file selection.

### Changed

- Default OpenAI reading guides to `gpt-6.1-sol`; configured and remembered model choices still take precedence.

- Show only the active guide, keep its copy expanded, pin its position tracker, and continue diff navigation across guides.

- Promote Guide to a top-level tab alongside Description, Files, and Commits, with number shortcuts 1–4.

- Wrap diff lines at word boundaries in unified and side-by-side views; use left/right to scroll long tokens.

- Replace the working indicator with a single-cell breathing-dot animation beside the loading label.

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
