# Overview conversation implementation

Approved scope: [Overview conversation spec](../docs/spec/SPEC-overview-conversation.md).
The user approved the spec and requested implementation directly on 2026-10-05.

1. Integrate activity into Overview with semantic keyboard/mouse selection,
   direct D access, scroll restoration, and existing editor routing.
2. Render exact captured code before inline bodies, with historical fallbacks;
   navigate by comment ID into Files and restore prior navigation on Escape.
3. Verify extended targets, unavailable states, refresh identity, editor ownership,
   real Bubble Tea journeys, and wide/narrow screens; update reviewer documentation.

Validation: focused Go tests, screen baseline review, then `./scripts/verify.sh`
for formatting, vet, race-enabled full tests, and build. Git fixture tests require
host access because sandboxed Git cannot read their temporary repositories.
