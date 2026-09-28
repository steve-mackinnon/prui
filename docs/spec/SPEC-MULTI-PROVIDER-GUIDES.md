# Spec: Configurable guide models with Charm Fantasy

Status: implemented in this worktree; terminal acceptance remains pending.

The original single-selection and no-picker interaction in this spec describes
the first release. [SPEC-GUIDE-GENERATION-MODAL.md](SPEC-GUIDE-GENERATION-MODAL.md)
supersedes those interaction requirements with a guide modal, an editable model
ID, key-aware provider choices, and a remembered last confirmed pair.

## Objective and scope

Reviewers can generate a guide with their chosen model through OpenAI, Anthropic,
Google Gemini, or a user-supplied OpenAI-compatible endpoint. One global preference
selects the provider and model. The existing explicit `g` action and source-upload
confirmation remain required. Guide input, validation, raw review, and saved
sessions continue to work when a model is unavailable.

The first release supports one active model per user and at most one outbound
model request per `g` action. It does not add a model picker, project-controlled
configuration, automatic provider fallback, provider login, or API-key storage.
It does not add guide-generation flags to `open`, `resume`, `--plain`, or `verify`.

## Technical choice

Use `charm.land/fantasy` at an exact version compatible with this repository's
Go 1.26.8 toolchain; v0.41.3 declares Go 1.26.6. Use Fantasy for provider
request/response protocols and structured-object generation. Keep the existing
`guide.Analyzer` interface and implement one thin Fantasy-backed analyzer.
`guide.InputFrom`, privacy filtering, prompt assembly, guide reconciliation,
`guide.Validate`, and session creation stay owned by this application.

Fantasy's OpenAI provider exposes Responses selection, custom HTTP clients,
and structured-object generation. Its v0.41.3 OpenAI implementation sets
`store:false` by default and disables SDK retries. The same behavior must be
observed in local request tests before replacing the current adapter. Fantasy's
OpenAI-compatible provider defaults to tool-based object output; compatibility
therefore depends on the selected endpoint and model supporting that request
shape. A failing capability is reported as guide unavailable, with no automatic
switch to a less constrained mode.

