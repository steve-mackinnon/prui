# Releasing prui

Git tags are the source of release versions. GoReleaser embeds the tag's version
and commit in `prui --version`; ordinary local builds report `dev` and `unknown`.
There is no separate version file to bump.

## Version policy

- `0.1.0` is the first public release.
- During 0.x, increment the patch for compatible fixes and the minor for new
  features or breaking changes. Document breaking changes and migration steps.
- Release `1.0.0` when CLI, configuration, and saved-data compatibility are stable.
  After that, use SemVer: major for breaking changes, minor for compatible
  features, patch for compatible fixes.
- Use tags such as `v0.2.0-rc.1` for release candidates. GoReleaser marks these
  as GitHub prereleases automatically.
- Never move a published tag or replace published binaries. Fix defects in a new
  version. Users can install a previous release when needed; saved-data migrations
  must explicitly document whether downgrades are supported.

## Prepare and publish

1. Add user-facing changes under `Unreleased` in `CHANGELOG.md` as work lands.
2. Before release, move those entries under an exact heading such as `## [0.2.0]`.
   Include any migration instructions. The workflow uses this section as release
   notes and fails if it is missing or empty.
3. Run `./scripts/verify.sh`, `golangci-lint run` using CI's pinned version
   (**v2.13.2**), and GoReleaser **v2.18.2**:

   ```sh
   goreleaser check
   goreleaser release --snapshot --clean
   ```

   Snapshot builds create all four archives and checksums in `dist/` without
   publishing. Smoke-test the native binary with `--version` and `--help`.
4. Merge the changes into `main` and wait for its verification checks to pass.
   Tag that exact verified commit (replace the example version and SHA):

   ```sh
   git tag -a v0.2.0 <verified-commit-sha> -m "Release 0.2.0"
   git push origin v0.2.0
   ```

5. Watch the **Release** workflow. It reruns the reusable Verify workflow on the
   tagged commit, including Linux/macOS terminal tests, lint, vulnerability checks,
   and secret scanning. Only then does it publish archives, checksums, and notes.
6. Check the GitHub Release has four archives and `checksums.txt`. Download the
   native archive, verify its checksum, and run `prui --version` and `prui --help`.
   Confirm the embedded version and commit match the tag.

The release job uses GitHub's built-in token with `contents: write`; no personal
token or extra repository secret is needed. Action revisions and the GoReleaser
version are pinned. Update those pins deliberately and validate snapshot builds.

If a run fails before publishing, fix the cause and rerun only when the tagged
source remains correct. If source changes are needed, use a new version. Inspect
the release page before retrying a publishing failure to avoid replacing assets.
