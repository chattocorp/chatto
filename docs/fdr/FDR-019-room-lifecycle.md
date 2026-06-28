# FDR-019: Room Lifecycle

**Status:** Active
**Last reviewed:** 2026-06-25

## Overview

A channel room goes through a lifecycle of create, edit, archive, unarchive, and delete. Each transition is permission-gated and (where appropriate) audit-logged. This FDR focuses on channel rooms — DM room lifecycle is much simpler and lives in FDR-007.

## Behavior

- **Create** — server admins (or anyone with `room.create` in the target group) create a channel room by giving it a name (1–30 chars, alphanumeric / hyphen / underscore, case-insensitive unique across the server), an optional description, a room group, and optionally the Universal setting.
- **Edit** — `room.manage` holders can change the name, description, group, and Universal setting of an existing channel room.
- **Display** — when set, the optional description appears after the channel room name in the desktop room pane header.
- **Universal** — a channel room with Universal enabled behaves as joined for every server member who is currently eligible to join it. The system does not fan out `UserJoinedRoomEvent` facts for implicit membership. Existing explicit memberships remain intact, so disabling Universal restores the prior explicit membership set.
- **Bootstrap defaults** — fresh servers seed `#announcements` as Universal and `#general` as a normal channel room in the default Lobby group.
- **Join / leave** — joining a Universal room succeeds without writing an explicit membership event. Leaving a Universal room is rejected; users should mute it instead. DMs cannot be Universal.
- **API surface** — ConnectRPC `RoomService` exposes create, edit, archive, unarchive, Universal, join, leave, ban, and unban commands. ConnectRPC `RoomDirectoryService` exposes the complementary room list, room-group/sidebar list, single-room refresh, per-room viewer capability state, and group join-all command.
- **Archive** — `room.manage` toggles an `archived` flag on the room. Archived rooms vanish from the sidebar, the Browse Rooms page, and search results, but members stay joined and history is intact. The owner can still navigate to the room directly.
- **Unarchive** — same permission, flips the flag back. The room reappears in the sidebar and discovery surfaces.
- **Ban member** — `room.ban-member` holders can ban a user from a channel room with a required reason and optional expiry. The banned user loses room read/write/live access immediately and cannot rejoin until the ban is removed or expires.
- **Delete** — `room.manage` appends `RoomDeletedEvent` to `EVT`, releases the room from its group layout, and causes projections to remove the room, its name claim, and its memberships.
- Moving a room between groups requires `room.manage` in both groups (see FDR-017).

## Design Decisions

### 1. Room name uniqueness via EVT projection and OCC

**Decision:** Room names are unique server-wide (case-insensitive). Uniqueness is enforced by checking a room catalog projection snapshot and appending name-changing room events with wildcard OCC against the room aggregate event set.
**Why:** Race-tolerant name claiming is the only way to safely handle two operators creating the same-named room at the same moment. EVT OCC lets the event log remain the source of truth without maintaining a legacy KV name mirror.
**Tradeoff:** Renames must coordinate through the event log and projection readiness instead of a single KV claim. The snapshot carries the matching `evt.room.>` sequence so stale projections conflict and retry instead of committing a duplicate claim. The payoff is no dual-write divergence.

### 2. Every channel room belongs to exactly one group

**Decision:** `groupID` is non-nullable on channel rooms. The public create-room API requires an explicit `groupId`; lower-level bootstrap/import paths may still pass an empty group ID to fall back to the seed room group while constructing first-boot state.
**Why:** Optional grouping means an "unsorted" branch in the permission resolver and sidebar layout — extra cases that nobody actually wants. Requiring a group simplifies the resolver and gives operators a consistent unit of permission scope. See ADR-031 and FDR-017.
**Tradeoff:** Bulk room creation tools need to know which group to drop rooms into. The API surfaces a clear error if `groupID` is missing.

### 3. Archive is a flag, not a state machine

**Decision:** Archive is a single boolean on the room record. The room stays in the same KV bucket, keeps its event history, keeps its members; only the discovery affordances filter on `archived: false`.
**Why:** Archive's purpose is "stop showing this room everywhere, but don't lose the history". A full archived-rooms-elsewhere migration would mean different code paths for archived rooms, divergent reads, and a hard road back to active state. A flag is enough.
**Tradeoff:** Every "show me rooms" query needs to remember to filter on `archived`. Centralised in the resolver layer.

### 4. Delete is a durable tombstone

