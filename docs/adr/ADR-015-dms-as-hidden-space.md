# ADR-015: Direct Messages as a Hidden Space

**Date:** 2026-03-01

**Status:** Superseded by ADR-027 (instance/space → server consolidation). The hidden DM space was dissolved by Phase 4 of #354 (#373 merged the DM rooms into the unified `SERVER_*` storage with `kind: dm`); `IsDMSpace` and the synthetic `spaceID = "DM"` partition are gone. The decision recorded here is preserved as historical context for the storage shape that came before.

## Context

Direct messages need their own storage, event delivery, and permission model. The options are:

- **Separate DM entity type**: A first-class `DirectMessage` model with its own storage, subscriptions, and API. More explicit but duplicates much of the room infrastructure.
- **Per-user inbox**: Each user has an inbox stream. Doesn't scale to group DMs and requires cross-inbox coordination.
- **Reuse space/room infrastructure**: Treat DMs as rooms within a special hidden space. Reuses existing message posting, event delivery, and subscription machinery.

## Decision

Implement DMs as rooms within a well-known hidden space with `spaceID = "DM"`. Key design choices:

- **Deterministic room IDs**: DM room IDs are computed as the first 14 hex characters of `SHA-256(sorted participant IDs joined by ".")`. This enables find-or-create semantics without a lookup table — any process computes the same ID from the same participants.
- **No space membership**: Users have room-level memberships within the DM space but no DM space membership. The space itself is hidden from discovery.
- **Hardcoded permissions**: DM permissions bypass the RBAC engine entirely. A dedicated `resolveDMPermission` function returns fixed grants for the small set of applicable permissions (`message.post`, `message.edit-own`, `message.delete-own`, etc.).

## Consequences

- **Massive infrastructure reuse**: DM rooms use the same JetStream stream (originally `SPACE_DM_EVENTS`, now the unified `SERVER_EVENTS` post-Phase-4), the same body storage, the same subscription fan-in, the same GraphQL types. Very little DM-specific code is needed.
- **Find-or-create is coordination-free**: Starting a DM with a user doesn't require checking a lookup table or acquiring a lock. The deterministic ID means concurrent "start DM" requests from both participants create the same room.
- **Auth has DM special cases**: `requireSpaceMember` checks `IsDMSpace` and substitutes a `PermDMView` check instead of normal space membership. Every new auth helper must consider the DM case.
- **Group DMs are natural**: The SHA-256 input is a sorted set of participant IDs, so 3+ participant group DMs work with the same mechanism.
- **No RBAC customization for DMs**: Since permissions are hardcoded, operators cannot customize DM permissions (e.g., disable DMs for certain roles). This is acceptable for now but may need revisiting.
