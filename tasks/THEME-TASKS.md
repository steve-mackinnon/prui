# Interactive TUI Themes Tasks

## Task 1: Semantic theme domain

**Description:** Define the fixed semantic token vocabulary, all four built-in
palettes, and strict conversion of documented color syntax into safe color
values. Encode `terminal` from the current palette before changing rendering.

**Acceptance criteria:**

- [ ] `terminal`, `light`, `dark`, and `high-contrast` each resolve every
      semantic token to a complete immutable palette.
- [ ] `terminal` preserves the current color mappings for every styled line
      class, including selection and pane borders.
- [ ] Accepted colors and every invalid syntax/token/theme-name case are
      deterministically tested without raw ANSI/config strings reaching render.

**Verification:**

- [ ] `go test ./internal/theme -count=1`
- [ ] `go test -race ./internal/theme -count=1`

**Dependencies:** None.

**Files likely touched:**

- `internal/theme/theme.go`
- `internal/theme/theme_test.go`
- `internal/tui/style.go`
- `internal/tui/style_test.go`

**Estimated scope:** Medium (4 files).

## Task 2: Global configuration and atomic persistence

**Description:** Add platform-aware discovery of `theme.json`, whole-file JSON
validation, deterministic encoding, and atomic selection updates that preserve
valid token overrides. Keep malformed configuration read-only.

**Acceptance criteria:**

- [ ] macOS, Linux/XDG, and Linux fallback paths are derived from injected
      environment/platform inputs, never the actual home directory in tests.
- [ ] Missing config falls back silently; malformed/unknown-key config produces
      a safe diagnostic and cannot be overwritten by selection persistence.
- [ ] A successful selection creates private parent/file paths as needed and
      atomically changes only `theme`, retaining valid `colors` overrides.

**Verification:**

- [ ] `go test ./internal/theme -run 'Test(Config|Persistence)' -count=1`
- [ ] `go test -race ./internal/theme -count=1`
- [ ] Manual fixture inspection confirms a failed replacement retains original
      file bytes and a successful write is valid JSON.

**Dependencies:** Task 1.

**Files likely touched:**

- `internal/theme/config.go`
- `internal/theme/config_test.go`
- `internal/theme/theme.go`

**Estimated scope:** Small (3 files).

## Checkpoint: Theme foundation

- [ ] Tasks 1–2 pass their focused tests and race checks.
- [ ] The compatibility palette and persistence/error behavior have been
      reviewed before touching the live TUI.

## Task 3: Model-owned palette application

**Description:** Refactor interactive style construction so each `tui.Model`
owns its resolved palette instead of sharing mutable process-global styles.
Keep plain rendering independent of theme configuration.

**Acceptance criteria:**

- [ ] Two models can render with different themes without affecting each other.
- [ ] Every current styled class maps through semantic tokens; existing markers,
      reverse/bold attributes, clipping, and escaped text are unchanged.
- [ ] All built-ins and a valid override satisfy the existing profile matrix;
      ANSI stripping reproduces identical unstyled text and `Plain` remains
      byte-identical.

**Verification:**

- [ ] `go test ./internal/tui -run 'Test(Theme|Color|Style)' -count=1`
- [ ] `go test -race ./internal/tui -count=1`

**Dependencies:** Task 1.

**Files likely touched:**

- `internal/tui/style.go`
- `internal/tui/model.go`
- `internal/tui/{style,color,model}_test.go`

**Estimated scope:** Medium (5 files).

## Task 4: CLI selection and deterministic startup

**Description:** Add `--theme` to supported interactive commands, resolve CLI
over config over default before starting the TUI, and deliver non-fatal config
diagnostics through existing safe notification/stderr paths.

**Acceptance criteria:**

- [ ] Valid `--theme` is accepted by `current`, `open`, `resume`, and `prs`;
      it wins over valid global selection for that process.
- [ ] Invalid CLI selection exits 1 before storage, Git, GitHub, or TUI setup.
- [ ] Plain invocations retain byte-identical output without config reads, and
      `verify` remains independent of personal config and rejects `--theme`.

**Verification:**

- [ ] `go test ./cmd/prui -run 'Test(Options|Theme)' -count=1`
- [ ] `go test ./internal/verify -count=1`
- [ ] `go build ./cmd/prui`

**Dependencies:** Tasks 2–3.

**Files likely touched:**

- `cmd/prui/options.go`
- `cmd/prui/main.go`
- `cmd/prui/wiring.go`
- `cmd/prui/{options,wiring}_test.go`
- `internal/verify/*_test.go`

**Estimated scope:** Medium (5 files).

## Checkpoint: Startup contract

- [ ] Tasks 3–4 pass focused tests.
- [ ] A manual terminal run confirms a configured/CLI theme changes interactive
      color only; `--plain`, `NO_COLOR`, and `verify` remain deterministic.

## Task 5: Persistent keyboard theme picker

**Description:** Add the `t` modal and input routing using the resolved theme
domain and persistence interface. Make the modal readable and controllable
without color at ordinary and tiny terminal sizes.

**Acceptance criteria:**

- [ ] `t` opens the named built-in list in interactive review/picker screens;
      arrows or `j`/`k`, `enter`, `esc`, and repeated `t` obey the approved
      modal contract.
- [ ] A successful `enter` persists global selection, retains valid overrides,
      and updates only the active model after persistence succeeds.
- [ ] Editor/action/loading/confirmation states retain exclusive key ownership;
      persistence errors preserve the old active palette and show safe feedback.

**Verification:**

- [ ] `go test ./internal/tui -run 'Test(ThemePicker|Theme|Color)' -count=1`
- [ ] `go test -race ./internal/tui -count=1`
- [ ] Manually inspect `terminal`, `light`, `dark`, and `high-contrast` in a
      true-color terminal, ANSI-16 profile, and with `NO_COLOR=1`.

**Dependencies:** Tasks 2–4.

**Files likely touched:**

- `internal/tui/theme_picker.go`
- `internal/tui/model.go`
- `internal/tui/render.go`
- `internal/tui/{model,render,theme_picker}_test.go`

**Estimated scope:** Medium (5 files).

## Task 6: Documentation and final regression gate

**Description:** Publish the exact theme paths/schema/precedence and update the
standing color contract, then verify the entire feature against repository and
manual terminal checks.

**Acceptance criteria:**

- [ ] README documents built-ins, CLI precedence, `theme.json`, picker
      persistence, malformed-file behavior, and accessibility limits.
- [ ] CONSTRAINTS preserves the non-color/plain/capability contract while
      allowing global theme selection and safe configuration persistence.
- [ ] All spec success criteria have direct automated evidence or an explicit
      recorded manual inspection.

**Verification:**

- [ ] `go vet ./...`
- [ ] `go test -race -count=1 ./...`
- [ ] `go build ./...`
- [ ] `git diff --check`

**Dependencies:** Tasks 1–5.

**Files likely touched:**

- `README.md`
- `CONSTRAINTS.md`
- `docs/spec/SPEC-custom-themes.md`

**Estimated scope:** Small (3 files).

## Completion checkpoint

- [ ] The focused checks after every task and the complete repository gate pass.
- [ ] The app is manually readable in light, dark, low-color, and no-color
      terminal contexts.
- [ ] The final diff is limited to the approved theme design, implementation,
      tests, and documentation.
