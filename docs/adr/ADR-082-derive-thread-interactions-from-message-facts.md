# ADR-082: Derive Thread Interactions from Message Facts

**Date:** 2026-08-25

> **Amended 2026-08-28:** ADR-040 now makes each registered dotted prefix a
> broader permission. This replaces the explicit inclusion that this ADR
> originally specified for `message.read` and `message.read.interactions`.

## Context

ADR-080 adds broad channel-room access through `message.read`. Chatto also
needs a narrow mode where a human or bot can read only threads that directly
involve that account.

RBAC decisions currently use server, room-group, or room scope. Writing a
normal RBAC grant for each account and thread would add a high-volume resource
scope to the global RBAC aggregate. It would also duplicate facts that already
exist in the room event log. Each message post records its author, thread root,
and typed mention causes.

The narrow model must give a direct mention enough earlier thread context to
be useful. It must also apply to request reads, realtime delivery, reconnect
replay, search, files, notifications, and other message-derived surfaces.

## Decision

Add `message.read.interactions` as a normal RBAC permission at server,
room-group, and room scope. Keep `message.read` as the broad channel-room read
permission. Keep room membership as a separate required boundary. Keep DM
reads membership-based under ADR-037.

Under ADR-040, registered dotted prefixes define permission inclusion. An
effective `message.read` allow therefore includes
`message.read.interactions`. An allow for `message.read.interactions` does not
include `message.read`. A deny for
`message.read.interactions` cannot restrict an effective `message.read` allow.
A deny for `message.read` does not restrict a separate
`message.read.interactions` allow. The resolver applies the same rules to human
accounts, bot allowlists, bot-owner ceilings, permission matrices, and
permission explanations.

Derive thread interaction relationships in the existing Threads projection.
Do not write RBAC events or another durable grant event when a relationship
starts. The projection uses `MessagePostedEvent` source facts:

- A channel-room root author gets an authored-root relationship.
- A different account named by a typed direct mention gets a direct-mention
  relationship with that message's thread.
- A legacy flattened mention recipient, self-mention, role mention, `@all`,
  `@here`, and authored reply do not create a relationship.

A relationship names one account, room, and thread-root event ID. It includes
the source event ID, source time, and cause. It gives access to the complete
thread while current membership and `message.read.interactions` allow access.
Broad `message.read` continues to allow every thread in the room.

Keep relationships after message edits and retractions. This slice does not
write an end fact. Permission loss or membership loss closes current access;
restoration opens the derived relationship again.

Extend the Threads snapshot with the message-to-thread index and interaction
causes. The snapshot remains an encrypted, disposable acceleration artifact.
Its protobuf schema fingerprint selects a new contract namespace. Cold replay
from EVT remains authoritative.

Make every message post a Threads-projection readiness dependency. Realtime
authorization must wait until the projection applies the source message that
can create access. A reaction, pin, or asset lifecycle fact waits for the same
projection at its source message before authorization. Current RBAC and
membership checks remain request-time decisions.

Do not add public operations that list or inspect relationships. Existing room
and thread APIs apply the derived relationship when they authorize known
message and thread IDs. Keep cause metadata inside the projection. Bots learn
known message and thread IDs from their normal notification occurrences. This
does not add an interaction-specific realtime operation, NATS subject, stream,
KV bucket, or durable worker.

Filter each message-derived surface by the canonical thread root. Main-room
typing has no thread target and therefore requires broad access. Thread typing
uses the thread relationship. A pending asset without a durable message owner
does not qualify for interaction-scoped reads.

Fresh empty-RBAC bootstrap grants only `message.read` to `everyone`. Its
effective allow includes `message.read.interactions`. Do not migrate, backfill,
or reconcile existing RBAC state. Bots do not inherit `everyone`. A bot needs
an explicit read grant, bounded by its owner's effective read authority.

## Consequences

- Operators can grant narrow message access without creating one RBAC object
  for each thread.
- Operators can see the narrower capability in its three-component name and in
  permission explanations. Each future registered descendant is also included
  by its registered dotted ancestors.
- The durable room event log remains the source of relationship truth.
- A direct mention gives access to content that was already in the thread.
- Typed mention provenance is required. Ambiguous legacy mention rows fail
  closed.
- Relationship reads add bounded projection indexes keyed by message, thread,
  room, and account. Projection diagnostics and snapshots must include this
  retained state.
- The authorization boundary is more detailed than one room-level decision.
  List surfaces must filter individual messages or threads.
- Clients cannot enumerate the relationship set. They can load a known thread
  and receive its content only when the current access rules allow it.
- Bots receive direct-mention targets through the normal notification
  occurrence projection. The relationship remains an internal authorization
  input.
- Old replicas do not understand the narrow permission. They deny narrow reads
  when broad `message.read` is absent. A mixed rollout can reduce availability
  but does not give broad message access.
- A future interaction-end feature needs a new durable end fact and must define
  whether permission restoration or room re-entry can reopen an ended
  relationship.
