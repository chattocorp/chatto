# ADR-111: Keep Old Message State Cold in EVT

**Date:** 2026-09-29

**Status:** Proposed. Amends [ADR-088](ADR-088-componentized-projections-behind-one-apply-barrier.md), [ADR-089](ADR-089-server-content-view.md), and [ADR-090](ADR-090-hydrate-room-timeline-payloads-from-evt.md).

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

Most per-message state is a set of identity facts that never change after the
post: the event ID, stream sequence, room, author, creation time, thread root,
and echo source. EVT keeps these facts permanently, and ADR-090 already reads
EVT records by exact sequence with a process-local cache.

ADR-088 requires that the projector prepares and commits each event under its
apply barrier without external I/O. A projection can therefore not read an old
message from EVT while it applies a new event that refers to that message.

The measurement covers only writes. EVT does not record reads, so the share of
scrollback, permalink, and search reads that reach old messages is not known.

## Decision

Chatto keeps full per-message state in RAM only for messages inside a hot
window. For older messages, the projections keep a small skeleton, and EVT is
the cold tier for all other message facts. Chatto does not add fields to EVT
events for this purpose.

### Hot window

- The hot window is defined in event time: a message is hot while its
  creation time is within the window of the newest applied event. Wall-clock
  time does not affect it. All replicas, replays, and restores at one applied
  sequence therefore hold the same hot set, and the snapshot of a projection
  stays deterministic.
- The Server Content View uses a 30-day window. The Notification Decisions
  Badge index uses its source lifetime of 90 days (ADR-109), because its source
  lists refer to message records for that time.
- A projection can make its window longer, but not shorter than its own
  semantic horizon. The window is configurable. An unlimited window keeps all
  per-message state in RAM, as before this decision, and operators can use it
  to disable the cold tier without a new release.
- Commit evicts the state of messages that leave the window. Eviction is
  incremental, so no single event pays for a complete sweep.

### Cold skeleton

For every message, hot or cold, the projections keep only:

- one global event ID to stream sequence index, using the event ID table of
  ADR-110. A lookup by event ID does not need a room ID;
- the ordered stream sequences of each room timeline and of each thread. Only
  this ordering is room-shaped, because clients page one room timeline at a
  time;
- one flag byte for each timeline entry with the facts that timeline filters
  use under the barrier: event kind, thread reply, echo, historical import,
  and hidden. Readers that can read only their thread interactions also need
  the thread root. A root message is its own thread root, so only echoes keep
  their thread root, in a sparse map; and
- sparse overlays for mutable state that changes after a message becomes cold:
  current body reference, retraction, hidden echo, and pin.

Active reactions, thread summaries, follow state, and thread interactions stay
in RAM, because they are current state and not identity facts.

### Cold reads

A read that reaches cold messages plans under the barrier from the skeleton and
the overlays. It then reads the identity facts from EVT outside the barrier,
together with the payloads that ADR-090 already reads. Identity facts never
change, so the read revalidates only the mutable references, as ADR-090
requires. Cold reads can be slower than hot reads.

A shredded author (ADR-007) makes a cold message a tombstone at read time. The
read compares the author from EVT with the shredded users of the projection. A
shred therefore does not have to find all cold messages of the author.

### Cold references during apply

The projector gets an optional preload step. It calls the step for each event
before it takes the apply barrier. The step can read immutable records, for
example the EVT record of a cold message that the event refers to. It stores
the results in a projection-owned cache that preparation reads. Preparation and
commit keep the ADR-088 rule: they do no external I/O.

The preload step is part of the shared `pkg/events` module. It stays neutral to
envelopes and applications (ADR-056): the framework calls the step and waits
for it, and the application decides what to load. A preload failure fails the
event like a preparation failure.

During replay, most referenced messages are still hot at the time of the
referring event, so replay does few cold reads.

### Snapshots

Snapshots contain the skeleton, the hot state, and the overlays. The component
snapshot contracts change, so the first start after the upgrade cold-replays
the Server Content View and the Notification Decisions projection once.

## Consequences

- RAM for old history drops from about 600 bytes to about 20–25 bytes for each
  message. Total RAM then depends mainly on the activity of the hot windows,
  not on the length of the history. On the measured copy, the average would be
  about 200 bytes for each message: 17% of the posts are inside the 30-day
  window of the Server Content View, and 71% are inside the 90-day window of
  the Badge index.
- Snapshots become smaller and restore faster.
- About 0.1% of the referring events do one EVT read before apply. Cold reads
  of scrollback, permalinks, and search results do EVT reads for identity
  facts. The ADR-090 cache serves repeated reads.
- Secure deletion must keep the body history of a cold message until it
  deletes the obsolete body records.
- The shared framework gets a new optional preload step. It needs its own
  tests and must keep replay order and snapshot semantics.
- The upgrade needs one cold replay of the affected projections. A rollback to
  an earlier binary also needs one cold replay, because it rejects the new
  snapshot contracts. EVT does not change, so both directions are safe.
- The benefit depends on the access pattern. A community that often refers to
  old messages pays more EVT reads. The configurable window limits this cost.

## Measurement

EVT does not record reads, so the share of reads that reach cold messages is
not known before the change. The implementation adds a histogram metric,
`chatto_message_read_age_seconds`, labeled by read surface: room page, jump to
message, thread, search result, and single message. Operators use it to tune
the hot window after deployment.

## Rollout

One change implements the complete decision for all affected projections,
together with the metric and the window setting. It can be reverted with the
window setting or with a normal rollback.

## Related

- [ADR-007](ADR-007-per-user-encryption-with-crypto-shredding.md)
- [ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md)
- [ADR-056](ADR-056-extractable-nats-event-sourcing-framework.md)
- [ADR-088](ADR-088-componentized-projections-behind-one-apply-barrier.md)
- [ADR-089](ADR-089-server-content-view.md)
- [ADR-090](ADR-090-hydrate-room-timeline-payloads-from-evt.md)
- [ADR-109](ADR-109-compute-badge-attention-from-projections.md)
- [ADR-110](ADR-110-share-process-local-event-id-interning.md)