Sources: [Fantasy repository](https://github.com/charmbracelet/fantasy),
[v0.41.3 module](https://raw.githubusercontent.com/charmbracelet/fantasy/v0.41.3/go.mod),
[OpenAI provider](https://raw.githubusercontent.com/charmbracelet/fantasy/v0.41.3/providers/openai/openai.go),
[Responses implementation](https://raw.githubusercontent.com/charmbracelet/fantasy/v0.41.3/providers/openai/responses_language_model.go),
[OpenAI-compatible provider](https://raw.githubusercontent.com/charmbracelet/fantasy/v0.41.3/providers/openaicompat/openaicompat.go).

## User configuration

Read `config.json` from `$XDG_CONFIG_HOME/pr-review/config.json` when
`XDG_CONFIG_HOME` is absolute, otherwise `~/.config/pr-review/config.json`.
This rule applies on both macOS and Linux. It follows the convention used by
terminal-first tools such as Fish, GitHub CLI, and OpenCode. The file is
separate from the session store and is never read from the reviewed checkout.
An absent file selects the current OpenAI
model and preserves `OPENAI_API_KEY` and `OPENAI_BASE_URL` behavior. A present
file is the complete guide selection; environment variables supply secrets,
not an alternative provider selection.

Example for a native provider:

```json
{
  "guide": {
    "provider": "anthropic",
    "model": "<provider-model-id>",
    "api_key_env": "ANTHROPIC_API_KEY"
  }
}
```

Example for a custom endpoint:

```json
{
  "guide": {
    "provider": "openai-compatible",
    "model": "<endpoint-model-id>",
    "base_url": "https://models.example/v1",
    "api_key_env": "MY_MODEL_API_KEY"
  }
}
```

The root object requires a `guide` section. `provider` is one of `openai`,
`anthropic`, `google`, or
`openai-compatible`. Native providers have documented default endpoints;
`base_url` is required for `openai-compatible` and may override OpenAI's
endpoint for compatibility with the existing override. Legacy `OPENAI_BASE_URL`
values are origins; resolution must append `/v1` exactly once. The base URL is a URL
prefix including its API version when needed; the selected Fantasy provider
appends its operation path. `model` is a nonempty provider model ID, with no
local model allowlist. `api_key_env` names a nonempty environment variable
containing the key; native providers default to `OPENAI_API_KEY`,
`ANTHROPIC_API_KEY`, or `GEMINI_API_KEY` when omitted. A custom endpoint may
omit `api_key_env` only for an explicit loopback URL that needs no credential.

The loader rejects malformed JSON, duplicate or unknown keys, invalid provider
names, empty model IDs, embedded URL credentials, query strings, fragments,
non-HTTPS remote URLs, and non-loopback HTTP URLs. Invalid configuration is
reported on an attempted guide action without sending source or a credential.
The consent screen may show a safe fixed error but never raw file contents.
The file contains no credential value and is not saved into a session.
`--store` remains a session-storage override, not a config-location override.
The existing `theme.json` keeps its current location and remains the theme
preference file for this release. Merging theme settings into `config.json`
requires a separate migration of its strict parser and picker persistence
behavior.

Path sources: [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir/0.8/),
[Fish configuration](https://fishshell.com/docs/current/language.html),
[GitHub CLI configuration](https://cli.github.com/manual/gh_help_environment),
[OpenCode configuration](https://dev.opencode.ai/docs/config/).

## Selection, consent, and execution

For the current modal interaction and selection precedence, follow
[SPEC-GUIDE-GENERATION-MODAL.md](SPEC-GUIDE-GENERATION-MODAL.md). The steps below
record the first release's fixed selection workflow; the one-request and
upload-safety limits still apply.

1. Resolve and validate one immutable guide selection when constructing the
   interactive model. The same selection supplies the consent text, cache
   lookup, and analyzer factory, so they cannot drift during a run.
2. Show provider, model, and sanitized destination origin before upload.
   Describe source categories and the credential filter as today. State
   `store:false` only for requests where that setting is actually sent;
   do not claim a common retention policy for all providers.
3. On `g` confirmation, check offline status before reading the key or building
   a client. Resolve the named environment variable, build the Fantasy model,
   and send only the assembled `guide.Input` through the existing prompt and
   structured guide schema. Do not enable application tools, agent loops,
   streaming, schema repair, or text-mode fallback. Fantasy's forced schema
   tool for OpenAI-compatible endpoints is allowed only as its output format.
4. Translate the returned object to `guide.Bundle`, set provider/model/prompt
   provenance, and let `guide.Analyze` run its existing bounded and inventory
   validation. Reject refusals, incomplete output, malformed objects, and
   unsupported schema behavior as unavailable analysis. Cancellation leaves
   the current session intact.

The request path must retain the current 2 MiB request and 4 MiB response
caps. A shared HTTP client/transport enforces those caps before sending and
while reading, refuses redirects, and allows plaintext only for explicit
loopback endpoints. Verify that Fantasy does not issue a second request through
retries, tool fallback, or discovery. Treat provider errors as untrusted:
redact the key, omit raw bodies and prompt text, strip controls, and bound the
durable reason to the current limit. No telemetry or full transcript is kept.

## Cache and persistence

An existing generated guide remains readable in its saved session regardless
of the current selection. Automatic cache reuse on a PR-list open requires a
match to the selected provider, model, normalized endpoint identity, prompt
version, and guide schema. Add an optional non-secret selection fingerprint to
the generated bundle, derived from those values; never hash or persist the
credential. The existing single cache slot per comparison may be replaced by
a successful guide for a different selection. This avoids a SQLite schema
migration; switching back can require regeneration. A legacy bundle without a
fingerprint is eligible for automatic reuse only under the unchanged default
OpenAI selection with no endpoint override. Unavailable guides remain uncached.

The fingerprint is an equality marker, not an authorization decision. Consent
is still required for every new upload. A cache hit makes no provider call.

## Package boundaries and implementation order

| Area | Change |
| --- | --- |
| `internal/guide` | Fantasy-backed analyzer; retain input, prompt, and output validation. |
| `internal/guideconfig` | Read and validate global provider/model selection, endpoint, and key-variable name. |
| `cmd/pr-review/wiring.go` | Resolve one selection, inject analyzer factory and consent identity. |
| `internal/tui` | Show selected provider, model, destination, and accurate retention wording. |
| `internal/session` and `cmd/pr-review/lifecycle.go` | Compare selection fingerprint before cache reuse; keep old sessions readable. |
| `CONSTRAINTS.md`, `README.md`, `docs/REFERENCE.md` | Replace OpenAI-only guide rules and explain configuration. |

Implement in this order: pin and test Fantasy with local HTTP fixtures; add
config resolution; wire selection and consent; add cache matching; update
documentation and remove the old OpenAI transport only after parity tests pass.
Keep the change in small reviewable commits.

## Style and verification

Continue the current small Go interface and constructor style. The boundary
remains:

```go
type Analyzer interface {
	Analyze(context.Context, Input) (Bundle, error)
}
```

Use `gofmt`, ordinary errors with safe user-facing text, and local `httptest`
servers with fake credentials. No live provider or GitHub calls run in tests.
For each provider, capture the outbound request and test exact destination,
method, authentication, prompt scope, schema mode, request count, response
limit, refusal, cancellation, and error redaction. Also test malformed config,
legacy environment behavior, offline refusal before credential access, consent
identity, cache hits/misses across selections, and session readability after
a selection change. Existing privacy and inventory tests remain green.

Repository checks:

```sh
go test ./internal/guide/... ./internal/guideconfig/... ./cmd/pr-review/... ./internal/session/... ./internal/tui/...
go vet ./...
go test -race -count=1 ./...
go build ./...
```

## Acceptance criteria and boundary updates

- With no config file, the existing OpenAI guide workflow remains usable.
- Each listed provider can generate a structurally valid guide from a local
  fixture through one confirmed request, with correct provenance.
- A configured unsupported model or endpoint fails visibly without sending
  source to another provider or altering the current review.
- Consent identifies the exact selected provider, model, and destination.
- A changed selection cannot silently reuse a different selection's guide.
- Raw review, saved guides, offline mode, and non-interactive commands retain
  their present behavior.
- `CONSTRAINTS.md` is updated as part of implementation: the single Responses
  request, OpenAI-only credential, provider-default-model, and current cache
  key rules become provider-neutral while preserving privacy and one-request
  guarantees. No exception is taken silently.

## Verified library behavior and remaining limit

Fantasy v0.41.3 compiles with Go 1.26.8. The local OpenAI contract test proves
one Responses request, `store:false`, the existing schema, and no retry. That
release omits `strict:true` by default; the OpenAI constructor sets it with
`option.WithJSONSet("text.format.strict", true)`. The bounded client enforces
the byte limits and blocks redirects for all four providers. Its OpenAI
wrapper replaces SDK authorization with only the configured key so an
unrelated `OPENAI_API_KEY` cannot reach a keyless loopback endpoint.

Fantasy's Anthropic wrapper does not expose the underlying SDK's option to
disable all environment defaults. The constructor supplies an explicit key
and endpoint, and fixtures verify they take precedence. An unrelated malformed
Anthropic SDK profile environment may still cause a local setup error. It
does not authorize a different upload destination.
