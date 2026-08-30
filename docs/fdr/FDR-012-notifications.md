# FDR-012: Notifications

**Status:** Experimental
**Last reviewed:** 2026-08-30

## Overview

Notifications and Badge indicators are user-scoped ways to show activity that
deserves attention. Notifications form a persistent list of exact activity.
Badge indicators add only neutral unread dots. Both forms cover direct
messages, root messages in channel rooms, replies, direct and role mentions,
`@here`, `@all`, followed threads, and reactions. Notification
occurrences use a small lifecycle: Unread, Read, or explicitly Deleted. Badge
attention uses a latest-value marker that becomes inactive through read,
visibility, and expiry boundaries.

The server records occurrences individually. The bundled frontend may
consolidate them for presentation without changing occurrence identity, jump
targets, unread counts, read state, or deletion semantics.

## Behavior

- The notification page is one newest-activity-first chronological list that
  contains both Unread and Read activity. Unread reactions are Ambient and use
  a neutral treatment. Every other current cause is Important and uses Chatto's
  notification orange. Read rows are visually muted while remaining fully
  interactive. The list does not use a separate unread dot on each row.
- Badge activity does not add a row to the notification page. It adds a neutral
  unread dot to the applicable room. A thread-scoped Badge contributes to its
  parent room. An orange notification indicator takes priority when both types
  of attention apply.
- The list is divided into Today, Yesterday, This Week, and month sections
  using the preferred time zone of the account on each server.
- Rows use concise, full localized sentences without message previews.
  Reaction rows show the emoji that were given.
- Opening a row navigates to the selected occurrence's exact room, thread, and
  event. The occurrence is marked Read only after the target is displayed.
- Reading a room or thread marks covered occurrences Read. A reaction is
  covered according to the reacted-to message and reaction horizon.
- Notifications cannot be marked Unread. There is no Done state or Inbox/Done
  split.
- The Delete action deletes the exact visible occurrences in the current row.
  On devices that support hover, this action appears when the row has hover or
  keyboard focus. It remains visible on touch devices.
- Dismiss read deletes only the loaded occurrences that were Read when the user
  selected the action. It does not delete Unread occurrences that arrived
  before or during the action. Both deletion actions update the UI
  optimistically and then reconcile with the server.
- Every occurrence leaves application-visible state exactly 90 days after its
  source activity. Reading or deleting it does not extend that lifetime.
  Physical cleanup may continue during ADR-076's 24-hour grace period without
  extending user-visible retention.
- A Badge marker expires 90 days after its latest source activity. A read,
  visibility loss, target removal, or reaction removal can make it inactive
  sooner.
- The combined multi-server list preserves healthy results when another server
  fails and exposes the failure as partial.
- Notification delivery rules and client sound choices are User Preferences.
  The server saves delivery rules. The client saves sound and sound-filter
  choices for each server. Notifications from different registered servers can
  use different sounds.

## Design Decisions

### 1. Exact occurrences, client-side grouping

**Decision:** The server exposes one occurrence for each recipient, source
activity, and notification cause. List and notification totals count exact unread
occurrences, independently of pagination and presentation grouping. The bundled
frontend groups DMs by conversation, root room messages by room, reactions by
reacted-to target, and followed activity by thread. It leaves mentions and
replies separate because they have distinct jump targets.

**Why:** Exact server resources preserve identity, triage, navigation, and
integration semantics. Client-side grouping can evolve without a migration or
loss of information. Two unread DMs may appear as one row while still
contributing two to the badge.

**Tradeoff:** Clients must maintain presentation groups and their exact member
IDs. A group opens its newest unread occurrence, or its newest occurrence when
all members are Read.

### 2. Persistent read state with explicit deletion

**Decision:** Reading is monotonic: an occurrence may move from Unread to Read
but not back to Unread. Read occurrences remain in the list until the user
deletes them or the 90-day retention window expires. Deletion is idempotent and
privacy-oriented rather than a synonym for reading.

