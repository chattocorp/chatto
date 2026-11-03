# Gobotto terminal interface

Read `../../AGENTS.md` for Gobotto runtime and verification rules. These rules
preserve the terminal design agreed with the user.

## Layout and navigation

- Keep two full-screen views, selected with `1` (Sessions) and `2` (Brains).
  Sessions has sessions on the left and events above details on the right.
  Brains has collection headers and documents on the left and contents on the
  right. Preserve each view's selection, scrolling, and divider proportions.
  All separators are draggable. Keep both sides usable at their minimum size.
  While dragging, highlight only the moved divider's line and junctions. Keep
  the outer frame and panel title text unchanged.
- Sessions, turns, and events within each turn appear newest first.
- Brain documents are ordered by file modification time, newest first within
  each brain. Sort before paging; use stored IDs to break equal-time ties.
- Turns are permanent group headers. Do not add folding or expansion carets.
- Brain groups can fold: Left in the document panel folds the current brain,
  Right unfolds it, and Enter/Space or a caret click toggles a header. Folding
  from a document selects its brain header. Preserve fold choices on refresh.
  New brains start collapsed. Selection and completed reads never expand them.
- Selecting a session updates events; selecting a turn or event updates details.
  Preserve selection during live updates. Keep startup selection on the newest
  session until the user navigates.
- Keep list rows limited to labels and activity. Put metadata and recorded text
  in details. Do not add dashboards, columns, or separate views without a request.
- Brains is a local operator inspector, not an agent capability. Read collections
  asynchronously and without embeddings, network calls, or writes. Never send
  inspected journals to model context or use the browser to bypass tool scope.
- Page document lists and load bodies on selection. Keep one browser read in
  flight; late responses must not replace the current selection. Refresh must
  discard stale responses and rediscover dynamic app brains. Show safe errors
  and invalid-document counts without copying raw library diagnostics into logs.
- Filesystem notifications invalidate cached brain data without clearing visible
  snapshots or navigation. Reload the selected item and expanded groups through
  the same serialized reader. Keep watcher lifetime tied to the terminal and
  join it on shutdown. Do not add polling reads on the animation clock.

## Rendering contract

- Panel titles (`Sessions`, `Events`, `Details`) are plain border labels. They
  must not have background bands or inline keyboard hints.
- Turn headers, the selected-item heading in details, and detail section headers
  use full-width background bands. Selected list rows also fill the full width.
  Full width means every cell between the panel borders, including side padding.
- Use a two-space horizontal inset for labels and body content. Keep indentation
  already present in recorded code. Reserve the same two cells for an animated
  Braille activity indicator; a label must not move when activity starts or ends.
- Use `layout.go` for terminal-cell width and content padding, `separator.go` for
  background bands, and `activity.go` for activity indicators. Do not add padding
  around a rendered band. Do not apply nested styles that reset its background.
- Row components receive the full panel interior width and return a complete
  row. The outer layout adds borders only. Content padding belongs inside the
  row component, not in each caller.
- Measure terminal cells, not bytes. Truncate list labels; wrap detail content.
  Sanitize recorded text before adding terminal styles. Keep panel titles plain
  even when group headers and selection use background colors.
- Detail metadata uses aligned columns when space permits. Give recorded content
  its own section. Format durations for reading and distinguish missing usage
  from reported zero. Do not treat model output as proof of successful delivery.

## Verification

- Exercise rendering, hit testing, divider dragging, selection preservation, and
  narrow layouts when changing geometry. Rendering and mouse handling must use
  the same geometry functions.
- Check background coverage as well as row width. A correctly sized string can
  still contain unstyled padding or ANSI resets that create background gaps.
- Use synthetic records for visual checks. Include a title with status, metadata,
  a section header, long code, and active/idle rows. Never use private runtime
  content in committed fixtures or screenshots.
- When the user supplies a screenshot, identify the specific row and edges before
  changing code. Do not assume a fix for one header also fixes other header types.
- Tests are not visual confirmation. State whether a change was checked in a
  rendered terminal, a synthetic render, or only automated tests.
