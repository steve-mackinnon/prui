# Spec: Consistent Diff Style Composition

Status: Approved; user authorized implementation on 2026-10-05 after reviewing
this specification. User requested a systematic fix specification that day.

## Objective

Make source foregrounds independent of syntax-token availability, fragment
boundaries, and nested terminal style resets. A reviewer should see consistent
code colors while addition/deletion backgrounds, markers, changed-word emphasis,
search matches, and selection remain distinguishable.

This supplements [diff syntax highlighting](SPEC-diff-syntax.md),
[shared diff rendering](SPEC-shared-diff-rendering.md),
[diff text search](SPEC-diff-text-search.md), and the repository's
[constraints](../../CONSTRAINTS.md). Upon approval, the foreground contract below
supersedes the syntax spec's fallback to whole-line added/removed foregrounds for
source text with no usable syntax metadata. Capture limits and fallback to
unhighlighted source remain unchanged.

## Assumptions and scope

- Ordinary source uses `theme.Foreground`, including unsupported languages,
  legacy captures, and explicit empty highlighting results. Diff identity remains
  visible through markers and backgrounds.
- Keep the existing syntax palette, word matching, search colors and selection
  conventions. This is a presentation correction, not a lexer or diff algorithm
  change.
- Cover every existing source presentation: Files, Guide, raw inventory, main
  unified/split diffs, Commits' numbered unified diffs, and existing OLD/NEW or
  expanded-source views. Do not add search or split support to Commits.
- No dependency, configuration, capture, persistence, or review-target changes.

## Evidence and diagnosis

Two synthetic TypeScript reproductions were run through the current renderer:

1. `.map(toItem),`, `}));`, and `}` produce no retained syntax spans. In
   `syntaxText`, the empty-span early return bypasses the normal foreground.
   An enclosing added-row style therefore colors all their text green.
2. `slug` changed to `id` is a whole-word replacement. `wordTokens` groups each
   identifier as one token; the shared letter `d` is not matched independently.
   Word rendering subdivides the line around that token. Fragments without syntax
   spans inherit the diff foreground, and nested foreground resets can leave a
   later character at the terminal default. The emitted sequence reproduces a
   green `i`, then a foreground reset before `d`, even though both characters are
   bold and underlined.

These are two manifestations of an inconsistent composition boundary. Existing
syntax-color tests check explicit keyword colors, but do not prove that ordinary
cells retain their base foreground after resets or fragment splitting. An outer
style wrapper alone is not sufficient evidence of correct rendered cells.

## Rendering contract

### Foreground and structural regions

In color-capable output, every visible ordinary source cell resolves to the active
theme's normal foreground, regardless of whether its line or fragment has syntax
spans. If the theme intentionally inherits its normal foreground, preserve that
inheritance; never substitute the added/removed foreground for source cells.

Valid syntax spans override only their covered source cells using today's category
palette. Plain text before, between, and after tokens follows the same base rule.
Strings may legitimately be green because their syntax role uses `theme.Added`.

Patch markers are structural: visible `+` and `-` markers retain the theme's added
and removed colors, respectively. Context markers, line-number columns, cursor
chevrons, continuation prefixes, borders, and separators keep their established
structural styling. They must not be accidentally recolored as source. Prefixes
and marker-free split source must be handled explicitly; a source character that
happens to be `+` or `-` is not a patch marker.

### Backgrounds and precedence

Composition is defined by region and channel, not by incidental ANSI nesting:

| Region / layer | Foreground | Background | Attributes |
|---|---|---|---|
| Ordinary source | Theme foreground or intentional inheritance | Owning row | Existing attributes |
| Syntax token | Existing token category | Owning row | Existing attributes |
| Added/removed row | Source/structural rules above | Existing 25% diff tint, full row width | No new attributes |
| Changed word | Preserve source/token foreground | Preserve owning row | Bold and underline over the complete changed span |
| Search match | Existing search composition | Existing match background | Existing active-match distinction; retain word emphasis |
| Selected row | Preserve resolved source/token foreground | Existing selection policy | Existing focus/reverse and cursor behavior |

