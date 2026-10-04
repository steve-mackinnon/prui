# Issue #29: multiline and file comment targets

Implements [#29](https://github.com/steve-mackinnon/prui/issues/29) under
[#39](https://github.com/steve-mackinnon/prui/issues/39).
Branch: `codex/issue-29-comment-targets`.
Parent: [#41](https://github.com/steve-mackinnon/prui/pull/41),
`codex/issue-26-review-drafts` at
`289da6b7327da325dc5ba6a08908a9b02e45c900`.
The stacked PR base is the parent branch. Merge #41 first, then retarget and
revalidate this PR against main. Do not merge or enable auto-merge here.
Issue #28 runs independently on the #27 conversation stack.

## Behavior and provenance

- Ctrl+v starts/cancels a range; j/k chooses its end and Enter opens the editor.
  Selection shows both raw anchors. Ctrl+o selects the old/new side of paired
  split rows and unchanged context rows. Reverse selection normalizes increasing raw coordinates.
- Ranges preserve start_line/start_side/line/side through immediate creation,
  local queue editing, persistence, refresh, and submission. Membership requires
  every coordinate within one captured raw hunk on one side and pinned path,
  including raw old-side context between/adjacent to deletions. Single-line
  context semantics remain RIGHT.
- Ctrl+f targets the selected changed file. File comments have subject_type=file
  and omit line/side/start fields. Captured text, binary, gitlink, metadata-only,
  and other non-text files are eligible; unavailable source and invalid paths
  are explicitly rejected. Deletions use the old path; other files use new.
- Ctrl+p queues line/range comments. It rejects file comments while retaining the
  editor. Immediate Enter is available for eligible files.
- Changed remote pins reject all writes. Extended targets additionally require
  captured inventory at the application write seam. Historical ranges/file
  comments remain explicitly unsupported; existing verified single-line
  historical behavior is retained. Unified/split projection never rewrites raw
  coordinates.
- New mutable payloads use version 2. Version 1 remains readable, while older
  binaries reject version 2 rather than silently ignoring extended fields.
  Immutable attempts and current/original REST reconciliation compare complete
  targets. REST and GraphQL refresh retain complete current/original range/file
  targets; GraphQL explicitly requests startDiffSide, never inferring it.
  Malformed range history returns an error and cannot unlock a retry. Offline recovery displays range/file anchors without a remote read.

## API evidence (verified 2026-10-04)

Official GitHub documentation:

- [Create a review comment](https://docs.github.com/en/rest/pulls/comments#create-a-review-comment-for-a-pull-request)
  lists start_line and start_side for ranges and subject_type=file, for which
  line is not required. File payloads omit all invented coordinates.
- [Create a review](https://docs.github.com/en/rest/pulls/reviews#create-a-review-for-a-pull-request)
  lists line, side, start_line, start_side within comments. It does not document
  subject_type for those comments. This implementation therefore explicitly
  rejects queued file comments instead of relying on undocumented behavior or
  silently dropping targets. No live remote review writes were used as tests.

- [GraphQL PullRequestReviewThread](https://docs.github.com/en/graphql/reference/pulls#pullrequestreviewthread)
  documents startDiffSide, startLine, originalStartLine and subjectType for
  reconstructing refreshed targets without guessing the range side.

## Verification evidence

Focused synthetic target, payload, raw-hunk, write-preflight, keyboard/split,
queue editing, and durable offline/immutable-attempt tests pass. Initial candidate
`f3fdb60bafbe140bc2e0a85acd284c5bf136acd8` passed `go vet ./...` and
`go test -race -count=1 ./...` across every package.

Fresh independent reviewer `/root/independent_review` requested two corrections:
malformed range histories must block reconciliation, and published refresh must
retain complete targets. Both have dedicated regressions and are corrected.
The reviewer inverted the range-side validation condition; the focused test
failed as expected, then the file was restored byte-for-byte. Coordinator feedback
on old-side context ranges also has raw-provenance and keyboard regressions.
Candidate `ad1b9146aae9ad236ba89d03abde1c215bd63b15` passed
`go vet ./...`, `go test -race -count=1 ./...`, and `go build ./...`.
A second independent review found contradictory file coordinates could also
prove absence; strict raw file/orphan range validation and dedicated regressions
now cover that case. Durable extended attempts also reject unsupported historical
commits. `96da95e6ca454ee60e757d2816ee5de441a80953` passed `./scripts/verify.sh`
(formatting, vet, full race suite with five-minute package timeout, build),
`golangci-lint run ./...` (0 issues), and `git diff --check`.
The next independent review caught an existing-reply regression in the newly
added historical creation guard. The guard now distinguishes creation from
reply/root provenance; `TestRangeReplyAttemptPreservesAssociatedRawSHA` verifies
full raw range/SHA persistence and reconciliation after restart. Focused TUI and
session reply/attempt regressions pass. A new full gate is required for this
correction; the shared large-gate slot is held by #30, so it has not been started.
Independent reviewer `/root/independent_review` approved code at
`7fa6eb69e2e78af3199dbb61ee5e0cabc77cc024`: all required findings resolved.
No implementation changes follow that approval; only help wording and an
additional file payload identity/subject assertion are added. Final full gates
and exact-SHA confirmation remain pending; keep this PR draft.

Human terminal usability/accessibility remains unverified by render tests alone.
No checks, thresholds, existing test assertions, or skipped-test policy weakened.
Only the host's existing Go cache is used; no duplicate temporary cache is created.
