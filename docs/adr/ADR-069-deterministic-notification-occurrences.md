# ADR-069: Derive Deterministic Notification Occurrences into Runtime State

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

Each occurrence has one deterministic identity derived from the recipient ID
and the canonical source event ID. One source event can therefore create at
most one occurrence for a recipient, even when several notification reasons
match or several replicas attempt the work. Creation uses KV `Create`; later
state transitions use revision-based `Update` with conflict retries.

The new key family is versioned separately from the legacy random-ID records.
Its concrete prefix is an implementation detail, but it must preserve efficient
recipient-scoped watching and must not place user-controlled text in keys.

### Recoverable derivation from source-bound decisions

The source command evaluates notification policy against its authoritative
projections and writes the complete recipient/reason/intensity decision into
the same durable `EVT` fact as the source activity. This boundary is important:
the asynchronous materializer never re-evaluates a later preference, follow,
membership, or presence state for an older activity.

Source-bound decision evaluation is part of committing the source command. If
recipient discovery or policy evaluation cannot complete, the command fails
before appending the source fact; it must not commit an ambiguous "nobody"
decision. Once the source fact is committed, occurrence materialization and
delivery are recoverable best-effort effects and cannot roll back the source
action.

A domain-owned ordered incremental effect consumer discovers
notification-relevant facts in stream order and retries failed idempotent
effects. Its cursor and failed-work queue are process-local; a new process
safely replays matching history from the beginning. A failed effect blocks
later notification effects on that process until it succeeds, preserving
causal order when a later retraction, reaction removal, membership loss, or
account deletion supersedes earlier creation. Prompt request-path attempts
still provide low latency for newly committed sources.

After committing a message or another source fact, its request path makes one
prompt best-effort materialization attempt for low latency. Failure does not
roll back the already committed source action; the background consumer
rediscovers the work after crashes and replica turnover. More than one replica
may replay or briefly overlap the same work. Deterministic KV creation makes
that overlap safe, and only the replica that establishes or successfully
claims the occurrence may initiate its alert delivery.

Source facts must contain enough immutable provenance to reproduce the
recipient and reason decision without later policy evaluation. In particular, a message event must distinguish
direct-user, role, `@here`, and `@all` matches instead of exposing only one
combined mentioned-user list. Any eligibility that depends on transient state,
such as who counted as present for `@here`, is resolved when the source fact is
created and retained as durable provenance. Ordered preference, membership,
and thread-follow facts may be applied by the notification subsystem as it
advances through the stream.

The evaluator gathers every matching reason once, evaluates each reason's
effective delivery intensity, stores the complete matched-reason set, and
selects the strongest intensity. `Off` creates no visible occurrence; `Badge`
and `Alert` create the same durable occurrence, while only `Alert` is eligible
for interruptive delivery.

### Occurrence contents

An occurrence retains the stable facts needed to explain, reconcile, and open
it:

- recipient, canonical source event ID, source kind, actor ID, and source time;
- exact destination: room, optional thread root, and target event;
- all matched reasons and their evaluated intensities;
- strongest effective intensity and policy-evaluation time;
- inbox state, saved state, alert-delivery state, lifecycle timestamps, and
  absolute expiry time.

It does not copy message bodies, room names, avatars, display names, or other
presentation data. Public assemblers hydrate current visible resources from
their authoritative projections. If the target is retracted or the recipient
loses visibility, the occurrence cannot preserve stale copied content.

### Read-state and lifecycle convergence

Inbox state is distinct from room and thread read cursors. When an occurrence
is first derived, the notification subsystem compares its exact target with the
authoritative read cursor. Covered activity starts as read; newer activity
starts as unread. Read-cursor advancement also transitions covered existing
occurrences from unread to read. Both orders therefore converge without
deleting notification history.

User triage mutations, read reconciliation, retraction, reaction removal, and
visibility changes all use KV OCC. Retraction, lost visibility, explicit
deletion, and other conditions that must prevent rediscovery replace the
visible record with a minimal tombstone. The tombstone keeps recipient, source
identity, removal reason, and expiry only, so replay cannot recreate the
notification and inaccessible presentation references are removed. Account
deletion purges the recipient's records, and replay skips candidates whose
recipient account no longer exists. A room-leave or member-removal fact removes
only occurrences whose source time is at or before that lifecycle fact; replay
of an old leave therefore cannot delete activity created after a later rejoin.

Notification policy changes affect future source activity. They do not rewrite
or erase existing inbox history; users triage existing items explicitly.

### Absolute retention

Every occurrence and tombstone has an absolute expiry 90 days after its source
activity. Every KV mutation applies only the remaining lifetime. Marking an
item read or unread, saving it, moving it to Done, or rewriting it as a
tombstone never restarts the 90-day clock.

### Authoritative reads and delivery

Each Chatto process owns one filtered `RUNTIME_STATE` watcher and an in-memory
notification index. The watcher's initial latest-value delivery is a startup
readiness barrier. KV remains authoritative; successful writes wait for their
revision to reach the local index when read-your-writes matters. Public list,
count, and realtime replacement assembly use the index instead of scanning a
KV prefix per request or connection. Index reads also prune records whose
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

Notifications 2.0 uses a new additive persisted protobuf rather than breaking
the existing `chatto.core.v1.Notification` storage contract. Existing legacy
notification records are neither migrated nor read by Notifications 2.0. The
cutover starts with an empty 2.0 inbox; legacy records remain inert until their
existing retention removes them. This is an intentional pre-1.0 product reset,
not a period in which two notification stores remain authoritative.

The public
`chatto.api.v1` surface grows additive inbox resources and mutations. The old
notification RPCs remain mounted for wire compatibility but do not translate
or expose 2.0 occurrences. The old coarse preference API remains as a cheap
deprecated preset layer: Muted resolves every cause Off, Normal selects product
defaults, and All Messages selects product defaults plus followed-room Alert;
an explicit 2.0 cause value at the same scope wins. The bundled client switches
to the new resources as one release boundary. An older client connected to an
upgraded server therefore sees an empty legacy notification centre, and a new
client requires a server version that advertises Notifications 2.0 support.
This explicitly accepts the user's pre-1.0 clean-cutover direction and avoids
a second compatibility projection for notification records.

Every upgraded replica writes and reads only the 2.0 key family. A rolling
deployment can therefore briefly contain an older replica that still writes
legacy records and a newer replica that writes 2.0 records, but neither family
is translated into the other. Once all replicas are upgraded, only 2.0 records
are produced. Rolling back restores the legacy implementation and its old
inbox view; 2.0 state remains isolated and is not interpreted by that binary.

## Consequences

- Source commands fail before commit when their durable notification decision
  cannot be evaluated; after commit, notification processing is recoverable
  and cannot make the source action fail retroactively.
- Recipient/source identity, KV OCC, and tombstones make retries and
  multi-replica races idempotent.
- Notification creation and read advancement converge in either order, closing
  the late-notification race.
- Exact targets and durable reason provenance support richer policy and reliable
  navigation without copying mutable or private presentation data.
- A process-wide index makes list, count, and realtime replacement assembly
  proportional to the user's result set rather than repeated KV scans.
- The notification materializer needs explicit lag, retry-queue, and health
  observability; a persistent failed effect stalls that process's replay, and
  restarts trade a durable checkpoint for safe full ordered replay.
- Interruptive effects are recoverable but not exactly once; rare duplicate
  provider delivery remains possible.
- The clean cutover deliberately discards legacy pending-notification history
  from the new inbox and avoids a dual-read migration path.
