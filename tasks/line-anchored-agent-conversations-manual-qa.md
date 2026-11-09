# Human terminal acceptance record: line-anchored agent conversations

This is the remaining acceptance check for the approved spec. Run it in a real
terminal with a configured guide provider and an actual PR that changes source
and has captured test evidence. Do not record or share provider keys, source
excerpts, or raw model requests here. Record observations, including failures,
before checking an item.

PR / session (non-sensitive identifier):

Terminal and size(s):

Provider/model (no key):

Observed 2026-11-04: a question on LEFT line 857 using
`anthropic/claude-haiku-5-5` displayed "Agent request failed; question kept for
retry." The question remained available. The provider's rejection category
was hidden, so the underlying cause is unconfirmed. The follow-on change
shows a privacy-safe HTTP status or output/timeout category; retry in a real
terminal and record the new result before accepting item 1.

Retest 2026-11-04: the same question received an answer. It correctly warned
that removing the call could not be judged safe from the available surrounding
source, but incorrectly claimed the selected `mod.rs` line was absent while
citing optional `params.rs` context. Tests confirm the selected LEFT line was
in the sent `hunk.lines`; the provider prompt now names that hunk as primary,
citable evidence. A live answer retest is still needed to judge the correction.

1. From this checkout, run `go run ./cmd/prui open <PR-URL>`. Generate a Guide
   with `g` and explicit confirmation. In Files or Guide, select a canonical
   hunk line, press Enter to open the inline editor, then Tab once to select
   Ask agent. Confirm the model ID appears in the box's top-right border with
   no scope or instruction rows, then press Enter to send a hunk-only question.
   Confirm the framed answer is
   readable and stays below its selected range while scrolling and resuming
   guided review. Check a narrow terminal, confirm the card border highlights
   when its cursor is inside, and confirm a follow-up editor opens below the
   answer card. Confirm the card omits side/line labels and each Question:
   block is visually distinct from its answer.
   - [ ] Result and observation:
2. Ask about a changed behavior whose captured test evidence is available.
   Open a supplied citation with Enter and confirm it shows the exact sent
   excerpt. Ask a follow-up in the same thread; reopen that anchor and confirm
   both turns remain. Ask at a second anchor and confirm its thread is separate.
   - [ ] Result and observation:
3. Ask which caller invokes a changed function when that caller is absent from
   captured evidence. Confirm the answer marks the evidence unavailable and
   does not silently search the repository or invent a citation.
   - [ ] Result and observation:
4. Check `Ctrl+V` plus movement, forward and reverse source drag, and source
   click then Shift-click on old and new sides, including a wrapped line. Use
   both unified and split layouts. Narrow and resize the terminal, scroll, and
   confirm the inclusive line range and highlight stay on the intended side.
   Invalid cross-boundary input must leave the prior valid selection usable.
   - [ ] Result and observation:
5. Confirm source drag release leaves the clipboard unchanged. Use `Alt+Y` or
   **Copy selected lines** and confirm only the selected source lines are copied.
   Check that prose copy and divider drag still work.
   - [ ] Result and observation:
6. Start a Send and press `X` while it is pending. Confirm the old answer does
   not appear later, the draft can be retried explicitly, and switching PR tabs
   or refreshing the comparison cannot place it in another review. Resume Guide
   navigation and confirm review progress and position remain usable.
   - [ ] Result and observation:

Overall acceptance / defects to fix:
