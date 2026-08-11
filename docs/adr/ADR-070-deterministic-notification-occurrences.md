# ADR-070: Derive Deterministic Notification Occurrences into Runtime State

**Date:** 2026-08-10

## Context

Chatto currently creates recipient-specific notification records directly from
message-posting request paths. The records use random IDs, several independent
fanout paths can match the same recipient, and creation is not recovered after
a crash. Read-cursor advancement deletes whatever records happen to exist at
that moment. A delayed creator can therefore add a notification after the user
has already read the message, producing a phantom badge.

The records also mix several concerns. A notification's type stands in for why
it was created, its existence stands in for unread state, deletion stands in
for dismissal, and Web Push is launched from the same best-effort request
callback. This makes it difficult to add independent preferences for direct
mentions, role mentions, `@here`, `@all`, replies, followed conversations, and
reactions without introducing more fanout paths and more races.

The source activity and notification preferences are durable domain facts, but
a recipient's notification inbox is bounded, mutable, user-runtime state. We
need a design that preserves that boundary while making derivation recoverable,
idempotent, and safe across replicas.

## Decision

### Authority boundaries

Notification source activity and user notification preferences remain durable
facts in `EVT`. A recipient-specific **notification occurrence** is a bounded
latest-value record in `RUNTIME_STATE`; it is not appended to `EVT` and is not
a second copy of the source content.

Per-cause policy changes use one `UserNotificationPreferenceChangedEvent` for
both server and room scope and for setting or clearing an override. The legacy
notification-level event variants remain unchanged only as replay-decodable
history: their tags and payloads are already persisted, but current projections
ignore them. New writes use only the per-cause event. Removing or reusing those
persisted oneof fields would corrupt EVT decoding.

Each occurrence has one deterministic identity derived from the recipient ID
and the canonical source event ID. One source event can therefore create at
most one occurrence for a recipient, even when several notification reasons
match or several replicas attempt the work. Creation uses KV `Create`; later
state transitions use revision-based `Update` with conflict retries.

The new key family is versioned separately from the legacy random-ID records.
Its concrete prefix is an implementation detail, but it must preserve efficient
recipient-scoped watching and must not place user-controlled text in keys.

### Recoverable derivation from runtime work

The source command evaluates notification policy against its authoritative
projections and prepares the exact recipient/reason/intensity occurrences in
`RUNTIME_STATE` before appending the existing message or reaction fact. These
short-lived work keys use the triggering event ID and recipient ID; their value
is the prepared `NotificationOccurrence`, not a second domain-event schema. A
companion marker at the triggering event ID lets consumers reject events with
no prepared recipients using one direct KV lookup rather than a filtered scan.
Message and reaction payloads remain unchanged, and notification
materialization does not introduce an `EVT` aggregate or event type.

Notification evaluation is part of committing the source command. If
recipient discovery, policy evaluation, or runtime-work preparation cannot
complete, the command fails before appending the source fact; it must not
commit an ambiguous "nobody" decision. A failed source append can leave an
untriggered work key, but its absolute source-time-plus-90-days TTL bounds that
orphan. Once the source fact commits, occurrence materialization and delivery
are recoverable effects and cannot roll back the source action.

An OCC command retry replaces the complete prepared recipient set for its
trigger. It deletes recipients retained from an earlier failed attempt and
removes the marker when the new authoritative decision has no work. Prepared
work is therefore a latest exact decision, not an append-only union of attempts.

All replicas share one durable JetStream pull consumer with one globally
in-flight delivery over the existing
`MessagePosted`, `ReactionAdded`, `ReactionRemoved`, retraction, membership,
room-deletion, and account-deletion facts. A delivery waits for the projections
needed by that fact, checks the marker, loads recipient work by the triggering
event ID, applies it idempotently, deletes completed work and its marker, and
acknowledges only after the effect succeeds. The consumer begins at its initial
creation boundary because older facts cannot have Notifications 2.0 work;
server boot readiness waits for that consumer to exist before commands may be
served. The worker also skips facts beyond the 90-day retention boundary
without touching KV.
Crashes, shutdown, and transient failures therefore cause redelivery through
the shared `events.DurableWorker` framework without replaying unrelated message
history at rollout. The single consumer lane preserves source-before-lifecycle
order across replicas.

