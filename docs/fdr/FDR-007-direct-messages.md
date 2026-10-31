# FDR-007: Direct Messages

**Status:** Active
**Last reviewed:** 2026-10-25

## Overview

Human users can start a private, one-to-one conversation with anyone they can
see in a server. They can also start a self-DM for personal notes. DMs are
rooms with `kind: dm`. They use the same message, reaction, attachment,
notification, unread, and live-delivery machinery as channel rooms. They also
have a smaller DM-specific creation and privacy policy. Each Chatto server has
its own DM scope. Chatto does not have a cross-server DM inbox.

## Behavior

- A DM is started from user context menus inside the chat UI (member list clicks, @mention clicks, message author clicks, and clicks on user names in system events such as joins, leaves, and call starts, and on the avatar of a single-user system event).
- A user's page at `/chat/<server>/user/<userId>` shows their profile and the viewing user's direct conversation with them. Opening the page finds or creates that conversation. The URL keeps the target user's ID, including after account deletion.
- Thread and message links use the same user-page prefix. Old DM entry links and one-to-one room links redirect to the user page and preserve their destinations.
- Human and bot users can start a DM with themselves. New DMs have at most two distinct participants, including the caller. The API rejects new group DMs. Existing group conversations remain accessible through room URLs.
- Bots use the same DM creation and lookup rules as humans. Creation needs
  an explicit DM-scoped `message.post` grant within the owner's authority.
- Starting a DM creates the durable room and participant memberships immediately so the complete composer is available, but the empty conversation stays out of every participant's navigation until its first message is sent.
- The bundled web client starts DMs through ConnectRPC `RoomService.StartDM`, which delegates to the shared core DM model.
- Selecting **Send message** or a quick-switcher user result first opens that recipient's DM destination. The destination creates or finds the room, shows a loading state until room data is available, and then opens the conversation. A failed attempt stays at the destination with a retry action. Opening a user menu does not create a room.
- DM rooms appear in the per-server room sidebar with their participants' names and avatars rather than a room name.
- DMs are sorted by latest message time, newest first, including thread replies.
  A new message moves an older conversation into its current position without a
  page reload. **Show older** uses the same order. Unknown activity times follow
  known times; equal times keep their existing navigation order.
- The expanded DM section shows conversations with a message in the last seven
  days. Thread replies count. Unread conversations, conversations with
  notifications that need attention, the current conversation, and conversations
  with active calls stay visible regardless of age.
- If these rules show fewer than three conversations, the section adds the
  most recently active older conversations until three are visible. If fewer
  than three conversations have message history, it shows all of them.
- **Show older (N)** appears only when more conversations are hidden and shows
  those conversations. **Show less** restores the seven-day filter with the
  minimum of three conversations. This display choice is local to
  the sidebar and does not archive conversations. The Quick Switcher still
  includes older conversations. The collapsed section keeps its attention rules.
- When a DM has no notification badge, its row shows the elapsed time since
  the latest message, including replies: `now` for less than one minute, then
  `min`, `h`, or `d`. The unread dot
  and active call indicator remain visible. Unknown activity times have no age.
  The full date and time appear on hover. Narrow sidebar rows hide ages to
  leave more space for names; notification badges stay visible.
- A conversation with an unknown activity time stays visible. Edits, reactions,
  reads, and retractions do not change its posting activity.
- Right-click or touch long-press on a one-to-one DM sidebar row opens the
  other participant's user menu. A self-DM opens the current user's menu.
  These menus offer **Copy User ID**, with no room actions. **Send Message**
  opens the selected conversation when the viewer can start DMs. A deleted
  partner is shown as **[deleted user]**, with no **Send Message** action.
  Group DMs keep the conversation menu. If participant data is not ready,
  the row keeps the conversation menu until the user can be identified.
- The sidebar, room header, My Threads, and Quick Switcher show a self-DM
  with the current account's display name and a localized **You** badge when
  participant data is available. The badge uses the account ID, so text in
  another user's display name cannot create it.
