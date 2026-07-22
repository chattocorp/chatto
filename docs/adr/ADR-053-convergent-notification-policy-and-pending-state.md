# ADR-053: Convergent Notification Policy and Pending State

**Date:** 2026-07-15

**Tracking issue:** [#1556](https://github.com/chattocorp/chatto/issues/1556)

## Context

Chatto currently decides notification eligibility in several message-side fanout
paths. It stores pending notifications in `RUNTIME_STATE`, stores notification
preferences as event-sourced user configuration, tracks room and thread reads
with separate runtime cursors, and uses transient live events to refresh client
lists and counts. Sounds, Web Push, installed-app badges, notification-centre
entries, and room indicators all consume some part of that state.

This grew from a small set of notification cases into a system covering direct
messages, replies, direct mentions, role and room-wide mentions, followed
threads, and all-message room subscriptions. The current coarse notification
level combines several different concerns: whether activity is interesting,
whether a room is considered unread, whether a pending notification exists,
and whether the user should be interrupted.

The split also leaves correctness gaps. In particular, a committed message can
reach a focused client through the EVT republish before its best-effort pending
notification is created. The client advances its read cursor and the server
finds no notification to dismiss; the later notification then remains pending
for content the recipient already read. Similar inconsistencies arise when a
notification lacks exact thread context, when its source message is retracted,
or when a client misses transient synchronization events.

Users also want receiver-controlled, per-room choices for distinct causes:
followed-room activity, followed-thread activity, replies to their messages,
direct username mentions, role mentions, `@here`, and `@all`. A message can
match several of these causes at once, so independent fanout paths cannot be
allowed to create duplicate attention records.

## Decision

Chatto will adopt one backend-owned notification policy evaluation and one
authoritative pending-notification resource. The migration will be incremental
and additive; existing clients and persisted data must remain usable while the
new model rolls out.

### Separate notification cause, delivery intensity, and read state

Notification policy has three separate inputs and outcomes:

- A **cause** explains why an event is relevant to a recipient. Initial causes
  are followed-room activity, followed-thread activity, reply to the
  recipient's message, direct username mention, role mention, `@here`, and
  `@all`. Direct-message policy remains explicit rather than being inferred
  from a room presentation detail.
- A **delivery intensity** decides how that cause is surfaced. Intensities form
  an ordered scale: off, pending attention without an interruptive alert, and
  pending attention with alert delivery. FDR-012 owns the final user-facing
  names and defaults.
- **Read state** records which room or thread content the user has consumed. It
  is not a notification preference. Disabling an alert does not mark content
  read, and advancing a read cursor resolves pending notifications covered by
  that cursor.

Preferences inherit from a user's server defaults and may be overridden per
room for each cause. Effective values are resolved when policy is evaluated;
they are not copied into every room. Follow state, reply attribution, resolved
mention recipients, room membership, and room visibility remain authoritative
domain inputs to evaluation rather than being duplicated as notification
preferences.

### Evaluate all reasons once and persist one result

For each source event and potential recipient, the backend collects every
matching cause, resolves the effective intensity for each cause, and creates at
most one pending notification. The notification records:

- the source event ID;
- its exact room and optional thread destination;
- all matching causes that remain enabled; and
- the strongest effective delivery intensity.

The source-event/recipient pair is the idempotency boundary. Retried fanout,
post-commit recovery, and multiple replicas must converge on the same pending
resource rather than create duplicates. Persisted runtime records use OCC for
create, update, dismissal, and repair. The public notification ID may remain an
opaque API identity, but it must not be the only protection against duplicate
fanout for one source event.

Policy is evaluated from the recipient's effective preferences when the source
event is processed. Later preference changes govern future events by default;
FDR-012 may define explicit actions, such as muting a room, that also clear
already-pending attention.

### Make notification creation and reads converge in either order

Notification creation and read-cursor advancement live in different runtime
records and cannot rely on one process-local ordering. Both operations must
reconcile:

- After advancing a room or thread read cursor, Chatto dismisses pending
  notifications whose source events are covered by the cursor.
- After creating a pending notification, Chatto re-checks the applicable read
  cursor and dismisses or suppresses the notification if the source event is
  already covered.

If creation wins the race, the read-side scan sees it. If the read-side scan
wins, the creation-side re-check sees the advanced cursor. The operations must
use authoritative shared state and remain safe when different replicas perform
the two sides.

Explicit dismissal remains a separate acknowledgement action. Retraction,
loss of visibility or membership, account deletion, expiry, and other source
lifecycle changes must either remove the pending notification or turn it into
an explicitly designed, still-actionable representation; they must not leave a
notification that navigates to nowhere.

### Treat delivery surfaces and realtime events as consumers

The authoritative notification list and counts come from pending resources.
Room indicators, the notification centre, sounds, Web Push, native
notification dismissal, and installed-app badges consume the same evaluated
result. Alert delivery occurs only for a pending notification whose evaluated
intensity permits it.

Transient realtime events are invalidation and synchronization hints. Missing,
duplicated, or reordered live events may delay a local update but must not
permanently change notification correctness. Initial load, reconnect, and
explicit repair fetch authoritative pending state and counts.

Every delivery payload retains enough target identity to open the exact room,
thread, and event. Clients do not infer a thread destination from notification
kind or fetch unrelated room state to guess where an alert belongs.

### Preserve compatibility during migration

The new preference and notification fields will be additive protobuf changes.
Existing persisted core messages will not be renumbered or have field types
changed. The existing `DEFAULT`, `MUTED`, `NORMAL`, and `ALL_MESSAGES` API and
stored configuration require an explicit compatibility mapping while mixed
versions are supported.

New writers must not emit equivalent preference or pending-notification facts
under incompatible OCC scopes during a rolling deployment. The implementation
plan must define which version owns policy evaluation, how legacy notification
records are read or repaired, what rollback can preserve after new writes, and
when old API behavior can be retired.

## Consequences

- Notification fanout becomes a policy decision with one result instead of a
  collection of loosely coordinated side effects.
- Users can control distinct attention causes without conflating those choices
  with read state or device-specific sound settings.
- Overlapping causes do not produce duplicate notifications; retaining the
  complete reason set also lets clients explain why an alert was delivered.
- Two-sided read reconciliation adds an authoritative cursor lookup after
  creation and bounded pending-notification work after a read. This cost is
  required to close the cross-replica ordering hole without a distributed
  transaction across runtime keys.
- Pending notification creation remains best-effort relative to committing a
  message. Post-commit retry or repair must therefore use the same idempotency
  boundary and cannot depend only on a process-local callback.
- Exact targets make thread and DM notifications actionable, but require
  additive persisted and public API context for legacy records that do not
  carry it today.
- Preference changes normally affect future events rather than rewriting
  notification history. Any product behavior that clears existing pending
  items must be stated explicitly in FDR-012.
- Realtime clients become simpler and more robust because refetching repairs
  state instead of live-event arrival order defining it.
- The transition is larger than fixing individual races. It requires a staged
  compatibility path, race-focused backend tests, public API updates, frontend
  preference UI work, and multi-user end-to-end coverage tracked in #1556.
