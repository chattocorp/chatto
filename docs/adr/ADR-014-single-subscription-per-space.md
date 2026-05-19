# ADR-014: Single GraphQL Subscription Per Space

**Date:** 2026-03-01

**Status:** Superseded by ADR-030 (Retire the Space tier). The "per-space" framing is obsolete now that the Space tier is gone; the GraphQL surface is the single deployment-wide `myEvents` subscription backed by one `live.server.>` NATS Core sub plus per-event authorization (see ARCHITECTURE.md). The pattern — one multiplexed subscription with server-side membership filtering — survives, but at server scope rather than space scope. The decision recorded here is preserved as historical context.

## Context

The frontend needs real-time updates for all activity in a space: messages from all joined rooms, presence changes, typing indicators, reactions, room lifecycle events. The options are:

- **Per-room subscriptions**: One WebSocket per room. Clean isolation but O(n) connections for n rooms, with reconnect complexity when switching rooms.
- **Per-event-type subscriptions**: Separate subscriptions for messages, presence, typing, etc. Clean schema but multiplies the number of WebSockets.
- **Single subscription per space**: One WebSocket carries all event types for all rooms in a space. Multiplexed but efficient.

## Decision

Use a single `mySpaceEvents` GraphQL subscription per active space. The backend fans in all event sources (JetStream consumer, NATS Core subscriptions, KV presence watcher) into one Go channel. Server-side filtering ensures the user only receives events for rooms they've joined — the membership set is cached in the subscription goroutine and updated dynamically on join/leave events.

The frontend dispatches received events through a `spaceEventBus` that allows components to register handlers for specific event types and rooms.

## Consequences

- **One WebSocket per space**: Minimal connection overhead. Switching rooms within a space doesn't require new subscriptions.
- **Server-side membership filtering**: The backend filters events by room membership before sending. Clients never receive events for rooms they haven't joined, even though all room events flow through one subscription.
- **Event bus complexity on the frontend**: Components register/unregister handlers with the `spaceEventBus`. This is a pub/sub pattern within the client that mirrors the server-side fan-in.
- **Reconnect invalidates everything**: After a WebSocket reconnect, all cached state for the space may be stale. The `reconnectCount` state drives cache invalidation, and components refetch their data.
- **Presence lifecycle is tied to the subscription**: Presence signaling starts when the space subscription opens and stops when it closes. There's no separate presence connection.
- **Multi-space requires multiple subscriptions**: Users active in multiple spaces have one subscription per space. This is uncommon in practice (users typically focus on one space at a time).
