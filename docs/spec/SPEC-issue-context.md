# Issue #36: Reviewer and issue context

Issue context is mutable PR metadata, never pinned code or AI input. `I` opens
saved context without a remote read. `r` on that screen explicitly refreshes
GitHub (requested users/teams, aggregate review decision, latest opinionated
reviews, labels, authoritative closing issue references) and optional Linear.
The screen labels repository/PR, captured head and timestamp, partial results,
and historical saved data. Refresh never changes frozen inventory or progress.

GitHub reads use one bounded GraphQL page of 100 per connection. More pages,
missing fields, parse failures and GraphQL errors are visible partial states.
GitHub issue detection is only `closingIssuesReferences`; arbitrary description
URLs and #numbers are not trusted issue metadata.

Optional Linear requires a global secret-free `issue-context.json` beside guide
configuration with explicit Linear enablement, allowed GitHub repositories, Linear
workspace and credential environment variable name. No credentials are requested
for implementation or tests. Canonical HTTPS `linear.app/<workspace>/issue/KEY-N`
links in the live PR body identify candidates only, capped at 10. The configured
workspace must match; titles/status/descriptions/URLs come from authenticated
Linear GraphQL `issue(id:)`. No arbitrary URL is fetched, no image is fetched,
no redirects, retries, mutations, or AI uploads occur. API-key and existing OAuth
token authentication are supported. Missing configuration/access preserves GitHub.

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
- Linear [GraphQL getting started](https://linear.app/developers/graphql): official
  endpoint, API-key vs OAuth authorization headers, issue(id:) identifiers,
  description/state and GraphQL errors. We use reads only and inspect errors even
  on HTTP 200. No live account access is needed for synthetic implementation tests.

The final UI uses a separate `I` screen so existing Description/Files/Guide/Commits
geometry is unchanged. Keyboard paging and mouse wheel scroll bounded escaped
text. Links are shown as usable HTTPS text for the reviewer to open; no automatic
browser launch. A failed cache write still displays refreshed data with an explicit
unsaved notice. Optional Linear has a shorter child deadline to preserve GitHub.