**Decision:** Deleting a room appends a durable `RoomDeletedEvent` to `EVT`. Projections remove the room from user-visible catalogs and membership state; historical facts remain in the event log.
**Why:** `EVT` is both source of truth and audit log. Purging the room's event history would destroy the forensic trail and make replay semantics dependent on destructive stream operations.
**Tradeoff:** Deleted-room history still consumes storage. User-visible reads must consistently respect the tombstone.

### 5. Membership survives archive

**Decision:** Archiving doesn't kick anyone out. Members can still see the room if they navigate to it directly; they just can't find it through normal browse paths.
**Why:** Forcibly leaving members would mean re-joining them on unarchive, which the membership system doesn't model. Keeping membership intact lets archive be reversible without ambiguity.
**Tradeoff:** A user with a deep-link to an archived room can still post in it. In practice, archived rooms are usually emptied or muted first.

### 6. Live layout updates broadcast on archive / unarchive

**Decision:** Archive and unarchive both publish a `RoomLayoutUpdatedEvent` so all connected clients refresh the sidebar.
**Why:** Without this, archiving a room would still show it in everyone's sidebar until they refresh. Live update keeps the visual state consistent across sessions.
**Tradeoff:** One more event class to maintain. Fits cleanly into the existing live-event pattern (FDR-012's mechanism).

### 7. Channel member bans use dedicated moderation events

**Decision:** Banning someone from a channel room appends a normal `UserLeftRoomEvent` with the target user as actor, plus `RoomMemberBannedEvent` with the target user, required reason, optional expiry, and moderator actor. Unbanning appends `RoomMemberUnbannedEvent` with a required moderator reason. DMs are excluded; their participant set is fixed by DM creation policy in FDR-007.
**Why:** Other room members should see an ordinary leave in room history, while the moderation/audit fact remains explicit and prevents the banned user from immediately rejoining. The public leave event does not reveal that the user was banned.
**Tradeoff:** A ban is represented by two durable facts: one public membership transition and one moderation fact.

### 8. Join and leave events remain actor-only

**Decision:** `UserJoinedRoomEvent` and `UserLeftRoomEvent` do not carry a target user. The event actor is the user who joined or left. Moderator bans additionally use dedicated moderation events. To the target user, an active ban is evaluated as an ordinary join authorization denial rather than a distinct API/UI state.
**Why:** Join and leave are ordinary membership facts. Keeping the user in the envelope avoids dual-subject ambiguity. A ban-generated leave intentionally uses the target user as actor so public room history remains indistinguishable from a normal leave.
**Tradeoff:** Projections that need moderation state must listen to the moderation event family as well as join/leave.

### 9. Server-admin exposes active room bans

**Decision:** Server-admin includes a Moderation page listing active room bans with target, room, moderator, reason, creation time, and optional expiry. Unbanning from the list prompts for a moderator reason and appends `RoomMemberUnbannedEvent`.
**Why:** Operators need a way to audit and reverse room-level bans without spelunking the event log or editing RBAC state by hand.
**Tradeoff:** The first page lists active bans only. Historical moderation audit remains in the durable event log.

### 10. Universal rooms derive membership from join eligibility

**Decision:** Universal is a durable boolean on channel rooms, changed through `RoomUniversalChangedEvent`. Effective membership is explicit membership plus, for Universal channel rooms, every user for whom `room.join` currently resolves allow and no active room ban applies.
**Why:** Operators often need "everyone can see this channel" behavior without writing per-user membership events for every current and future server member. Deriving membership keeps the event log compact and makes disabling Universal restore the previous explicit membership state.
**Tradeoff:** Member-derived surfaces such as member lists, mentions, unread state, attachment access, voice calls, and live event delivery must use effective membership rather than the explicit membership projection alone.

## Permissions

- `room.create` — create a new channel room in a group. Configurable per group.
- `room.manage` — edit, archive, unarchive, delete, and change Universal state for a channel room. Configurable per group and per room.
- `room.ban-member` — ban members from a channel room. Configurable per group and per room.
- `room.join` — gates whether a user can become an explicit member of an unarchived room and whether a user is an implicit member of a Universal room. Configurable per group and per room.

## Related

- **ADRs:** ADR-031 (room-group-centric ACL), ADR-033 (event-sourced state with projections), ADR-035 (per-aggregate phased migration)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-007 (Direct Messages), FDR-017 (Room Groups & Sidebar Layout)
