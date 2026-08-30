# FDR-019: Room Lifecycle

**Status:** Active
**Last reviewed:** 2026-08-30

## Overview

A channel room goes through a lifecycle of create, edit, archive, unarchive, and delete. Each transition is permission-gated and (where appropriate) audit-logged. This FDR focuses on channel rooms — DM room lifecycle is much simpler and lives in FDR-007.

## Behavior

- **Create** — server admins (or anyone with `room.create` in the target group) can start room creation from that group in the sidebar. They give the channel a visible 1–30-code-point Unicode name, an optional description, a room group, and the desired Threading Mode, with Enabled as the default. They may also enable Universal. Names are unique across the server after Unicode compatibility normalization and full case folding.
- **Edit** — `room.manage` holders can change the name, description, group, Universal setting, Threading Mode, and explicit member set of an existing channel room.
- **Settings access** — a visible room's sidebar action menu links `room.manage` holders directly to that room's management page and lets them start the archive flow. Effective `room.manage` holders can change general settings; server-wide `role.manage` holders can configure the room's role permission matrix without receiving general room-management authority. The management read can load private-room metadata for either capability and is deliberately separate from the visibility-gated room directory.
- **Display** — when set, the optional description appears after the channel room name in the desktop room pane header.
- **Join preview** — a non-member who is allowed to list and join a visible channel room sees its group, description, exact effective member count, and up to five member identities before joining. Messages, files, and activity remain hidden. A user who cannot join sees only the access-denied state.
- **Universal** — a channel room with Universal enabled behaves as joined for every server member who is currently eligible to join it. The system does not fan out `UserJoinedRoomEvent` facts for implicit membership. Existing explicit memberships remain intact, so disabling Universal restores the prior explicit membership set.
- **Bootstrap defaults** — fresh servers seed `#announcements` as Universal with announcement-only posting defaults and `#general` as a normal channel room in the default Lobby group. Those posting defaults are an explicit trusted seed option; a user-created room merely named `announcements` receives ordinary permissions.
- **Join / leave** — joining a Universal room succeeds without writing an explicit membership event. Leaving a Universal room is rejected; users can instead configure that room's notification policy. DMs cannot be Universal.
- **API surface** — ConnectRPC `RoomService` exposes create, edit, archive, unarchive, Universal, Threading Mode, join, leave, manager add/remove, ban, and unban commands. ConnectRPC `RoomDirectoryService` exposes the complementary room list, room-group/sidebar list, single-room refresh, per-room viewer capability state, and group join-all command.
- **Archive** — `room.manage` toggles the room's durable `archived` flag.
  Archived rooms vanish from the sidebar, server Overview, and search results.
  Membership and history stay intact, but the room is read-only until an
  administrator unarchives it.
- **Unarchive** — same permission, flips the flag back. The room reappears in the sidebar and discovery surfaces.
- **Manage members** — `room.manage` holders can list, inspect, add, or remove members of channel rooms, including when they are not themselves members or eligible to join. Adding can bring a user into a private room even when that user could not self-join through `room.join`. Active room bans still block adding; the user must be unbanned first. DM membership remains visible only to its participants.
- **Ban member** — `room.ban-member` holders can ban a user from a channel room with a required reason and optional expiry. The banned user loses room read/write/live access immediately and cannot rejoin until the ban is removed or expires.
- **Delete** — `room.manage` commits `RoomDeletedEvent` and the room-group removal in one atomic EVT batch. Projections remove the room from the catalog and memberships.
- Leaving, removal, a room ban, loss of Universal eligibility, a group move, or
  an RBAC change removes notification occurrences the user can no longer see.
  Room deletion removes all occurrences targeting that room. A durable
  visibility boundary prevents older queued activity from reappearing after a
  quick regain of access.
- Moving a room between groups requires `room.manage` in both groups (see FDR-017).

## Design Decisions

### 1. Room name uniqueness via EVT projection and OCC

**Decision:** Room names are unique server-wide under the canonical comparison described in Decision 12. Uniqueness is enforced by checking every matching room in a catalog projection snapshot and appending name-changing room events with wildcard OCC against the room aggregate event set. Rename checks exclude only the room being renamed. Pre-existing newly-equivalent names do not prevent startup, but no further equivalent name may be created or claimed until operators rename the colliding rooms apart.
**Why:** Race-tolerant name claiming is the only way to safely handle two operators creating the same-named room at the same moment. EVT OCC lets the event log remain the source of truth without maintaining a legacy KV name mirror.
**Tradeoff:** Renames must coordinate through the event log and projection readiness instead of a single KV claim. The snapshot carries the matching `evt.room.>` sequence so stale projections conflict and retry instead of committing a duplicate claim. The payoff is no dual-write divergence.

