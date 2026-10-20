# Issue #36: Reviewer and issue context

Issue context is mutable PR metadata, never pinned code or AI input. `I` opens
saved context without a remote read. `r` on that screen explicitly refreshes
GitHub (requested users/teams, aggregate review decision, latest opinionated
reviews, labels, authoritative closing issue references) only.
The screen labels repository/PR, captured head and timestamp, partial results,
and historical saved data. Refresh never changes frozen inventory or progress.

GitHub reads use one bounded GraphQL page of 100 per connection. More pages,
missing fields, parse failures and GraphQL errors are visible partial states.
GitHub issue detection is only `closingIssuesReferences`; arbitrary description
URLs and #numbers are not trusted issue metadata.

Context is saved separately in an optional private SQLite extension, keyed by
normalized repository and PR, with payload checksum and latest capture timestamp.
Old version-1 stores remain readable; the extension is created on explicit save.
Offline access never reads integration configuration or credentials. Failed
refresh retains the previous context labelled stale. Human terminal QA is required
for usability claims; automated tests establish mechanics only.

## Primary API evidence

- GitHub [PullRequest fields](https://docs.github.com/en/graphql/reference/pulls):
  reviewDecision, latestOpinionatedReviews, reviewRequests, labels,
  closingIssuesReferences and headRefOid. Nullable or omitted fields stay unknown.

The final UI uses a separate `I` screen so existing Description/Files/Guide/Commits
geometry is unchanged. Keyboard paging and mouse wheel scroll bounded escaped
text. Links are shown as usable HTTPS text for the reviewer to open; no automatic
browser launch. A failed cache write still displays refreshed data with an explicit
unsaved notice.

## Integration on merged main

The context screen coexists with `Alt+R` readiness, `C` commit filtering, `F`
path filtering, `/` source search, and `Alt+C` generated collapse. Its open,
refresh, cancellation and late results never alter reading layout, source cursor,
frozen inventory, canonical targets, or private v3/v4 attempts. Foreign PR and
frozen-session results are rejected. Tests use real pinned Git fixtures and the
actual `Model.Update` dispatch with private SQLite drafts and captured full source.

The optional Linear integration has been removed. Leftover `issue-context.json`
files and credential environment variables are unused. Legacy cached payloads
ignore Linear fields while preserving GitHub context; opening them remains local.
