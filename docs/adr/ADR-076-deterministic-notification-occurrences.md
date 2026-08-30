# ADR-076: Store Notification Lifecycle Facts in a Bounded Event Stream

**Date:** 2026-08-10

**Updated:** 2026-08-30

## Context

Chatto's legacy notifications were random, recipient-specific runtime records
created by request handlers. Their existence meant unread, deletion meant both
read and dismissed, and Web Push was a best-effort callback. Multiple matching
causes could race, a crash could lose materialisation, and delayed work could
recreate activity the user had already read or dismissed.

Notifications are event-like but are not durable domain history. A direct
mention, reply, or reaction is a recipient-specific signal derived from an
authoritative source fact. It needs ordered lifecycle facts, replayable
projections, recoverable push delivery, explicit deletion, and bounded
retention. Putting those facts in `EVT` would pollute the permanent domain log;
putting each notification in KV would require one JetStream subject per item
and would make lifecycle ordering and worker consumption less idiomatic.

## Decision

### Dedicated bounded log

Chatto stores Notifications 2.0 lifecycle facts in a dedicated file-backed,
S2-compressed, replicated `NOTIFICATIONS` JetStream stream. It is a normal Loom
event log built through the shared `events.EncodedEventLog`, decoded projector,
projection snapshot, and `events.DurableWorker` mechanics. Chatto owns its
protobuf envelope, subjects, retention, authorization, and delivery policy.

The stream has four fixed low-cardinality subjects:

- `notifications.signalled`
- `notifications.read`
- `notifications.removed`
- `notifications.alert_resolved`

Subjects describe lifecycle fact kinds, not recipients or notification IDs.
Recipient and notification coordinates stay in the protobuf payload. This
avoids per-notification JetStream subject-index overhead.

The stream is included in normal backups. It contains the authoritative
90-day notification list, user triage, and pending push work; excluding it
would discard user-visible history and accepted delivery work at an arbitrary
backup boundary. Backup captures `EVT`, then `RUNTIME_STATE`, then
`NOTIFICATIONS`, including the durable consumer state owned by each stream.
Because the materializer acknowledges an EVT source only after its notification
output is durable, every captured source is either represented in the later
`NOTIFICATIONS` snapshot or remains replayable from the restored materializer
consumer position.

### Rich signals and exact identity

The `NotificationEvent` envelope owns notification identity, recipient,
lifecycle time, and expiry. `NotificationSignalled` contains immutable source
coordinates, materialization-time delivery and attention decisions, and a rich
`NotificationSignal` oneof. The
projection constructs `NotificationOccurrence` current-state resources from
that fact and later lifecycle facts; the event never embeds its projection.
Current producers use direct message, root channel-room message, direct
mention, reply, role mention, `@here`, `@all`, followed-thread activity, and
reaction received. The wire contract also retains a deprecated followed-room
compatibility branch that current code does not derive. Each supported variant
owns the typed data needed to authorize, render, and navigate that signal;
reaction signals carry their
emoji, and a consolidated role-mention signal carries the sorted source-time
role handles that selected the recipient. The record
references source resources but does not copy message bodies, room names,
avatars, or display names.

Notification policy uses one explicit field per built-in signal class instead
of mirroring the signal `oneof` with an enum-keyed table. A room inherits from
its current room group and then from the server. A direct-message room skips
the room-group level. The product default supplies a concrete server value when
the user has no server override. Effective policy populates every supported
field. Future notification features, such as room invitations, add a rich
signal branch and, when independently configurable, an additive policy field
with defined authorization, lifecycle, rendering, navigation, and delivery
behavior.

Policy updates are sparse: a field mask selects the override fields being
changed, a selected value sets an override, and a selected absent value clears
it to inherit. The service applies that patch to the latest projected scope
inside the per-user config OCC retry and commits the complete resulting scope
as one fact. Concurrent updates to different fields therefore compose instead
of replacing one another, and older clients leave future fields untouched.

