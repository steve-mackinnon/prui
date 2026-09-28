# Capability Map: Agent PR Verifier

The agent PR verifier is an explicitly invoked, local developer utility for
opening a pinned review of a real GitHub pull request and reporting whether the
review application completed a defined acceptance journey. It is not a CI job.

| Module id | Responsibility | Depends on |
| --- | --- | --- |
| `live-pr-verifier` | Open a real PR safely, drive the defined review journey, and emit a machine-readable verdict | — |
| `terminal-artifacts` | Save a transcript and deterministic terminal-screen images for agent inspection and chat attachment | `live-pr-verifier` |
| `performance-reporting` | Measure local startup separately from GitHub and Git work | `live-pr-verifier` |
| `guide-evaluation` | Validate stored guide structure and score curated semantic expectations | `live-pr-verifier` |

Build order: `live-pr-verifier` → `terminal-artifacts`, `performance-reporting`,
`guide-evaluation`.

The verifier never executes reviewed code, writes to the reviewed checkout, or
enables guide-source upload. It uses the user's existing authenticated `gh`
installation only for the read-only GitHub operations already performed by
`prui open`.