Search continues to preserve explicit syntax foregrounds as the existing nested
composition does. Ordinary match text uses the existing search foreground when
available, falling back to normal source foreground. Search background wins over
diff background within the match; diff styling resumes immediately afterward.
Selection retains its existing focused reverse behavior and unfocused background
policy; this change must not redesign which selection channels override search.
Tests must verify both stored style channels and effective reverse presentation.

Neither foreground resets nor complete SGR resets may cause source to inherit a
diff color, lose required emphasis, or leak a match style into neighboring text.
Background repair must preserve explicit match/selection backgrounds. Source
foreground resolution must not overwrite syntax, search, or structural colors.

### Fragment and geometry invariance

Rendering a source interval as one piece or subdividing it into pieces must resolve
to the same source foregrounds, except for intended word/search attributes and
match colors. Entering search with no match on a row must not recolor that row.
Closing search must restore the same non-search presentation.

Wrapping, panning, clipping, and split projection transform spans and source
coordinates together. Styling must not change grapheme content, display width,
tabs/escaping, visible source, row ordering, hyperlinks, or comment anchors.
Emphasis spans clipped at the viewport edge remain correct on all visible cells.

Blank added/removed rows and right-side padding keep the existing full-width diff
background. Prefix clipping and panning entirely past source must remain safe.
No style may spill into an adjacent split cell, divider, gutter, or following row.

### Fallback, colorless output, and cached data

Absent, empty, unsupported, budget-exhausted, or rejected syntax metadata yields
ordinary source with the same foreground rule, and still carries row semantics.
Do not re-tokenize at repaint or change existing span-validation rules.

Ascii/NO_COLOR, NoTTY, and CLI `--plain` retain their existing logical bytes and
capability behavior. Do not emit new color escapes or characters to compensate
for missing colors. Preserve existing non-color focus cues where applicable.

Cached rows, raw source, patches, snapshots, and byte coordinates remain immutable
and free of ANSI/theme data. Keep URL hyperlink boundaries intact. Reviewed escape
sequences remain escaped text and never become trusted terminal instructions.

## Structure and implementation constraints

Existing Go code uses Bubble Tea, Lip Gloss, Chroma, and Ultraviolet. Relevant
presentation code is under `internal/tui`:

- `syntax.go`: source clipping and syntax foregrounds.
- `word_diff.go`: whole-word spans and emphasis fragments.
- `diff_search_view.go`: match fragments and search styling.
- `style.go`, `theme_canvas.go`: row styles and final trusted-cell composition.
- `render.go`, `commits.go`, `diff_wrap.go`: callers, prefixes and transformations.
- Existing syntax, word-diff, search, background, theme-canvas, and screen tests:
  regression coverage and final rendered-cell assertions.

Keep a single source-color contract shared by these paths. Prefer narrow typed
helpers and reuse existing trusted-cell tools where appropriate. Do not introduce
a generic styling framework or duplicate fixes in each view. The implementation
plan must choose the composition boundary and explain structural/source separation
before coding; this spec does not prescribe a particular helper signature.

Follow `gofmt`, existing `diffLine` types, and by-value presentation transforms.
For example, preserve this existing immutable-row pattern:

```go
piece := line
piece.searchID = searchSourceID{}
piece.Text = text[a:b]
piece.syntax = cropSpans(line.syntax, start+a, start+b, 0)
```

Any final cell repair is limited to visible output. Do not add lexing, patch
reparsing, or whole-inventory traversal to rendering. Theme changes reuse numeric
spans and immediately resolve colors from the new palette.

## Acceptance tests

Use Go's `testing` package and `canvasCells`/`sameCanvasColor` to assert actual
foreground, background and attribute channels after complete composition, not
just the existence of ANSI escapes. Synthetic fixtures only.

1. **Screenshot regressions:** A TypeScript added hunk containing `.map(toItem)`,
   closing delimiters, and lines with real syntax tokens renders ordinary source
   uniformly in the normal foreground. A `slug` to `id` replacement emphasizes
   all of `slug` and all of `id`; both letters of `id` have the same normal color.
   The removed identifier receives equivalent treatment. Colored patch markers
   and tinted row backgrounds remain present.
2. **Missing spans:** Exercise no spans, explicit empty capture, unsupported
   extension, rejected malformed span metadata, and token-free fragments inside
   otherwise highlighted lines. All use the same source foreground rule. Verify
   leading/trailing ordinary text and whitespace around colored tokens.
