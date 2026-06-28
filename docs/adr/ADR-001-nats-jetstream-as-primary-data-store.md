# ADR-001: NATS JetStream as Primary Data Store

**Date:** 2026-03-01

## Context

Chatto needs persistent storage for messages, user profiles, space/room configuration, memberships, permissions, and more. The conventional choice would be a relational database (PostgreSQL, SQLite) or a document store (MongoDB), paired with a separate pub/sub system for real-time event delivery.

However, Chatto's core goal is a single self-hosted executable with minimal operational overhead. Running a separate database adds deployment complexity, backup coordination, and connection management. Additionally, the chat domain is inherently event-driven — messages are events, presence is events, typing indicators are events — so the storage and pub/sub layers serve the same data.

## Decision

Use NATS JetStream as the sole persistent data store. Specifically:

- **KV buckets** (backed by JetStream streams) for current-state data: user profiles, space/room configuration, memberships, permissions, roles.
- **Event streams** for ordered, append-only event logs: room messages, space events, instance events.
- **Object store buckets** for binary assets: avatars, file attachments.
- **Core NATS pub/sub** for ephemeral real-time signals: presence, typing indicators, live events.

No relational database, no ORM, no SQL migrations.

## Consequences

- **Simpler deployment**: No database to provision, configure, or back up separately. NATS data lives on disk alongside the binary.
- **Unified data and messaging**: The same system that stores messages also delivers them in real-time. No CDC, no polling, no sync layer.
- **No ad-hoc queries**: KV is key-based lookup only. There's no `SELECT * FROM messages WHERE body LIKE '%search%'`. Full-text search requires a separate pluggable system.
- **No joins**: Related data must be fetched with multiple KV gets or denormalized. API handlers and read models compose these lookups explicitly; complex cross-entity ad-hoc queries aren't practical.
- **Backup is NATS-native**: `chatto backup` exports streams and KV buckets. Restoring means replaying into a fresh NATS instance.
- **Operational knowledge shifts**: Operators need to understand NATS streams, consumers, and KV semantics rather than SQL and database tuning.
- **Scaling story is NATS-native**: Horizontal scaling means NATS clustering (JetStream Raft consensus), not database replicas.
