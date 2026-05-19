# FDR-007: Direct Messages

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

Users can start a direct conversation (1-to-1 or small group, up to 10 participants) with anyone they can see in a server. DMs appear in the same per-server room sidebar as channel rooms, distinguished by a `kind: dm` discriminator. Each Chatto server has its own DM scope; there is currently no cross-server "unified DM inbox".

## Behavior

- A DM is started from user context menus inside the chat UI (member list clicks, @mention clicks, message author clicks).
- Starting a DM with a user (or set of users) navigates to the resulting DM room. If a DM with the same participant set already exists, the user lands in that room rather than creating a duplicate.
- DM rooms appear in the per-server room sidebar with their participants' names and avatars rather than a room name.
- Maximum 10 participants per DM.
- Inside a DM room, ordinary message-related features apply: posting, replies, threads, reactions, edits, deletes, mentions, attachments.
- Server admins / moderators cannot moderate DM contents — `message.manage`, `room.manage`, and `message.echo` are unconditionally denied in DM rooms regardless of role grants. The channel-style `room.create` is also denied inside DMs; DMs have their own creation and membership APIs.

## Design Decisions

### 1. DMs live in a hidden "DM space"

**Decision:** All DM rooms hang off a single system space with ID `"DM"`, created automatically at startup. DM rooms appear in the unified `SERVER_*` buckets alongside channel rooms, distinguished by `kind` segment in KV keys.
**Why:** Modelling DMs as their own top-level concept would duplicate room infrastructure (membership, messaging, events, notifications). Treating them as rooms in a hidden space reuses all of that for free. See ADR-015.
**Tradeoff:** A few code paths (space-membership checks, room listing APIs) have to special-case the DM space. The `IsDMSpace()` helper centralizes this.

### 2. Permission-based access, not space membership

**Decision:** Access to DMs is gated by `dm.view` / `dm.write`, not by joining the DM space. There's no concept of "leave the DM space"; users have the permissions or they don't.
**Why:** Space-membership semantics ("join to see rooms") don't fit DMs — you don't join DM-as-a-space, you just have DM conversations. Permission gating models intent without forcing an awkward membership lifecycle.
**Tradeoff:** Operators who want to globally disable DMs do it by revoking `dm.view` from `everyone`. Not as intuitive as "no one is a member of the DM space", but consistent with the rest of the permission model.

### 3. Deterministic room IDs

**Decision:** A DM room ID is a hash of the sorted participant user IDs.
**Why:** Find-or-create needs to be cheap and race-free. Hashing the participant set gives a content-addressable ID — starting a DM with the same group always lands in the same room without a database lookup.
**Tradeoff:** Adding or removing a participant from a DM would change the room ID, which means group membership is fixed at creation. Acceptable: in practice, group DMs are short-lived and re-creating with the new set is fine.

### 4. Per-server scope (no unified inbox)

**Decision:** Each Chatto server's DMs are scoped to that server. There's no cross-server aggregation that shows "all your DMs across all the servers you're connected to" in one inbox.
**Why:** A unified inbox was tried and removed. The complexity of cross-server aggregation (auth, real-time aggregation, navigation routing) outweighed the benefit for the current user base, which mostly works in one server at a time.
**Tradeoff:** Users in multiple servers have to switch servers to see DMs in each. If a unified inbox is reintroduced, this FDR needs a rewrite.

### 5. Moderation deny-list inside DMs

**Decision:** Even users with admin/moderator roles cannot edit others' messages, delete others' messages, or otherwise moderate inside a DM room. The deny-list is unconditional regardless of role.
**Why:** DMs are private by design. An admin who could moderate DMs would have a privacy boundary problem. Treating the deny as a static rule (not a configurable permission) prevents accidental misconfiguration.
**Tradeoff:** Genuine abuse inside DMs has no in-product moderation path — operators have to address it at the user level (suspend, kick from server) instead. See `dmBoundaryDeniedPermissions` in `permission_resolver.go`.

## Permissions

- `dm.view` — see DM rooms and read DM messages.
- `dm.write` — start DMs and post messages in them.

(All other permissions like `message.post`, `message.react`, etc. apply inside DM rooms just like in channel rooms, subject to the moderation deny-list above.)

## Related

- **ADRs:** ADR-015 (DMs as a hidden space)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-002 (Replies & Threads), FDR-012 (Notifications)
