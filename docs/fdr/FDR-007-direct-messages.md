# FDR-007: Direct Messages

**Status:** Active
**Last reviewed:** 2026-09-04

## Overview

Human users can start a private, one-to-one conversation with anyone they can
see in a server. They can also start a self-DM for personal notes. DMs are
rooms with `kind: dm`. They use the same message, reaction, attachment,
notification, unread, and live-delivery machinery as channel rooms. They also
have a smaller DM-specific creation and privacy policy. Each Chatto server has
its own DM scope. Chatto does not have a cross-server DM inbox.

## Behavior

- A DM is started from user context menus inside the chat UI (member list clicks, @mention clicks, message author clicks).
- Starting a DM with another user navigates to the resulting two-person DM room. If that conversation already exists, the user lands in it rather than creating a duplicate.
- Human users can start a DM with themselves. The product does not let users create group DMs, even though the underlying room model and public API can represent a larger fixed participant set.
- A bot cannot call `RoomService.StartDM`. This rule applies to self-DMs, new
  DMs, group DMs, and DMs that already exist. A human must start a DM that
  includes a bot. The bot then needs explicit DM-scoped message grants.
- Starting a DM creates the durable room and participant memberships immediately so the complete composer is available, but the empty conversation stays out of every participant's navigation until its first message is sent.
- The bundled web client starts DMs through ConnectRPC `RoomService.StartDM`, which delegates to the shared core DM model.
- DM rooms appear in the per-server room sidebar with their participants' names and avatars rather than a room name.
- Active DM navigation uses message history to include and order DMs for their
  participants. Exhaustive authenticated state also retains membership-derived
  room metadata for routing.
- `RoomDirectoryService.ListRooms` returns accessible empty DMs as authorized
  room state. Each DM `RoomWithViewerState` contains its participant user IDs
  and an optional `has_message_history` value. Navigation hides a DM only when
  that value is explicitly false. An absent value is unknown and stays visible.
- Inside a DM room, the room extras sidebar is available but starts closed and does not show the Members panel. The current Files panel and future non-member panels are shared, while channel-style moderation actions such as banning/removing room members remain unavailable.
- A user can discover a DM only when they are a participant. The main timeline
  also needs `message.read`. An interaction thread can instead use
  `message.read-interactions` when the account authored the root or another
  account directly mentioned it in that thread.
- Operators can prevent a human user from creating new DMs, or any user from
  sending messages in existing DMs, by revoking `message.post`. A human user
  can still open an existing DM that they are a participant in.
- Operators cannot ban or remove participants from an existing DM room. Channel member bans are a `room.ban-member` action and are rejected for DMs.
- Inside a DM room, ordinary message features apply. This includes threads,
  follows, unread state, echoes, reactions, edits, deletes, mentions, and
  attachments. DMs always use Enabled threading behavior.
- A participant with `message.manage` can manage another participant's
  message. The permission does not expose a DM to a non-participant.
- All `message.*` permissions apply at the global Direct messages scope. No
  `room.*` permission applies there. DMs keep their dedicated creation and
  membership rules.

## Design Decisions

### 1. DMs are rooms, not a parallel messaging model

**Decision:** A DM is a room with `kind: dm`, not a separate entity type, inbox stream, or hidden space.
**Why:** Room infrastructure already models the hard parts: membership, messages, reactions, attachments, unread state, live delivery, and notification fan-out. Reusing the room aggregate keeps DMs boring and makes the event-sourced room model apply uniformly. See ADR-033, ADR-034, and ADR-037.
**Tradeoff:** Some room code still has to branch on `kind` for DM-only policy, but those branches should be about behavior (creation, privacy boundary, presentation), not storage or delivery plumbing.

### 2. Membership and message permissions authorize DM reads

**Decision:** Membership authorizes discovery and participant metadata.
`message.read` authorizes the main timeline and broad message-derived state.
`message.read-interactions` can authorize one derived interaction thread. A
human needs DM-scoped `message.post` to create a DM. Every participant needs it
to post a root in an existing DM.
**Why:** The fixed participant set remains the privacy boundary, while the
global Direct messages scope gives operators a useful content and automation
control without a permission object for each conversation. See ADR-091.
**Tradeoff:** A participant can know that a DM exists while its message content
and message-derived state are hidden.

