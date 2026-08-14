# ADR-074: Derive Deterministic Notification Occurrences into Runtime State

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
a recipient's notification list is bounded, mutable, user-runtime state. We
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

Each source-command OCC attempt evaluates notification policy against the
authoritative projections behind that attempt. It captures the current config
subject tail and waits for the local Config projection through that boundary
before preparing the exact recipient/reason/intensity occurrences in
`RUNTIME_STATE` and appending the existing message or reaction fact. A policy
write that completed before the attempt is therefore observed; a policy write
overlapping the source command may order on either side, and every OCC retry
captures the boundary again. Message attempts also recompute mention expansion
and the persisted `mentioned_user_ids`, so a retry ordered after a membership
change cannot commit the earlier recipient set. These
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
room visibility, room-group placement, relevant RBAC, room-deletion, and
account-deletion facts. Verified-email facts are also included so configured
owner identities converge on the durable RBAC state used by notification
visibility. A delivery waits for the projections needed by that
fact, checks the marker, loads recipient work by the triggering event ID,
applies it idempotently, deletes completed work and its marker, and acknowledges
only after the effect succeeds. The consumer begins at its initial
creation boundary because older facts cannot have Notifications 2.0 work;
server boot readiness waits for that consumer to exist before commands may be
served. The worker also skips facts beyond the 90-day retention boundary
without touching KV.
Crashes, shutdown, and transient failures therefore cause redelivery through
the shared `events.DurableWorker` framework without replaying unrelated message
history at rollout. The single consumer lane preserves source-before-lifecycle
order across replicas.

The durable consumer is the sole owner of occurrence creation and lifecycle
cleanup; request handlers do not run an overlapping prompt writer. A committing
request may wait for the consumer's acknowledgement when read-your-writes
matters, but all effects still pass through the shared causal lane. Delayed
creation checks current account existence, room membership, message retraction,
exact reaction-add state, and the recipient's latest room-visibility-loss
sequence before writing. A leave or removal records that 90-day runtime
boundary immediately after commit, and the durable worker repeats the write
during ordered recovery.

Effective membership can also disappear without an explicit leave: a universal
room can be disabled, moved across group permission scopes, or made inaccessible
by a `room.join` RBAC or role change. The same ordered worker consumes those
existing domain facts after a dedicated Notification Visibility projection
captures the minimal room, membership, room-group, and RBAC state at the fact's
exact EVT sequence. The worker scans authoritative occurrences and tombstones
only recipient/room pairs that lacked effective membership at that boundary. A
projection that already observed a later regain therefore cannot erase an
intermediate visibility loss, and activity sourced after the regain is outside
the earlier cleanup boundary. Snapshot restore is capped at the shared worker's
acknowledged floor, so pending boundaries are replayed rather than skipped by a
newer projection snapshot. Pending facts share one full visibility checkpoint
plus a compact event-delta journal and an incrementally evaluated cursor; the
projection does not copy full membership/RBAC state for every boundary or
replay lifetime EVT history on the single notification lane. Boundary data is
released only after the shared consumer's acknowledged floor confirms the
delivery, so a failed acknowledgement can redeliver safely on the same replica.
Snapshot publication uses that full floor too: a capture beyond it is deferred
even when the pending notification-worker delivery is not itself a visibility
boundary. On restart, the safe full-EVT prefix is reconstructed immediately
before the earliest worker-filtered fact after the consumer's sparse AckFloor.
The repository therefore retains a generation that restart can accept and
needs to replay only its tail without another persisted watermark.

Configured `owners.emails` identities are materialized as durable owner-role
assignments at boot and through the same retryable durable lane after email
verification. Verification waits for that source fact to complete when the
email is configured; live authorization recognizes only the durable role.
While a verified email remains configured, that role cannot be revoked. The
event-time RBAC projection and live owner authorization therefore cannot
disagree about room visibility after a transient assignment failure.

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

Visual attention is a separate source-time property. Reaction-only activity is
Ambient; every other current cause is Important, and the strongest matching
cause wins. This classification is deliberately fixed for now. Persisting the
resolved level lets a future preference affect new activity without
reclassifying retained history or coupling visual emphasis to push policy.

