# Feature removal audit

Date: October 20, 2026. Code baseline: `bd5a70d`.

prui should concentrate on opening a GitHub PR, understanding its changes,
discussing the code, and submitting a review. The strongest initial cuts are PR
lifecycle controls and applying suggestions as remote commits. Issue integrations,
cross-repository inbox triage, and arbitrary commit-subset diffs are the next
candidates. AI guides require a separate decision because they could be part of
the product's identity, but have a substantial implementation and dependency cost.

This is a proposed product boundary, based on the README's opening description.
It remains provisional until the maintainer chooses the intended focus. Rankings
reflect scope, overlap, and implementation burden; they do not establish that a
feature is unused. The README explicitly says there is no telemetry.

## Current size and sources of complexity

The current tree has 186 production Go files containing 38,072 physical lines
across `cmd` and `internal`, excluding files ending in `_test.go`. Counts include
comments and blank lines. `internal/testutil` is included in this mechanical
count even though it supports tests.

| Area | Production Go lines | Implication |
| --- | ---: | --- |
| Terminal UI | 19,097 | About 50% of the total; product breadth concentrates here. |
| Git and GitHub source layer | 6,119 | Acquires code and implements many unrelated remote workflows. |
| Session storage | 2,656 | Persists snapshots, progress, drafts, and feature-specific payloads. |
| Command layer | 2,200 | Wires the capabilities together and owns external effects. |
| Other packages | 8,000 | Includes commit composition, AI guides, themes, syntax, and tooling. |

`internal/tui/model.go` alone contains 2,728 lines. It defines 21 page states,
and its per-review state includes guides, issue context, source navigation,
search, readiness, lifecycle controls, discussions, commits, commit filtering,
drafts, and suggestion application. The Actions implementation contains 40
action-label occurrences across base and conditional entries; the help catalogue
contains 61 binding entries. These are source counts, not the number of commands
simultaneously shown to a user.

The [earlier approachability audit](AUDIT-tui-ux.md) deliberately retained all
capabilities and improved their discovery. That work addressed navigation and
presentation. Reducing the underlying feature set requires a different decision.

Evidence: [model](../internal/tui/model.go),
[Actions](../internal/tui/actions.go), [bindings](../internal/tui/bindings.go),
[architecture](ARCHITECTURE.md).

## Feature inventory and proposed disposition

Keep means part of the proposed core. Simplify means retain the useful outcome
with fewer modes or controls. Remove means a candidate for actual deletion.
Decide means its value depends strongly on the product's intended focus.

### Opening reviews and managing saved work

| Capability | Proposal | Reason and consequence |
| --- | --- | --- |
| Open PR by URL or number | Keep | Essential entry point. |
| Current-checkout PR browser and explicit repository PR list | Keep | Basic discovery without changing branches. |
| Remembered repositories and repository picker | Simplify | Keep a small recent list; avoid expanding repository management. |
| PR list preview and status metadata | Keep | Helps select the right PR before opening it. |
| Linear stack positions inferred from PR branches | Remove | Secondary discovery aid; branch graph inference adds another special case. This is separate from the Linear issue service. |
| Animated overflowing PR titles | Remove | Use truncation and a static selected-item detail; eliminates recurring timer state. |
| Multiple simultaneous review tabs, up to nine | Decide | Useful for parallel reviews; multiplies asynchronous ownership and draft state. Prefer saved-session switching if concurrent work is uncommon. |
| Frozen source comparisons and isolated Git objects | Keep | Central promise: inspect code without changing the checkout. |
| Save, list, resume, and delete sessions | Keep | Supports reviews across several sittings and gives users control of stored source. |
| Offline resume | Keep | A natural benefit of saved comparisons; preserve explicit unknown freshness. |
| Freshness checks and explicit new comparison | Keep | Prevents a saved review from silently becoming a different review. |
| Per-file reading progress | Keep | Directly supports the README's large-PR workflow. |
| Carry progress for provably unchanged files | Keep | Reduces repeated work after a push; retain conservative provenance checks. |
| Dedicated changes-since-review diff and draft-anchor outcome view | Simplify | Keep new-comparison and original-draft access; consider a changed-file summary instead of another full diff surface. |
| Cross-repository inbox, authored/participated/requested views, filters and read markers | Remove | Separate triage product with its own account cache and state. Users lose account-wide discovery; repository browsing remains. |
| Plain escaped CLI output | Keep | Useful for scripts and terminals where the interactive interface is unsuitable. |

### Reading and navigating code

| Capability | Proposal | Reason and consequence |
| --- | --- | --- |
| Overview with PR description | Keep | Establishes intent before reading code. |
| Files and canonical PR diff | Keep | Primary review surface. |
| Unified and side-by-side diffs | Keep | Both directly improve code reading. Reconsider only if one is rarely used by the maintainer. |
| Syntax highlighting and explicit capture diagnostics | Keep | Supports comprehension; unavailable highlighting must remain visible. |
| Word-level change emphasis | Keep | Small presentation feature with direct reading value; not an early removal priority. |
| Filename/path filter | Keep | Essential on large PRs. |
| Literal diff-text search | Keep | Directly helps find relevant changes. |
| On-demand expanded file context | Keep | Understanding nearby unchanged code is part of reviewing a change. |
| Separate full OLD/NEW file modes and whitespace presentation | Simplify | Prefer one context-expansion path before retaining multiple full-file modes. |
| Opt-in full-source cache for offline navigation | Decide | Valuable if offline full-file reading matters; adds stored source and coverage policy beyond saved patches. |
| File categories and generated-file collapse | Simplify | Keep useful defaults and the ability to reveal every file; reduce persistent toggles. |
| Full raw inventory and evidence-scope screens | Simplify | Keep incomplete-source diagnostics near affected files; retain a separate evidence screen only if guides stay. |
| Individual commit list and commit diffs | Decide | Valuable for history inspection, but the canonical PR diff already supports the core review. |
| Arbitrary selected-commit net diff in Files and Guide | Remove | Adds a second inventory and special read-only mode with different search, filter, progress, and comment behavior. |
| Historical comment placement on unchanged code | Decide | Useful for returning reviewers; requires original-source and ambiguity handling. Keep if multi-round reviews are central. |

### Discussion and submission

| Capability | Proposal | Reason and consequence |
| --- | --- | --- |
| Read inline threads, general comments, and review decisions | Keep | Necessary review context. |
| Readable Markdown and links | Keep | Descriptions and conversations need to be readable in the terminal. |
| Overview-to-Files navigation | Keep | Connects a discussion to the exact code under review. |
| Overview-to-original-commit navigation | Decide | Depends on retaining historical commit inspection. |
| Line and range comments | Keep | Core review output. |
| File-level comments | Simplify | Keep only if used; currently differs from inline comments because queuing is unsupported. Avoid extending a separate submission path. |
| Replies and general PR comments | Keep | Completes the discussion workflow. |
| Pending comments and review summary/decision submission | Keep | Core review output, including approval and changes requested. |
| Durable drafts and recovery | Keep | Protects substantial writing from interruption. |
| Submission-outcome checks after uncertain delivery | Keep | Avoids duplicate comments and reviews. This is required while remote writes remain. |
| Edit/delete own published comments | Keep | Lets reviewers correct their writing. |
| Resolve/reopen threads | Decide | Useful to some reviewers, but expands published-state reconciliation; can be handled on GitHub. |
| Comment reactions | Remove | Low contribution to reviewing code; use GitHub when needed. |
| Compose suggested replacement code | Keep | A review comment format with direct review value. |
| Apply a suggestion by creating a remote commit | Remove | Moves the product into changing the author's branch; keep composing and reading suggestions. |

### PR context and administration

| Capability | Proposal | Reason and consequence |
| --- | --- | --- |
| Basic checks/review status in the PR list | Keep | Helps select and orient a review. |
| Detailed live readiness, required-review and merge-policy evaluator | Simplify | Prefer a compact live status with links. Full merge-policy interpretation is harder to justify once lifecycle writes are removed. |
| Merge, auto-merge, queue entry/removal | Remove | Administration beyond the proposed review boundary. |
| Draft/ready and close/reopen controls | Remove | Same boundary; GitHub already provides these actions. |
| Requested reviewers, teams, labels, review decisions | Simplify | Put a short summary in Overview if retained; avoid a dedicated context workspace. |
| Fetch and cache linked GitHub closing issues | Remove | Keep issue links in the description; opening the issue elsewhere is a reasonable handoff. |
| Linear issue fetching, authorization configuration and cached issue bodies | Remove | Separate service integration with its own credentials, repository allowlist, budgets, and historical-access semantics. |

