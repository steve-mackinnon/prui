# Mouse support audit — 2026-10-20

This is the baseline audit requested before delegated implementation. Findings
describe the starting code, not the final implementation status.

## Existing support

`internal/tui/mouse.go` routes review tabs, header actions, list/diff selection,
PR-list preview scrolling, discussion selection, and some vertical scrolling.
Separate handlers cover divider dragging, text selection, review links, commit
filters, search results, and Actions item activation.

## Missing cases

| Surface | Missing mouse interactions at audit time |
| --- | --- |
| PR list and review switcher | Double-click selected semantic PR/open-tab row to open it |
| Saved sessions and repositories | Double-click to open; wheel navigation |
| Theme picker | Double-click to apply; wheel navigation |
| Picker chrome | Visible open/apply, reload, inbox, preview focus/scroll, help, and back controls |
| Review inbox | Select rows, double-click to open, wheel navigation, mark read, filters, refresh, back |
| Inbox filters | Focus fields, cycle choices, apply, cancel |
| Files and Guide rails | Double-click to focus detail |
| Files/Guide/Commits detail | Double-click a commentable line to compose or an existing comment to open its actions |
| Active comment range | Compose/cancel controls |
| Overview activity | Visible reply, edit, delete, refresh controls; expansion already works on click, as do Files and Original links |
| Discussions | Double-click to open detail; visible reply/edit/delete/original-source/refresh/back controls |
| Actions | Wheel navigation; close |
| Search and commit-filter popovers | Close/back controls; existing result/choice clicking remains supported |
| Readiness and linked issues | Refresh/back controls; readiness lifecycle shortcut; scrolling already works |
| Help | Wheel scrolling; shortcuts toggle; back |
| Evidence and URL | Back; URL activation on the URL page |
| Secondary readable pages and PR preview | Activate displayed HTTP links; retain drag-to-copy behavior |
| Guide consent | Provider cycling, model focus, confirm upload, cancel |
| Inline/reply/general/published editors | Mouse focus, scrolling, and visible submit/queue/cancel/refresh controls |
| Comment actions | Select actions and reactions; delete confirmation/cancel |
| Review submission | Event selection, body focus, pending-comment edit/remove, confirmation, cancel |
| Draft recovery | Scroll, continue, discard, delivery check |
| Quit with drafts | Save, discard, cancel |
| Incremental comparison | Scroll/pan, previous/next file, drafts/diff toggle, old comparison, back |
| PR lifecycle | Select actions, scroll, refresh, review action, final confirmation, back |
| Suggestion application | Scroll, review confirmation, final confirmation, cancel |
| Loading/error states | Existing keyboard cancel/dismiss/retry actions exposed to mouse |
| Horizontally pannable panes | Horizontal wheel events |

Evidence: `mouseAvailable` excludes most modal/page states and every composer,
comment menu, general editor, and published editor. `mousePickerClick` explicitly
selects only. `mouseWheel` supports PRs, readiness, issues, discussions, and review
panes, and ignores horizontal wheel buttons. `linksAvailable` only permits the
main review page. `actionsMouseClick` activates rows but its route ignores wheels.
The keyboard equivalents live in `Model.Update`, `pageKey`, and each surface's
existing key handler.

## Acceptance criteria

- Single click selects; double-click activates the same semantic target within
  a bounded interval. A different item, stale layout, wheel, key, modal change,
  or drag must not accidentally activate a previous selection.
- Hit testing follows the rendered viewport, scroll offset, clipping, and
  responsive geometry. Blank space and clipped controls do nothing.
- The foremost modal/editor owns mouse input, including when loading. No click
  reaches the obscured review or another review tab.
- Mouse actions call the existing guarded behavior. Guide upload, immediate
  comments, pending review submission, deletion, lifecycle changes, suggestion
  commits, draft discard, and local theme persistence retain their existing
  explicit action and confirmation semantics.
- Add synthetic regressions for activation, wheel routing, bounds, modal
  ownership, and confirmation boundaries. No test contacts GitHub or a provider.
- Run the repository's vet, race tests, and build. Automated tests establish
  routing and geometry; terminal usability still requires human assessment.

## Implementation and verification

The implementation subagent added support for the inventory above, including
picker/footer activation, modal ownership, editor caret placement, range controls,
secondary-page links, and preserved drag-to-copy. Single-click selection and
existing confirmation/write handlers remain authoritative. Controls that do not
fit the viewport cannot be activated; resize the terminal to expose them.

Verification completed:

- `./scripts/verify.sh`: installer checks, formatting, vet, full race suite,
  terminal smoke tests, and build passed.
- A subsequent full TUI race run passed after integration; targeted race tests
  covered the final range controls and adjacent mouse interactions.
- Final `go vet ./...` and `go build ./...` passed.
- `golangci-lint run ./...` passed with the locally installed v2.12.2. CI pins
  v2.13.2, which was not installed or run locally.
- Real SGR terminal input opened an existing PR by double-click and exercised
  Help's toggle, wheel, and Back controls without GitHub access.
- Inverting the double-click semantic identity condition caused its regression
  test to fail; the original condition was restored.
- `git diff --check` passed. Human terminal/accessibility assessment remains.