The committing request also makes one prompt materialization attempt for low
latency and passes the committed EVT stream sequence into the same causal
checks used by the worker. More than one replica may overlap the same work;
deterministic recipient/source occurrence identity and KV OCC make that overlap
safe. Delayed creation additionally checks current account existence, room
membership, message retraction, exact reaction-add state, and the recipient's
latest room-visibility-loss sequence before writing. A leave or removal records
that 90-day runtime boundary immediately after commit, and the durable worker
repeats the write during ordered recovery.

Prepared work contains enough immutable provenance to reproduce the recipient
and reason decision without later policy evaluation. In particular, message
work distinguishes direct-user, role, `@here`, and `@all` matches instead of
relying on the message event's combined mentioned-user list. Eligibility that
depends on transient state, such as who counted as present for `@here`, is
resolved before source commit and retained in the prepared occurrence.
Reaction occurrences retain the exact emoji as internal provenance, allowing
an existing `ReactionRemoved` fact from an older replica to remove precisely
the corresponding v2 occurrence even though that writer prepared no v2 work.

The evaluator gathers every matching reason once, evaluates each reason's
effective delivery intensity, stores the complete matched-reason set, and
selects the strongest intensity. `Off` creates no visible occurrence; `Badge`
and `Alert` create the same durable occurrence, while only `Alert` is eligible
for interruptive delivery.

### Occurrence contents

An occurrence retains the stable facts needed to explain, reconcile, and open
it:

- recipient, canonical source event ID, actor ID, source time, and an internal
  EVT stream sequence used only for causal cleanup;
- exact destination: room, optional thread root, and target event;
- all matched reasons and their evaluated intensities;
- strongest effective intensity and policy-evaluation time;
- inbox state, alert-delivery state, lifecycle timestamps, and absolute expiry
  time.

It does not copy message bodies, room names, avatars, display names, or other
presentation data. Public assemblers hydrate current visible resources from
their authoritative projections. If the target is retracted or the recipient
loses visibility, the occurrence cannot preserve stale copied content.

### Read-state and lifecycle convergence

Inbox state is distinct from room and thread read cursors. When an occurrence
is first derived, the notification subsystem compares its exact target with the
authoritative read cursor. Covered activity starts as read; newer activity
starts as unread. Read-cursor advancement also transitions covered existing
occurrences from unread to read. Creation waits for the occurrence index and
then checks the cursor again; once that check begins, a later cursor advance
must see the indexed occurrence. Both orders therefore converge without
deleting notification history. A reaction remains new when it arrives, but a
later room/thread read covers it according to the reacted-to message's
timestamp rather than the reaction's source time.

User triage mutations, read reconciliation, retraction, reaction removal, and
visibility changes all use KV OCC. Retraction, lost visibility, explicit
deletion, and other conditions that must prevent rediscovery replace the
visible record with a minimal tombstone. The tombstone keeps recipient, source
identity, removal reason, and expiry only, so replay cannot recreate the
notification and inaccessible presentation references are removed. Account
deletion repeatedly purges the recipient's records through OCC races until no
keys remain, and replay skips work recipients whose account no longer exists.
A room-leave or member-removal fact removes only occurrences whose source EVT
sequence precedes that lifecycle fact. Materialization requires the committed
source sequence and rejects work at or before the latest persisted visibility
boundary, even if the recipient has since rejoined. Replaying an old leave
cannot delete activity created after a later rejoin, regardless of replica
clock skew.

Notification policy changes affect future source activity. They do not rewrite
or erase existing inbox history; users triage existing items explicitly.

### Absolute retention

Every occurrence and tombstone has an absolute expiry 90 days after its source
activity. Every KV mutation applies only the remaining lifetime. Marking an
item read or unread, moving it to Done, or rewriting it as a
tombstone never restarts the 90-day clock.

### Authoritative reads and delivery

