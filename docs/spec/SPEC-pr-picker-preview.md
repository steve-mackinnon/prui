# PR picker preview

The PR picker uses a 40/60 list and preview split at widths of 100 columns or
more. Narrower terminals stack the list over the preview. At tiny heights,
selection takes precedence over the preview. The footer reserves the last row;
only a one-row terminal omits it to keep the selected PR visible.

Selected overflowing list titles scroll horizontally with pauses at either end;
PR numbers, usernames, check marks, and other rows remain stationary. Selection,
resize, loading, and leaving the picker invalidate queued animation ticks.
Usernames use the existing hunk accent; passed, failed, pending, and unknown check
marks use added, removed, warning, and metadata accents respectively. Selection
uses a background highlight so the inline foreground accents remain visible.

The selected PR's full title, author, opened date, latest commit author, check
status, viewer review, and safely rendered Markdown description appear in the
preview. Descriptions arrive in the existing bounded, cancellable GraphQL list
request (`body`), with no requests on selection, no polling, and no persistence.
Null or absent bodies display `No description provided.` The existing aggregate
response limit stays at 1 MiB; oversized lists fail explicitly. Opening a PR
still captures an independent frozen description for the review session.

Tab switches focus between list and preview; j/k and arrow keys navigate the
focused panel. Shift+J/K scroll the preview by the existing diff step from either
panel without changing focus or selection. Page Up/Down and Home/End scroll the preview. Escape returns from
preview to list before leaving the picker. Enter retains explicit review opening.
Selection changes reset preview scroll. Resize clamps scroll and rewraps Markdown;
theme changes invalidate the preview rendering cache. Mouse clicks select/focus;
wheel input moves the panel beneath the pointer. Switcher filtering remains
available, and known PRs use the same preview.

Verification includes source description parsing, viewport bounds from 24 to 180
columns and 1 to 24 rows, footer placement, preview scrolling, selection reset,
mouse routing, and a Bubble Tea event-loop scenario covering focus and resize.
Human terminal verification is still needed to assess usability.
