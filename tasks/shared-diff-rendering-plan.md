# Shared Diff Rendering plan

Spec: [Shared Diff Rendering](../docs/spec/SPEC-shared-diff-rendering.md).

## Execution

Implement in the existing review worktree. Each delegated task has one owner;
serialize edits to shared production files. The coordinator integrates and reviews
the final diff, runs the repository gate, and records evidence in the task list.

1. DR-01 (renderer agent): consolidate source text-hunk rendering for both views.
2. DR-02 (viewport agent, parallel with DR-01): reproduce and fix editor resize.
3. DR-03 (cache agent, after DR-01): split commit source cache from overlays.
4. DR-04 (frame agent, after DR-02/DR-03): share pane-body assembly.
5. DR-05 (coordinator and independent reviewer): integrate, review, verify.

DR-01 owns render.go, style.go if needed, the commitDiffRowsFor portion of
commits.go, and new patch-render tests. DR-02 owns model.go's resize branch and
new resize tests. DR-03 subsequently owns commit cache/rows implementation and
new cache tests. DR-04 subsequently owns frame assembly in model.go/commits.go
and a small new helper. Agents must report required edits outside ownership.

## Decisions and risks

- Share source parsing without constructing a synthetic Session. Keep posting
  eligibility in the existing commit/source validation layers.
- Preserve numbered commit output while retaining raw patch markers internally;
  do not enable side-by-side commits as a side effect.
- Cache only one selected commit's source rows; mutable overlays must not leak
  into retained source or survive editor cancellation.
- A shallow shared body-frame helper is enough. Do not absorb domain-specific
  labels, navigation, or progress into a layout abstraction.
- Existing screenshot and program tests are the compatibility checkpoint after
  renderer/cache changes and again after framing. New regressions prove the
  resize correction and reduced editor allocation growth.

## Completion gate

All task acceptance criteria, focused tests, independent code review, full
scripts/verify.sh, and git diff --check must complete before reporting success.
