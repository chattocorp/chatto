# ADR-062: TanStack Query for Snapshot-Style Frontend Reads

**Date:** 2026-07-31

## Status

Accepted

## Context

The frontend performs two materially different kinds of server-state work.
The resumable realtime projection owns ordered, convergent state such as room
summaries, retained timelines, notifications, presence, calls, and viewer
permissions. Other screens issue independent ConnectRPC reads for bounded
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
  scope even when the server and user IDs are unchanged.
- The query cache is memory-only. Disposing a server store removes every query
  under that server's key prefix during logout, credential replacement, and
  server removal. Authentication failure purges the same prefix immediately
  and unmounts private route content so active observers cannot retain a
  visible copy.
- Query functions pass TanStack's `AbortSignal` to ConnectRPC so superseded or
  unmounted reads can be cancelled.
- Mutations update or invalidate only explicitly related keys. Mutation
  completion is fenced to the server, connection, and resource that initiated
  it so a late result cannot update a reused route's next resource.
- Authorization or visibility loss must remove affected private results when
  it can occur without disposing the whole server store. Invalidating them for
  a later refetch is not a sufficient privacy fence. Active observers are
  synchronously scrubbed before an authoritative refetch.
- Query defaults use a short stale window, bounded garbage-collection time, no
  focus refetch, no mutation retry, and at most one retry for transient reads.
  Authentication, permission, invalid-argument, and not-found failures are not
  retried.

TanStack Query does not own the server-scoped realtime projection, retained
room or thread timelines, notifications and unread state, presence, active
calls, message search, authentication, or expiring asset URLs. Those remain in
their established per-server owners. Realtime reducers may explicitly update,
invalidate, or remove a snapshot query, but a query must not become a second
unordered copy of canonical projection state.

The initial pilot applies this decision to admin member lists and details,
permission matrices, and event-log lists and details. Other snapshot reads can
move incrementally when doing so removes meaningful custom lifecycle code.
The first follow-up applies it to paginated moderation bans and the bounded
system-diagnostics snapshot.

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
