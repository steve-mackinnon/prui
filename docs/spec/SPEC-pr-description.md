> Navigation update (2026-10-05): [Overview conversation](SPEC-overview-conversation.md)
> supersedes the separate Description/Discussions navigation: Overview combines
> the captured description and live activity; D jumps to Discussions; f opens
> a verified inline comment in Files and Escape returns. Existing data, draft,
> and write-safety contracts remain applicable.

# Spec: Frozen Pull Request Description View

## Objective

Populate the Description context view with the PR author's frozen GitHub body.
It lets a reviewer read the PR's rationale, change summary, and review notes
inside the same pinned review session—including offline/resumed sessions—without
claiming that prose is source truth or current remote state.

## Tech Stack

- Go 1.26.8+ with `encoding/json`, Bubble Tea, and Lip Gloss
- `charm.land/glamour/v2` for width-aware GitHub-flavored Markdown rendered
  as ANSI terminal content
- GitHub CLI `gh api` through the existing bounded `source.GH` boundary
- Existing immutable session snapshot and private atomic store

## Commands

```sh
go test ./internal/source -run 'Test.*(Metadata|Description)' -count=1
go test ./internal/session ./internal/review ./internal/tui -count=1
go test -race ./internal/source ./internal/session ./internal/review ./internal/tui
./scripts/verify.sh
git diff --check
```

## Project Structure

```text
internal/source/github.go      validated GitHub PR presentation data
internal/source/git.go         pin revision comparison that ignores presentation-only fields
internal/review/review.go      attach frozen description while constructing a session
internal/session/store.go      immutable snapshot field and validation
internal/tui/{model,render}.go Description view state, input sanitization, and cached Markdown rendering
internal/{source,session,review,tui}/*_test.go  boundary, persistence, and display coverage
README.md                      frozen-description and offline behavior
```

## Data and Lifecycle Contract

`source.Metadata` continues to be the single response boundary for
`GET repos/{owner}/{repo}/pulls/{number}`. It gains a presentation-only
description field with the exact GitHub `body` string; `null` maps to the empty
string. The external response is rejected when body is not valid UTF-8 or
exceeds 1 MiB. It is presentation data only: it is never treated as
instructions, executed HTML, or an executable terminal payload.

The source package must provide a comparison-only predicate, equivalent to:

```go
func SamePinnedRevision(a, b Metadata) bool {
    return a.Identity == b.Identity &&
        a.BaseRepository == b.BaseRepository && a.HeadRepository == b.HeadRepository &&
        a.BaseSHA == b.BaseSHA && a.HeadSHA == b.HeadSHA
}
```

Pin/fetch retry logic uses that predicate rather than Go struct equality. A
description edit with unchanged source revisions is therefore allowed to create
a new frozen session without forcing a misleading source re-fetch; changed
repository identities or SHAs retain today's retry/failure semantics.

`session.Snapshot` gains an optional `PullRequestDescription` string. Opening a
new review copies the validated body into that immutable field before the first
store write. A cached comparison only reuses a snapshot if it contains the same
validated frozen presentation data returned by the metadata call; otherwise it
creates a new snapshot session from the existing frozen source material. This
keeps Description accurate for the session while source cache identity remains
base/head based.

Old snapshots lacking the optional field remain valid and readable. Their
Description view displays a clear, non-error empty state: `Description was not
captured for this session.` It makes no background request and offline mode
continues to access no GitHub client or credentials.

## Rendering Contract

- Empty newly captured bodies display `No description provided.`
- Nonempty bodies render with one configured Glamour v2 terminal renderer.
  Its wrap width is the available Description-body width. Use the repository's
  theme-appropriate style; do not read the renderer's environment-driven
  style configuration.
- Interpret GitHub-flavored Markdown presentation syntax: headings, emphasis,
  lists, blockquotes, fenced code blocks, task lists, tables, and links.
  Links are display-only; they do not become actions, and images do not fetch
  or display remote content.
- Normalize `CRLF` and lone `CR` to `LF` before Markdown parsing. This removes
  the current visible `\\r` artifact and gives equivalent layout for all valid
  Markdown line endings.