- When a DM participant deletes their account, the account stays a DM
  participant. The other participant keeps the conversation and sees the
  deleted participant as **[deleted user]** with a deleted-user avatar. The DM
  is never shown as a self-DM and has no **You** badge. Posting rules do not
  change. The timeline keeps deleted-message placeholders, including those
  without attachments, previews, reactions, or thread replies. A notice above
  the timeline states that the other user has been deleted.
- Active DM navigation uses message history to include and order DMs for their
  participants. Exhaustive authenticated state also retains membership-derived
  room metadata for routing.
- `RoomDirectoryService.ListRooms` returns accessible empty DMs as authorized
  room state. Each DM `RoomWithViewerState` contains its participant user IDs,
  including deleted accounts, and an optional `has_message_history` value.
  Navigation hides an empty DM when that value is explicitly false. An absent
  value is unknown. The sidebar then applies its activity and attention rules.
  Readable DMs also include `last_message_at` for roots and replies and
  `last_root_message_at` for roots only. Both exclude historical imports and
  remain unchanged by edits and retractions. Servers without these fields
  remain connectable; clients do not interpret absent times as old activity.
- Inside a DM room, the room extras sidebar is available but starts closed and does not show the Members panel. The current Files panel and future non-member panels are shared, while channel-style moderation actions such as banning/removing room members remain unavailable.
- A user can discover a DM only when they are a participant. The main timeline
  also needs `message.read`. An interaction thread can instead use
  `message.read-interactions` when the account authored the root or another
  account directly mentioned it in that thread.
- Operators can prevent a human user from creating new DMs, or any user from
  sending messages in existing DMs, by revoking `message.post`. A human user
  can still open an existing DM that they are a participant in.
- Operators cannot remove participants from an existing DM room. Moderated channel-room removal uses `room.remove-member` and is rejected for DMs.
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
**Why:** Find-or-create needs to be cheap and race-free. Hashing the participant set gives a content-addressable ID, so the same two users always land in the same DM without a database lookup. Stored group conversations keep their IDs, but new conversations are limited to two participants.
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

### 8. Humans and bots use the same DM rules

**Decision:** Humans and bots can start DMs with `message.post`. Bots retain
explicit grants and the owner authority ceiling. Participants can fetch an
existing DM without posting permission.
**Why:** Automated workflows need to contact people directly. Restricting bot
accounts does not prevent spam from human credentials.
**Tradeoff:** A DM posting grant also permits initiation. There is no separate
approval step for bots.

### 9. Deleted accounts stay DM participants

**Decision:** A deleted account stays a DM participant. The room directory
ignores `UserLeftRoom` for DM rooms. Account deletion still records DM leave
facts, which end the account's DM call participation, but they do not remove
the participant, in current or replayed state. DM participant IDs include
deleted accounts. The realtime snapshot sends deleted participants as
deleted-user tombstones, and followed-thread and search includes resolve them
to deleted users.
**Why:** DM membership is fixed at creation (Decision 4), and `LeaveRoom`
already rejects DMs. When deletion removed the membership, a one-to-one DM with
a deleted account had the same member list as the remaining participant's
self-DM. The UI then showed the viewer's own name and a **You** badge for
someone else's conversation.
**Tradeoff:** A deleted account remains in DM membership indexes. Read paths
that list active accounts already omit deleted users. The conversation stays
writable; a read-only DM state is not part of this decision.

### 10. Derive sidebar visibility from activity

**Decision:** Use a fixed seven-day window and existing attention state, with
a minimum of three conversations when available.
Keep the complete conversation directory available to navigation and search.
**Why:** This reduces long sidebar lists without a per-conversation archive
choice that must be stored and shared across clients. Seven days includes a
normal working week. The minimum keeps recent contacts available when no
conversation has activity in that window.
**Tradeoff:** Old unread conversations remain visible. The cutoff is not
configurable.

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
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-012 (Notifications), FDR-018 (Account Lifecycle), FDR-038 (Bot Accounts), FDR-039 (Message Access & Interactions), FDR-045 (Realtime Event Stream)
