# ADR-075: Store Notification Lifecycle Facts in a Bounded Event Stream

**Date:** 2026-08-10

## Context

Chatto's legacy notifications were random, recipient-specific runtime records
created by request handlers. Their existence meant unread, deletion meant both
read and dismissed, and Web Push was a best-effort callback. Multiple matching
causes could race, a crash could lose materialisation, and delayed work could
recreate activity the user had already read or dismissed.

Notifications are event-like but are not durable domain history. A direct
mention, reply, or reaction is a recipient-specific signal derived from an
authoritative source fact. It needs ordered lifecycle facts, replayable
projections, recoverable alert delivery, explicit deletion, and bounded
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
- `notifications.dismissed`
- `notifications.alert_resolved`

Subjects describe lifecycle fact kinds, not recipients or notification IDs.
Recipient and notification coordinates stay in the protobuf payload. This
avoids per-notification JetStream subject-index overhead.

The stream is included in normal backups. It contains the authoritative
90-day notification list, user triage, and pending alert work; excluding it
would discard user-visible history and accepted delivery work at an arbitrary
backup boundary. The stream and its durable consumer state are restored
together.

### Rich signals and exact identity

`NotificationSignalled` contains immutable source coordinates, source-time
policy and attention decisions, and a rich `NotificationSignal` oneof. The
projection constructs `NotificationOccurrence` current-state resources from
that fact and later lifecycle facts; the event never embeds its projection.
Current variants are direct message, direct mention, reply, role mention,
`@here`, `@all`, followed-thread activity, followed-room activity, and reaction
received. Each variant owns the typed data needed to authorize, render, and
navigate that signal; reaction signals also carry their emoji. The record
references source resources but does not copy message bodies, room names,
avatars, or display names.

`NotificationPolicyKind` remains a small enum because it is a stable preference
key. It is not the notification payload. Future notification features, such as
room invitations, add a rich signal branch and define their authorization,
lifecycle, rendering, navigation, and delivery behavior.

One source fact may generate several notification signals for the same user.
For example, one message may independently be a reply and a direct mention.
Each exact occurrence ID is derived from recipient ID, source event ID, and
policy kind. Retries are idempotent while distinct causes retain independent
identity and triage.

The source-time delivery intensity is `Off`, `Badge`, or `Alert`. `Off` creates
no signal. `Badge` and `Alert` create the same durable list item; only `Alert`
is eligible for interruptive delivery. Visual attention is independent:
reactions are currently Ambient and other current signals are Important.

### Source derivation remains outside EVT

Message and reaction commands evaluate recipients and notification policy on
every source-command OCC attempt. They replace one temporary
`notification_work.{sourceEventId}` value in `RUNTIME_STATE` before committing
the existing source fact. The value contains the complete exact work set for
that attempt. A conflicting retry therefore cannot retain stale recipients.

The shared `chatto-notification-materializer-v2` durable consumer reads only
existing domain-changing `EVT` facts. After the source fact commits it loads the
prepared work, appends deterministic `NotificationSignalled` facts to
`NOTIFICATIONS`, and removes the temporary value. Retraction, reaction removal,
visibility loss, room deletion, and account deletion use their existing EVT
facts to append notification dismissals. No notification-only event is added
to `EVT`.

The materializer also maintains the event-time visibility boundary required
for universal rooms, room-group moves, and RBAC changes. Current list and alert
reads fence the user, room, room-group-layout, and RBAC projections before
treating a target as visible or absent. A quick access loss and regain cannot
allow an older private signal to survive or be pushed.

### Projected current state

Every Chatto process projects `NOTIFICATIONS` into one in-memory
`NotificationProjection`. The projection is the current occurrence list and
contains minimal dismissal tombstones. It supports shared encrypted snapshots
whose stream incarnation and sequence are bound to `NOTIFICATIONS`, not `EVT`.
List, mutation, realtime, and delivery paths wait for the relevant notification
stream position or current tail before reading it.

Read appends `NotificationRead`. It leaves the occurrence in the list and
silences a pending Alert. Dismissal appends `NotificationDismissed`; after the
projection has observed that tombstone, Chatto securely deletes the original
rich `NotificationSignalled` record by stream sequence. The tombstone prevents
materializer redelivery from recreating the item and contains no presentation
content. Repeating either mutation is idempotent.

Room/thread read reconciliation and visibility-loss boundaries remain bounded
latest-value records in `RUNTIME_STATE`. They are cross-stream coordination
state, not notification history. They retain their existing direct-read and OCC
handshakes.

Realtime `NotificationOccurrenceChanged` messages are transient invalidations
identified by opaque notification ID. They do not expose JetStream coordinates.
The receiving replica fences the notification projection and sends an
authoritative finite replacement, so missing or reordered invalidations cannot
permanently corrupt client counts.

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
accelerates realtime convergence.

### Alert delivery consumes the signal log directly

There is no notification work-queue stream. The
`chatto-notification-alert-delivery-v2` durable pull consumer filters
`notifications.signalled` directly and runs through `events.DurableWorker`.
It waits for the notification projection through the delivered stream
sequence, fences the EVT materializer, reloads current occurrence state, and
revalidates policy, visibility, exact reaction/target existence, DND, and push
subscription ownership.

Only unread, pending, source-time `Alert` occurrences may contact a provider.
The immutable delivery deadline is two minutes after source time. The worker
appends `NotificationAlertResolved` as Delivered or Silenced before
acknowledging. Redelivery is an ack-only no-op once the projected state is
terminal. Provider delivery remains at least once: a crash after provider
acceptance but before the terminal fact commits can duplicate a push.

### Compatibility

Notifications 2.0 replaces Notifications 1.0 at the upcoming pre-1.0 release
boundary. Legacy notification records are neither migrated nor read. Retained
legacy protobuf messages and old EVT variants remain decodable but current code
does not write them.

The public notification API is intentionally breaking relative to legacy and
earlier unreleased Notifications 2.0 drafts. It exposes exact occurrences and
rich signal oneofs; the bundled client owns presentation grouping. New signal
branches are wire-additive after release, but a server must preserve and reject
unsupported variants rather than guessing their visibility or deleting them.

## Consequences

- Notification history and lifecycle are ordered, replayable, bounded, and
  backed up without becoming permanent domain history.
- Fixed subjects avoid the RAM cost of indexing one subject per notification.
- The same stream powers projections and durable Alert delivery; there is no
  second queue or occurrence KV to reconcile.
- Exact per-signal-class identities let clients group presentation without losing
  jump targets, unread counts, or triage semantics.
- Dismissal physically removes rich content while a minimal retained fact keeps
  redelivery idempotent.
- Application expiry is deterministic even though JetStream cleanup is
  asynchronous.
- One additional replicated file-backed stream and durable consumer add bounded
  NATS cluster overhead.
