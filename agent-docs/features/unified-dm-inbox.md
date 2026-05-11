# DMs (Unified Inbox is currently retired)

> **Status note:** the multi-server DM inbox described in earlier
> revisions of this document is currently NOT present in the codebase.
> The dedicated `/chat/dm/...` route tree, the `DMConversationList`
> sidebar, and the cross-server DM aggregation have been removed. DMs
> are accessed through the per-server room sidebar like any other room,
> with the `kind: dm` discriminator distinguishing them from channels.
>
> If a unified inbox is reintroduced, this document should be rewritten.

## How DMs Work Today

- DM rooms live in the unified `SERVER_*` buckets alongside channel
  rooms, distinguished by a `kind` segment in the KV keys
  (`room.dm.{roomId}` vs `room.channel.{roomId}`).
- The DM space is a system space with ID `"DM"` created automatically
  at startup; DM rooms hang off it without space membership being the
  gating concept.
- Room IDs are deterministic hashes of sorted participant IDs, enabling
  find-or-create semantics without database queries.
- Maximum 10 participants per DM conversation.
- DM rooms are listed via the dedicated `ListDMConversations` API, not
  the regular room browsing.

## Starting a DM

- DMs are started from user context menus inside the per-server chat UI
  (member list clicks, @mention clicks, message author clicks).
- `startDMWith(serverId, userId)` in `frontend/src/lib/dm/startDM.ts`
  uses the correct server's GraphQL client (looked up via
  `graphqlClientManager.getClient(serverId)`) and navigates to the
  resulting room under `/chat/[serverId]/(chrome)/[roomId]`.

## Authorization

- The DM space uses permission-based access (`dm.view`, `dm.write`)
  rather than space membership. The backend's `requireSpaceMember` has
  a special case for `IsDMSpace(spaceID)` that checks the DM
  permissions instead of `SpaceMembershipExists`.
- Individual DM rooms use standard room membership checks.
- New DM messages publish to `live.server.user.{userId}.dm_message` for
  toast display and to drive sidebar updates.

## Key Files

| File | Purpose |
|------|---------|
| `frontend/src/lib/dm/startDM.ts` | Starts a DM and navigates to the resulting room |
| `cli/internal/graph/authz.go` | `requireSpaceMember` DM special case |
| `cli/internal/core/dm.go` | DM space constants, `IsDMSpace`, `FindOrCreateDM` |