### 2. Every channel room belongs to exactly one group

**Decision:** `groupID` is non-nullable on channel rooms. The public create-room API requires an explicit `groupId`; lower-level bootstrap/import paths may still pass an empty group ID to fall back to the seed room group while constructing first-boot state. Room creation commits the room fact and initial group-membership fact in one atomic EVT batch.
**Why:** Optional grouping means an "unsorted" branch in the permission resolver and sidebar layout — extra cases that nobody actually wants. Requiring a group simplifies the resolver and gives operators a consistent unit of permission scope. See ADR-031 and FDR-017.
**Tradeoff:** Bulk room creation tools need to know which group to drop rooms into. The API surfaces a clear error if `groupID` is missing.

### 3. Archive is a flag, not a state machine

**Decision:** Archive is one boolean in the room projection. Durable room
archive and unarchive facts change it. The room keeps its event history and
members; active-room discovery filters on `archived: false`.
**Why:** Archive's purpose is "stop showing this room everywhere, but don't lose the history". A full archived-rooms-elsewhere migration would mean different code paths for archived rooms, divergent reads, and a hard road back to active state. A flag is enough.
**Tradeoff:** Every "show me rooms" query needs to remember to filter on `archived`. Centralised in the resolver layer.

### 4. Delete is a durable tombstone

**Decision:** Deleting a room appends a durable `RoomDeletedEvent` and, for a grouped channel room, `RoomRemovedFromGroupEvent` in one atomic EVT batch. Projections remove the room from user-visible catalogs, room-group layout, and membership state; historical facts remain in the event log.
**Why:** `EVT` is both source of truth and audit log. Purging the room's event history would destroy the forensic trail and make replay semantics dependent on destructive stream operations.
**Tradeoff:** Deleted-room history still consumes storage. User-visible reads must consistently respect the tombstone.

### 5. Membership survives archive

**Decision:** Archiving does not remove membership, but it makes the room
read-only and removes it from ordinary user navigation. Existing membership
becomes usable again after unarchive.
**Why:** Forcibly leaving members would require the system to rejoin them later.
Keeping membership intact makes archive reversible without ambiguity while the
read-only boundary prevents new room activity.
**Tradeoff:** Membership records for an archived room remain stored even while
users cannot use the room.

### 6. Archive state converges through the realtime projection

**Decision:** Durable archive and unarchive facts make connected clients remove
or restore the room and reconcile the current room-group layout.
**Why:** Archiving must remove the room from every connected navigation surface
without waiting for a page refresh. Unarchive must restore the authoritative
room and layout state in the same convergence model.
**Tradeoff:** Clients must handle room removal and restoration as projection
operations. This follows the authoritative realtime pattern in ADR-051.

### 7. Channel member bans use dedicated moderation events

**Decision:** Banning someone from a channel room appends a normal `UserLeftRoomEvent` with the target user as actor, plus `RoomMemberBannedEvent` with the target user, required reason, optional expiry, and moderator actor. Unbanning appends `RoomMemberUnbannedEvent` with a required moderator reason. DMs are excluded; their participant set is fixed by DM creation policy in FDR-007.
**Why:** Other room members should see an ordinary leave in room history, while the moderation/audit fact remains explicit and prevents the banned user from immediately rejoining. The public leave event does not reveal that the user was banned.
**Tradeoff:** A ban is represented by two durable facts: one public membership transition and one moderation fact.

### 8. Join and leave events remain actor-only

**Decision:** `UserJoinedRoomEvent` and `UserLeftRoomEvent` do not carry a target user. The event actor is the user who joined or left. Manager-controlled add/remove writes a normal join/leave fact with the target user as actor plus a dedicated moderation audit fact with the manager as actor. Moderator bans use the same split. To the target user, an active ban is evaluated as an ordinary join authorization denial rather than a distinct API/UI state.
**Why:** Join and leave are ordinary membership facts. Keeping the user in the envelope avoids dual-subject ambiguity and keeps room history focused on membership transitions. Separate moderation facts preserve who performed manager actions without changing public timeline semantics.
**Tradeoff:** Audited manager actions are represented by two durable facts: one public membership transition and one moderation fact.

### 9. Server-admin exposes active room bans

**Decision:** Server-admin includes a Moderation page listing active room bans with target, room, moderator, reason, creation time, and optional expiry. Unbanning from the list prompts for a moderator reason and appends `RoomMemberUnbannedEvent`.
**Why:** Operators need a way to audit and reverse room-level bans without spelunking the event log or editing RBAC state by hand.
**Tradeoff:** The first page lists active bans only. Historical moderation audit remains in the durable event log.

