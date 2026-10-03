# Historical pre-release code audit

Reviewed commit: `42468f813eb13141f179761ec299fd4706f56aca`.
Audit date: 2026-09-25 (America/New_York).

The package architecture is a reasonable foundation for this application. The
findings below describe the reviewed commit; the remediation status records the
subsequent fixes. Original reproduction probes are retained for that historical
commit only. Permanent regression tests now live beside the fixed code.

## Remediation verification at the time of the fixes

The four concrete bugs have been fixed:

| Finding | Resolution |
| --- | --- |
| Background refresh race | Workers return immutable requests/results; the event loop merges freshness into the latest progress. Generation checks protect foreground actions, canceled/replaced sessions, and drafts. |
| Credential upload filtering | Quoted keys, assignment forms, and recognizable key material exclude the affected file, including metadata, sibling hunks, and evidence across rename aliases; full-blob retrieval exclusions survive assembly and user exclusion patterns stay local; an HTTP-level regression checks the actual request body. Filtering remains heuristic. |
| Mixed GitHub comments | Nullable remote anchors are parsed separately from strict write targets; file and outdated comments no longer discard current inline threads. |
| Description-only write refusals | All write preflights use the existing pinned-revision comparison helper. |

The consent screen now identifies the configured provider origin, explains that
both source and the API credential go there, and warns that filtering can miss
secrets. The displayed destination is captured for the model's lifetime.

At remediation verification, Goldmark was pinned to v1.7.17, `x/text` to v0.39.0, and `x/net` to v0.56.0.
`govulncheck` v1.8.0 reported no vulnerabilities in that dependency graph. CI now
runs that pinned checker. The configured GolangCI-Lint v2.13.2 gate is clean;
its existing rules and exclusions were preserved. Theme file paths are normalized
caller-selected global paths; this is not a claim of untrusted-path confinement.

Remediation verification passed on macOS/arm64: `./scripts/verify.sh` (formatting,
vet, full race suite, compiled PTY tests, and build), GolangCI-Lint v2.13.2 (zero
issues), and govulncheck v1.8.0 (no vulnerabilities found). Synthetic regressions
cover progress persistence, refresh ordering and draft retention, mixed comment
pages, description-only write preflights, upload bodies, and provider consent.
No real provider upload or GitHub write was performed.

As of 2026-10-02, the project includes an [MIT license](../LICENSE). A private
security-reporting channel still requires a maintainer choice. Dependency versions
and architecture have evolved since this audit; the verification results below
are historical, not a current release certification.
The two scaling suggestions under Architecture are longer-term improvements,
not demonstrated release-blocking defects; no storage-format migration or broad
TUI rewrite was introduced as part of those remediation fixes.

## Original findings

### P1: Background freshness refresh mutates UI-owned session state

Locations: [TUI worker](../internal/tui/lifecycle.go), lines 150–163;
[application refresh](../cmd/prui/lifecycle.go), lines 425–450;
[store save](../internal/session/store.go), line 870.

The TUI passes its live `*review.Session` to a background command. The application
writes `RevisionStatus` and `Store.Save` replaces `State` through that pointer
while the event loop can render or mark the same session. The store mutex protects
store methods, not the model's reads or `review.Mark`'s copies of session state.
This violates Bubble Tea's event-loop ownership and can race with progress
updates. A focused race-detector probe reproduced writes at `lifecycle.go:444`
and `store.go:870` racing with reads of the displayed session.

Remedy: make the background operation return metadata/freshness as an immutable
result, and reconcile and persist it on the event-loop owner against the current
session/generation. Simply copying the struct before launching work avoids one
race but still needs reconciliation so a stale result cannot replace newer
reading progress. Add a real-program scenario that marks a file while metadata
is pending, releases the result, and verifies both in-memory and reopened progress.

### P1: Upload filtering misses ordinary JSON credentials

Locations: [privacy policy](../internal/privacy/policy.go), line 16;
[upload assembly](../internal/guide/input.go), lines 61–66.

