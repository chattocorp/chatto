# FDR-012: Notifications

**Status:** Experimental
**Last reviewed:** 2026-08-11

> **Implementation status:** Implemented for the upcoming 0.5.0 release by
> [#1556](https://github.com/chattocorp/chatto/issues/1556), using the documented
> clean cutover from legacy notification records.

## Overview

Notifications are a persistent, user-scoped inbox for activity that deserves
attention. They cover direct messages, replies, mentions, followed
conversations, reactions, and future attention causes. The inbox is modeled
after GitHub notifications: users can keep read items, dismiss them to Done
without opening them, or delete them. Related activity is grouped without
losing the exact events and reasons underneath.

## Behavior

- The notification page is one chronological list. Unread and Read groups use
  the normal treatment; Done groups remain in place as subdued handled history.
- Opening a notification navigates to the exact room, thread, and event. It
  marks that notification read; the room or thread becomes read only when the
  target is actually displayed.
- Mark Read and Mark Unread organize the notification inbox without changing
  the room or thread read cursor.
- Reading a room or thread marks covered notifications read. For a reaction,
  coverage follows the reacted-to message rather than the later reaction time.
  It does not remove notifications from Inbox.
- The check action marks a notification or group Done without requiring the
  user to open or otherwise handle its source activity. It remains reviewable
  in the same list with subdued styling.
- Checking a Done item returns it to Inbox as Read. Delete removes it from the
  visible list entirely.
- Notifications expire 90 days after their source activity. Read, Done, and
  other updates never extend that absolute lifetime.
- Related occurrences are grouped by conversation or target: DM room, thread,
  reacted-to message, or channel room. Later activity makes the grouping target
  appear as active again while its earlier Done items remain subdued.
- A group opens its newest unread occurrence, or its newest occurrence when all
  members are read. Individual occurrences retain exact destinations.
- The bell count is the number of unread groups. Group rows may show how many
  occurrences they contain.
- Retraction, reaction removal, lost room visibility, and account deletion
  remove notifications that the user can no longer act on or view. List and
  mutation requests validate the exact current target after waiting local
  projections through freshly captured recipient and server-wide room-event
  boundaries. List validation scans only far enough to fill the requested
  offset page and validates each page-sized overfetch chunk once when stale
  groups are removed. The ordered writer also reconciles effective membership
  after universal-room, room-group placement, and relevant RBAC/role changes,
  including visibility loss without an explicit leave. A snapshot-capable
  Notification Visibility projection retains effective membership at each
  pending change's exact EVT boundary, so a later regain that reaches ordinary
  projections first cannot preserve pre-loss history or remove post-regain
  activity. Snapshot restore stops at the worker's acknowledged floor and
  replays only its pending tail. Pending facts share one full checkpoint plus a
  compact event-delta journal, and exact boundary data remains available until
  the consumer's acknowledgement is confirmed; an administrative fact never
  copies the full visibility graph or replays lifetime membership/RBAC history
  on the notification lane. Snapshot publication pauses whenever a captured
  generation would cross the worker's full acknowledged floor, including when
  a non-boundary notification fact is pending. This preserves the last safe
  restore point instead of rotating it away. Before exhaustive totals and badge summaries are read,
  Chatto waits that writer through a captured tail of every relevant EVT filter,
  appends a read fence to `RUNTIME_STATE`, then waits the serving replica's
  occurrence index through that fence's KV revision. Temporary projection,
  worker, or replica-watcher lag is therefore never interpreted as permanent
  visibility loss or an authoritative stale count.
- Inbox state, groups, counts, sounds, Web Push, and installed-app badges
  reconcile from authoritative server state after reconnect. Missing one live
  update cannot leave the client permanently wrong.

## Notification Policy

Every supported cause has an independent delivery intensity:

- **Off** — do not create a notification occurrence for this cause.
- **Badge** — create an occurrence and update inbox/badges without an
  interruptive sound, Web Push, or native notification.
- **Alert** — create the same occurrence and allow configured interruptive
  delivery.

Preferences inherit independently for each cause from the Chatto product
default, through the user's server-level preference, to an optional room-level
override. A user can return any override to Inherit. Effective values are
computed by the server; clients do not reproduce policy evaluation.

The initial product defaults are:

| Cause | Default intensity |
| --- | --- |
| Direct message | Alert |
| Direct username mention | Alert |
| Reply to the user's message | Alert |
| Mention of a role the user belongs to | Alert |
| `@here` | Alert |
| `@all` | Alert |
| New activity in a followed thread | Badge |
| New activity in a followed room | Off |
| Reaction to the user's message | Badge |

Room invitations are not an implemented notification cause and therefore do
not appear in policy responses or settings. The protobuf reason value is kept
for future additive support, but cannot be persisted as a preference today.

Direct username mentions, role mentions, `@here`, and `@all` remain separate
causes. A message can match several causes for one recipient, but it produces
one occurrence containing every matched reason and uses the strongest effective
intensity. The user's own activity does not notify them.

Notification policy affects future activity. Changing a preference does not
mark content read, rewrite existing notification intensity, or erase inbox
history. Do Not Disturb and other temporary delivery conditions may silence an
Alert while preserving its occurrence for later review.

## Conversation Subscriptions

- Posting in a thread follows it. A delivered direct username mention follows
  the thread unless the recipient previously opted out; role, `@here`, and
  `@all` mentions do not implicitly follow it.
- Following a thread or room establishes an ambient activity source whose
  delivery intensity is still controlled by notification policy.
- Conversation subscriptions are changed through their owning room or thread
  controls, not through the initial notification-inbox API.

## Design Decisions

### 1. A triageable inbox replaces delete-on-read pending alerts

**Decision:** Notification occurrences have Unread, Read, or Done inbox state;
Delete is explicit.
**Why:** Users need to review a read notification later and dismiss noise
without pretending they opened it. This follows the useful parts of GitHub's
Inbox/Done model. See ADR-071.
**Tradeoff:** The inbox contains more state than a delete-only list, and the
single list must merge and paginate active and Done groups without making their
different states ambiguous.

### 2. Content read state and inbox triage are separate

**Decision:** Room/thread read cursors can mark covered occurrences read, but
notification actions do not advance content read state until the target is
actually displayed.
**Why:** “I cleared this alert” and “I read this conversation through here” are
different claims. Keeping them separate closes accidental read receipts and
allows direct Inbox cleanup. See ADR-028 and ADR-071.
**Tradeoff:** A user can intentionally mark a notification read while its room
still has unread messages.

### 3. One occurrence records every reason

**Decision:** One source event produces at most one occurrence per recipient.
It records all matched reasons and uses their strongest effective intensity.
**Why:** A followed-thread reply that also mentions the recipient is one piece
of activity, not two alerts. Retaining all reasons makes the decision
explainable and prevents independent fanout paths from disagreeing. See
ADR-070.
**Tradeoff:** Policy evaluation must gather every cause before committing the
occurrence instead of stopping after the first match.

### 4. Delivery intensity is independent per cause

**Decision:** Each cause inherits an Off, Badge, or Alert value through server
and room scopes.
**Why:** The legacy Muted/Normal/All Messages level combines too many choices.
A user may want direct mentions to alert, reactions to appear silently, and
ambient room activity off in the same room.
**Tradeoff:** The settings UI becomes a matrix. Presets and inheritance cues
must keep the common case understandable.

### 5. Notification policy no longer hides ordinary unread rooms

**Decision:** Turning notification causes Off suppresses new notification
occurrences but does not suppress ordinary room unread state.
**Why:** Read state describes unseen content; notification policy describes how
strongly to surface selected activity. Coupling them makes “quiet but still
unread” impossible. See ADR-071.
**Tradeoff:** This deliberately retires the legacy behavior where Muted also
removed the room's unread indicator.

### 6. Groups are derived from exact occurrences

**Decision:** DM, thread, reaction, and room groups are presentation resources
derived from member occurrences rather than independently mutable canonical
records.
**Why:** Grouping should reduce inbox noise without creating a second lifecycle
that can drift from exact targets. A group-level action updates the members
present at its authoritative boundary; later activity remains new. See ADR-071.
**Tradeoff:** Group assembly and group actions require an indexed membership
view and explicit concurrency semantics. Because later activity can reuse a
derived group ID, group mutations are not safe for automatic retries after an
ambiguous transport failure; their responses are bounded affected-count
acknowledgements and realtime delivery is coalesced to one invalidation.

### 7. Notifications retain references, not presentation copies

**Decision:** Occurrences retain stable source, reason, actor, and destination
IDs. Names, avatars, message text, and room presentation are hydrated from
current visible resources.
**Why:** Copied presentation becomes stale and can outlive authorization or
content deletion. Exact references are sufficient to navigate and reconcile.
See ADR-070.
**Tradeoff:** Rendering a notification depends on current projection hydration;
an inaccessible or deleted target removes the occurrence rather than showing a
stale preview.

### 8. Ninety days is an absolute lifetime

**Decision:** Every occurrence, including Done items, expires 90 days
after the source activity. Mutations do not reset the clock.
**Why:** The inbox is bounded attention state, not a permanent activity archive.
The limit gives predictable storage and privacy behavior while retaining three
months of useful history. See ADR-070.
**Tradeoff:** Notifications cannot be retained as a permanent personal archive.

### 9. Persistent state precedes interruptive delivery

**Decision:** Sounds, Web Push, native notifications, and installed-app badges
are driven from a committed occurrence evaluated as Alert. Badge occurrences
stay silent, and Off creates nothing.
**Why:** Every interrupt should correspond to something the user can find in
the app, and every surface should reflect the same policy decision. See
ADR-070 and FDR-013.
**Tradeoff:** Delivery waits for durable occurrence creation and may be delayed
while the notification worker catches up.

### 10. Policy and subscription changes affect future activity only

**Decision:** Changing a cause intensity or conversation subscription does not
retroactively rewrite existing occurrences.
**Why:** Existing notifications explain decisions made when their activity
occurred. Silent retroactive cleanup would make inbox history unpredictable.
**Tradeoff:** After turning a cause Off, users may still need to triage older
items from that cause.

### 11. Notifications 2.0 is a clean replacement

**Decision:** The grouped inbox and per-cause policy replace the legacy pending
notification list and coarse Muted/Normal/All Messages preferences at one
release boundary. Existing pending rows and coarse preferences are not migrated
or interpreted.
**Why:** Maintaining two APIs, stores, and policy systems would make it unclear
which state is authoritative and would preserve the limitations this redesign
exists to remove. Historical persisted event variants remain decode-only so EVT
replay stays valid. See ADR-070.
**Tradeoff:** The 2.0 inbox starts empty after upgrade, prior preference choices
must be set again, and older clients cannot use notifications on the upgraded
server.

## Permissions

Notification policy and inbox triage are user-scoped and require no RBAC
permission. Visibility of the source room, message, thread, actor, or reaction
still governs whether the notification can be listed and opened.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-028 (event-ID-keyed read state), ADR-036 (runtime state in `RUNTIME_STATE`), ADR-038 (room-owned thread state), ADR-051 (server-scoped resumable client projection), ADR-070 (deterministic notification occurrences), ADR-071 (triageable notification inbox)
- **FDRs:** FDR-002 (Replies & Threads), FDR-005 (Reactions), FDR-006 (@Mentions), FDR-007 (Direct Messages), FDR-013 (Web Push Notifications)