**Why:** Read state answers whether attention is outstanding; deletion answers
whether the item should remain in personal history. Combining them would make
it impossible to dismiss without handling or to retain handled activity.

**Tradeoff:** The server must preserve bounded triage and deletion history so a
replay cannot recreate a deleted item.

### 3. Delivery policy is separate from attention level

**Decision:** Each configurable notification signal class resolves independently.
A room uses its override, its current room-group override, and the user's server
preference in that order. A direct-message room skips the room-group level. If
the user has no server preference, the concrete product default supplies the
server value.

- **Off** — do not create attention for this cause.
- **Badge** — add only a neutral unread dot. Do not create a notification
  occurrence, play a sound, or send push.
- **Notification** — create an in-app notification without push delivery. The
  client can play the configured notification sound.
- **Push notification** — create the same in-app notification and make it
  eligible for Web Push or native delivery.

| Cause                          | Default           |
| ------------------------------ | ----------------- |
| Direct message                 | Push notification |
| Root message in a channel room | Badge             |
| Direct username mention        | Push notification |
| Reply to the user's message    | Push notification |
| Role mention                   | Push notification |
| `@here`                        | Push notification |
| `@all`                         | Push notification |
| Followed thread activity       | Notification      |
| Reaction to the user's message | Notification      |

Attention level controls presentation separately: reactions are Ambient and all
other current causes are Important. Bell, server, room, and app indicators use
notification orange when at least one contributing unread occurrence is
Important and a neutral treatment when every contributing occurrence is
Ambient. Attention levels are not user-configurable in this iteration.

**Why:** Whether activity is stored, whether it leaves the app, and how
strongly it is presented are different choices. The delivery names state where
the notification goes. Sound remains a client preference for both notification
modes.

**Tradeoff:** More than one policy dimension exists conceptually, although the
current product exposes only delivery-mode preferences.

The notification settings page shows the nine supported causes as matrix rows.
It shows the server, visible room groups, current-member channel rooms, and
current-member direct-message rooms as columns. Each group column is followed
by its room columns. A server cell always shows a concrete value. When no user
preference exists, it shows the product default at full intensity without an
inheritance marker. Server cells cycle through Off, Badge, Notification, and
Push notification.

Room-group and room cells cycle through Inherit, Off, Badge, Notification,
Push notification, and back to Inherit. Off uses a grey crossed bell. Badge
uses a grey filled bell. Both notification modes use notification orange, with
a bell for Notification and a phone for Push notification. An inherited cell
shows the effective mode at reduced intensity. The legend and distinct icons
make the state clear without color alone.

Badge decisions use the existing room and thread read boundaries. A thread
Badge rolls up to its parent room. Reading the thread clears its contribution
to the room indicator. The Message Read Cursor remains separate and places the
New messages separator. Cursor lag alone does not create a room dot. Thus,
setting Room messages to Off prevents neutral dots for ordinary root messages
without disabling last-read tracking. Badge does not update an operating-system
or application-icon badge.

Posting a room message advances the poster's Message Read Cursor and records
the same notification read boundary as an explicit room read. Thus, posting
also clears older Badge attention without coupling the dot to cursor lag.

The scope filter always keeps the server column. A room match also keeps its
parent group. A group match keeps all current-member rooms in that group.
Direct-message policy applies at server scope and to individual direct-message
rooms. Its room-group and channel-room cells are not applicable and cannot be
changed. Room-message policy applies at server, room-group, and channel-room
scope. Its direct-message-room cells are not applicable and cannot be changed.

A room uses the group that contains it at the exact source-event sequence. A
room move changes future effective policy. It does not change historical
notification decisions. Deleting a group leaves its saved user preferences
inert. Group IDs are not reused, and deletion does not fan out cleanup writes
to user configuration aggregates.

Room-group and room policy writes validate scope access at request time and use
OCC on the user's configuration aggregate. They do not advance the
authorization fence. A concurrent membership loss, room deletion, or group
deletion can leave a newly committed preference inert, but it cannot change
another user's state or grant access to the deleted scope.

