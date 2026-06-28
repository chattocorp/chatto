# FDR-007: Direct Messages

**Status:** Active
**Last reviewed:** 2026-06-26

## Overview

Users can start a direct conversation (1-to-1 or small group, up to 10 participants) with anyone they can see in a server. DMs are rooms with `kind: dm`: they use the same message, thread, reaction, attachment, notification, unread, and live-delivery machinery as channel rooms, while applying a smaller DM-specific creation and privacy policy. Each Chatto server has its own DM scope; there is currently no cross-server "unified DM inbox".

## Behavior

- A DM is started from user context menus inside the chat UI (member list clicks, @mention clicks, message author clicks).
- Starting a DM with a user (or set of users) navigates to the resulting DM room. If a DM with the same participant set already exists, the user lands in that room rather than creating a duplicate.
- The bundled web client starts DMs through ConnectRPC `RoomService.StartDM`, which delegates to the shared core DM model.
- DM rooms appear in the per-server room sidebar with their participants' names and avatars rather than a room name.
- Inside a DM room, the room extras sidebar is available but starts closed and does not show the Members panel. The current Files panel and future non-member panels are shared, while channel-style moderation actions such as banning/removing room members remain unavailable.
- Maximum 10 participants per DM.
- A user can read a DM if and only if they are a participant in that DM room. There is no separate "can view DMs" permission.
- Operators can prevent a user from starting new DMs or sending root messages in existing DMs by revoking `message.post`; thread replies follow `message.post-in-thread`.
- Operators cannot ban or remove participants from an existing DM room. Channel member bans are a `room.ban-member` action and are rejected for DMs.
- Inside a DM room, ordinary message-related features apply: posting, replies, threads, reactions, edits, deletes, mentions, attachments.
- Server admins / moderators cannot moderate DM contents — `message.manage`, `room.manage`, and `message.echo` are unconditionally denied in DM rooms regardless of role grants. The channel-style `room.create` is also denied inside DMs; DMs have their own creation and membership APIs.

## Design Decisions

### 1. DMs are rooms, not a parallel messaging model

**Decision:** A DM is a room with `kind: dm`, not a separate entity type, inbox stream, or hidden space.
**Why:** Room infrastructure already models the hard parts: membership, messages, threads, reactions, attachments, unread state, live delivery, and notification fan-out. Reusing the room aggregate keeps DMs boring and makes the event-sourced room model apply uniformly. See ADR-033, ADR-034, and ADR-037.
**Tradeoff:** Some room code still has to branch on `kind` for DM-only policy, but those branches should be about behavior (creation, privacy boundary, presentation), not storage or delivery plumbing.

### 2. Reading is membership-based, writing uses message permissions

**Decision:** DM read access comes only from room membership. Starting DMs and posting root messages in them use `message.post`; thread replies use `message.post-in-thread`.
**Why:** A user who is a participant in a private conversation should be able to read that conversation, and sending a DM is still just sending a message. Reusing the message permissions avoids a lonely DM-only permission while preserving the abuse-control lever. See ADR-037.
**Tradeoff:** There is no soft "hide DMs from this user" switch, and revoking `message.post` blocks channel posting as well as DM starts / root posts. Operators who need a total messaging timeout can use that; finer abuse controls should be modeled separately if needed.

### 3. Deterministic room IDs

**Decision:** A DM room ID is a hash of the sorted participant user IDs.
**Why:** Find-or-create needs to be cheap and race-free. Hashing the participant set gives a content-addressable ID — starting a DM with the same group always lands in the same room without a database lookup.
**Tradeoff:** Adding or removing a participant from a DM would change the room ID, which means group membership is fixed at creation. Acceptable: in practice, group DMs are short-lived and re-creating with the new set is fine. Users who need a different participant set start a new DM.

### 4. Per-server scope (no unified inbox)

**Decision:** Each Chatto server's DMs are scoped to that server. There's no cross-server aggregation that shows "all your DMs across all the servers you're connected to" in one inbox.
**Why:** A unified inbox was tried and removed. The complexity of cross-server aggregation (auth, real-time aggregation, navigation routing) outweighed the benefit for the current user base, which mostly works in one server at a time.
**Tradeoff:** Users in multiple servers have to switch servers to see DMs in each. If a unified inbox is reintroduced, this FDR needs a rewrite.

### 5. Moderation deny-list inside DMs

**Decision:** Even users with admin/moderator roles cannot edit others' messages, delete others' messages, or otherwise moderate inside a DM room. The deny-list is unconditional regardless of role.
**Why:** DMs are private by design. An admin who could moderate DMs would have a privacy boundary problem. Treating the deny as a static rule (not a configurable permission) prevents accidental misconfiguration.
**Tradeoff:** Genuine abuse inside DMs has no in-product moderation path — operators have to address it at the user level (suspend, kick from server) instead. See `dmBoundaryDeniedPermissions` in `permission_resolver.go`.

## Permissions

- `message.post` — start DMs and send root messages in DM rooms.
- `message.post-in-thread` — send thread replies in DM rooms.
- `message.react` — add and remove reactions in DM rooms.

DMs have no `dm.*` permissions. Message and reaction permissions apply inside DM rooms subject to the moderation deny-list above.

## Related

- **ADRs:** ADR-033 (event-sourced state), ADR-034 (single event stream), ADR-037 (DM access via membership)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-012 (Notifications)
