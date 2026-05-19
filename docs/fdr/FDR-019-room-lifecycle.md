# FDR-019: Room Lifecycle

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

A channel room goes through a lifecycle of create, edit, archive, unarchive, and delete. Each transition is permission-gated and (where appropriate) audit-logged. This FDR focuses on channel rooms — DM room lifecycle is much simpler and lives in FDR-007.

## Behavior

- **Create** — server admins (or anyone with `room.create` in the target group) create a channel room by giving it a name (1–30 chars, alphanumeric / hyphen / underscore, case-insensitive unique across the server), an optional description, and a room group.
- **Edit** — `room.manage` holders can change the name, description, and group of an existing room.
- **Archive** — `room.manage` toggles an `archived` flag on the room. Archived rooms vanish from the sidebar, the Browse Rooms page, and search results, but members stay joined and history is intact. The owner can still navigate to the room directly.
- **Unarchive** — same permission, flips the flag back. The room reappears in the sidebar and discovery surfaces.
- **Delete** — `room.manage` permanently removes the room: the KV record, the name claim, the stream events filtered by the room's subject namespace, and the membership records.
- Moving a room between groups requires `room.manage` in both groups (see FDR-017).

## Design Decisions

### 1. Room name uniqueness via atomic KV claim

**Decision:** Room names are unique server-wide (case-insensitive). Uniqueness is enforced by `kv.Create()` on a `room_name_index.*` key — atomic with the room record creation. Read-then-write would race; the create-claim doesn't.
**Why:** Race-tolerant name claiming is the only way to safely handle two operators creating the same-named room at the same moment. The KV `Create` semantics (fails if key exists) give us atomicity for free.
**Tradeoff:** Renames are a delete-claim-then-recreate dance — slightly more complex than a simple update. The name index also has to be kept in sync with the room record, but the operations live in one transaction.

### 2. Every channel room belongs to exactly one group

**Decision:** `groupID` is non-nullable on channel rooms. Creation without an explicit group falls back to the server's seed group ("Lobby") only at first boot; afterwards the API requires an explicit group.
**Why:** Optional grouping means an "unsorted" branch in the permission resolver and sidebar layout — extra cases that nobody actually wants. Requiring a group simplifies the resolver and gives operators a consistent unit of permission scope. See ADR-031 and FDR-017.
**Tradeoff:** Bulk room creation tools need to know which group to drop rooms into. The API surfaces a clear error if `groupID` is missing.

### 3. Archive is a flag, not a state machine

**Decision:** Archive is a single boolean on the room record. The room stays in the same KV bucket, keeps its event history, keeps its members; only the discovery affordances filter on `archived: false`.
**Why:** Archive's purpose is "stop showing this room everywhere, but don't lose the history". A full archived-rooms-elsewhere migration would mean different code paths for archived rooms, divergent reads, and a hard road back to active state. A flag is enough.
**Tradeoff:** Every "show me rooms" query needs to remember to filter on `archived`. Centralised in the resolver layer.

### 4. Delete is destructive and scoped to the room's subjects

**Decision:** Deleting a room publishes an audit event first, then deletes the room record, releases the name claim, and purges all JetStream events under the room's subject prefix (`server.room.{kind}.{roomId}.*`).
**Why:** Half-deleted rooms — record gone but events still in the stream — are worse than no deletion at all. They show up in event queries as orphans. Scoped purging is the only way to actually be done.
**Tradeoff:** The purge is a non-trivial JetStream operation. We log progress and treat partial failure as an incident; in practice it's reliable.

### 5. Membership survives archive

**Decision:** Archiving doesn't kick anyone out. Members can still see the room if they navigate to it directly; they just can't find it through normal browse paths.
**Why:** Forcibly leaving members would mean re-joining them on unarchive, which the membership system doesn't model. Keeping membership intact lets archive be reversible without ambiguity.
**Tradeoff:** A user with a deep-link to an archived room can still post in it. In practice, archived rooms are usually emptied or muted first.

### 6. Live layout updates broadcast on archive / unarchive

**Decision:** Archive and unarchive both publish a `RoomLayoutUpdatedEvent` so all connected clients refresh the sidebar.
**Why:** Without this, archiving a room would still show it in everyone's sidebar until they refresh. Live update keeps the visual state consistent across sessions.
**Tradeoff:** One more event class to maintain. Fits cleanly into the existing live-event pattern (FDR-012's mechanism).

## Permissions

- `room.create` — create a new channel room in a group. Configurable per group.
- `room.manage` — edit, archive, unarchive, and delete a room. Configurable per group and per room.
- `room.join` — gates whether a user can become a member of an unarchived room. Configurable per group and per room.

## Related

- **ADRs:** ADR-006 (KV as source of truth), ADR-031 (room-group-centric ACL)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-007 (Direct Messages), FDR-017 (Room Groups & Sidebar Layout)