One source fact may generate several notification signals for the same user.
For example, one root room message may independently be room activity, an
`@all` mention, and a direct mention.
Each exact occurrence ID is derived from recipient ID, source event ID, and
signal kind. Retries are idempotent while distinct causes retain independent
identity and triage.

The materialization-time delivery mode is `Off`, `Badge`, `Notification`, or
`Push notification`. `Off` creates no output. `Badge` updates one neutral
unread marker for the applicable room or thread. It does not create a list
item, request local sound, or permit push delivery. Both notification modes
create the same durable list item and can request the configured local sound.
Only `Push notification` is eligible for push delivery. Visual attention is
independent: reactions are currently Ambient and other current signals are
Important.

### Notification decisions use current projected state

Message commands resolve mention handles on every room-OCC attempt and persist
the resulting user-and-mention-kind facts on the existing `MessagePostedEvent`.
This is durable message semantics: it preserves otherwise transient `@here`,
role, and `@all` expansion without recording a notification plan. A conflicting
retry therefore cannot retain stale mention recipients.

The same existing `MessagePostedEvent` is the source for ordinary root-message
attention. When the durable materializer processes it, the materializer uses
current room membership, authorization, room-group placement, notification
policy, and thread-follow state. Thread messages and direct messages use their
existing separate causes.
Joined-room activity does not produce followed-room activity. That branch is a
deprecated compatibility slot; Room messages at room scope provide the
supported control. No notification-specific source event or marker is added to
`EVT`.

For compatibility, `MessagePostedEvent.mentioned_user_ids` remains a flattened
view of recipients selected by direct, role, `@here`, and `@all` handles. It
cannot recover which cause selected a user. During a mixed-version rollout, a
source event without rich `mentions` therefore omits only the ambiguous mention
signal instead of applying the wrong policy or persisting a false cause; DM,
root-room, reply, and follow signal kinds that remain independently knowable
are still derived.
Current writers populate `mentions` with every rich cause.

The EVT-backed Notification Decisions projection consumes the compact current
state needed for notification derivation: active accounts, room membership and
kind, universal-room authorization, room-group layout, RBAC, notification
policy, thread followers, and reply counts. The materializer waits until this
projection includes the delivered source fact, then makes one decision from its
current state. Later facts that the projection has already applied intentionally
affect that decision. This avoids retaining historical decision boundaries or
putting notification plans in permanent domain history.

The shared `chatto-notification-materializer-v1` durable consumer reads only
existing domain-changing `EVT` facts. It derives durable delivery output from
current projected state. Notification modes append `NotificationSignalled`
facts to `NOTIFICATIONS`. Badge updates a monotonic latest-value room or thread
marker in `RUNTIME_STATE`. The consumer acknowledges the EVT delivery only
after all output writes succeed. A crash before the confirmed acknowledgement
redelivers the source; deterministic occurrence IDs and source-sequence Badge
markers make partial or repeated output idempotent.
Retraction, reaction removal, visibility loss, room deletion, and account
deletion use their existing EVT facts to append notification dismissals. No
notification-only event is added to `EVT`, and there is no prepared
notification work record in `RUNTIME_STATE`. A Badge marker is final
user-visible output, not queued work.

The materializer uses `DeliverNew` for the Notifications 2.0 rollout boundary.
It processes facts committed after its durable consumer was first established;
legacy notifications and older retained EVT history are not backfilled.

Current list and push-delivery reads still fence the user, room,
room-group-layout, and RBAC projections before treating a target as visible or
absent. Persistent
visibility-loss boundaries suppress older source facts after a quick regain.

### Projected current state

Every Chatto process projects `NOTIFICATIONS` into one in-memory
`NotificationProjection`. The projection is the current occurrence list and
contains minimal dismissal tombstones. It supports shared encrypted snapshots
whose stream incarnation and sequence are bound to `NOTIFICATIONS`, not `EVT`.
List, mutation, realtime, and delivery paths wait for the relevant notification
stream position or current tail before reading it.

