# Diff syntax investigation: nova-rooms PR #28

Investigated 2026-10-11 from commit 74bafa8 in branch
`investigate/diff-syntax`, worktree `/private/tmp/review-syntax-investigation`.
No production code or saved review was changed.

## Finding

The pictured `apps/tides/src/graph/lane/derive.ts` receives no syntax spans.
Read-only inspection of the local saved PR #28 snapshot found an explicit empty
syntax map for each of its four text hunks. This is not a theme contrast issue:
there are no keyword, type, comment, or number colors to render.

`internal/tui/render.go:354` invokes the legacy hunk lexer only when metadata is
nil. An explicitly empty map intentionally suppresses fallback, so reopening
this saved review preserves the unhighlighted source. Red/green backgrounds and
patch markers still work because they are independent of syntax spans.

## Capture evidence

`internal/inventory/inventory.go:157` allocates 500 ms for syntax capture across
an entire inventory. Files consume that shared budget in capture order. At
lines 243–268, exhausted capture still stores explicit empty maps for hunks.
`internal/syntax/syntax.go:69` also enforces a cooperative 100 ms deadline per
side and discards all spans when it expires.

The saved snapshot contains 172 retained spans in 76 patch rows. Only README.md
and build.ts contain any syntax metadata. App.tsx is empty, and constants.ts,
derive.ts, samples.ts, options.ts, and all subsequent JSON files are empty.

The exact pinned old/new blobs were read from the existing local nova-rooms
Git object database and tokenized in temporary diagnostic Go tests:

| File | Old/new size | Measured behavior |
| --- | --- | --- |
| README.md | 162,875 / 164,763 bytes | Roughly 110–117 ms per side, including initialization |
| App.tsx | 228,904 / 228,879 bytes | Both sides time out at roughly 103–104 ms and return nil |
| build.ts | 6,194 / 6,942 bytes | Roughly 6–7 ms per side; valid spans |
| constants.ts | 3,024 / 3,071 bytes | Roughly 4 ms per side; valid spans |
| derive.ts | 6,220 / 7,855 bytes | Roughly 6 ms per side; valid spans |
| samples.ts | 4,893 / 4,916 bytes | Roughly 6–7 ms per side; valid spans |
| options.ts | 3,927 / 3,945 bytes | Roughly 5–6 ms per side; valid spans |

An uncapped diagnostic lexer iteration completed App.tsx in roughly 129 ms,
with 54,984 tokens and exactly matching source offsets. No text-normalization
failure was observed. The two README sides and two failed App sides together
consume approximately 430–440 ms. Both derive.ts sides are small, valid UTF-8,
supported TypeScript and well below the per-file byte/span bounds.

Sequential replay of the first seven files used approximately 484–494 ms on
this machine and successfully highlighted derive.ts. This places capture very
close to the shared deadline: slightly slower capture explains the saved cutoff
after build.ts. The persisted data does not record skip reasons or timings, so
historical exhaustion is inferred from this cutoff and the measured budget
pressure, rather than directly recorded. A fresh capture may succeed or fail
depending on timing; the saved result is stable.

## Suggested correction

Give small source files a fair opportunity before expensive files consume the
shared budget. Retain bounded work, and record capture outcomes so unsupported
files, timeouts and exhausted budgets can be distinguished. Simply increasing
the deadline leaves file-order and machine-speed sensitivity. Retrying empty
metadata during painting would defeat the capture/persistence contract.

Correcting future capture does not repair this immutable saved review; a new
capture or an explicit supported metadata refresh would also be needed.
These are follow-up design choices, not implemented in this investigation.

## Validation

- `go test ./internal/syntax -count=1`: passed.
- Temporary pinned-blob and shared-budget diagnostic tests: passed; removed after
  recording results because they depended on local source data outside the repo.
- `go test ./internal/tui -run 'Test(Syntax|SymbolColors|DiffStyle|SourceForeground)' -count=1`: passed.
- All commands used `GOCACHE=/private/tmp/review-go-cache` to keep writes in the
  permitted workspace.
- The broader TUI suite stopped in
  `TestDescriptionViewKeepsGlobalActionsAndDisablesProgressCommentActions` with
  an unrelated fixture panic, `no live network allowed`, during Git pinning.
- No full gate or live-terminal visual QA was completed. This investigation
  changes only this report.

## Follow-up implementation

The user authorized increasing the budget and surfacing omissions. The worktree
now raises the per-comparison syntax time budget from 500 ms to 5 seconds and the
per-side deadline from 100 ms to 1 second, including lexer initialization in the
per-side deadline. Byte and span limits remain bounded as before.

Optional per-file OLD/NEW statuses persist capture outcomes in main and commit
inventories. Capture failure banners appear beside affected hunks in Files and
Guide and below affected file headers in Commits. The footer reports unavailable
syntax, and Health & help lists paths, reasons and the new-comparison recovery
step. Unsupported file types receive an informational label. Older explicitly
empty captures are labeled with an unrecorded reason without inventing a timeout.
Ready captures with no tokens do not warn. Diagnostics are cached per immutable
review for repainting. Storage omission copies statuses and preserves earlier
failure reasons; existing records and raw comment anchors remain compatible.

Regression coverage includes deterministic budget exhaustion, invalid UTF-8,
unsupported languages, timeout/cancellation, token limits, offline persistence,
storage omission without source mutation, legacy captures, narrow/wide screens,
commit warnings, review switching, and unchanged source anchors. A deliberate
mutation of the unsupported-language warning condition was caught by the tests.
The token-limit test uses an independent test-only deadline so race instrumentation
does not conflate the token and time limits; production thresholds are unchanged.

The new screen baselines were inspected. Human live-terminal readability and
accessibility QA remains pending.

Final verification: `GOCACHE=/private/tmp/review-go-cache ./scripts/verify.sh`
passed (formatting, install checks, vet, all race tests including PTY smoke tests,
and build). `git diff --check` passed. Verification ran outside the filesystem
sandbox because macOS Git's sandbox-denied `confstr()` warnings contaminate
synthetic fixture SHA output; no live provider/GitHub services were used.
