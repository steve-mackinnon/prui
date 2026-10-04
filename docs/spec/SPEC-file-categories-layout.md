# File categories and persistent layout (issue #38)

Files remain identified by raw pinned inventory IDs. `B` toggles a category
hierarchy, `C` collapses/reveals generated file bodies and their rail entries,
and `/` finds paths including collapsed files and both rename paths. These
choices and `S` split/unified, keyboard `[`/`]` rail widths, mouse divider widths,
and the commit rail width are remembered across launches. Every file remains
available through reveal, path filtering, or `i` raw inventory. Counts always
refer to the complete inventory; generated counts stay visible when collapsed.
The current file stays visible and expanded. Category ordering affects only
rail navigation; raw diff boundaries, text, targets and progress stay unchanged.

## Committed attributes and safety

At capture, ordinary/added/renamed files use the pinned new tree and new path;
deleted files use the old merge-base tree and old path. Rename-old attributes do
not override the new file. Each frozen classification stores that tree SHA,
resolved recognized boolean attributes and category. Offline reopen reads those
values without Git, checkout files or network. Older sessions without evidence
use Support and never reinterpret current checkout policy. The evidence view
(`e`) shows the selected file's provenance; partial classifications are labeled
`category unavailable` in the file rail. Raw completeness is independent.

The trusted Git `check-attr --cached -z --stdin` evaluator operates against a
unique disposable index populated by `read-tree` in the existing isolated object
view. It supports installed Git's committed `.gitattributes` semantics, including
root/nested precedence, per-attribute later-line overrides, slash-relative globs,
`*`, `?`, classes, `**`, C-quoted paths, macros and `!attribute` resets. Directory
patterns are not recursive; use `directory/**`. Negative patterns are invalid
and Git ignores them. Unknown attributes, filters, diff/textconv drivers and
command definitions have no category effect and are never executed. Reviewed
`.git/info/attributes`, system/global attributes, index, configuration and dirty
checkout files are never authoritative and are not borrowed into the view.

Supported booleans are bare/set or `=true`, and `-attribute`/`=false`; unspecified
attributes restore path defaults. Nonboolean recognized values produce visible
unavailable classification with Support fallback. Other malformed rules follow
Git's ignore/default behavior; this is not a separate rule-validator and does
not surface individual Git parse warnings. Syntax unsupported by installed Git
has no added interpretation. No complete Linguist language/generated-content
heuristic implementation is claimed.

Recognized attributes, in deterministic conflict priority:

1. `review-implementation`
2. `review-test`
3. `review-documentation`
4. `review-generated`
5. `review-assets`
6. `review-agent-guidance` and `review-localization` (Support)
7. `linguist-generated`
8. `linguist-documentation`
9. `linguist-vendored` (Support)

Explicit positive review categories override Linguist. When none is positive,
path defaults apply: `_test.go`, `.test.`, `.spec.`, `test/`, `tests/` are Tests;
`docs/`, `.md`, `.rst` are Documentation; `generated/`, `.min.js`, `.pb.go` are
Generated; common image/font/audio/video extensions are Assets; `.github/`,
`scripts/`, dotfiles, `go.mod`, `go.sum`, lock files are Support; others are
Implementation. Binary content alone does not imply generated; asset extensions
still work for binary/missing content. Explicit false suppresses its matching
path default and returns Implementation (or Support for suppressed
Implementation). Attribute failures never hide files as Generated.

Preflight bounds the tree listing to 8 MiB/100,000 entries, each attribute blob
to 64 KiB, total attribute content to 2 MiB, the disposable index to 16 MiB,
paths to 10,000/4 MiB total/4 KiB each, and resolved output to 8 MiB. The existing
operation deadlines/process-group cancellation apply. Exhaustion, unsupported
attribute blob modes, invalid paths or failed retrieval produce partial Support
classification; they do not replace raw source with empty success. Temporary
indices are removed on success/failure/cancellation. Commit-only inventories do
not capture categories; main PR inventory owns this presentation evidence.

## Preferences

`layout.json` sits beside the app's global `theme.json` (macOS Application
Support/prui, Linux XDG config/prui). It is separate from immutable snapshot
payloads, SQLite generations, drafts and reading progress. Version 1 stores
`split`, `rail_width`, `commit_width`, `group_files`, `collapse_generated`.
Missing config defaults to unified, automatic widths, grouping/collapse off.
Changes are atomically saved with private permissions and file/directory sync.
Corrupt/unsupported/oversized/nonregular files stay preserved, with a visible
warning and defaults. Plain output never reads or writes this preference file.

Open tabs retain independent layout state; new tabs inherit the latest global
choices. Below 100 columns the active pane fills the screen; below 160 columns
split falls back to unified. These fallbacks never overwrite desired widths or
split choice. Only deliberate resizing changes the saved desired width.

## Sources and verification

- [Git attributes](https://git-scm.com/docs/gitattributes)
- [Git check-attr](https://git-scm.com/docs/git-check-attr)
- [Linguist overrides](https://github.com/github-linguist/linguist/blob/main/docs/overrides.md)
- [Linear review categories](https://linear.app/docs/diffs#organize-files-with-gitattributes)

Synthetic tests cover pinned Git semantics/isolation, budgets, nested overrides,
deletions, defaults, raw target/cache preservation, generated reveal/filter,
offline source/progress restart, persistent layout and narrow fallback. Human
terminal usability remains unverified by automated render tests.
