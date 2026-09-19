# Spec: Current-Checkout Pull Request Picker

## Objective

Let a developer start the interactive review flow from the root of a local
GitHub checkout without first finding or pasting a pull-request URL. Running
`pr-review` with no subcommand will identify the checkout's GitHub repository,
list its open pull requests in the terminal UI, and open the selected pull
request through the existing pinned, read-only review path.

The intended user is a reviewer already working in a local checkout. Success
means they can run one command, choose `#number title` from the current
repository's open-PR list, and reach the normal review screen without manually
supplying a GitHub URL, owner, or repository name.

This is one capability. It extends the current `prs` browser, which lists
remembered repositories, but does not change session storage, review pinning,
GitHub metadata retrieval, or guide analysis.

## Tech Stack

Go 1.26.8; Bubble Tea v2.0.9; the existing authenticated `gh` CLI; the
repository's existing safe process runner and source-pinning implementation.
No dependency or storage-schema change is needed.

## Command Contract

New interactive entry point:

```sh
cd /path/to/github-checkout
pr-review
```

Behavior:

1. Require the current directory to be the checkout root, using the same root
   validation as `open` and `verify`.
2. Resolve a canonical `owner/repository` from that checkout's GitHub `origin`
   remote before the TUI starts. This local read makes no network request.
3. Start directly on the existing open-pull-request picker and asynchronously
   request at most 100 open PRs with the existing `gh api` read-only operation.
4. Render each result as `#<number> <escaped title>`; preserve the existing
   picker keyboard behavior, loading state, cancellation, error display, empty
   state, dimensions, and selection markers.
5. On Enter, open the selected PR using the resolved checkout and existing
   `application.open` flow. It must retain its current GitHub metadata pinning,
   isolated object access, immutable session save, explicit freshness check,
   and repository-memory update.
6. Escape from the initial picker exits normally. `q` and Ctrl+C retain their
   existing quit/cancel semantics.

Existing commands remain compatible:

```sh
pr-review open https://github.com/owner/repo/pull/42
pr-review open 42 --github-repo owner/repo
pr-review prs
pr-review prs owner/repo --plain
```

`prs` continues to browse remembered repositories when interactive; it is not
silently redefined as the current-checkout entry point. No-argument launch
requires an interactive terminal. In plain or redirected mode it exits with a
clear error directing the caller to `prs owner/repo --plain` or `open`.

### Repository Resolution Contract

Repository resolution is deliberately local and narrow.

- Reuse the existing `.git`/linked-worktree path validation, then read only the
  applicable Git configuration files needed to find `[remote "origin"] url`.
- Accept only an unambiguous GitHub.com remote in conventional HTTPS or SSH
  forms, normalize an optional trailing `.git`, and validate the resulting
  `owner/repository` through the existing identity validation.
- Read bounded regular files only; reject symlinks, oversized files, malformed
  sections, duplicate origin URLs, credentials in URLs, non-GitHub hosts,
  query/fragment paths, and unsupported transports. Do not process `include`
  directives or execute Git to interpret configuration.
- Support ordinary checkouts and linked worktrees by consulting their resolved
  common Git directory and a worktree-specific config only when Git declares
  it applicable. A worktree-specific `origin` takes precedence over the common
  configuration; ambiguity is an error.
- If no safe GitHub `origin` can be determined, do not start a network request.
  Print a stated error that keeps the established alternatives visible:
  `pr-review prs owner/repo` or `pr-review open URL`.

This explicit parser is preferable to invoking `git config` or running `gh`
inside the checkout: the repository contract forbids allowing
repository-selected config, hooks, helpers, or network behavior to influence
the tool.

## Commands

```sh
go test ./cmd/pr-review ./internal/source ./internal/tui -count=1
go test -race ./cmd/pr-review ./internal/source ./internal/tui -count=1
go vet ./...
go test -race -count=1 ./...
go build ./...
./scripts/verify.sh
git diff --check
```

The final verification command remains synthetic-only and must not require an
authenticated GitHub account. A manually invoked real-checkout smoke test is
optional after the synthetic gate:

```sh
cd /path/to/github-checkout
pr-review
```

## Project Structure

```text
cmd/pr-review/options.go       command parsing and usage text
cmd/pr-review/main.go          no-argument entry dispatch and terminal checks
cmd/pr-review/wiring.go        initial current-checkout picker wiring
cmd/pr-review/lifecycle.go     reuse existing list/open operations
internal/source/               bounded Git-dir/config and GitHub-origin resolver
internal/tui/model.go          direct-current-repository browser constructor/state
internal/tui/lifecycle.go      picker startup, rendering, and lifecycle actions
cmd/pr-review/*_test.go        CLI and application wiring regressions
internal/source/*_test.go      safe remote-resolution contract tests
internal/tui/*_test.go         picker loading, selection, error, cancel regressions
README.md                      user-facing launch and fallback documentation
```

## Code Style

Keep the security boundary explicit: parse and validate untrusted local config
before passing a repository identity to the trusted GitHub client. Return
stated errors; do not fall back to a guessed remote or broaden the accepted URL
forms.

```go
repository, err := source.RepositoryFromCheckout(checkout)
if err != nil {
    return fmt.Errorf("current checkout has no supported GitHub origin: %w", err)
}

m := tui.NewCurrentRepositoryBrowser(ctx, app.store, repository, checkout,
    app.listPullRequests, app.open)
```

Use existing Go conventions: exported APIs only at package boundaries, typed
messages for asynchronous Bubble Tea actions, context cancellation throughout,
and escaped text at every TUI render boundary. Preserve the existing `gh` API
request and `source.PullRequest` model rather than creating a second listing
client.

## Testing Strategy

- Add source unit tests for accepted HTTPS/SSH GitHub-origin forms, optional
  `.git` suffixes, ordinary `.git` directories, gitdir files, and linked
  worktrees.
- Add rejection tests for absent origin, non-GitHub hosts, credential-bearing
  URLs, includes, malformed/duplicate origin values, symlinks, oversized
  configuration, and invalid repository names. Assert that resolver failures
  do not invoke `gh` or Git.
- Add command-parser and entry-point tests proving no-argument launch requires
  a checkout root and an interactive terminal, while existing `open`, `prs`,
  `sessions`, `resume`, `verify`, and plain-output behavior remain unchanged.
- Extend TUI scenario/program tests to prove the direct picker loads the
  resolved repository, shows loading/error/empty states, supports cancellation
  and retry, opens the selected PR with the supplied checkout, and preserves
  existing keyboard/back behavior after entering review.
- Use synthetic fixtures and a fake GitHub client only. Do not contact GitHub,
  read real credentials, execute checkout code, or execute Git configuration
  in tests.

## Boundaries

- Always: validate the checkout root and every derived `owner/repository`; use
  bounded local reads; escape TUI text; use existing cancellation and source
  pinning; retain direct URL/number and remembered-repository workflows; run
  focused tests plus the full verification gate before implementation is
  considered complete.
- Ask first: support GitHub Enterprise or additional remote transports; use a
  remote other than `origin`; add dependencies; change session storage; change
  the GitHub API list fields/limit; alter CI; or make listing available in
  `--plain` mode without an explicit repository.
- Never: execute `git` in the checkout to resolve the repository; honor Git
  config includes, aliases, hooks, credential helpers, URL rewrites, or
  repository-specified helpers; mutate the checkout or Git metadata; persist
  credentials; poll GitHub; send source to a provider; or weaken synthetic
  test coverage.

## Success Criteria

- From the root of a supported GitHub checkout, `pr-review` opens an
  interactive list of that repository's open PRs without a pasted URL.
- Selecting a PR reaches the existing review screen through the same pinned,
  read-only opening path as `pr-review open`.
- The startup list is cancellable, retryable, bounded to the current
  100-result API contract, safe to render, and handles zero PRs and GitHub
  failures without losing the picker or starting a review.
- A checkout with no safe GitHub `origin` fails before contacting GitHub and
  tells the user how to use the existing explicit commands.
- No-argument use outside a checkout root and noninteractive no-argument use
  fail clearly; all pre-existing CLI contracts continue to pass their tests.
- The full synthetic verification gate and `git diff --check` pass.

## Open Questions

None for the initial GitHub.com/origin-only scope. GitHub Enterprise, arbitrary
remote selection, search/filtering, PR state filters, and pagination are
deliberately deferred rather than inferred into this launch-flow feature.
