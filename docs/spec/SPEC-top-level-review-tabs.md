# Spec: Top-Level Review Tabs

## Objective
Expose Description, Files, Guide, and Commits as peer review views in that order.
Guide becomes directly discoverable without entering Diff and finding a nested
selector. Files is the name of the existing raw file-and-diff review surface.

## Behavior and success criteria
- Render one top-level strip: Description [1], Files [2], Guide [3], Commits [4].
  Exactly one tab has the existing textual selection marker and selected style.
- Remove the nested File/Guide selector. The left pane header identifies Files,
  Guide, or Full inventory; the right pane still identifies the diff.
- Preserve Files as the initial view of newly opened reviews.
- Number keys select the corresponding view. F/G select Files/Guide from any
  review view. v/V cycle forward/backward in the displayed order, wrapping.
- Mouse clicks on the top-level labels select the corresponding view. The old
  nested header no longer changes views. Keyboard and mouse share selection.
- Files and Guide retain their existing raw sources, navigation, commenting,
  marks, scrolling, resizing, and unified/split layout support. Existing reading
  and guide expansion state survive view changes and workspace PR switching.
- Guide remains selectable with no generated guide. Preserve its current empty,
  unavailable, and generated presentations; selection never requests analysis.
  The existing lowercase g action and consent modal remain the generation path.
- Description and Commits retain their current rendering and controls. Pending
  review editing returns to Files; historical discussion navigation still opens
  Commits. Update help, user documentation, and screen fixtures to the new keys.
- This is UI-local state; no stored session/schema or network behavior changes.

## Implementation plan
1. Add focused failing tests for tab order, number/alias keys, cycling, mouse
   selection, absence of nested controls, and generated/empty Guide behavior.
2. Introduce Guide as a review view and share the existing diff workspace between
   Files and Guide. Centralize displayed tab order for rendering and hit testing.
3. Update existing navigation tests, help/docs, and intentionally changed screen
   baselines; run the full repository verification gate.

## Stack, structure, and style
Go with Bubble Tea v2. Review state/rendering: internal/tui/model.go; tab mouse
routing: internal/tui/mouse_review.go; tests: adjacent *_test.go files and
internal/tui/testdata. Specs live in docs/spec. Follow existing gofmt formatting
and enum/method conventions, for example `func (m *Model) selectReviewView(view
reviewView)`. Reuse current presentation and immutable source-target mapping.

## Commands and testing
Focused: `go test ./internal/tui -run 'Test(PRContext|TopLevel|ReviewStarts)' -count=1`
Screens: `go test ./internal/tui -run '^TestScreenSnapshots$' -update-golden -count=1`
Full gate: `./scripts/verify.sh` (formatting, vet, race tests including PTY tests,
and build). Use only synthetic fixtures, no live GitHub/provider requests.
Terminal usability still requires human verification under CONSTRAINTS.md.

## Boundaries
Always preserve immutable source/comment targets and generation consent, and
run the existing verification gate. Ask before broadening scope to persistence,
provider, or GitHub write changes. Never weaken checks or alter reviewed source.

## Open questions
None. The requested hierarchy authorizes specification followed by implementation.

## Verification result

Implemented and verified with `./scripts/verify.sh`: formatting, vet, all race
tests (including compiled-binary PTY navigation), and build passed. Screen
baselines cover Files, generated Guide, Description, and Commits at narrow and
wide widths. Human terminal usability verification remains outstanding.