Read appends an empty `NotificationRead` fact. It leaves the occurrence in the
list and suppresses pending push delivery. Dismissal appends
`NotificationRemoved`;
after the projection has observed that tombstone, Chatto securely deletes the original
rich `NotificationSignalled` record by stream sequence. The tombstone prevents
materializer redelivery from recreating the item and contains no presentation
content. Repeating either mutation is idempotent, and duplicate JetStream
acknowledgements are not counted as a second successful deletion. The private
secure-delete coordinate remains projected through the broker's physical
cleanup grace even after the tombstone stops affecting application state.

Room/thread read reconciliation, visibility-loss boundaries, and Badge output
remain bounded latest-value records in `RUNTIME_STATE`. The boundary records
are cross-stream coordination state, not notification history. A Badge record
stores only the latest source needed to compute neutral unread attention. One
process-wide filtered KV watcher indexes all three families; successful local
writes wait for their exact KV
revision to enter that index before dependent work continues. Badge marker
keys use bounded concurrent OCC writes and one collective applied-revision
barrier. Because the read boundary is recorded before matching
`NotificationRead` facts, every replica performs one startup repair and
thereafter reconciles only the room/thread scope whose watched boundary
changed. Large occurrence fanouts likewise read visibility boundaries from the
index and publish their coalesced realtime invalidations with one flush rather
than one broker round trip per recipient.

Badge uses the same room/thread read coordinates and visibility rules as
occurrences. A thread marker contributes to the parent room. A source sequence
can only replace an older marker in the same scope, so delayed delivery cannot
regress attention. The public room `has_unread` value reports this Badge
attention only. The independent Message Read Cursor continues to place the New
messages separator and cannot create a room dot by itself. The marker expires
90 days after its latest source and account deletion removes it. A content-free
transient invalidation rebuilds the affected room state and followed-thread
viewer state.

Posting a room message records a covered-read boundary through the poster's new
root event, in addition to advancing the Message Read Cursor. This operation
makes older Badge attention inactive and uses the same repair handshake as an
explicit room read.

Realtime `NotificationOccurrencesInvalidated` messages are transient hints.
They can carry one opaque sound-candidate notification ID but never expose
JetStream coordinates. The receiving replica fences the notification
projection, revalidates any candidate, and sends an authoritative finite
replacement plus a positive `play_notification_sound` instruction. The legacy
alert-candidate field remains for older replicas and is set only for a
push-eligible occurrence. Missing or reordered invalidations cannot play a
sound across the current policy, DND, visibility, read, or removal boundary.
Clients deduplicate the one-shot sound by projection-event ID and quietly
reconcile the authoritative first page once per minute. This reconciliation
bounds stale counts when a best-effort invalidation is lost.

### Retention and automatic expiry

The application expiry of every lifecycle fact is exactly 90 days after its
source activity. The immutable `expires_at` field gives projections, APIs, and
workers the semantic boundary even if broker cleanup is delayed. Reads and
dismissals never extend it.

Each stream record also receives a JetStream per-message TTL ending 24 hours
after application expiry. The stream `MaxAge` is the same 91-day upper bound.
The grace period lets projections hide an item deterministically at 90 days
while JetStream performs physical cleanup later. Broker expiry does not need to
emit a synthetic event: every read prunes expired state, and a projection timer
accelerates realtime convergence. Dismissal cleanup retries retain their
content-free signal coordinate through this entire grace period.

### Push delivery consumes the signal log directly

There is no notification work-queue stream. The
`chatto-notification-alert-delivery-v1` durable pull consumer filters
`notifications.signalled` directly and runs through `events.DurableWorker`.
It waits for the notification projection through the delivered stream
sequence, fences the EVT materializer, reloads current occurrence state, and
revalidates policy, visibility, exact reaction/target existence, DND, and push
subscription ownership.

