# Diff text search implementation plan

User authorized proceeding on 2026-10-04; the previous specification review gate is closed.

Implement in dependency order: frozen source match index → scoped query state and input → result activation → display spans and popover → regression verification.

Keep matches separate from comment targets. Carry unit/patch-row identity and escaped-source offsets through wrapping and split cells, then apply highlighting only at the final rendering boundary. Use cancellable query commands with snapshot/scope generations, a 10,000-result cap, and a per-review immutable source index. Guide membership comes from the selected section's unit IDs. No new dependencies or persistence.

Risks: byte/rune/cell coordinates differ; split context appears twice; guide sections repeat files; input must not leak to posting commands. Focused tests precede each behavioral increment; full race/PTY/build checks conclude the work. Existing source provenance, filename filter, and comment behavior remain authoritative.

Tasks: [diff-text-search-todo.md](diff-text-search-todo.md).