### Occurrence contents

An occurrence retains the stable facts needed to explain, reconcile, and open
it:

- recipient, canonical source event ID, actor ID, source time, and an internal
  EVT stream sequence used for causal cleanup and read-boundary reconciliation;
- exact destination: room, optional thread root, and target event;
- all matched reasons and their evaluated intensities;
- strongest effective intensity and policy-evaluation time;
- resolved Ambient or Important visual attention level;
- attention state, alert-delivery state, lifecycle timestamps, and absolute expiry
  time.

It does not copy message bodies, room names, avatars, display names, or other
presentation data. Public assemblers hydrate current visible resources from
their authoritative projections. If the target is retracted or the recipient
loses visibility, the occurrence cannot preserve stale copied content.

### Attention-state and lifecycle convergence

Notification attention state is distinct from room and thread read cursors. A read action also
writes a bounded notification-read record containing the target timeline EVT
sequence and the reaction projection's applied EVT horizon. Occurrence creation
reads that boundary directly from KV after its initial write, while the read
action scans the recipient's authoritative occurrence keys after writing the
boundary. This two-sided handshake converges across replicas without depending
on either process's watcher timing. Coverage uses stream order, not protobuf
wall-clock timestamps. Ordinary activity is covered through the target
sequence; a reaction is covered only when both its reacted-to message and its
source sequence were visible to the later read. A reaction arriving after a
read therefore remains new until another read action.

New occurrence rows are initially persisted in an unfinalized, non-deliverable
state. Finalization applies the read boundary and only then makes an unread
Alert eligible for queueing. A durable redelivery finalizes an interrupted row, but never
reconciles an already-finalized row or turns a Read occurrence back to Unread.

User read/delete mutations, read reconciliation, retraction, reaction removal, and
visibility changes all use KV OCC. Causal cleanup and read reconciliation scan
authoritative KV state rather than a potentially lagging replica-local index.
Retraction, lost visibility, explicit
deletion, and other conditions that must prevent rediscovery replace the
visible record with a minimal tombstone. The tombstone keeps recipient, source
identity, removal reason, and expiry only, so replay cannot recreate the
notification and inaccessible presentation references are removed. Account
deletion repeatedly purges the recipient's records through OCC races until no
keys remain, and replay skips work recipients whose account no longer exists.
A room-leave, member-removal, or implicit-visibility-loss fact removes only
occurrences whose source EVT sequence precedes that lifecycle fact.
Materialization requires the committed source sequence and rejects work at or
before the latest persisted visibility boundary, even if the recipient has
since rejoined. Replaying an old visibility loss cannot delete activity created
after a later rejoin, regardless of replica clock skew.

Notification policy changes affect future source activity. They do not rewrite
or erase existing notification history; users delete retained items explicitly.

### Absolute retention

Every occurrence and tombstone has an absolute expiry 90 days after its source
activity. Every KV mutation applies only the remaining lifetime. Marking an
item read or rewriting it as a tombstone never restarts the 90-day clock.

### Authoritative reads and delivery

Each Chatto process owns one filtered `RUNTIME_STATE` watcher and an in-memory
notification index. The watcher's initial latest-value delivery is a startup
readiness barrier. KV remains authoritative; successful writes wait for their
revision to reach the local index when read-your-writes matters. Public list,
count, and realtime replacement assembly use dedicated index views instead of
scanning a KV prefix per request or connection. Index reads also prune records whose
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
presentations of a committed occurrence. An unread Alert occurrence is handed
off as an opaque coordinate to the file-backed `NOTIFICATIONS_QUEUE` work-queue
stream. The `chatto-notification-alert-delivery-v1` durable pull consumer runs
through `events.DurableWorker`; queue acknowledgement occurs only after a
terminal occurrence state is persisted. The occurrence is the durable
idempotency fence, while JetStream message-ID deduplication covers prompt
source-worker redelivery. Current transient conditions such as Do Not Disturb
may silence delivery without suppressing the occurrence. Provider delivery is
at least once, so a crash after provider acceptance can produce a duplicate.
Marking an occurrence Read silences a pending Alert.