Each Chatto process owns one filtered `RUNTIME_STATE` watcher and an in-memory
notification index. The watcher's initial latest-value delivery is a startup
readiness barrier. KV remains authoritative; successful writes wait for their
revision to reach the local index when read-your-writes matters. Public list,
count, pending-Alert claims, and realtime replacement assembly use dedicated
index views instead of scanning all retained records or a KV prefix per request,
connection, or delivery poll. Index reads also prune records whose
absolute expiry has passed, so a delayed or missing KV expiry notification
cannot leave an occurrence visible in a long-running process.

Realtime messages remain convergence accelerators. A transition signal carries
the source identity and written KV revision internally; the serving replica
waits until its local notification index has observed that revision before it
assembles the authoritative replacement. Initial connection, reconnect, and
authorization changes publish the same finite replacement. Missing a transient
signal cannot permanently corrupt counts, and a cursor cannot advance with a
replacement assembled from stale local notification state.

Sound, Web Push, native notifications, and installed-app badges are downstream
presentations of a committed occurrence. Interruptive delivery begins only
after the occurrence exists and is still eligible. A revision-claimed delivery
state lets another worker recover an abandoned attempt without sending from two
replicas concurrently. Current transient conditions such as Do Not Disturb may
silence delivery without suppressing the occurrence. Effect delivery is
retryable and at least once; provider-level deduplication is used where
available, but a crash after provider acceptance may produce a duplicate alert.
Marking an occurrence Read or Done silences any pending or claimed Alert.
Workers verify the exact claim immediately before delivery and revalidate
current target visibility before hydration and again before sending. Failed
delivery remains claimed until a bounded retry delay, avoiding a hot loop. The
worker renews the exact claim for a delivery-sized interval immediately before
calling the provider. Delivery completes once any current device accepts the
push; it retries only when no device accepted and at least one current endpoint
failed transiently. This occurrence-level success rule avoids repeatedly
alerting successful devices because another endpoint is persistently broken.
A crash after provider acceptance but before claim completion can still cause
a duplicate alert, consistent with the at-least-once contract.

### Compatibility and rollout

Notifications 2.0 uses a new persisted occurrence protobuf rather than
changing the immutable `chatto.core.v1.Notification` storage message. Existing
legacy notification records are neither migrated nor read. The cutover starts
with an empty 2.0 inbox; old rows remain inert until their retention removes
them. This is an intentional pre-1.0 product reset, not a dual-store period.

The public `chatto.api.v1` notification and coarse-preference RPCs are removed
at the same release boundary and replaced by the grouped inbox, occurrence,
group, and per-cause policy operations. The bundled client contains no fallback
to the old API or preference levels. Older clients are therefore incompatible
with an upgraded server for notifications, and the 0.5 client requires a 0.5
server through the normal feature-version gate. This intentional breaking
change avoids maintaining a second API, compatibility projection, or preset
translation layer.

Every upgraded replica writes and reads only the 2.0 key and work families. A rolling
deployment can therefore briefly contain an older replica that still writes
legacy records and a newer replica that writes 2.0 records, but neither family
is translated into the other. Once all replicas are upgraded, only 2.0 records
are produced. Rolling back restores the legacy implementation and its old
inbox view; 2.0 occurrences and temporary work remain isolated and are not
interpreted by that binary. No notification-only `EVT` variants are added, and
message and reaction protobufs retain their existing wire shape.

## Consequences

- Source commands fail before commit when notification policy cannot be
  evaluated or their temporary work cannot be prepared. Failed source appends
  can leave bounded work orphans; after commit, durable delivery makes
  notification processing recoverable without adding domain events.
- Recipient/source identity, KV OCC, and tombstones make retries and
  multi-replica races idempotent.
- Notification creation and read advancement converge in either order, closing
  the late-notification race.
- Exact targets and durable reason provenance support richer policy and reliable
  navigation without copying mutable or private presentation data.
- A process-wide index makes list, count, and realtime replacement assembly
  proportional to the user's result set rather than repeated KV scans.
- The notification materializer needs explicit consumer-lag, retry, and health
  observability. Work older than the 90-day occurrence lifetime is deliberately
  allowed to expire rather than create an already-expired inbox item.
- Interruptive effects are recoverable but not exactly once; rare duplicate
  provider delivery remains possible.
- The clean cutover deliberately discards legacy pending-notification history
  from the new inbox and avoids a dual-read migration path.