### 3. DMs use fixed Enabled threading

**Decision:** Every DM behaves like an Enabled channel room for thread writes.
The room does not store a configurable Threading Mode. The existing thread
event model, follows, unread state, notifications, links, echoes, and My
Threads feed apply.
**Why:** One thread model lets people and DM-only bots keep structured private
conversations without channel access.
**Tradeoff:** Clients must identify DM rows from participant metadata instead
of a channel name.

### 4. Deterministic room IDs

**Decision:** A DM room ID is a hash of the sorted participant user IDs.
**Why:** Find-or-create needs to be cheap and race-free. Hashing the participant set gives a content-addressable ID, so the same two users always land in the same DM without a database lookup. The more general participant-set model remains an implementation capability rather than a product promise.
**Tradeoff:** DM membership is fixed at creation. The product has no way to add participants to a conversation or turn a one-to-one DM into a group conversation; users who need a shared conversation with more people use a channel room.

### 5. Per-server conversation scope with combined notifications

**Decision:** Each Chatto server's DM conversations are scoped to that server.
There is no cross-server conversation inbox. The client may still combine
notification occurrences from authenticated servers into its notification
page.
**Why:** A unified inbox was tried and removed. The complexity of cross-server aggregation (auth, real-time aggregation, navigation routing) outweighed the benefit for the current user base, which mostly works in one server at a time.
**Tradeoff:** Users in multiple servers switch servers to browse each DM
conversation. The notification page may combine exact DM occurrences from all
authenticated servers, but it is an attention list rather than a cross-server
conversation browser. Each incoming DM is one exact occurrence; the client may
group a conversation into one row while its badge still counts every unread
message. Self-DMs do not notify their author, and ordinary DMs default to Push
notification.

### 6. DM moderation requires participation

**Decision:** `message.manage` applies in DMs, but the normal membership check
remains mandatory. An effective owner does not receive access to a DM where
they are not a participant.
**Why:** A participant with delegated moderation authority can respond to
abuse without giving server operators global access to private conversations.
**Tradeoff:** Operators still need account-level action when no authorized
participant can manage the content.

### 7. Empty rooms are latent conversations

**Decision:** Starting a DM creates its deterministic room and memberships immediately, but navigation surfaces show it only after the first root message. Once activated, retracting every message does not hide the conversation again.
**Why:** Early creation keeps routing, permissions, attachments, previews, and the ordinary room composer simple while avoiding an unsolicited empty conversation in another participant's UI.
**Tradeoff:** Empty DM rooms remain in durable history and authorized client state even though they are absent from navigation. This small amount of latent state avoids a separate draft-conversation model and disappears automatically from presentation after replay.

### 8. Only humans can start DMs

**Decision:** A bot cannot call `RoomService.StartDM`. No permission or owner
override can change this rule. A human must start a DM that includes a bot.
**Why:** A bot must not create an unsolicited private conversation. A bot does
not need the start operation to participate after a human creates the DM.
**Tradeoff:** A bot cannot use the idempotent start operation to get the room
for an existing DM. It must learn the room from its normal room state or
realtime events.

## Permissions

- `message.post` — let a human create DMs, and let a human or bot send messages
  in existing DM rooms. It does not prevent a human participant from opening
  an existing DM.
- `message.read` — read the main DM timeline and all accessible DM message
  state.
- `message.read-interactions` — read complete DM threads that the account
  started or where another account directly mentioned it.
- `message.post-in-thread` — create a thread or post a thread reply.
- `message.echo` — echo a thread reply to the main conversation timeline. The
  action also needs `message.post`.
- `message.manage` — manage another participant's message while the manager is
  also a DM participant.
- `message.react` — add and remove reactions in DM rooms.

DMs have no `dm.*` permission names. They use the normal `message.*` names at
the Direct messages scope. No `room.*` permission applies at that scope.

## Related

- **ADRs:** ADR-033 (event-sourced state), ADR-034 (single event stream), ADR-037 (DM membership boundary), ADR-076 (deterministic notification occurrences), ADR-077 (persistent notification list), ADR-080 (explicit message-read permissions), ADR-093 (public realtime event union), ADR-095 (DM permission scope and threads)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-012 (Notifications), FDR-038 (Bot Accounts), FDR-039 (Message Access & Interactions), FDR-045 (Realtime Event Stream)
