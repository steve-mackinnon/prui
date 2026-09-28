# Spec: Performance Reporting

## Objective

Report repeatable timing observations from a verifier run without treating
network latency as application startup performance.

## Tech Stack

Go's monotonic `time` package and the live verifier's existing lifecycle
events. No benchmark framework or dependency is required.

## Command Contract

`pr-review verify` gains `--measure-runs N` (default 1, allowed 1–10). A report
contains per-run and aggregate milliseconds for `process_start`,
`first_review_frame`, `github_metadata`, and `pin_and_inventory`. The report
labels every run `cold` or `warm` based on whether the verifier's private store
already has the required session; normal verification uses fresh stores and is
therefore cold.

No command fails because a timing target is exceeded in the first version.
Performance regressions are assessed by comparing the median and maximum to a
recorded prior report from the same host and checkout state.

## Project Structure

```text
internal/verify/timing.go          event timestamps and aggregation
internal/verify/timing_test.go     monotonic aggregation and serialization
```

## Code Style

Durations remain typed until JSON serialization and report zero only for a
step that did not run.

## Testing Strategy

Use synthetic clock/event fixtures to test aggregation and omitted events.
Run `go test ./internal/verify -run Timing`. Live measurements are manual
evidence, not CI assertions.

## Boundaries

- Always: report the run count, cache label, and measurements separately.
- Ask first: establish a merge-blocking performance threshold or add remote
  telemetry/storage.
- Never: conflate live GitHub/fetch time with local process startup or claim
  a cross-host comparison is a regression.

## Success Criteria

- A report distinguishes local startup from network-backed opening.
- Multi-run reports provide median and maximum observations with enough context
  for an agent to explain their limits.

## Open Questions

- None; baseline storage and enforcement are explicitly out of scope.
