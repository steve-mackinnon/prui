# Spec: Live PR Verifier

## Objective

Provide an explicit, agent-operated acceptance command that opens a real GitHub
pull request through the shipped `prui` binary, exercises a small terminal
journey, and reports a concise verdict suitable for a user reading Codex on a
phone. It establishes that the production source-opening path, TUI navigation,
durable local progress, and resume behavior compose correctly against a pinned
real PR.

The command is for an authenticated developer working from an existing local
checkout. It is manually invoked by an agent or developer and is never part of
the normal verification gate or CI.

## Tech Stack

Go 1.26.8, Bubble Tea v2, Git, the user's authenticated GitHub CLI, and the
existing Python 3 standard-library PTY support. No new runtime dependency is
introduced by this module.

## Command Contract

The proposed public command is:

```sh
cd /absolute/checkout
prui verify PR-URL --artifacts /absolute/output-directory
```

Required inputs:

- `PR-URL` is parsed by the existing pinned-identity parser.
- The command must be launched from the root of an existing checkout for the
  PR repository; a checkout path flag is intentionally not accepted.
- `--artifacts` names a new, empty directory owned by this run. It must be
  outside the reviewed checkout and the session store.

The command creates a private, temporary session store and invokes the compiled
binary's normal `open` and `resume` paths under a fixed-size PTY. It reports one
JSON object to standard output and writes no human-oriented terminal UI to
standard output. Progress and diagnostics go to standard error.

Successful result shape:

```json
{
  "schema_version": 1,
  "status": "passed",
  "pr": "owner/repo#42",
  "head_sha": "40-character-pinned-sha",
  "session_id": "32-hex-id",
  "checks": [
    {"name": "open", "status": "passed"},
    {"name": "navigate", "status": "passed"},
    {"name": "mark_and_resume", "status": "passed"}
  ],
  "artifacts": {"report": "report.json", "transcript": "terminal.txt"}
}
```

Every completed run emits the same schema, including failure and cancellation.
Each check has status `passed`, `failed`, `skipped`, or `not_run`; a failed run
states a short, sanitized reason and includes only artifacts created before the
failure. Exit code is 0 only when every required check passed, 1 for a failed
verification or invalid invocation, and 130 for cancellation.

The first journey is intentionally small:

1. Open the requested PR using normal production source pinning.
2. Wait for a complete or explicitly incomplete frozen review screen.
3. Navigate to a second selectable review item when one exists; otherwise mark
   the only item.
4. Mark one file slice, quit normally, and resume the created session offline.
5. Confirm the marked slice is visibly present after resume.

The verifier records an incomplete inventory as a failed `open` check rather
than claiming a complete acceptance result. A PR with no review units is
reported as `skipped` for navigation and marking, with an overall `passed`
result if opening and safe exit succeed.

## Project Structure

```text
cmd/prui/              command parsing and application wiring
internal/verify/            verifier contract, journey driver, JSON report
internal/verify/testdata/   synthetic PTY fixtures only
cmd/prui/testdata/     shared terminal-screen support if extracted
SPEC-live-pr-verifier.md    this contract
```

## Code Style

The verifier has a typed result at its external boundary and records a check
before moving to the next step. Internal failures do not leak credentials,
unbounded terminal output, or source content into JSON.

```go
result.Checks = append(result.Checks, Check{Name: "open", Status: Passed})
if err != nil {
    result.Status = Failed
    result.Reason = stated(err)
    return result
}
```

Names follow existing Go conventions. JSON field names are snake_case because
the stored session format already uses it. Artifact paths are relative to the
explicit artifact directory, never absolute machine paths.

## Testing Strategy

- Unit-test input validation, stable report serialization, exit status, and
  sanitized failure reasons in `internal/verify`.
- Use a synthetic `gh` executable and disposable Git fixture for the full PTY
  journey; it must prove that `verify` invokes the production command path and
  preserves marked progress on offline resume.
- Keep real-PR runs manual. They are evidence for a particular PR and host,
  not deterministic automated tests.
- Run `go test -race ./internal/verify ./cmd/prui -count=1`, then
  `./scripts/verify.sh` before merging.

## Boundaries

- Always: create a fresh private store; use fixed PTY dimensions; bound every
  wait and captured output; preserve a machine-readable result on failure;
  use the existing source-pinning path.
- Ask first: add dependencies, add CI execution, alter session storage format,
  accept arbitrary shell commands, or make guide generation noninteractive.
- Never: execute PR code, hooks, filters, or repository-selected helpers; write
  inside the checkout; persist credentials; emit source or access tokens in a
  report; run automatically on a PR or push.

## Success Criteria

- An agent can invoke one command with a real PR URL, checkout, and artifact
  directory, then decide pass/fail from one JSON object.
- A passing report proves open, terminal navigation, durable marking, and
  offline resume of a pinned session.
- Invalid inputs, GitHub/auth failures, PTY timeout, incomplete inventories,
  and cancellation produce stable non-success results without a panic or
  secret/source disclosure.
- The normal `./scripts/verify.sh` remains synthetic-only and requires no
  GitHub credentials.

## Open Questions

- None for this module. It deliberately excludes screenshots, timings, and
  guide evaluation, which have their own approved modules.
