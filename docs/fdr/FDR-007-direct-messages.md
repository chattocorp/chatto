# FDR-007: Direct Messages

**Status:** Active
**Last reviewed:** 2026-08-25

## Overview

Users can start a private, one-to-one conversation with anyone they can see in a server. They can also start a self-DM for personal notes. DMs are rooms with `kind: dm`: they use the same message, reaction, attachment, notification, unread, and live-delivery machinery as channel rooms, while applying a smaller DM-specific creation and privacy policy. Each Chatto server has its own DM scope; there is currently no cross-server "unified DM inbox".

## Behavior

- A DM is started from user context menus inside the chat UI (member list clicks, @mention clicks, message author clicks).
- Starting a DM with another user navigates to the resulting two-person DM room. If that conversation already exists, the user lands in it rather than creating a duplicate.
- Users can start a DM with themselves. The product does not let users create group DMs, even though the underlying room model and public API can represent a larger fixed participant set.
- Starting a DM creates the durable room and participant memberships immediately so the complete composer is available, but the empty conversation stays out of every participant's navigation until its first message is sent.
- The bundled web client starts DMs through ConnectRPC `RoomService.StartDM`, which delegates to the shared core DM model.
- DM rooms appear in the per-server room sidebar with their participants' names and avatars rather than a room name.
- Active DM navigation uses message history to include and order DMs for their
  participants. Exhaustive authenticated state also retains membership-derived
  room metadata for routing.
- Inside a DM room, the room extras sidebar is available but starts closed and does not show the Members panel. The current Files panel and future non-member panels are shared, while channel-style moderation actions such as banning/removing room members remain unavailable.
- A user can read a DM only when they are a participant. `message.read` does
  not apply to DMs, and there is no `dm.*` read permission.
- Operators can prevent a user from starting new DMs or sending messages in existing DMs by revoking `message.post`.
- Operators cannot ban or remove participants from an existing DM room. Channel member bans are a `room.ban-member` action and are rejected for DMs.
- Inside a DM room, ordinary message-related features apply: posting, flat reply attribution, reactions, edits, deletes, mentions, and attachments.
- DMs do not support threads. The client does not offer thread actions, and the server rejects attempts to create or extend a DM thread even for owners. Thread data written by older versions remains readable but read-only.
- Server admins / moderators cannot moderate DM contents — `message.manage`, `room.manage`, and `message.echo` are unconditionally denied in DM rooms regardless of role grants. The channel-style `room.create` is also denied inside DMs; DMs have their own creation and membership APIs.

## Design Decisions

### 1. DMs are rooms, not a parallel messaging model

**Decision:** A DM is a room with `kind: dm`, not a separate entity type, inbox stream, or hidden space.
**Why:** Room infrastructure already models the hard parts: membership, messages, reactions, attachments, unread state, live delivery, and notification fan-out. Reusing the room aggregate keeps DMs boring and makes the event-sourced room model apply uniformly. See ADR-033, ADR-034, and ADR-037.
**Tradeoff:** Some room code still has to branch on `kind` for DM-only policy, but those branches should be about behavior (creation, privacy boundary, presentation), not storage or delivery plumbing.

### 2. Membership authorizes DM reads

**Decision:** DM membership authorizes complete DM reads for humans and bots.
`message.read` grants and denials do not change DM access. Starting DMs and
posting messages in them use `message.post`. Reply attribution does not change
that permission, and thread posting does not apply to DMs.
**Why:** Membership is the fixed private participant boundary. Chatto has no
permission UI for one DM, and hiding a DM from its participant is surprising.
See ADR-037 and ADR-080.
**Tradeoff:** Operators cannot use `message.read` to hide an existing DM from
one of its participants. They can revoke posting authority or suspend the
account.

### 3. Threads are channel-room-only

**Decision:** DMs support reply attribution in the room timeline but do not support thread containment. The prohibition is a room-kind invariant rather than an RBAC restriction, so owner permission overrides cannot bypass it. Historical DM threads remain readable for compatibility.
**Why:** A direct conversation is already a focused message timeline; branching it into parallel threads makes the conversation model and navigation unnecessarily complex. Enforcing the distinction in Core keeps every transport and privileged caller consistent.
**Tradeoff:** Older DM thread history can still be opened but cannot receive new replies. Users who need threaded discussions should use a channel room.

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
message. Self-DMs do not notify their author, and ordinary DMs default to Alert.

### 6. Moderation deny-list inside DMs

**Decision:** Even users with admin/moderator roles cannot edit others' messages, delete others' messages, or otherwise moderate inside a DM room. The deny-list is unconditional regardless of role.
**Why:** DMs are private by design. An admin who could moderate DMs would have a privacy boundary problem. Treating the deny as a static rule (not a configurable permission) prevents accidental misconfiguration.
**Tradeoff:** Genuine abuse inside DMs has no in-product moderation path — operators have to address it at the user level (suspend, kick from server) instead. See `dmBoundaryDeniedPermissions` in `permission_resolver.go`.

### 7. Empty rooms are latent conversations

**Decision:** Starting a DM creates its deterministic room and memberships immediately, but navigation surfaces show it only after the first root message. Once activated, retracting every message does not hide the conversation again.
**Why:** Early creation keeps routing, permissions, attachments, previews, and the ordinary room composer simple while avoiding an unsolicited empty conversation in another participant's UI.
**Tradeoff:** Empty DM rooms remain in durable history and authorized client state even though they are absent from navigation. This small amount of latent state avoids a separate draft-conversation model and disappears automatically from presentation after replay.

## Permissions

- `message.post` — start DMs and send messages in DM rooms.
- `message.react` — add and remove reactions in DM rooms.

DMs have no `dm.*` permissions. Membership authorizes reads. Message-action and
reaction permissions apply inside DM rooms subject to the moderation deny-list
above. `message.post-in-thread` does not apply to DMs regardless of the
viewer's effective permissions.

## Related

- **ADRs:** ADR-033 (event-sourced state), ADR-034 (single event stream), ADR-037 (DM access via membership), ADR-076 (deterministic notification occurrences), ADR-077 (persistent notification list), ADR-080 (explicit message-read permissions)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-012 (Notifications), FDR-039 (Message Access & Interactions)
