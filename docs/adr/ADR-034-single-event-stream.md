# ADR-034: Single Event Stream, Subject-Per-Aggregate

**Date:** 2026-05-24

## Context

[ADR-033](ADR-033-event-sourced-state-with-projections.md) commits to event sourcing. The next decision is the shape of the event log itself: **one stream containing all events across the deployment**, or **many streams** (per aggregate type, per scope, etc.).

NATS JetStream supports either shape. The tradeoffs:

- **One stream**: a single position to track, one backup target, one replication policy, one stream config to tune. Cross-aggregate retention is uniform. All operational tooling sees one resource.
- **Many streams**: per-type retention and replication factors, bounded blast radius for corruption, independent throughput scaling. Multiplies the operational surface: backup orchestration, consumer fanout, subject-namespace coordination.

A common worry with the single-stream shape is *ordering*: that "per-aggregate ordering" — events for room X are linearly ordered — would somehow require a dedicated stream. It does not. NATS provides per-subject sequence numbers within a single stream. The subject `evt.room.{roomId}` has its own monotonic sequence inside the larger stream, and OCC against `Nats-Expected-Last-Subject-Sequence` operates at that granularity. Per-subject ordering is a stream-level guarantee, not a stream-per-aggregate one.

Cross-aggregate ordering — "did the user join the room before or after sending this message?" — is intentionally not provided. Two events on different subjects have no guaranteed order relative to each other. Projections that need to relate state across aggregates do so through their own bookkeeping (e.g. a `RoomMemberJoined` event carrying a `joined_at` timestamp).

## Decision

Use a single JetStream stream named `EVT` for all event-sourced domain state.

### Subject layout

`evt.{aggregateType}.{aggregateId}.{eventType}`

- **Aggregate types** are stable identifiers like `room`, `user`, `rbac`, `config`. The list grows as ADR-035 migrates aggregates over.
- **Aggregate IDs** are the existing NanoIDs from [ADR-022](ADR-022-nanoid-with-entity-prefixes.md). No renaming required.
- **Event types** are snake_case tokens derived from the protobuf oneof variant on `corev1.Event` (`user_joined`, `room_created`, `config_changed`). The canonical mapping lives in `events.EventTypeOf(*corev1.Event)` — the payload is the single source of truth, the subject token is computed from it, never authored independently.
- **Singleton aggregates** (server-wide config and similar) use a stable sentinel id like `server` rather than introducing a different subject shape. Keeps the parser, the OCC formula, and the framework code uniform.

We deliberately do **not** nest the new event log under `server.>`. The legacy `SERVER_EVENTS` stream already claims `server.>` as its subject root, and NATS won't allow two streams to share an overlapping subject space. The new stream is named simply `EVT`: the word "server" already has a specific product meaning in Chatto (the user-facing concept), and reusing it as a NATS-level prefix on the event log conflated infrastructure naming with domain naming. `EVT` is short, unambiguous, and parallels the `evt.>` subject root.

### Event type in the subject — the rationale

The subject identifies both **the aggregate** and **what happened**. An earlier draft of this ADR put event type in the payload only and treated `evt.room.{R}` as the single subject for every room event. We moved off that shape because the projection-side cost was too high: a projection that only cared about, say, joins and leaves had to receive every `MessagePosted` event for every room and discard it in a Go switch. That ratio gets pathological once messages migrate.

The original three objections to event-type-in-subject and why they don't hold up:

- **"Single source of truth"** — The protobuf oneof is still the only place event type is *defined*. The subject token is *derived* from it via `EventTypeOf`. There is no convention to keep in sync; the framework computes the subject from the payload.
- **"OCC scope is wrong"** — Per-(aggregate, event-type) OCC is the new default. Two different event types on the same aggregate are no longer mutually serialised, which is usually what you want (a message post shouldn't contend with a member join). Cross-event-type invariants — "no joins after delete" and similar — use wildcard-filter OCC via `Aggregate.AllEventsFilter()` (the `Nats-Expected-Last-Subject-Sequence-Subject` JetStream header). The framework exposes both forms; callers pick the OCC scope they need.
- **"Slot creep"** — `{aggregateType}.{aggregateId}.{eventType}` is the cap. Adding more tokens is a deliberate ADR-level decision, not a casual subject change.

Concretely, the gains are:

- **Narrow projection filters.** A `RoomMembership` projection subscribes to `evt.room.*.user_joined` + `evt.room.*.user_left` + `evt.room.*.room_deleted` — server-side filtering, no `MessagePosted` events delivered. Cold-start replay only loads relevant events.
- **Easier ops.** `nats stream view -s 'evt.room.R1.user_joined'` shows only joins for room R1; no in-process tooling needed.

Adding a new event type to an aggregate is still nearly zero-change: add the oneof variant in `proto/`, add a `case` to `EventTypeOf`, add a constant for the token, add a `case` to the relevant projection's `Apply`. Subscriptions filter against constants by name; the framework derives subjects from events. Nothing in the code authors a raw subject string.

### Cascading writes: emit per-aggregate events, don't double-write

When one logical action affects multiple aggregates — e.g. a user deletion that should remove the user from every room they're in — emit **one event per affected aggregate**, each on that aggregate's own subject. Don't publish one "user deleted" event and have a projection derive the per-room consequences from it.

Concretely, deleting user U who is in rooms R₁..Rₙ produces:

- 1 × `UserDeleted` on `evt.user.{U}` — the user aggregate's invariant change.
- N × `UserLeftRoom` on each `evt.room.{Rᵢ}` — each room aggregate's invariant change.

The N+1 events are emitted by the actor code (`DeleteUser` calls into the existing `LeaveRoom` machinery for each affected room), each carries its own OCC, and each appears as a first-class entry in its aggregate's history. This is "Approach A" from the design discussions.

Rationale:

- **Live-event delivery falls out for free.** The RePublish `evt.>` → `live.evt.>` mapping means every per-room event reaches frontend subscribers in that room. With a single "user deleted" event, room subscribers would not see it and would silently miss the membership change unless we built derived live-event machinery.
- **Per-room audit moments.** Each room's history records exactly when each member was removed and by which action. Derivable from a single upstream event is not the same as a recorded fact.
- **Projections stay decoupled.** A projection consuming `evt.room.>` doesn't have to know about user-deletion semantics; it just reacts to membership events. Cross-aggregate coupling lives in actor code, where the cascade *originates*.

When *not* to use per-aggregate fan-out: pure internal-state cleanup that no other consumer subscribes to. Dropping a user's preferences cache when the user is deleted, for example, can be handled by a preferences projection subscribing to `evt.user.>` and reacting to `UserDeleted` — no per-aggregate event needed. The criterion is "does anyone besides this projection care that this individual effect happened?" If yes, emit per-aggregate events; if no, let the projection derive.

### Live delivery

The stream's `RePublish` config forwards every accepted event from `evt.>` to `live.evt.>`. Subscribers do not hold JetStream consumers — they take NATS Core subscriptions on `live.evt.>` and the stream feeds them after persistence. This is the same pattern that today maps `server.>` → `live.server.>` for `SERVER_EVENTS`.

A consequence worth naming: **the event stream is also the live event stream.** There is no separate "live event" path for these aggregates. Today's `live.server.user.{userId}.*` and `live.server.config.*` subjects (direct NATS Core publishes for transient signals — typing, presence, reactions) continue to use their existing path; they are not migrated by ADR-033 and remain ephemeral.

### Replication and retention

- **Replication is stream-level.** R3 applies to the entire event log, not per aggregate type.
- **Retention is forever** for the foreseeable future. Trimming is deliberately deferred; addressed alongside snapshot orchestration in a future ADR.
- **Compression** uses S2, matching today's `SERVER_EVENTS`.

### Coexistence with the legacy stream

During the migration window (ADR-035), the existing `SERVER_EVENTS` stream continues to serve aggregates that have not yet been migrated. The two streams operate side by side until the last aggregate moves off `SERVER_EVENTS`, after which the legacy stream is decommissioned.

## Consequences

- **One stream to back up, replicate, consume.** Operational surface stays small. `chatto backup` and clustering both treat the event log as a single resource. Operator mental model is simpler than "track N streams and reconcile their states."
- **No fanout consumer multiplexing.** A projection that needs events for all rooms takes one consumer with a wildcard filter (`evt.room.>`). The per-process consumer count grows with projections, not aggregates.
- **Subject cardinality is bounded by aggregate count.** Rooms, users, RBAC namespaces — orders of magnitude lower than per-message subjects. This is the property that makes the NATS subject index manageable, and the direct reason ADR-033 unlocks a RAM win.
- **Single point of contention for hot streams.** Writes across all aggregates serialize through one stream leader. For Chatto's scale (one server per deployment, not a multi-tenant SaaS) this is acceptable. If we ever need to scale past a single stream's write throughput, [ADR-013](ADR-013-per-space-stream-sharding.md) shows the codebase can carry a sharding abstraction — that's a future option, not a current need.
- **Wildcard filters become first-class.** A `User.rooms` projection consumes `evt.room.>` and indexes by member; a per-room projection consumes `evt.room.{thisRoom}`. The framework wraps consumer creation around the projection's declared subjects.
- **No cross-aggregate ordering guarantee.** Projections that need to reason across aggregates carry timestamps in their events. This is conventional event sourcing discipline and not unique to our design.
- **Two streams during migration.** `EVT` and `SERVER_EVENTS` coexist. The names are visually similar; ops tooling, log searches, and code review need a bit of care for the duration. Acceptable but not free.

## Out of scope for this ADR

- Retention, archival, and snapshot orchestration — deferred.
- Sharding the event log across multiple streams — not needed today; revisit if write contention forces it.
- Cross-deployment replication / federation — entirely out of scope.