The pattern expects the sensitive key to be followed immediately by whitespace
and `:` or `=`. A quoted key such as `{"api_key": "synthetic-value"}` in
`config.json` does not match because of the closing quote. The probe confirmed
that its patch survives `InputFrom` into the provider request material. The
second filter pass uses the same pattern, so it does not catch the omission.
Both patches and contextual evidence can be affected after a user confirms guide
generation. This does not imply an automatic upload or an observed credential leak.

Remedy: cover quoted JSON/YAML keys, common assignment syntax, and recognizable
key material in both evidence and patch tests. Treat filtering as defense in
depth, not a guarantee; retain explicit consent and document the actual recipient
when a custom endpoint is configured. The README now states this limitation.

### P2: One unsupported GitHub comment discards all usable comments

Location: [GitHub comment reader](../internal/source/github.go), lines 373–376
and 384–410.

List parsing reuses the stricter create-comment validation, which requires a
positive line. File-level comments and outdated comments with no current line
cannot satisfy that requirement. A single such entry makes the entire page fail,
including otherwise valid comments on the current diff. A mixed file-comment /
line-comment fixture reproduced `invalid review comment list` with zero results.
GitHub explicitly supports `subject_type: file` in its
[review-comment API](https://docs.github.com/en/rest/pulls/comments).

Remedy: distinguish remote comment records from locally writable line targets.
Parse nullable anchors and subject type, then omit or separately present valid
but unanchorable records before building the line overlay. Keep malformed-record
checks and the outbound target validator strict. Add mixed-page fixtures for
file comments, outdated comments, and current inline threads. Also communicate
the existing first-page, 100-comment limit to users.

### P2: Description edits incorrectly block inline writes

Location: [comment preflights](../cmd/prui/lifecycle.go), lines 83 and 162.

Inline posting, replies, reactions, and deletion compare the entire `Metadata`
struct, which includes `Description`. A description-only edit causes a “pull
request changed” refusal even when repository identities and both SHAs match.
Meanwhile, freshness uses `SamePinnedRevision` and reports the comparison current;
whole-review submission also accepts those same revisions. A probe changing only
`Description` reproduced the refusal before any write.

Remedy: use the existing `source.SamePinnedRevision` in all write preflights.
Keep description capture immutable as presentation data. Test description-only
changes separately from repository/base/head changes for each write action.

## Original dependency and release checks

`govulncheck` v1.8.0 examined 29 modules and Go 1.26.8. It exited with findings:

| Advisory | Pinned version | Fixed version | Assessment |
| --- | --- | --- | --- |
| [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970) | `golang.org/x/text` v0.24.0 | v0.39.0 | Scanner reports symbol reachability. Advisory concerns invalid-UTF-8 normalization loops; exploitability through this application's validated/escaped input was not demonstrated. |
| [GO-2026-5320](https://pkg.go.dev/vuln/GO-2026-5320) | `github.com/yuin/goldmark` v1.7.8 | v1.7.17 | Scanner reports a path through Glamour. Its terminal renderer replaces Goldmark's HTML renderer, and the app does not serve HTML; a browser XSS exploit was not demonstrated. |

These are dependency remediation items, not two proven exploitable application
vulnerabilities. Update each dependency with compatibility checks and rerun
Markdown/terminal tests and the scanner. Do this before release; no indefinite
exception is proposed. The scan also found seven package-level and three
module-level advisories without identified calls, primarily in `x/net`. Include
those in dependency maintenance, without presenting them as reachable exploits.
Add a pinned vulnerability check to CI; the current workflow does not run one.

The exact CI linter, GolangCI-Lint **v2.13.2**, reports **11 issues**:

- `errcheck`: unchecked directory close in `internal/theme/config.go:246`.
- `gocritic`: conditional chains in `internal/tui/chrome.go:49` and
  `internal/tui/model.go:701,710`.
- `gosec`: configurable file access in `internal/theme/config.go:71,242` and an
  integer conversion in `internal/tui/model.go:852`.
- `staticcheck`: switch suggestions in `internal/tui/model.go:900` and
  `internal/tui/review_submit.go:181`; capitalized error in
  `internal/tui/theme_picker.go:11`.
- `wastedassign`: `internal/tui/model.go:1982`.

These fail the configured lint gate. The file-access warnings refer to intended
local theme configuration, and the conversion is a modulo-three view selector;
neither is established as a vulnerability. Resolve findings with focused code
changes or justified, narrowly scoped treatment, preserving the existing gate.

A license and private security-reporting instructions are absent. Select a
license and add a reporting channel before public release. There is no need to
invent a license choice or contact address in this audit. The README now exposes
the pending license decision and provides basic contribution/test guidance.

## Architecture assessment

The strong boundaries are worth preserving:

- Source acquisition isolates Git from repository-selected configuration and
  helpers; comparison identity is pinned and ancestry-checked.
- Inventory is deterministic source truth; guides interpret it without taking
  ownership of file completion.
- Persistence separates immutable snapshots from mutable progress, with private
  files, atomic replacement, integrity checks, and stale-writer rejection.
- Online/write guards live in the application layer. Comments use JSON stdin,
  explicit actions, frozen targets, and freshness preflight.
- Tests span package behavior, event-loop journeys, rendering, and compiled PTYs.

The main structural improvement is ownership, rather than additional layers.
Make the event loop the sole owner of mutable tab/session state; return data from
workers. Reuse one comparison-identity rule across refresh and all write actions.
Separate API response shapes from outbound command validation. These moves
address the observed defects directly.

Two scaling concerns merit follow-up:

1. `internal/tui/model.go` is 2,015 lines and its lifecycle file adds 988. Split
   event handling by feature/state owner (navigation, comment composition,
   background results), retaining one explicit update dispatcher. Moving branches
   into arbitrary helpers alone will not reduce state coupling.
2. Cached PR opening creates another full snapshot; cached-guide reuse can create
   another derived copy. Session lookup/listing still loads full snapshots and
   storage has no automatic retention. At scale, add a small validated identity
   index and shared immutable snapshot storage, retaining per-session progress.
   Explicitly handle guide-cache deletion separately from session deletion.

`review.Session` currently aliases `session.Record`, making persistence types the
shared domain model. This is manageable for the present single-binary scope;
avoid a large domain-layer rewrite unless storage formats begin dictating UI or
review behavior. GitHub transport being in `source` is also acceptable provided
write interfaces stay narrow and the application remains the policy boundary.

## Original verification and limits

- `./scripts/verify.sh`: passed on macOS/arm64 with Go 1.26.8, including formatting,
  vet, race tests, compiled PTY tests, and build. Initial sandbox attempts failed
  on cache access, loopback listeners, and Git fixture diagnostics; the permitted
  run outside the sandbox passed. This does not establish a Linux CI result.
- Exact CI linter v2.13.2: failed with the 11 existing issues above.
- Focused synthetic probes: reproduced all four findings, including the race.
  The existing passing suite did not cover these cases.
- Encoded terminal-control probe: the Markdown renderer did not preserve the
  tested OSC-52 or clear-screen sequences; no finding from that probe.
- Searched 1,003 reachable historical blobs across all local refs (128 commits)
  for common private-key headers and GitHub/OpenAI/AWS/Slack credential formats;
  no candidates matched. This was a limited pattern scan, not exhaustive secret
  detection. Remote-only refs, deleted/unreachable objects, and service settings
  were not audited. Review repository secret-scanning results before publication.
- No real provider uploads or GitHub mutations were performed. No live review,
  deployment, branch-protection, license-compatibility, or human accessibility
  sign-off is implied.

### Reproduce the original findings

The [probe patch](audit-reproductions.patch) adds only synthetic tests using
existing fixtures at the original reviewed commit. It is not intended to apply
to the remediated implementation, whose refresh interface has changed. From a
clean checkout at the reviewed commit (with this patch available), run:

```sh
git apply docs/audit-reproductions.patch
go test -race ./internal/guide ./internal/source ./cmd/prui -run '^TestAudit' -count=1 -timeout=1m
# The four tests are expected to fail before fixes.
git apply -R docs/audit-reproductions.patch
```

The tests need the same Git/process permissions as the normal suite. The race
probe schedules concurrent reads during the actual application refresh; it
reported two races in this audit. For a permanent regression test, also exercise
progress reconciliation through the real TUI program driver.

To repeat the dependency and lint checks without changing `go.mod`:

```sh
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -show verbose ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./...
```