3. **Nested composition:** Cover a changed identifier with no syntax category,
   a changed colored token, multiple changed tokens, syntax boundaries inside an
   emphasized span, foreground-only resets, and complete style resets. Whole-word
   matching stays unchanged; emphasis and colors resume after each boundary.
4. **Search and selection:** A match inside a changed word, a match crossing syntax
   boundaries, active/inactive matches, unmatched fragments, and a row with no
   matches while search is open. Test focused/unfocused selection with search and
   word emphasis; closing search restores base colors. No background or attributes
   leak outside their applicable ranges.
5. **Geometry and views:** Unified and split, Files and Guide, raw inventory,
   numbered Commits, and existing source-only modes. Cover wrapped continuation
   rows, nonzero pan, clipped word/match edges, marker clipped away, number gutters,
   blank rows, padding, and split separators. Use representative end-to-end model
   fixtures plus focused transformation tests rather than every Cartesian product.
6. **Text and identities:** Tabs, combining marks, wide Unicode, escaped controls,
   and a linked URL. Assert exact stripped visible text and widths; unchanged
   cached rows, hyperlink boundaries, raw targets and suggestion source. Colorless
   profiles and `--plain` retain existing output behavior.
7. **Themes and capabilities:** Run the foreground/marker/reset assertions across
   all built-in themes, including inherited foreground/background themes, plus a
   validated custom foreground override. Exercise TrueColor, ANSI256 and ANSI
   profiles with expected quantization, and the existing colorless profiles. Theme
   switching must not leave stale colors in cached presentation.

The two screenshot regressions must fail on current production code and pass with
the fix. Add negative assertions that token-free source does not acquire an
added/removed color when normal foreground differs, and positive assertions that
legitimately green string tokens keep their syntax color. Golden changes must be
limited to intended styling differences; text/geometry golden churn is a failure.

## Commands and completion criteria

From the repository root:

```sh
# Focused rendering regression suite.
go test ./internal/tui ./internal/syntax -count=1

# Full required gate: formatting, vet, race tests and build.
./scripts/verify.sh

# Whitespace / patch integrity.
git diff --check

# Existing executable for terminal QA with synthetic offline captures.
go run ./cmd/prui --help
```

Use the existing offline fixture/resume workflow identified during planning for
terminal QA; do not invent a CLI flag or open a remote PR as a side effect. A human
checks the two screenshots' equivalents, dark/light theme legibility, search,
selection, and narrow/split views in a real terminal. Automated tests cannot
certify terminal accessibility; report any uncompleted human QA explicitly.

Done means the cell-level regression suite and full verification gate pass, real
terminal QA is recorded, the syntax spec reflects the approved fallback rule, and
no logical text, review provenance, capability handling, or source limits change.

## Boundaries and review decisions

- **Always:** Preserve repository constraints, immutable raw data and anchors,
  source escaping, existing geometry and color capability rules. Prove rendered
  cells, document the revised fallback, and report remaining validation limits.
- **Ask before expanding scope:** New palettes/settings, changed search/selection
  design, lexer/word-matching changes, dependencies, or persistence changes.
- **Never:** Silence checks, remove failing coverage, interpret reviewed source as
  terminal styling, mutate captured source to fix presentation, or write to GitHub
  as part of this fix.

No blocking technical questions remain. The product decision for approval is that
token-free and unsupported source becomes normal-colored consistently; additions
and deletions remain identified by marker colors and row backgrounds. This also
makes those markers consistently colored on lines that contain syntax tokens.

## Implementation and validation record

Implemented through the shared source presentation path. Patch markers are
separated before source styling; background-only presentation classes prevent
inherited source foregrounds from taking on diff accent colors. Existing semantic
status indicators retain their colors. Source foreground channels resolve in
trusted visible cells; word and active-search emphasis also uses trusted cells.
The theme picker previews this same contract.

The screenshot regressions and theme/profile/fragment, geometry/link, and
search/selection/reset tests pass. Focused tests and the full formatting/vet/race/
build gate (including PTY smoke tests) pass. Existing text/geometry goldens are
unchanged. See the [task verification record](../../tasks/diff-style-composition-todo.md).
Human live-terminal readability/accessibility QA remains pending.
