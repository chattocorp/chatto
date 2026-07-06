# ADR-003: GraphQL as the Primary API

**Date:** 2026-03-01

**Status:** Superseded by [ADR-042](ADR-042-protobuf-first-public-api.md). GraphQL was removed after the protobuf-first API migration; this record is preserved as historical context for the original API choice.

## Context

Chatto's frontend needs to query structured data (users, spaces, rooms, messages), perform mutations (post message, join room, update settings), and receive real-time updates (new messages, presence changes, typing indicators). The API must support all three patterns efficiently.

Options considered:

- **REST + WebSocket**: Familiar, widely supported, but requires separate endpoint design for each resource and a custom WebSocket protocol for real-time.
- **gRPC**: Excellent for service-to-service, but browser support requires grpc-web proxying and tooling is heavier for frontend consumption.
- **GraphQL**: Single endpoint, typed schema, built-in subscriptions over WebSocket, and strong codegen ecosystem for both Go and TypeScript.

## Decision

Use GraphQL (via gqlgen for Go) as the primary client-facing API. Specifically:

- **Queries** for data fetching with field-level resolution (avoids over-fetching)
- **Mutations** for all write operations with typed input objects
- **Subscriptions** over WebSocket for real-time event delivery (messages, presence, space events)

The NATS request-reply API exists as a secondary internal/extension API for trusted contexts.

## Consequences

- **Single schema as contract**: The `.graphqls` schema files are the source of truth for the API. Both backend resolvers and frontend types are generated from them.
- **Typed end-to-end**: gqlgen generates Go types and resolver interfaces; graphql-codegen generates TypeScript types and urql hooks. Type mismatches are caught at build time.
- **Subscriptions are native**: GraphQL subscriptions over WebSocket map naturally to NATS pub/sub. No custom WebSocket protocol needed.
- **N+1 risk**: Field resolvers can cause excessive KV lookups. Addressed with dataloader patterns where needed.
- **Schema changes are potentially breaking**: Changes to `.graphqls` files affect all clients. Managed carefully according to the project status in `AGENTS.md`.
- **No file upload via GraphQL**: Binary uploads use separate HTTP endpoints, not GraphQL mutations.