Only unread, pending occurrences whose materialization-time mode is `Push notification`
can contact a provider.
The immutable delivery deadline is two minutes after source time. When a
still-pending push delivery reaches a delivery or suppression decision, the worker
appends one `NotificationAlertResolved` fact carrying the terminal outcome
before acknowledging. A notification already made terminal by read, removal,
or expiry needs no additional alert-resolution fact; redelivery is an ack-only
no-op. Provider delivery remains at least once: a crash after provider
acceptance but before the terminal fact commits can duplicate a push.

### Compatibility

Notifications 2.0 replaces Notifications 1.0 at the upcoming pre-1.0 release
boundary. Legacy notification records are neither migrated nor read. Retained
legacy protobuf messages and old EVT variants remain decodable but current code
does not write them.

The public notification API intentionally replaces the released legacy API. It
exposes exact occurrences and rich signal oneofs; the bundled client owns
presentation grouping. New signal
branches are wire-additive after release, but a server must preserve and reject
unsupported variants rather than guessing their visibility or deleting them.
New top-level `NotificationEvent` lifecycle variants require a readers-first
rollout: every serving projection must understand the variant before any writer
appends it. An older projector stops safely on an unsupported lifecycle fact
rather than skipping state that could affect privacy or delivery.
Notifications 2.0 also uses a fresh realtime projection-operation tag. The
released Notifications 1.0 operation tag is reserved rather than being reused
with an incompatible nested payload, so mixed-version clients fail closed on an
unknown operation instead of accepting an empty replacement and advancing.

The public and persisted delivery-mode enums keep wire values 2 and 3. The
names `IN_APP_NOTIFICATION` and `PUSH_NOTIFICATION` are aliases for those
values. The previous `SILENT` and `ALERT` names remain as deprecated aliases.
Badge adds wire value 4. Older binaries preserve that value and fail closed by
producing neither a notification occurrence nor push delivery. The new scoped
policy service is additive and leaves the legacy server/room methods unchanged.

Room messages adds policy field 10 and signal branch 10. Sparse field masks let
older clients update known policy fields without changing the additive field.
If they receive a Room-message occurrence, they show the existing generic
dismissible row and do not infer navigation. The default Badge mode uses the
existing public `has_unread` field, which older clients already understand.

An older server does not derive new Room messages decisions. Thus, the default
Badge output and future occurrences are temporarily inactive during rollback
instead of being interpreted as another cause. If an upgraded server already
persisted a Room-message occurrence for Notification or Push notification, the
older server's notification occurrence RPCs return `Unimplemented` until a
supporting binary serves the occurrence again. The older server does not
reinterpret or discard the unsupported signal.

The public and persisted field 8 followed-room policy and signal contracts are
deprecated compatibility slots. Current code accepts and preserves the policy
field but does not derive the signal. Existing stored values remain inert.
Keeping the field and branch prevents older data or clients from being
reinterpreted as another notification cause.

Room-group policy changes use the separate persisted
`UserRoomGroupNotificationPolicyChangedEvent` variant. An older binary ignores
this unknown variant instead of interpreting it as a server policy. The
room-group maps also select new configuration and notification-decision
snapshot contract IDs. During rollback, room-group overrides are temporarily
inactive and cannot replace a newer snapshot with one that omits those
overrides.

## Consequences

- Notification history and lifecycle are ordered, replayable, bounded, and
  backed up without becoming permanent domain history.
- Fixed subjects avoid the RAM cost of indexing one subject per notification.
- The same stream powers notification projections and durable push delivery;
  there is no second queue, prepared-work KV, or occurrence KV to reconcile.
  Badge uses one bounded latest-value runtime marker per active room or thread
  scope.
- Exact per-signal-class identities let clients group presentation without losing
  jump targets, unread counts, or triage semantics.
- Dismissal physically removes rich content while a minimal retained fact keeps
  redelivery idempotent.
- Application expiry is deterministic even though JetStream cleanup is
  asynchronous.
- One additional replicated file-backed stream and two durable consumers—the
  EVT materializer and the notification push worker—add bounded NATS cluster
  overhead.
