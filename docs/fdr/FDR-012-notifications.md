# FDR-012: Notifications

**Status:** Experimental
**Last reviewed:** 2026-08-23

## Overview

Notifications are a persistent, user-scoped list of exact activity that
deserves attention. They cover direct messages, replies, direct and role
mentions, `@here`, `@all`, followed conversations, and reactions. They borrow
GitHub's durable-history idea but use a smaller lifecycle: Unread, Read, or
explicitly Deleted.

The server records occurrences individually. The bundled frontend may
consolidate them for presentation without changing occurrence identity, jump
targets, unread counts, read state, or deletion semantics.

## Behavior

- The notification page is one chronological list containing both Unread and
  Read activity. Unread reactions are Ambient and use a neutral treatment;
  every other current cause is Important and uses Chatto's notification
  orange. Read rows are visually muted while remaining fully interactive. The
  list does not use a separate unread dot on each row.
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
- Trash deletes the exact visible occurrences represented by the current row.
  Dismiss All deletes every visible occurrence current at the server boundary.
  Both actions update the UI optimistically and then reconcile with the server.
- Every occurrence leaves application-visible state exactly 90 days after its
  source activity. Reading or deleting it does not extend that lifetime.
  Physical cleanup may continue during ADR-076's 24-hour grace period without
  extending user-visible retention.
- The combined multi-server list preserves healthy results when another server
  fails and exposes the failure as partial.
- Notification delivery rules and client sound choices are User Preferences.
  The server saves delivery rules. The client saves sound and sound-filter
  choices for each server. Notifications from different registered servers can
  use different sounds.

## Design Decisions

### 1. Exact occurrences, client-side grouping

**Decision:** The server exposes one occurrence for each recipient, source
activity, and notification cause. List and badge totals count exact unread
occurrences, independently of pagination and presentation grouping. The bundled
frontend groups DMs by conversation, reactions by reacted-to target, followed
activity by thread or room, and leaves mentions and replies separate because
they have distinct jump targets.

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

**Decision:** Each configurable notification signal class resolves independently
through a product default, a user/server override, and an optional room
override:

- **Off** — create no occurrence for this cause.
- **Silent** — create an occurrence without interruptive delivery.
- **Alert** — create the same occurrence and make it eligible for sound, Web
  Push, or native delivery.

| Cause                          | Default |
| ------------------------------ | ------- |
| Direct message                 | Alert   |
| Direct username mention        | Alert   |
| Reply to the user's message    | Alert   |
| Role mention                   | Alert   |
| `@here`                        | Alert   |
| `@all`                         | Alert   |
| Followed thread activity       | Silent  |
| Followed room activity         | Off     |
| Reaction to the user's message | Silent  |

Attention level controls presentation separately: reactions are Ambient and all
other current causes are Important. Bell, server, room, and app indicators use
notification orange when at least one contributing unread occurrence is
Important and a neutral treatment when every contributing occurrence is
Ambient. Attention levels are not user-configurable in this iteration.

**Why:** Whether activity is stored, whether it may interrupt, and how strongly
it is presented are different choices. Keeping them separate leaves room for
future per-cause configuration without changing occurrence identity.

**Tradeoff:** More than one policy dimension exists conceptually, although the
current product exposes only delivery-mode preferences.

### 4. Source-time decisions are durable and replayable

**Decision:** Notification recipients and effective source-time policy are
derived asynchronously from committed domain facts. Later membership,
preference, or follow changes do not rewrite that historical decision. A user's
own activity does not notify them. One source activity produces at most one
occurrence per recipient and cause; a message that is both a reply and a direct
mention intentionally creates two occurrences.

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
target. Removed reactions, retracted targets, deleted rooms, and lost room
access remove the corresponding occurrence. Durable visibility-loss boundaries
prevent old queued activity from reappearing after a quick regain of access.
Actor identity is hydrated from current account data; an unavailable or deleted
actor does not by itself expose copied profile data or make an otherwise valid
occurrence invisible.

**Why:** Source-time eligibility explains why the notification was created, but
it cannot override present-day privacy and target existence.

**Tradeoff:** An occurrence can disappear without direct user triage, and reads
must coordinate with current authorization state before reporting absence or
success.

### 6. Realtime delivery is a convergence hint

**Decision:** Realtime notification updates tell clients to replace their
finite notification view from authoritative server state. Unread totals remain
exact even when rows are grouped. The client also performs quiet periodic
reconciliation so a lost transient update cannot leave counts stale
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

### 8. Conversation subscriptions are distinct from notification rows

**Decision:** Posting in a thread attempts to follow it, even after an earlier
unfollow. A delivered direct username mention in a thread attempts to follow it
unless the recipient previously opted out; role, `@here`, and `@all` mentions do not. Following a thread or room
creates an activity source whose delivery is still controlled by notification
policy. Follow controls belong to rooms and threads, not to notification rows.

**Why:** A subscription describes future interest in a conversation; a
notification occurrence describes one past activity. Keeping them separate
avoids giving list triage surprising subscription side effects.

**Tradeoff:** Automatic follow is best-effort after the source message commits;
failure can omit later followed-activity notifications until the user follows
explicitly.

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
client keeps a small local-storage entry for each server. The server-synced
delivery rules continue to control whether an Alert can request sound.

## Compatibility

Notifications 2.0 supersedes Notifications 1.0 at the 0.5.0 pre-1.0 boundary.
Legacy records and coarse Muted/Normal/All Messages preferences are not migrated
or interpreted. Historical persisted event variants remain replay-decodable,
but current code adds no notification facts to `EVT`. Older clients cannot use
the replacement notification API on an upgraded server. After the 0.5.0
contract ships, new signal variants are additive.

## Permissions

Notification policy and triage are user-scoped. Current account, room,
message/thread target, and exact reaction visibility govern whether an
occurrence may be listed, opened, mutated, or delivered. There is no separate
permission to manage another user's notification list.

## Related

- **ADRs:** ADR-012, ADR-028, ADR-036, ADR-038, ADR-051, ADR-069, ADR-076,
  ADR-077
- **FDRs:** FDR-001, FDR-002, FDR-004, FDR-005, FDR-006, FDR-007, FDR-011,
  FDR-013, FDR-018, FDR-019, FDR-027
