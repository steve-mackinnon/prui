# Inline ask agent UI checklist

- [x] Enter opens the existing inline editor with Comment selected and Ask agent second in Tab order.
- [x] Ask agent mode shows only the model ID in the editor border; Enter sends and Shift+Enter inserts a newline.
- [x] Mode cycling keeps separate drafts; agent text is memory-only and cannot enter GitHub comment drafts or submit paths.
- [x] Sending, failure, retry, cancellation, follow-up, tab/comparison isolation, and citation inspection still work in focused tests.
- [x] Inline Q&A uses the comment card visual language with wide/narrow framed snapshots and unified/split/Files/Guide automated checks; human readability check remains below.
- [x] Follow-up editor renders immediately below its answer card, and focused cards use the solid highlighted border in unified and split layouts.
- [x] Agent card omits side/range and turn-number headings, and questions use an accent bold style distinct from answers.
- [x] Actions/help/reference/PTY and relevant golden screens reflect the integrated route; separate Ask entry is removed.
- [x] Focused and full automated gates pass; separate integrated review findings are fixed.
- [ ] Human terminal review confirms readability and mode routing on a real guided PR.
