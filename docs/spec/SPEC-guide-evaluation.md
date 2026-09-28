# Spec: Guide Evaluation

## Objective

Evaluate guides already stored in a verifier session without triggering a
provider call or source upload. The evaluator distinguishes structural
correctness from semantic expectations in a curated, synthetic corpus.

## Tech Stack

Go, `internal/guide.Validate`, and JSON test fixtures. It extends the existing
evaluation approach without reusing its saved-plan schema for guide bundles.

## Command Contract

`prui eval-guides SESSION_ID` evaluates the guides already stored in a
session and emits JSON. Its report has one of `not_available`, `passed`, or
`failed` and contains structural checks plus named semantic fixture results when
a selected corpus applies. A live verifier report may link this follow-up
command when its resulting session already has guides; a fresh verifier session
normally has none. The evaluator never presses `g`, reads `OPENAI_API_KEY`, or
contacts an analysis provider.

Semantic fixtures specify expected guide-title keywords, section-title keywords,
and expected unit-id groups. A fixture can make only explicit claims; it cannot
declare the evaluator's model interpretation to be source truth.

## Project Structure

```text
internal/guideeval/evaluate.go             structural and fixture evaluation
internal/guideeval/testdata/corpus.json     curated synthetic expectations
internal/guideeval/evaluate_test.go         fixture validation and scoring
```

## Code Style

The evaluator returns named results rather than a single opaque score:

```go
Result{Name: "authentication-flow", Status: "passed", Checks: checks}
```

## Testing Strategy

Test valid guide coverage, unavailable guides, unknown units, title/section
expectations, and malformed corpora. Run `go test ./internal/guideeval`.
Provider tests remain local-HTTP-only and are not changed.

## Boundaries

- Always: call structural validation before semantic checks; retain the raw
  inventory as source truth.
- Ask first: use a model judge, send a corpus to an external service, or claim
  semantic success is code-review approval.
- Never: generate a guide, bypass upload consent, or describe a guide score as
  a security finding or complete review.

## Success Criteria

- Stored generated guides receive deterministic structural validation.
- The corpus detects a missing expected grouping or misleading title/section
  claim with a named failed check.
- A verifier run with no stored guides reports `not_available`, not failure.

## Open Questions

- The first semantic corpus needs candidate synthetic PR shapes; it will be
  chosen during planning rather than copied from live repositories.
