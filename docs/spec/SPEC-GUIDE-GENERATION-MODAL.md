# Spec: Guide generation modal and remembered model selection

Status: implemented; human terminal acceptance remains pending. This updates the
guide-selection and consent portions of
[SPEC-MULTI-PROVIDER-GUIDES.md](SPEC-MULTI-PROVIDER-GUIDES.md).

## Objective

Pressing `g` in an interactive review opens a modal over the current review.
The modal always shows the provider and model that will receive the guide input,
the sanitized recipient, and the existing source-upload warning. The reviewer
can immediately confirm the preselected pair or change it first. The last
confirmed provider/model pair is remembered across launches, so the common path
requires only `g`, then Enter. Escape closes the modal without changing the
review or remembered choice.

This is one guide-launch workflow: selection and upload consent must describe
the same immutable request. It replaces the previous spec's explicit exclusion
of a model picker and its single fixed selection for the entire interactive run.

## Confirmed decisions and operating assumptions

1. "Remember the last selection" means across app launches, and means the
   provider/model pair last confirmed for generation, even if the request later
   fails or is cancelled.
2. A model is an editable provider model ID. The app does not query providers
   for a model catalog or maintain an allowlist; existing configuration already
   accepts any nonempty provider model ID.
3. A configured key means a nonempty value in the provider's designated
   environment variable. Key names may come from the user's guide config;
   credential values remain outside config, UI, sessions, and logs.
4. Native providers with their documented default key variables are eligible
   even when `config.json` is absent. The compatible provider is eligible only
   when its endpoint is configured, and an explicitly configured keyless
   loopback endpoint remains eligible.

## Tech stack and commands

- Go 1.26.8; Bubble Tea v2, Lip Gloss v2, and the existing ANSI-safe TUI
  rendering helpers.
- Focused tests: `go test ./internal/tui/... ./internal/guideconfig/... ./cmd/pr-review/...`
- Project checks: `go vet ./...`, `go test -race -count=1 ./...`,
  `go build ./...`, and `git diff --check`.

## Project structure and ownership

| Location | Responsibility |
| --- | --- |
| `internal/tui` | Modal state, keyboard handling, consent copy, overlay rendering, and narrow-viewport fallback. |
| `internal/guideconfig` | Secret-free provider availability, selection validation, and app-owned remembered preference. |
| `cmd/pr-review/wiring.go` | Supply candidates to the TUI and bind one confirmed selection to the guide request. |
| `cmd/pr-review/lifecycle.go` | Stamp the confirmed selection fingerprint on generated guides. |
| `docs/REFERENCE.md`, `README.md`, `CONSTRAINTS.md` | Document the updated interaction and selection rules. |

## Interaction contract

1. `g` opens a centered modal over the active review. The review remains visible
   behind it; modal keys do not navigate or edit the review. The modal has a
   bounded layout for small terminals, and it does not expose terminal control
   bytes from provider, model, or endpoint text. When the viewport cannot show
   the full upload disclosure, it asks for a resize and disables confirmation.
2. Provider and model rows are always visible. The modal opens with the last
   confirmed valid pair selected. If none exists, it uses the configured pair
   when available, otherwise the current OpenAI default when its key is present.
   The primary action is focused on open; pressing Enter immediately confirms
   the displayed pair and the source upload. The reviewer can instead focus and
   change either row before confirming. Escape cancels at any point before
   upload; `q` retains its existing quit behavior.
3. Provider choices are based on the current process environment and validated
   user-owned configuration. A missing key does not produce a selectable remote
   provider. No key is read to render its value, and no provider-discovery
   network request occurs. If no usable provider exists, the modal still shows
   provider and model rows, explains that a key or endpoint is needed, and
   disables confirmation.
4. The model row shows the selected model ID and supports editing it. Switching
   providers restores the last known model ID for that provider when available;
   otherwise it displays an explicit model-ID prompt rather than silently using
   a different provider's model. An empty or malformed ID cannot be confirmed.
   Changing a row updates the displayed recipient and provider-specific
   retention wording before confirmation.
5. Confirmation freezes one validated selection. The same value supplies the
   visible consent, analyzer construction, request destination, provider/model
   provenance, and cache fingerprint. A key disappearing after the modal opens
   fails before upload. There is no fallback to another provider or model.
6. The selected provider/model pair is saved in a private app-owned
   `guide-last.json` beside the global XDG `config.json`, outside the reviewed
   checkout. The preference contains no key value or source content. Saving
   leaves guide configuration untouched and refuses to overwrite malformed
   preference data. A failed preference write is
   reported without silently claiming that the choice was remembered; the
   current confirmed request can still proceed if its selection is valid.
7. Existing explicit consent, offline refusal, cancellation, one-request
   limit, privacy filtering, immutable derived sessions, and cached-guide
   fingerprint matching remain in force. PR-list opens never generate a guide.

## Code style and testing strategy

Keep the TUI callback free of provider-specific request logic. Pass one
validated, secret-free selection to the command layer, where credentials are
read only after confirmation and the offline check. Preserve the existing
small-interface style:

```go
type GuideLoader func(context.Context, *review.Session, guideconfig.Selection, func(string)) (*review.Session, error)
```

Use `gofmt`, `Escape`, `clip`, and measured terminal-cell widths for modal text.
Unit tests cover provider availability, preference precedence and persistence,
model validation, and modal keys. TUI render tests cover normal and small
terminals, control-byte safety, and the underlying review remaining visible.
Command integration tests use fake keys and local HTTP fixtures to prove that
consent, outbound request, provenance, and cache fingerprint name the same pair.
No test contacts a live provider or GitHub.

## Boundaries

- Always: require explicit `g` plus confirmation; validate the exact selection
  before reading a credential or uploading source; store only secret-free
  preference data; keep existing privacy and transport limits.
- Ask first: adding a model-catalog service, changing the guide-config schema,
  or changing the meaning of an existing explicit endpoint override.
- Never: store or render key values, read configuration from the reviewed
  checkout, silently switch providers, or auto-generate on open/resume.

## Success criteria

- `g` shows a modal over the same review at normal terminal sizes, with provider
  and model visible before upload. Escape returns to the unchanged review.
- With a valid remembered pair, `g` followed by Enter starts exactly one guide
  request using that pair, subject to the existing upload confirmation.
- A reviewer can change provider and model in the modal; only currently usable
  providers are selectable. The displayed recipient and request identity agree.
- Restarting the app preselects the last confirmed pair if still usable. A
  missing key or invalid config prevents upload; an invalid preference is
  ignored and cannot supply the request identity.
- Generated guide provenance and cache fingerprint match the exact confirmed
  selection; existing saved guides remain readable after a selection change.
- Focused and project checks above pass with synthetic fixtures; a human
  terminal walkthrough verifies the interaction and small-screen layout.

## Preference precedence

`guide-last.json` stores a provider and model ID only. A valid remembered pair
is preselected when its provider is currently usable. Otherwise the configured
pair is used when available, followed by the first usable provider. The
remembered model does not carry endpoint or key-variable settings; those are
resolved from current user configuration and environment. The model ID remains
editable in the modal.
