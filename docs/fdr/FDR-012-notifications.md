# FDR-012: Notifications

**Status:** Experimental
**Last reviewed:** 2026-08-13

> **Implementation status:** Implemented for the upcoming 0.5.0 release by
> [#1556](https://github.com/chattocorp/chatto/issues/1556), with a clean
> replacement of legacy notifications.

## Overview

Notifications are a persistent, user-scoped list of exact activity that
deserves attention. They cover direct messages, replies, direct and role
mentions, `@here`, `@all`, followed conversations, and reactions. They borrow
GitHub's durable-history idea but use a smaller lifecycle: Unread, Read, or
explicitly Deleted.

The server emits and stores occurrences individually. The bundled frontend may
consolidate them for presentation without changing occurrence identity, jump
targets, unread counts, read state, or deletion semantics.

## Behavior

- The notification page is one chronological list containing both Unread and
  Read activity. Unread reactions are Ambient and use a neutral indicator;
  every other current cause is Important and uses Chatto's notification
  orange. Read rows use neither unread treatment and their content is visually
  muted while remaining fully interactive.
- The list is divided into Today, Yesterday, This Week, and month sections
  using the preferred time zone of the account on each server.
- Rows use concise, full localized sentences without message previews. Reaction
  rows show the emoji that were given.
- Opening a row navigates to the chosen occurrence's exact room, thread, and
  event. The occurrence is marked Read only after the target is displayed.
- Reading a room or thread marks covered occurrences Read. A reaction is
  covered according to the reacted-to message and reaction horizon.
- Notifications cannot be marked Unread. There is no Done state or Inbox/Done
  split.
- Trash deletes the exact visible occurrence IDs represented by the current
  row. Dismiss All deletes every visible occurrence current at the server
  boundary. Both are optimistic; a failed server restores only that server's
  rows.
- Every occurrence and deletion tombstone expires exactly 90 days after source
  activity. Mutations never extend the lifetime.
- The combined multi-server list preserves healthy results when another server
  fails and exposes the failure as partial.

## Exact occurrences and frontend consolidation

`ListNotificationOccurrences` and the realtime notification replacement expose
individual newest-first occurrences. Totals are exact and independent of list
pagination or presentation grouping:

- each unread occurrence contributes one to the bell/server/app badge;
- each unread Important occurrence contributes one to the exact Important
  count that selects orange rather than the neutral Ambient treatment;
- each unread occurrence contributes one to its room's notification count;
- two unread DMs consolidated into one row still display a badge count of two.

The bundled frontend derives temporary groups as follows:

- DMs group by conversation room;
- reactions group by reacted-to target and consolidate actors and emoji;
- followed-thread activity may group by thread root;
- followed-room activity may group by room; and
- mentions and replies remain separate for each exact jump target.

A presentation group opens its newest unread occurrence, or newest occurrence
when all are Read. It contains the exact member IDs used by the idempotent batch
delete API. Groups are not persisted or transmitted and may evolve per client.
The bundled list does not display a redundant count of one. A group uses the
strongest attention level among its unread occurrences.

Bell, server, and room indicators remain visible for any unread occurrence.
They use notification orange when at least one contributing occurrence is
Important and a neutral treatment when every contributing occurrence is
Ambient. Attention levels are fixed by cause in this iteration and are not
user-configurable.

## Notification policy

Every cause independently inherits one delivery intensity through product
default, user/server override, and optional room override:

- **Off** — create no occurrence for this cause.
- **Badge** — create an occurrence without interruptive delivery.
- **Alert** — create the same occurrence and make it eligible for sound, Web
  Push, or native delivery.

| Cause | Default |
| --- | --- |
| Direct message | Alert |
| Direct username mention | Alert |
| Reply to the user's message | Alert |
| Role mention | Alert |
| `@here` | Alert |
| `@all` | Alert |
| Followed thread activity | Badge |
| Followed room activity | Off |
| Reaction to the user's message | Badge |

One source event produces at most one occurrence per recipient, even when it
matches several causes. The occurrence records all matches and uses the
strongest source-time intensity. A user's own activity does not notify them.
Policy changes affect future activity and never rewrite existing history.
Delivery intensity controls interruption only; it does not determine whether
an occurrence is visually Ambient or Important.

## Durable derivation and push delivery

Source-command OCC attempts recompute recipients, mention expansion, and
policy after waiting the current Config projection. They stage exact temporary
work in `RUNTIME_STATE` before appending the existing message or reaction fact.
No notification-only fact is added to `EVT`.

The shared `chatto-notification-materializer-v2` durable consumer applies
occurrence creation and lifecycle cleanup in source order. It handles
retraction, reaction removal, explicit and implicit visibility loss, room
deletion, and account deletion. The Notification Visibility projection retains
the event-time room/group/RBAC boundary needed to prevent a quick regain from
preserving pre-loss notification content.

An eligible Alert is published as an opaque occurrence coordinate to the
file-backed `NOTIFICATIONS_QUEUE`. The
`chatto-notification-alert-delivery-v1` consumer uses the shared durable-worker
runtime. Before provider delivery it fences materialization and current policy,
then revalidates unread state, target/reaction visibility, subscription
ownership, and DND. A current preference may downgrade an already queued Alert
but never upgrades an occurrence that was source-time Badge.

The queue and its consumer are included in normal backups. This preserves
accepted work across restore; excluding it would silently drop valid alerts at
an arbitrary backup boundary. A strict two-minute stream and worker age horizon
prevents restored jobs from producing stale interruptions. Delivery is at
least once, so a crash after provider acceptance can still duplicate a push.

## Visibility and consistency

List, realtime, mark-read, and delivery paths fence the materializer plus
current user, room, room-group-layout, and RBAC projections before treating
absence or access loss as authoritative. Delete operates only on opaque
occurrence IDs scoped to the authenticated viewer and does not hydrate target
content. The complete retained occurrence set is validated before exact totals
are returned, including rows outside the requested page. A `RUNTIME_STATE` read
fence then makes replica-local occurrence-index lag fail or wait instead of
exposing stale state.

Occurrences retain references and cause provenance, not copied room names,
profiles, or message bodies. Public assembly hydrates current visible data.
Retracted targets, removed reactions, and inaccessible rooms are tombstoned
instead of leaking stale presentation.

## Conversation subscriptions

- Posting in a thread follows it.
- A delivered direct username mention follows the thread unless the recipient
  previously opted out; role, `@here`, and `@all` mentions do not.
- Following a thread or room establishes an activity source whose intensity is
  still controlled by notification policy.
- Subscription controls belong to rooms and threads, not notification rows.

## Compatibility

Notifications 2.0 supersedes Notifications 1.0 at one pre-1.0 release boundary.
Legacy records and coarse Muted/Normal/All Messages preferences are not
migrated or interpreted. Historical persisted event variants remain
replay-decodable, but new code adds no notification facts to `EVT`. Older
clients cannot use the replacement notification API on an upgraded server.

## Permissions

Notification policy and triage are user-scoped. Current permission to see the
source room, message, thread, actor, and reaction still governs whether an
occurrence may be listed, opened, or delivered.

## Related

- **ADRs:** ADR-012, ADR-028, ADR-036, ADR-038, ADR-051, ADR-069, ADR-073,
  ADR-074
- **FDRs:** FDR-002, FDR-005, FDR-006, FDR-007, FDR-013
