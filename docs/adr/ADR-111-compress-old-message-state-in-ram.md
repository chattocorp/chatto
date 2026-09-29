# ADR-111: Compress Old Message State in RAM

**Date:** 2026-09-29

**Status:** Accepted. Amends [ADR-110](ADR-110-share-process-local-event-id-interning.md).

## Context

Each Chatto replica keeps the Server Content View and the Notification
Decisions projection in RAM. ADR-090 removed message payloads from RAM, and
ADR-110 made the remaining per-message state compact. The state still grows
linearly with the message history: on a copy of the largest production server,
the two projections retain about 600 bytes for each message. A server with one
million messages needs about 600 MB for each replica.

Most of this state describes old messages that nobody uses. A measurement on
the same copy (211 days, 38,653 posts) examined the 44,736 events that refer to
an existing message: replies, thread replies, echoes, reactions, edits,
retractions, pins, and thread follows. It compared the age of the target
message with the time of the referring event:

| Target age | Share of references |
| ---------- | ------------------- |
| < 1 day    | 96.8%               |
| 1–7 days   | 2.6%                |
| 7–30 days  | 0.5%                |
| 30–90 days | 0.1%                |
| ≥ 90 days  | 0.0%                |

At the end of the history, 83% of the posts were older than 30 days.

Most per-message records are rows of small integers: stream sequences, Unix
times, and `uint32` handles. In the rows of old messages, most values are
similar to the values of adjacent rows, so the rows compress well.

## Decision

Chatto keeps the per-message rows of old messages in compact, read-only blocks
in RAM. The compression is transparent: projection APIs, read results,
snapshot formats, and snapshot contracts do not change. Chatto does not read
EVT for this purpose and does not change the projector framework.

### Cold window

- A message is cold when its creation time is older than the cold window,
  measured from the creation time of the newest applied event. Wall-clock time
  does not affect it, so replays and restores at one applied sequence hold the
  same cold set.
- The default window is 30 days. Operators set it with
  `core.projection_cold_after`. The value `0` keeps all state uncompressed.
  Because results do not change, a change of the window needs no replay.

### Frozen blocks

- A cold slice is a dense slice whose leading rows can be frozen. Freezing
  packs 256 rows into one block. The block stores each column as offsets from
  the smallest value of the column, with the smallest bit width that holds the
  largest offset. A column with one value in all rows uses no bits.
- A read of a frozen row decodes it in place, without an allocation. A write to
  a frozen row stores the new value in a sparse overlay map, because blocks
  never change.
- Commit freezes complete blocks whose rows are all cold. Each apply freezes at
  most the blocks that became cold, so no event pays for a complete sweep. A
  restore builds the uncompressed state and then freezes it.

These structures use cold slices:

- Room Timeline rows, the row index by event ID handle, and body states;
- Threads message references;
- Reactions message rooms;
- Notification Decisions Badge message records.

### Cold boundary

The Room Timeline decides which messages are cold. It publishes a handle
boundary: all event ID handles below the boundary belong to cold rows or to
events without a row. Threads, Reactions, and the event ID table freeze their
handle-indexed state below this boundary. The Badge index has its own replay
frontier, so it applies the cold window to its own records.

### Event ID table

The event ID table of ADR-110 freezes location pages below the cold boundary.
It copies the IDs of a frozen page into one cold page and adds their hashes to
a sorted cold index. It then removes their hot hash entries and releases arena
chunks that hold only frozen IDs. Handles stay valid, and a lookup or ID read
returns the same result as before. A read of an ID from a handle still takes
no lock.

## Alternatives

Proposal: keep a small skeleton for old messages in RAM and read all other
facts from EVT on demand. This saves more RAM. It needs a preload step in the
shared projector framework, changed snapshot contracts, EVT reads during
apply and during reads, and new failure modes for missing records. The
compression in RAM has none of these costs and can be reverted with a
setting. Chatto can add the EVT approach later if RAM growth is still a
problem.

## Consequences

Measurements on the copy (134,671 EVT records, 38,653 posts) with the
Server Content View and the Notification Decisions projection:

| Window   | Retained heap | Change |
| -------- | ------------- | ------ |
| 0 (off)  | 17.8 MB       | —      |
| 30 days  | 14.2 MB       | −20%   |
| all rows | 13.2 MB       | −26%   |

- Snapshots and read results are byte-identical with and without compression.
  A differential check with windows of 0, 30 days, and one nanosecond
  compared all snapshots and a digest of all read results with the previous
  version.
- A complete replay of the copy takes about 6% longer with a 30-day window.
- Reads of cold messages are two to three times slower. A lookup of one cold
  message takes about 0.9 µs instead of 0.3 µs. A page of 50 old timeline
  entries takes about 0.3 ms instead of 0.13 ms. Reads of hot messages do not
  change.
- The state of old messages still grows linearly, but slower. Other
  structures now use most of the remaining RAM: thread indexes and summaries,
  follow state, active reaction lists, Badge room lists, and the arena of body
  event IDs. Chatto can convert them later with the same cold slice.
- The event ID table is no longer purely append-only storage. Handle
  assignment stays append-only.
- `CHATTO_TEST_COLD_STORAGE=1` runs the core package tests with two-row
  blocks and a one-nanosecond window, so almost all rows are frozen. Changes to
  a compressed structure must pass the tests in this mode.
- Rollback needs no replay, because snapshot formats do not change.

## Related

- [ADR-088](ADR-088-componentized-projections-behind-one-apply-barrier.md)
- [ADR-089](ADR-089-server-content-view.md)
- [ADR-090](ADR-090-hydrate-room-timeline-payloads-from-evt.md)
- [ADR-109](ADR-109-compute-badge-attention-from-projections.md)
- [ADR-110](ADR-110-share-process-local-event-id-interning.md)