### 4. Source-time decisions are durable and replayable

**Decision:** Notification recipients and effective source-time policy are
derived asynchronously from committed domain facts. Later membership,
preference, or thread-follow changes do not rewrite that historical decision.
A user's own activity does not notify them. One source activity produces at
most one delivery decision per recipient and cause. For example, a root
message that contains `@all` and a direct mention can produce room-message,
`@all`, and direct-mention decisions for one recipient. If these decisions use
a notification mode, they create separate occurrences. Badge decisions
coalesce into one latest-value marker for the applicable room or thread.
Channel-room recipients must have `message.read` at that same source sequence.
A direct mention also permits its recipient when `message.read-interactions`
applies, because the same message fact creates the interaction relationship.

**Why:** Notifications describe what happened under the policy and visibility
that applied at that moment. Deriving from committed facts makes retries
idempotent without adding notification-only trigger events to permanent domain
history.

**Tradeoff:** Delivery is eventually consistent with the source activity, and
the implementation must preserve a recoverable handoff between the domain log
and the bounded notification lifecycle. ADR-076 defines that architecture.

### 5. Current visibility remains a privacy boundary

**Decision:** An occurrence may be listed, opened, mutated, or delivered only
while the recipient still exists and can currently see its room and exact
target. Channel-room message-derived occurrences also require current
`message.read`, or `message.read-interactions` with a relationship to the
target's thread. DM membership authorizes DM occurrences. Without applicable
access, Chatto hides the occurrence. Removed reactions, retracted targets,
deleted rooms, and lost room access remove the corresponding occurrence.
Durable visibility-loss boundaries prevent old queued activity from
reappearing after a quick regain of room access.
Actor identity is hydrated from current account data; an unavailable or deleted
actor does not by itself expose copied profile data or make an otherwise valid
occurrence invisible. Badge markers use the same current room, target,
reaction, visibility-loss, and read boundaries.

**Why:** Source-time eligibility explains why the notification was created, but
it cannot override present-day privacy and target existence.

**Tradeoff:** An occurrence can disappear without direct user triage, and reads
must coordinate with current authorization state before reporting absence or
success.

### 6. Realtime delivery is a convergence hint

**Decision:** Realtime notification updates tell clients to replace their
finite notification view from authoritative server state. Badge updates tell
clients to replace the affected room state. My Threads can decorate a followed
thread from matching unread occurrences in the finite notification view. The
thread read cursor remains the only source of reply-unread state. Unread totals
remain exact even when rows are grouped. The client also performs quiet
periodic reconciliation so a lost transient update cannot leave counts stale
indefinitely.

**Why:** A transient notification invalidation is not durable notification
state. Rebuilding the finite projection avoids exposing internal storage
coordinates or asking clients to replay lifecycle facts.

**Tradeoff:** A change can cause a bounded list refresh rather than a tiny
row-level patch.

### 7. Notification signals are extensible, not domain authority

**Decision:** Each occurrence contains a typed signal with its exact destination
and cause-specific data. New signal variants must define authorization,
lifecycle, navigation, and delivery behavior. A notification may call attention
to a future resource such as a room invitation, but the notification itself
does not grant access or become authoritative invitation state.

**Why:** Rich variants can carry future cause-specific data without growing a
flat reason/target matrix, while keeping security-sensitive domain decisions in
their owning feature.

**Tradeoff:** Older clients render an unknown signal generically and cannot
navigate it. Older servers reject operations on variants they cannot safely
validate rather than guessing.

### 8. Thread subscriptions are distinct from notification rows

**Decision:** Posting in a thread attempts to follow it, even after an earlier
unfollow. A delivered direct username mention in a channel-room root or reply
attempts to follow that thread unless the recipient previously opted out. The
root message ID identifies the thread for a root mention. Role, `@here`, and
`@all` mentions do not follow threads. Following a thread creates an activity
source whose delivery is still controlled by notification policy. Follow
controls belong to threads, not to notification rows. Root channel-room
activity uses the Room messages cause. A room-specific Room messages policy
supplies the required opt-in control without a separate room-follow state.