- Before Markdown parsing, replace every C0/DEL control character other than
  `LF`, `CR`, and `TAB` with a visible escaped representation. Do not apply
  the generic Go-string `Escape` function to the Markdown source: it would
  destroy Markdown semantics. Do not strip ANSI from Glamour's output, because
  its ANSI styling is intentional.
- Raw HTML remains literal text. Never configure the Markdown parser or
  renderer to enable raw HTML, unsafe URLs, or terminal escape sequences from
  the PR body.
- Rendered content is cached by frozen description identity, rendered width,
  and active theme/style. Rebuild the cache when any of those inputs changes,
  including a terminal resize; scroll navigation consumes cached ANSI lines
  without reparsing the body.
- The view starts at top on each newly opened PR tab. Its scroll offset is
  tab-owned, stays independent of diff/guide offsets, survives view switches
  during the process, and resets after restart.
- The view includes compact provenance: `Frozen from GitHub when this review
  opened.` It must not say or imply that the description is current.

## Code Style

Validate once at the GitHub boundary, use value types for frozen presentation
data, and return concise generic availability errors without raw provider
output. Rendering receives already-frozen data, must not call `source.GH`, and
must retain enough renderer metadata to invalidate a stale width/style cache.

```go
func renderedDescription(body string, width int, style glamour.TermRendererOption) ([]string, error) {
    safeMarkdown := normalizeDescriptionControls(body)
    renderer, err := glamour.NewTermRenderer(style, glamour.WithWordWrap(width))
    if err != nil { return nil, err }
    out, err := renderer.Render(safeMarkdown)
    if err != nil { return nil, err }
    return strings.Split(strings.TrimSuffix(out, "\\n"), "\\n"), nil
}
```

## Testing Strategy

- Unit-test valid body, null body, invalid UTF-8, oversized data, and errors
  without using a real `gh` process or credential.
- Prove `SamePinnedRevision` ignores description edits but not source pins.
- Round-trip a new snapshot and an old fixture with no field; test cache reuse
  and re-session behavior for changed bodies at unchanged SHAs.
- Test CRLF and lone-CR descriptions render without visible `\\r` text and
  preserve intended paragraph/list layout.
- Test headings, emphasis, nested lists, blockquotes, fenced code, tables,
  task lists, and links at narrow and wide widths.
- Test raw HTML remains literal; image and link Markdown cause no network or
  action; and untrusted ANSI/OSC/control input cannot affect the terminal.
- Test cache reuse for scrolling and cache invalidation after resize, theme
  selection, or a newly opened frozen description. Retain colorless terminal
  coverage and ANSI-aware width assertions.
- Retain empty, absent-old-session, online, and offline description coverage.

## Boundaries

- **Always:** freeze, validate, bound, sanitize, render, and label GitHub
  description data; retain old session readability and strict offline behavior.
- **Ask first:** schema-version migration, a manual/live refresh action,
  changing source-cache identity, rendering remote images, activating links,
  or storing remote comments.
- **Never:** execute/render GitHub HTML, allow PR-supplied terminal controls,
  fetch on tab selection or resume, overwrite a snapshot, or imply description
  text is verified source truth.

## Success Criteria

1. A newly opened online review displays its validated PR description after
   switching to Description and remains readable offline after restart.
2. A changed body alone cannot break pinned-source comparison/retry semantics.
3. Empty and legacy-absent descriptions are unambiguous, safe, and non-networked.
4. CRLF and CR-only PR descriptions render without literal `\\r` artifacts.
5. Common GitHub Markdown is readable in the Description tab at narrow and
   wide terminal widths; resizing does not leave stale wrapping.
6. Raw HTML is literal text; untrusted body content cannot inject terminal
   controls, activate links, or trigger remote image loading.
7. Source, session, review, and TUI regressions pass without live GitHub access.

## Open Questions

None. A manual remote refresh, remote-image rendering, and link activation are
intentionally out of scope because they alter the trust and freshness contract.
