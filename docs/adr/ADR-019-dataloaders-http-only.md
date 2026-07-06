# ADR-019: Dataloaders Scoped to HTTP Requests Only

**Date:** 2026-03-01

**Status:** Superseded by the GraphQL API removal in ADR-042. This record is preserved as historical context for the former gqlgen resolver implementation.

## Context

GraphQL field resolvers for messages often need to fetch the same related data (user profiles, reaction counts) multiple times within a single response. DataLoaders batch and deduplicate these lookups — instead of N individual user fetches, a single batched fetch resolves all N users.

However, GraphQL subscriptions are long-lived WebSocket connections where events arrive one at a time, potentially minutes or hours apart. DataLoader caching across this timespan would serve stale data (e.g., a user renamed themselves 10 minutes ago, but the cached name is still "old name").

## Decision

DataLoader instances are created fresh per HTTP request and injected into request context. They are explicitly NOT available on WebSocket/subscription connections. The WebSocket `InitFunc` creates a fresh `context.Background()` with only the authenticated user — no dataloaders.

Subscription field resolvers detect the absence of dataloaders and fall back to direct `core.Get*()` calls without batching or caching.

## Consequences

- **Correct subscription data**: Every subscription event resolves user profiles and reactions from current KV state. No stale caches from minutes-old lookups.
- **Batching works for queries**: A query that loads 50 messages with 20 distinct authors still batches into a single user fetch. The optimization applies where it matters most.
- **Slightly less efficient subscriptions**: Each subscription event that needs a user profile makes an individual KV lookup. In practice this is fast (microsecond-scale KV gets) and the volume is low (one event at a time).
- **Dual code paths in resolvers**: Field resolvers like `getUser` must check for dataloader presence and fall back to direct core calls. This is a small amount of conditional logic but must be maintained.
