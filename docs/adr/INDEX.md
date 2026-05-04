# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for the Chatto project. ADRs document significant architectural decisions along with their context and consequences.

For more about ADRs, see [Michael Nygard's article](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions).

## Decisions

| # | Decision | Date |
|---|----------|------|
| [ADR-001](ADR-001-nats-jetstream-as-primary-data-store.md) | NATS JetStream as Primary Data Store | 2026-03-01 |
| [ADR-002](ADR-002-single-binary-with-embedded-nats.md) | Single Binary with Embedded NATS Server | 2026-03-01 |
| [ADR-003](ADR-003-graphql-as-primary-api.md) | GraphQL as the Primary API | 2026-03-01 |
| [ADR-004](ADR-004-authorization-at-api-boundary.md) | Authorization Enforced at the API Boundary, Not in Core | 2026-03-01 |
| [ADR-005](ADR-005-hierarchy-wins-rbac.md) | Hierarchy-Wins RBAC Permission Resolution | 2026-03-01 |
| [ADR-006](ADR-006-kv-source-of-truth-streams-audit-log.md) | KV as Source of Truth, Streams as Audit Logs | 2026-03-01 |
| [ADR-007](ADR-007-per-user-encryption-with-crypto-shredding.md) | Per-User Encryption Keys with Crypto-Shredding for GDPR | 2026-03-01 |
| [ADR-008](ADR-008-protobuf-for-event-serialization.md) | Protobuf for Event Serialization | 2026-03-01 |
| [ADR-009](ADR-009-webhook-driven-voice-call-state.md) | Webhook-Driven Voice Call State | 2026-03-01 |
| [ADR-010](ADR-010-svelte5-reactive-cache-whitelisting.md) | Svelte 5 Reactive Cache Whitelisting | 2026-03-01 |
| [ADR-011](ADR-011-message-body-event-split.md) | Message Body / Event Split | 2026-03-01 |
| [ADR-012](ADR-012-two-tier-realtime-events.md) | Two-Tier Real-Time Event System | 2026-03-01 |
| [ADR-013](ADR-013-per-space-stream-sharding.md) | Per-Space JetStream Stream Sharding with Lazy Initialization | 2026-03-01 |
| [ADR-014](ADR-014-single-subscription-per-space.md) | Single GraphQL Subscription Per Space | 2026-03-01 |
| [ADR-015](ADR-015-dms-as-hidden-space.md) | Direct Messages as a Hidden Space | 2026-03-01 |
| [ADR-016](ADR-016-occ-for-message-publishing.md) | Optimistic Concurrency Control for Message Publishing | 2026-03-01 |
| [ADR-017](ADR-017-cookie-session-auth-for-websocket.md) | Cookie-Session Authentication Propagated to WebSocket | 2026-03-01 |
| [ADR-018](ADR-018-sveltekit-spa-embedded-in-go.md) | SvelteKit SPA Embedded in Go Binary | 2026-03-01 |
| [ADR-019](ADR-019-dataloaders-http-only.md) | Dataloaders Scoped to HTTP Requests Only | 2026-03-01 |
| [ADR-020](ADR-020-build-tag-test-endpoints.md) | Build-Tag Gated Test Endpoints | 2026-03-01 |
| [ADR-021](ADR-021-dual-asset-storage.md) | Dual Asset Storage — NATS ObjectStore Default, S3 Optional | 2026-03-01 |
| [ADR-022](ADR-022-nanoid-with-entity-prefixes.md) | NanoID with Entity-Type Prefixes | 2026-03-01 |
| [ADR-023](ADR-023-hmac-signed-image-transform-urls.md) | HMAC-Signed Image Transform URLs | 2026-03-01 |
| [ADR-024](ADR-024-opaque-bearer-tokens-for-cross-origin-auth.md) | Opaque Bearer Tokens for Cross-Origin Authentication | 2026-03-03 |
| [ADR-025](ADR-025-multi-instance-client-architecture.md) | Multi-Instance Client Architecture | 2026-03-20 |
| [ADR-026](ADR-026-event-identity-via-nanoid.md) | Event Identity via NanoID, Not JetStream Sequence Numbers | 2026-03-26 |
| [ADR-027](ADR-027-instance-space-server-consolidation.md) | Consolidate Instance + Space into a Single "Server" Concept | 2026-05-04 |
