# ADR-062: TanStack Query for Snapshot-Style Frontend Reads

**Date:** 2026-07-31

**Updated:** 2026-09-23

## Status

Accepted

## Context

The frontend performs two materially different kinds of server-state work.
The resumable realtime projection owns ordered, convergent resource snapshots
such as room summaries, notifications, presence, calls, and viewer
permissions. Room and thread timelines and other screens use ConnectRPC reads for bounded
snapshots such as filtered admin members, permission matrices, and event-log
pages.

Snapshot screens had each implemented their own loading, error, cancellation,
pagination, stale-response fencing, and short-lived caching. That duplicated
lifecycle code and made back-navigation reload data that was still useful. A
single generic cache cannot replace the realtime projection, however: doing so
would weaken its ordering, cursor, authorization-loss, and privacy guarantees.

## Decision

Use TanStack Query for snapshot-style ConnectRPC reads and their related
mutations. Queries may deduplicate concurrent reads, retain recently used
results in memory, and model cursor pagination with infinite queries.

The cache has the following boundaries:

- Every private key starts with the server ID and an opaque scope owned by the
  current `ServerConnection`. Replacing credentials or transport creates a new
  scope even when the server and user IDs are unchanged. Use
  `serverSessionQueryRoot` from `$lib/query/keys` to make this prefix. A key
  outside the prefix is not purged.
- The query cache is memory-only. Disposing a server store removes every query
  under that server's key prefix during logout, credential replacement, and
  server removal. Authentication failure purges the same prefix immediately.
  A warm chat keeps its normal projection visible while the user reconnects.
  The client does not store chat data on the device; see
  [ADR-107](ADR-107-keep-chat-data-out-of-device-storage.md).
- Query functions pass TanStack's `AbortSignal` to ConnectRPC so superseded or
  unmounted reads can be cancelled.
- Mutations update or invalidate only explicitly related keys. Mutation
  completion is fenced to the server, connection, and resource that initiated
  it so a late result cannot update a reused route's next resource.
- Private-data resets and navigation completion have separate lifetimes.
  A connection generation rejects obsolete API response data. A page-visit
  token decides whether a successful request may navigate. Reset-driven route
  removal does not end the visit; user navigation and session replacement do.
  Completion runs outside the component's mutation observer so observer
  removal cannot lose a successful role create/delete navigation. A discarded
  response can report success without exposing its old data.
- Authorization or visibility loss must remove affected private results when
  it can occur without disposing the whole server store. Invalidating them for
  a later refetch is not a sufficient privacy fence. Active observers are
  synchronously scrubbed before an authoritative refetch.
- Query defaults use a short stale window, bounded garbage-collection time, no
  focus refetch, no mutation retry, and at most one retry for transient reads.
  Authentication, permission, invalid-argument, and not-found failures are not
  retried.

TanStack Query does not own the server-scoped realtime resource snapshot,
notifications and unread state, presence, active calls, room and thread
timeline stores, message search, authentication, or expiring asset URLs. Those
remain in their established per-server owners. Realtime reducers may
explicitly update, invalidate, or remove a snapshot query, but a query must not
become a second unordered copy of canonical projection state.

The initial pilot applies this decision to admin member lists and details,
permission matrices, and event-log lists and details. Other snapshot reads can
move incrementally when doing so removes meaningful custom lifecycle code.
The first follow-up applies it to paginated moderation bans and the bounded
system-diagnostics snapshot.
The notification-policy settings screen also uses a session-scoped query for
its visible policy scopes. TanStack owns the read, cancellation, and refresh
after a save. The screen keeps only cell-level pending and save-error state.
The policy query is removed when its scope list changes or the screen closes,
so lost room scopes do not remain in the cache. Server cache removal fences
late saves and removes cached policies.

[ADR-101](ADR-101-shared-client-user-profiles.md) defines one connection-scoped
owner for public user profiles shared by snapshot and realtime readers.
Directory queries retain request and membership state; they do not own a second
public-profile cache.

## Consequences

- Admin snapshot screens share consistent loading, cancellation, retry,
  pagination, and cache behavior, with fewer bespoke stores and request
  counters.
- Returning to a recently viewed member or filter can render cached data while
  normal stale-time rules control refetching.
- The frontend now has two deliberate server-state mechanisms. Maintainers
  must classify a read as snapshot or realtime-owned before choosing one.
- Correct key construction and invalidation become part of mutation and
  authorization review.
- TanStack Query is loaded by routes that use snapshot queries rather than by
  the application shell; server-store disposal reaches it through a small
  cache registry so privacy cleanup does not force it into unrelated bundles.
