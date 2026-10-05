# Durable conversation drafts (PR54)

Base: final reviewed PR48 `05bb584f042c173b3a32d8e48ee5386d35f6ec74`, including
actual main `a2d83a0899510d22574b5f72fbfe038d26c400f9`. Replay only focused local
general editor persistence/reconciliation and strengthened regressions. Parent
model/reset/readiness/binding implementations remain unchanged; incremental
review, full-source capture, search cancellation, Alt+R readiness and C filtering
remain intact. No unpublished PR34 changes or parent/main writes are included.

Original published PR54 `9f1302f768ac37b80c4296ed1e0976c4982170e3` is retained in
`codex/backup-conversation-drafts-9f1302f`; publication uses that exact remote
lease. Reviewed hold `5982e7b` is also retained in a backup branch. Root alone
coordinates merge after parent/main validation and CI.

Payload v4 reads versions 1–3 without losing range/file/suggestion or original
reply targets. Store only local general text, rune cursor, reply identity,
immutable attempted body, <=500 observed general IDs, and uncertain/matched
status. Earlier binaries refuse/preserve v4; an isolated proof against the actual
older reader at `1792142` passed without changing its validator.

Save intent before dispatch. Restart recovers interrupted posting as uncertain,
never dispatching. Editing cannot change the original attempt. Pre-attempt,
inline-only, partial/stale/failed/offline reads cannot prove absence. A complete
current-verified bounded conversation read reconciles without writing. Success,
explicit discard and last-session deletion clean up private work. Input limits
are checked before mutation so rejected text/ID counts remain editable and
savable. Fetched timeline/readiness evidence never enters drafts/snapshots/guides.

Two parent memory-only-general assumptions now assert exact durable text/cursor/
reply/attempt/status recovery; original range/review/privacy/no-write checks are
retained. New regressions include real Bubble Tea key/dispatch/SQLite/kill/restart/
edit/original-match/discard, abrupt child exit with combined general/range/
suggestion/reply/summary state, old payloads, CAS refusal, bounds and cleanup.
Comparison reset retains old immutable general/suggestion requests at their
original key and preserves readiness/search cancellation. Incremental outcome
navigation labels private general work without exposing/remapping bodies; old
snapshot access recovers it without retry. Alt+R/C ownership and separate live
readiness are verified with no extra write.

The reset-leak test and three input-limit regressions failed before their fixes;
focused combined TUI/storage/source/application tests pass on the final parent.
Final committed full-gate, exact-SHA independent review and CI evidence is in PR54.
Human terminal accessibility/usability and live GitHub behavior remain unverified.