### 10. Universal rooms derive membership from join eligibility

**Decision:** Universal is a durable boolean on channel rooms, changed through `RoomUniversalChangedEvent`. Effective membership is explicit membership plus, for Universal channel rooms, every user for whom `room.join` currently resolves allow and no active room ban applies.
**Why:** Operators often need "everyone can see this channel" behavior without writing per-user membership events for every current and future server member. Deriving membership keeps the event log compact and makes disabling Universal restore the previous explicit membership state.
**Tradeoff:** Member-derived surfaces such as member lists, mentions, unread state, attachment access, voice calls, and live event delivery must use effective membership rather than the explicit membership projection alone.

### 11. Member listing follows membership or discovery-and-join eligibility

**Decision:** Existing room members and effective `room.manage` holders may list a channel room's effective members and hydrate individual member rows. Other channel-room non-members may list members only when both `room.list` and `room.join` allow it at that room. DMs remain membership-only. The pre-join screen requests the first five alphabetically sorted members and uses the list's total count for its compact preview.
**Why:** Room managers need the same member resource they are authorised to change, even when management authority deliberately does not grant room participation. Room membership remains an existing paginated resource instead of adding a preview- or management-specific shape. Requiring both discovery and join eligibility for other nonmembers limits access to rooms they can knowingly enter.
**Tradeoff:** A manager or eligible nonmember can paginate the full member directory without joining. Messages, files, activity, and other membership-gated room content remain inaccessible until joining, and DM participant privacy is unchanged.

### 12. Room names are Unicode presentation metadata

**Decision:** Channel room names accept visible Unicode text, including spaces, punctuation, symbols, emoji, combining scripts, and embedded format characters needed by legitimate scripts and emoji sequences. Input is trimmed and stored in NFC form. Names must contain 1–30 Unicode code points and at least one visible character; control characters and Unicode line or paragraph separators are rejected. For server-wide uniqueness and name-based `in:` search selectors, names are compared using NFKC compatibility normalization, full Unicode case folding, then NFKC again. The immutable room ID, not the mutable name, identifies the room in links, protocols, permissions, and storage.
**Why:** A room name is human-facing presentation metadata. Communities should be able to write natural names such as `Team chat 💬`, Traditional Chinese, or combining scripts without introducing a second display-name concept. Compatibility normalization and full case folding prevent confusing width, styled-letter, ligature, and case variants such as `Straße` and `STRASSE` from claiming separate names.
**Tradeoff:** NFKC intentionally loses compatibility distinctions for comparison while preserving the NFC display spelling in storage. It does not apply stricter cross-script homoglyph matching, so visually similar Latin `a` and Cyrillic `а` remain distinct. The public protobuf fields and limits do not change: invalid or invisible names continue to produce `INVALID_ARGUMENT`, equivalent names produce `ALREADY_EXISTS`, and an older server may reject a newer client's expanded name without affecting other operations.

### 13. Threading Mode is direct room metadata

**Decision:** Every channel exposes one of Required, Encouraged, Enabled, or Disabled as ordinary room metadata. New and historically unspecified channels resolve to Enabled. DMs remain threadless and report Unspecified. `room.manage` changes append a dedicated room event and update every room-directory and realtime representation.
**Why:** Conversation shape is a durable property of a room, not a side effect of permissions or a client-local preference. Keeping it beside Universal and Slow Mode makes creation, administration, API use, replay, and live updates converge on one setting.
**Tradeoff:** Threading Mode and posting permissions are separate controls. Administrators must still grant the location permissions needed by the chosen mode; Required only waives `message.post-in-thread` for the automatic creation of a root's empty thread.

## Permissions

- `room.create` — create a new channel room in a group. Configurable per group.
- `room.manage` — edit, archive, unarchive, delete, change Universal and Threading Mode state, and list, inspect, add, or remove members for a channel room. Configurable per group and per room.
- `role.manage` — configure role permission decisions at room scope without granting general room-management authority.
- `room.ban-member` — ban members from a channel room. Configurable per group and per room.
- `room.join` — gates whether a user can become an explicit member of an unarchived room and whether a user is an implicit member of a Universal room. Configurable per group and per room.
- `room.list` + `room.join` — together allow a channel-room nonmember without `room.manage` to list its effective members. Existing members and room managers do not need these grants for member listing.

## Related

- **ADRs:** ADR-031 (room-group-centric ACL), ADR-033 (event-sourced state with projections), ADR-035 (per-aggregate phased migration), ADR-076 (notification occurrences), ADR-077 (persistent notification list), ADR-086 (atomic room-layout structural mutations)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-007 (Direct Messages), FDR-012 (Notifications), FDR-017 (Room Groups & Sidebar Layout)