### AI guides, appearance, and development tools

| Capability | Proposal | Reason and consequence |
| --- | --- | --- |
| AI grouping, summaries, and reading order | Decide | Could differentiate the product, but overlaps Files with another navigation/progress/rendering surface. |
| OpenAI, Anthropic, Google and compatible endpoints; model selection | Simplify if guides stay | Keep the provider set actually needed. Provider count alone understates shared SDK cost. |
| Supporting evidence retrieval, upload filtering and consent | Keep if guides stay | These are prerequisites for the guide feature; deleting safeguards is not a valid simplification. |
| Derived guide sessions, cache fingerprints and remembered model | Simplify if guides stay | Prefer one explicit generation path; preserve source identity and existing saved work. |
| Guide evaluation command | Move to development tooling | Maintainer evaluation need not be a user-facing command. Retain useful checks. |
| Theme presets and arbitrary token-color overrides | Simplify | Keep accessible light/dark defaults; remove customization breadth before removing essential contrast. |
| Mouse navigation, clipboard, resize and text selection | Keep | Ordinary usability, especially for users who do not memorize shortcuts. |
| Persisted pane widths/layout preferences | Keep | Small convenience tied directly to reading. |
| Actions menu, familiar navigation, contextual help | Keep | Makes the retained features discoverable. Trim the menu as features go. |
| Terminal acceptance artifacts and performance reporting | Move to development tooling | These verify product quality; separate public CLI breadth from necessary test capability. |
| Install/update instructions and release automation | Keep | Delivery infrastructure supports every retained capability. |

## Ranked removal candidates

The line counts below cover explicitly listed implementation files. They are
inspection footprints, not estimates of deletable lines or engineering effort.
Shared wiring, model fields, stored payload validation, tests and documentation
can add work; retained features can require parts of the same code.

| Rank | Candidate | Evidence and footprint | Tradeoff |
| --- | --- | --- | --- |
| 1 | PR lifecycle controls | 750 lines across `source/pr_lifecycle.go`, `source/pr_lifecycle_policy.go`, `tui/pr_lifecycle.go`, and `cmd/prui/pr_lifecycle.go` | Give up in-terminal merge/queue/state changes; remove a complete remote-write workflow. |
| 2 | Remote suggestion application | `source/suggestion_apply.go` alone is 271 lines; `tui/suggestions.go` mixes composing and applying | Keep suggesting edits; require GitHub to apply them. Retain any outcome handling needed by previously attempted applications. |
| 3 | Issue context, beginning with Linear | 770 lines across `internal/issuecontext`, source/TUI issue-context files and command wiring | Lose embedded issue bodies; keep description links and optionally a small reviewer summary. The full count includes retained GitHub metadata if that is kept. |
| 4 | Cross-repository inbox | 943 lines across `internal/inbox/cache.go`, source/TUI inbox files and command wiring | Lose account-wide triage and local activity markers; keep per-repository PR selection. |
| 5 | Selected-commit net diffs | 970 lines across `tui/commit_filter.go` and `commits/composition_{paths,source}.go` | Review the PR comparison or an individual commit instead of arbitrary subsets. Capture and validation integrations also need removal. |
| 6 | Reactions, stack indicators, title animation, theme customization | Title animation alone is 113 lines; other pieces share review/picker/theme code | Smaller wins with limited impact on the core workflow. Do not treat cosmetic cuts as the main bloat reduction. |
| 7 | Detailed readiness policy interpretation | 888 lines across source/TUI readiness and command wiring | Keep basic status, hand off detailed policy diagnosis to GitHub. Remove lifecycle consumers first. |

The first, third, fourth, and fifth groups cover 3,433 lines in distinct source
files. That establishes a substantial review surface; it does not imply 3,433
lines can be deleted unchanged. No runtime or binary-size savings are claimed.

## The larger product decision

AI guides and commit-centric review are the two largest scope choices after the
initial cuts.