The queue is file-backed and included in normal backups together with its
consumer state. This preserves accepted delivery work across restore instead
of silently turning a backup boundary into alert loss. Both stream retention
and the worker's `PublishedAt` check enforce a two-minute delivery horizon:
restoring inside it may resume the job; restoring later records it as silenced.

Before sending, the alert worker fences occurrence materialization and the
current notification-policy projection, reloads the exact unread occurrence,
permits current preferences only to downgrade the source-time Alert decision,
and revalidates account, membership, unretracted target message, exact
reaction, subscription ownership, and DND. Before account, room, message, or
reaction absence is treated as authoritative, the serving replica captures the
current recipient, server-wide room-event, room-group-layout, and RBAC tails
and waits its relevant projections through those boundaries.
List, realtime, delivery, and mark-read paths use the same causally fenced
target validation, so projection lag cannot tombstone a valid occurrence or
expose a removed target. Delete paths need no target hydration: they accept
only opaque occurrence IDs scoped to the authenticated viewer, but still wait
for the durable notification materializer before reading occurrence state.
Before a mutation reads occurrences, it waits that materializer
through a fresh worker boundary and then
fences the process-local occurrence index through the resulting KV writes.
Before list or realtime responses derive
exhaustive totals and notification summaries, they capture the latest sequence for
every notification-worker EVT filter and wait for the sole durable writer to
acknowledge that boundary. The read then appends a `RUNTIME_STATE` fence marker
and waits its process-local occurrence watcher through that marker's KV
revision. Because the marker follows the acknowledged worker's occurrence
mutations in the same KV stream, a retrying lifecycle cleanup or lagging replica
index fails or delays the read instead of leaking stale counts. List validation
covers the complete retained occurrence set before deriving exact totals,
including rows outside the requested page. Immediately before the provider
call, the transport repeats eligibility, subscription ownership, and DND
checks. Subscription storage and ownership read failures retry through the
durable worker instead of masquerading as an empty device set. An intermediate
visibility loss is therefore applied before an old occurrence can be
delivered, even when current authorization already reflects a later regain.
Delivery completes once any current device accepts the push; it retries only
when no device accepted and at least one current endpoint failed transiently.
This occurrence-level success rule avoids repeatedly alerting successful
devices because another endpoint is persistently broken.
A crash after provider acceptance but before terminal delivery state is persisted can still cause
a duplicate alert, consistent with the at-least-once contract.

### Compatibility and rollout

Notifications 2.0 uses a new persisted occurrence protobuf rather than
changing the immutable `chatto.core.v1.Notification` storage message. Existing
legacy notification records are neither migrated nor read. The cutover starts
with an empty 2.0 list; old rows remain inert until their retention removes
them. This is an intentional pre-1.0 product reset, not a dual-store period.
The later visual-attention field is additive within the v2 record. A current
reader derives it from retained reasons when an earlier v2 row omits it, which
keeps rolling upgrades deterministic without rewriting runtime history.

The public `chatto.api.v1` notification and coarse-preference RPCs are removed
at the same release boundary and replaced by an exact occurrence list,
single/batch occurrence deletion, and per-cause policy operations. Presentation
grouping is client-owned. The bundled client contains no fallback
to the old API or preference levels. Older clients are therefore incompatible
with an upgraded server for notifications, and the 0.5 client requires a 0.5
server through the bundled client's minimum-supported-server check. This intentional breaking
change avoids maintaining a second API, compatibility projection, or preset
translation layer.

Every upgraded replica writes and reads only the 2.0 key and work families. A rolling
deployment can therefore briefly contain an older replica that still writes
legacy records and a newer replica that writes 2.0 records, but neither family
is translated into the other. Once all replicas are upgraded, only 2.0 records
are produced. Rolling back restores the legacy implementation and its old
notification view; 2.0 occurrences and temporary work remain isolated and are not
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
  allowed to expire rather than create an already-expired notification item.
- Interruptive effects are recoverable but not exactly once; rare duplicate
  provider delivery remains possible.
- The clean cutover deliberately discards legacy pending-notification history
  from the new list and avoids a dual-read migration path.
