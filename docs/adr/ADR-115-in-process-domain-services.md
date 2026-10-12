# ADR-115: In-Process Domain Services

**Date:** 2026-10-12

## Context

A Chatto deployment serves one community. The design assumes that one application
replica can serve that community; additional replicas primarily provide
redundancy. Independent scaling of each core domain service is not a current
requirement.

Core operation models already provide domain entry points, but some obtain
unrelated dependencies through the complete `ChattoCore` facade. This makes
ownership hard to see and makes changes depend on knowledge of other models,
projections, and storage operations.

Converting these models into NATS request/reply services would add network
failure handling, retries, contract versions, and routing choices during rolling
upgrades. Per-replica leaf nodes or request subjects could address routing, but
the current core operations do not need that deployment boundary. Alternative
persistence implementations also have no concrete consumer requirement today.

## Decision

Keep core domain services in the Chatto application process. API handlers and
other core services call domain operations through Go methods. Redundant
application replicas run the same set of core services and use the shared
storage and coordination mechanisms.

Each service owns its domain validation, authorization, data access, write
orchestration, and required projection waits. It may use explicit collaborators
for shared policies and other domains. Callers request operations instead of
assembling them from another service's projections or storage handles. Follow
[ADR-044](ADR-044-connectrpc-service-conventions.md) for transport adapters and
[ADR-087](ADR-087-request-time-authorization-with-aggregate-occ.md) for
authorization and OCC.

The composition root constructs services and connects their dependencies. Use
concrete types or small interfaces required by actual consumers. Replacing a
core pointer with interfaces that all delegate to the same facade can expose
dependencies, but does not by itself establish domain ownership. Move behavior
and data access to their owners in small, verified steps. Existing core helpers
may remain during this work.

Service boundaries do not require separate projectors. Preserve the shared
`ServerContentView` apply barrier and read transactions from
[ADR-089](ADR-089-server-content-view.md). Preserve JetStream OCC, durable effect
recovery, and read-your-writes across redundant replicas. Direct Go calls do not
replace these guarantees with process-local locks. Versions that overlap during
an upgrade must still obey the persisted-contract rules in
[ADR-114](ADR-114-jetstream-storage-conventions.md).

Retain NATS request/reply for explicit integration and extension contracts,
including the search provider in
[ADR-055](ADR-055-pluggable-message-search-over-nats.md). Optional standalone
workers and providers continue to follow [ADR-041](ADR-041-runtime-units.md).
[ADR-053](ADR-053-versioned-nats-service-namespaces.md) governs a NATS contract
when one is needed; it does not require internal core calls to use NATS.

Introduce a separate service process or a replaceable persistence contract only
when a concrete consumer, operational need, or second implementation establishes
its requirements. This decision applies to Chatto. It does not change Authling,
Runling, or the application-neutral shared framework boundaries.

## Consequences

Core behavior can be understood and tested within smaller ownership boundaries
while deployment remains simple. A core call stays within its application
replica, without a service routing policy for that call.

Core services share a process failure boundary and deploy together. Independent
service scaling and arbitrary persistence replacement are not promised.

The current room refactor is an initial step: catalog reads belong to
`RoomModel`, and group joins belong to room commands. Shared policies and some
low-level mutations still run on `ChattoCore` through explicit interfaces.
Further extraction must improve ownership, rather than only add interfaces.

The decision does not change public APIs, persisted data formats, or existing
extension contracts. It requires no client or data migration.