The guide/evidence/provider/evaluation packages and dedicated guide UI files
cover 3,092 production lines before shared rendering, storage, and command wiring.
The code directly imports Fantasy and provider adapters, and `go.mod` includes
provider and cloud SDK dependencies. Retaining only one provider might leave much
of that transitive graph in place; measure the actual module graph and binary
after a change. Removing the complete guide feature is a different, larger cut
than trimming provider choices.

Commit review is also broader than a tab: individual commit capture, composition,
historical comment targeting, persisted bundles, and Overview navigation all
participate. `internal/commits` has 1,141 production lines, in addition to the
575-line commit UI and the 694-line commit-filter UI. These overlap the footprints
above and must not be added to them as independent savings.

Recommended default: retain the canonical PR diff and changes-since-review support;
remove arbitrary commit composition. Keep individual commit inspection only if it
is a recurring need. Retain guides only if AI-assisted reading is a reason people
choose prui, rather than an occasional extra.

## Optional capabilities currently affect opening a review

`internal/review/review.go` calls `captureDetails` for commit capture and repository
evidence retrieval during a normal open. Evidence retrieval happens even when
`Config.Analyzer` is nil; no provider upload is implied. Commit capture also
prepares composition source in `internal/commits/capture.go`.

Consequently, hiding a tab or action will not remove all of its acquisition work.
A complete cut must follow the feature through opening, snapshot payloads,
validation, and resume. If guides or commit inspection stay optional, consider
whether their capture work should become explicit too. Any delayed capture must
still use the original pinned objects and preserve offline expectations; it
cannot silently read a newer checkout or PR revision. Benchmark before claiming
an improvement.

## Removal order and completion conditions

1. **Remove lifecycle actions.** Delete their menu entries, callbacks, page state,
   remote mutations, and dedicated tests/spec obligations. Preserve review
   submission and freshness checks. Verify that reading and submitting a PR
   review still work online and that offline mode performs no writes.
2. **Remove remote suggestion application.** Preserve suggestion composition,
   rendering, pending review submission, and access to existing private drafts.
   Account for stored uncertain application attempts before dropping recovery
   code. Verify the retained suggestion path end to end.
3. **Remove Linear and fetched issue bodies.** Choose whether to retain a small
   reviewer/label summary. Stop remote reads and new cache writes; define how
   existing private issue data remains accessible or is explicitly deleted.
4. **Remove inbox triage.** Keep repository browsing and existing review sessions.
   Decide explicitly how its separate cache is handled. Verify current/open/prs,
   session resume, and normal PR switching.
5. **Remove commit-subset composition.** Keep the canonical inventory and any
   approved individual-commit view. Remove composition capture, UI modes, and
   special-case restrictions. Verify old sessions remain readable and progress
   and comment coordinates retain their original meaning.
6. **Choose the guide and commit boundary.** Make these product decisions before
   reorganizing the remaining TUI. Then trim reactions, animation, stack metadata,
   theme customization and redundant screens.

Each cut should be reviewable independently, though storage compatibility can
require temporary readers. Stop producing removed payloads first and retain only
the compatibility required for existing snapshots or unsent work. Do not silently
delete old source, sessions or draft text to make the implementation smaller.

For implementation, run the project's verification gates from README/TESTING,
including focused regressions for the surviving behavior, PTY journeys, and build.
Update README, reference controls, specs and CONSTRAINTS deliberately when the
product contract changes. Remove tests of intentionally deleted capabilities
while preserving safety and regression coverage for retained ones.

## Decisions that change the ranking

The maintainer's own usage is the next evidence to add: which capabilities were
used during the last several real reviews, which saved time, and which caused
confusion. This can be a short review diary; telemetry is unnecessary.

If prui is intended as an end-to-end GitHub administration client, lifecycle and
inbox cuts have a greater product cost. If AI-guided reading is its defining
purpose, guides belong in the core and historical commit administration becomes
an easier cut. If the purpose is reviewing large PRs over multiple sittings,
frozen snapshots, progress, draft recovery and safe handling of new pushes should
remain central.

This audit examined local documentation and implementation paths. It does not
rank features by observed adoption, measure runtime performance, or certify
runtime correctness. No application behavior was changed and no test run was
needed for this documentation-only audit.