**Why:** A subscription describes future interest in a conversation; a
notification occurrence describes one past activity. Keeping them separate
avoids giving list triage surprising subscription side effects.

**Tradeoff:** Automatic thread follow is best-effort after the source message
commits. Failure can omit later followed-thread notifications until the user
follows the thread explicitly.

### 9. Client-rendered sounds remain server-specific

**Decision:** The client stores notification sound and sound-filter choices for
each registered server. For a live notification, the client uses the choices
for the server that produced the notification. During an upgrade, the client
copies the old global sound choice when it first creates the slot for a server.

**Why:** The client plays the sound, but all notification behavior is a User
Preference. Storage for each server keeps this scope. It does not incorrectly
record a client audio filter as server state. The migration keeps the user's
existing sound choice.

**Tradeoff:** Sound choices do not sync to another browser or device. The
client keeps a small local-storage entry for each server. Both Notification and
Push notification can request the configured local sound. Do Not Disturb and
current notification policy can suppress that request.

### 10. Notification occurrences activate bot integrations

**Decision:** A bot uses the same notification occurrences and realtime
notification replacement as a human account. Direct messages, direct mentions,
replies, and followed-thread activity are the supported activation causes for
bot integrations. Chatto does not create a separate bot-interaction event or
realtime subscription. Other notification causes can be present in the shared
replacement. An integration must filter the replacement by cause.

**Why:** The occurrence already contains the source-time cause, stable identity,
exact message target, current visibility checks, and bounded durable history.
One semantic source can support realtime now and a durable webhook delivery
adapter later.

**Tradeoff:** Realtime sends a finite latest-value replacement instead of an
append-only activation feed. Integrations must checkpoint stable occurrence
IDs. One message can create more than one cause, so an integration that wants
one action per message must also deduplicate by the referenced message event
ID.

## Compatibility

Notifications 2.0 supersedes Notifications 1.0 at the 0.5.0 pre-1.0 boundary.
Legacy records and coarse Muted/Normal/All Messages preferences are not migrated
or interpreted. Historical persisted event variants remain replay-decodable,
but current code adds no notification occurrences, triggers, or Badge markers
to `EVT`. Notification policy changes remain user-configuration facts in
`EVT`. Older clients cannot use the replacement notification API on an
upgraded server. After the 0.5.0 contract ships, new signal variants are
additive.

The legacy server and room policy operations remain available, while the new
policy service adds explicit server, room-group, and room scopes. Deprecated
delivery names and followed-room slots remain readable so old stored values do
not acquire a new meaning. Current clients use Room messages at room scope
instead of the retired followed-room cause.

Older servers do not derive Badge or Room messages output and cannot apply
room-group policy. During rollback, those additions become temporarily
inactive instead of turning into another notification cause. Older clients
can render an unknown occurrence generically, but cannot infer its navigation
target. Deployments must replace all replicas before depending on the new
policy and occurrence behavior. Exact protobuf compatibility details belong in
the public schema and API compatibility guide.

## Permissions

Notification policy and triage are user-scoped. Current account, room,
applicable channel-room message-read authority, message/thread target, and
exact reaction visibility govern whether an occurrence may be listed, opened,
mutated, or delivered. DM membership authorizes DM occurrences. There is no
separate permission to manage another user's notification list.

## Related

- **ADRs:** ADR-012, ADR-028, ADR-036, ADR-038, ADR-051, ADR-069, ADR-076,
  ADR-077, ADR-080, ADR-082
- **FDRs:** FDR-001, FDR-002, FDR-004, FDR-005, FDR-006, FDR-007, FDR-011,
  FDR-013, FDR-018, FDR-019, FDR-027, FDR-038, FDR-039, FDR-044
