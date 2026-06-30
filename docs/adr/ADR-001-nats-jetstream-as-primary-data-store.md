# ADR-001: NATS JetStream as Primary Data Store

**Date:** 2026-03-01

**Update:** ADR-033, ADR-034, and ADR-036 changed the storage pattern inside
NATS/JetStream. The foundational decision here still stands: Chatto uses
NATS/JetStream, NATS object stores, and NATS Core rather than adding a separate
database or broker. The current domain source of truth is now the `EVT` stream
with in-memory projections; latest-value runtime state lives in `RUNTIME_STATE`.

## Context

Chatto needs persistent storage for messages, user profiles, space/room configuration, memberships, permissions, and more. The conventional choice would be a relational database (PostgreSQL, SQLite) or a document store (MongoDB), paired with a separate pub/sub system for real-time event delivery.

However, Chatto's core goal is a single self-hosted executable with minimal operational overhead. Running a separate database adds deployment complexity, backup coordination, and connection management. Additionally, the chat domain is inherently event-driven — messages are events, presence is events, typing indicators are events — so the storage and pub/sub layers serve the same data.

## Decision

Use NATS JetStream as the sole persistent data store. Specifically:

- **`EVT` JetStream stream** for durable domain facts: server configuration, users, rooms, memberships, RBAC, messages, threads, reactions, voice calls, assets, and safe auth/workflow audit facts.
- **In-memory projections** rebuilt from `EVT` for current domain reads.
- **KV buckets** for latest-value runtime, volatile, and key-management state: especially `RUNTIME_STATE`, `MEMORY_CACHE`, and `ENCRYPTION_KEYS`.
- **Object store buckets** for binary assets, with S3 as the optional external backend.
- **Core NATS pub/sub** for ephemeral real-time sync signals, currently under `live.sync.>`.

No relational database, no ORM, no SQL migrations.

## Consequences

- **Simpler deployment**: No database to provision, configure, or back up separately. NATS data lives on disk alongside the binary.
- **Unified data and messaging**: The same system that stores messages also delivers them in real-time. No CDC, no polling, no sync layer.
- **No ad-hoc queries**: NATS resources are not a relational query engine. Full-text search and analytics require dedicated projections or pluggable systems.
- **No SQL joins**: Related data is assembled through projections and API response assemblers rather than database joins.
- **Backup is NATS-native**: `chatto backup` exports streams, KV buckets, and object stores. Restoring means restoring NATS resources and replaying projections from `EVT`.
- **Operational knowledge shifts**: Operators need to understand NATS streams, consumers, and KV semantics rather than SQL and database tuning.
- **Scaling story is NATS-native**: Horizontal scaling means NATS clustering (JetStream Raft consensus), not database replicas.
