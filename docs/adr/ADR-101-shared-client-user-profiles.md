# ADR-101: Share Public User Profiles Within Each Client Connection

**Date:** 2026-09-21

## Context

The client stored public profiles in its realtime projection, room-directory
queries, and timeline summary cache. Each cache had different readers and
invalidation rules. A user could appear in a room member list but remain absent
from the quick finder. Joining the caches inside the finder made a navigation
component responsible for profile precedence and deletion.

## Decision

Use one in-memory `UserStore` for each server ID and connection scope. It owns
public user profiles keyed by user ID. The realtime projection uses this same
store. Directory and timeline API adapters hydrate it; render adapters derive
names, avatar URLs, bot metadata, and other public fields from it.

Keep room membership IDs, query pagination, permissions, and request status in
their existing owners. Timeline rows can contain render snapshots, but they do
not form another profile cache. Server-scoped profile views resolve current
fields from the shared store. Private admin fields are outside this store.

The signed-in account has a separate owner: one `CurrentUserState` per server.
Only complete live viewer responses populate it. Public profiles and saved
chat text cannot supply account data, mark it loaded, or establish session
validity. Routes and recovery share its pending account request. The registry
checks account changes and clears the previous account's private state before
publishing a new account. Reset and disposal reject late account responses.
Route loaders and mounted transport components do not keep another account
cache or clear account data when they unmount.

Account settings wait for this owner's data before they initialise edit buffers.
A refresh preserves those buffers. A failed refresh can retain the complete
account for display; an authentication rejection still disables private actions.

Profile contexts are read-only views of that owner. API adapters write directly
to it; there are no separate summary caches or profile-priming hooks.

The shared owner enforces these rules:

- Concurrent missing-user reads share requests with at most 100 IDs per batch.
- A realtime update replaces a profile and prevents an older read from replacing
  it. List and detail reads use per-user revisions for the same guarantee.
- Incidental timeline includes only seed unknown users.
- Deletion leaves a tombstone. A profile-change invalidation permits a new read.
- Reset clears profiles and fences pending reads. Connection disposal also
  prevents retained readers from publishing new data.
- Opaque realtime cursors are not compared. If a shared read omits a user at a
  different cursor, the caller retries that missing user at its own boundary.
- Custom-status expiry belongs to the profile owner and ends on reset/disposal.

This refines [ADR-062](ADR-062-tanstack-query-for-snapshot-reads.md): TanStack
Query still owns snapshot requests and pagination, but public identities have
one shared owner across snapshot and realtime consumers. The change adds no
server API, persisted cache, full-directory download, or external connection.

## Consequences

- Users discovered through rooms, DMs, or timelines are available to local
  search without a separate merge or request.
- Profile changes and privacy boundaries apply to all profile readers.
- API adapters must publish through the shared owner's fenced read methods.
  A component must not install an old response directly as a current profile.
- The known-user set is incomplete and lasts only for the connection's current
  authorized state. It is not evidence of complete server membership.
