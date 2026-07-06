# ADR-038: Room-Owned Thread State

**Date:** 2026-06-05

## Context

Chatto's event-sourced message model currently treats threads as a derived view
over room message events:

- root messages are `MessagePostedEvent` facts on
  `evt.room.{roomId}.message_posted`
  with no `in_thread`;
- thread replies are `MessagePostedEvent` facts on the same room aggregate and
  subject lane, with `in_thread` set to the root message event ID;
- reply attribution (`in_reply_to`) remains independent from thread containment;
- per-user read markers and thread follows live in `RUNTIME_STATE`.

This keeps message storage simple, but upcoming thread-shaped features need
durable state beyond "which replies belong under this root": forum-mode rooms,
thread/message labels, closed threads, and similar room-local conversation
metadata.

One possible direction is a separate thread aggregate:
`evt.thread.{threadRootEventId}.*`. That gives each thread its own consistency
boundary, but it makes messages harder to reason about. A root message would
belong to the room aggregate while replies, edits, retractions, and thread
lifecycle facts might belong to a thread aggregate. Every message mutation path
would then need to know which aggregate owns the target message.

Threads are room-local product concepts. They are rooted in room messages, use
room membership for read authorization, and are displayed as part of the room's
conversation surfaces.

## Decision

Keep thread state on the room aggregate.

Thread replies remain ordinary `MessagePostedEvent` facts on
`evt.room.{roomId}.message_posted` with `in_thread` set to the root event ID.
Thread lifecycle and metadata facts also use room-owned event-type lanes:

```text
evt.room.{roomId}.thread_created        // ThreadCreatedEvent
evt.room.{roomId}.thread_closed         // ThreadClosedEvent
evt.room.{roomId}.thread_reopened       // ThreadReopenedEvent
evt.room.{roomId}.thread_label_added    // ThreadLabelAddedEvent
evt.room.{roomId}.thread_label_removed  // ThreadLabelRemovedEvent
```

Those payloads carry `room_id` and `thread_root_event_id`. The thread root
event ID is the durable identity of the thread.

Use the top-level `Event` oneof range 425-449 for durable thread events. This
keeps breathing room between message events (400-424) and asset events
(450-474) while preserving the existing asset boundary at 450.

Create a `ThreadCreatedEvent` whenever a thread is created. In today's product
shape, a thread is created when the first reply is posted to a root message. The
write path appends `ThreadCreatedEvent` before that first reply in the same
atomic batch so projections never observe a first reply without the explicit
thread lifecycle fact.

Legacy message import does the same for fresh imports: when it imports a thread
reply, it emits `ThreadCreatedEvent` before the first imported reply for that
root.

The room aggregate is the consistency boundary for room-local conversation
state. Commands that post a thread reply, close a thread, reopen a thread, or
change thread labels read the room/thread projection, then append to the room
aggregate using optimistic concurrency over the room event filter
`evt.room.{roomId}.>`. For example, posting a reply must reject a closed thread
before appending its `MessagePostedEvent`.

Keep the existing event-type suffix in NATS subjects. The suffix is an
intentional storage and replay lane, not accidental duplication of the
protobuf oneof. Projections that need only selected event types can subscribe
to subjects like `evt.room.*.thread_created` or
`evt.room.*.message_posted`; projections that need the whole room history can
still consume `evt.room.{roomId}.>` or `evt.room.>`.

Thread projections are responsible for deriving richer thread state from the
room event history: root message, replies, participants, reply count, latest
activity, labels, closed/open state, and future forum-mode ordering metadata.

Per-user thread runtime state stays in `RUNTIME_STATE`: read cursors, follows,
and pending notifications are latest-value viewer state, not reconstructable
conversation history.

Do not introduce `evt.thread.{threadRootEventId}.*` unless a later ADR identifies
a concrete need, such as unacceptable room-level write contention or thread
lifecycle semantics that no longer fit a room-owned consistency boundary.

## Consequences

- Message storage stays uniform: all messages in a room, including thread
  replies, are room aggregate facts.
- Thread lifecycle is explicit, so future facts like close/reopen/labels do not
  appear without a corresponding creation fact.
- Event-type subject lanes continue to provide server-side filtering and a
  natural place for future content-bearing or high-volume room facts.
- Message edit/retract paths do not need an extra branch to discover whether a
  message is owned by a room aggregate or a thread aggregate.
- Closing a thread and posting a reply are serialized by the same room aggregate
  history filter, so "no replies after close" is enforceable with the existing
  OCC discipline even though events use event-type subject lanes.
- Forum mode can add room configuration and richer thread projections without
  changing where thread replies are written.
- The room aggregate remains the hot write subject for all activity inside a
  room. This is acceptable for Chatto's current deployment scale and simpler
  than splitting threads prematurely.
- A future split into thread aggregates remains possible, but it would be a
  deliberate migration of the room-owned thread event model rather than an
  accidental mixed ownership model.
