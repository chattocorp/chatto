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

Room catalog reads belong to `RoomModel`. Room commands own lifecycle writes,
DM coordination, deletion, and pin writes. Membership and moderation belong to
`RoomMembershipModel`; group and sidebar mutations belong to `RoomLayoutModel`.
`ReadStateModel` owns room/thread markers and unread calculations.
`ThreadFollowModel` owns follow persistence and queries. Shared authorization
belongs to `AuthorizationModel` and `PermissionResolver`. Search reads use these
owners and the timeline read model.

The following integration boundaries remain deliberate. Their removal is not a
condition of this decision:

- **Message writes and pins:** `MessageModel` and the message append helpers
  coordinate encrypted bodies, assets, automatic follows, notifications, and
  room OCC. Pin writes reuse that append preparation and canonical message
  identity. Keep one append protocol while these domains commit atomic facts.
- **Timeline reads:** `RoomTimelineReadModel` and core hydration helpers combine
  compact timeline entries, current bodies, encryption, echoes, and assets.
  Read-state, follows, search, and authorization use those reads. A separate
  body-hydration owner is useful only if it can keep current-body checks and
  retries together; an interface around every helper would not achieve that.
- **Reactions:** `ReactionModel` owns its mutation protocol. Shared message
  identity and current authorization remain collaborators. Do not create a
  second identity resolver for reactions.
- **Realtime:** `MyEventsModel` combines domain facts, current authorization,
  projection readiness, and transient delivery. This is an integration owner;
  its core dependency is not a reason to split the shared content view.
- **Notifications:** Policy, decision materialization, occurrences, and delivery
  keep their existing workers and recovery protocol. Membership calls the
  materializer to record a visibility cutoff; follow changes wait for decisions
  before sending unread hints. These calls do not transfer persistence ownership
  to the caller.
- **Assets, uploads, and media:** These models still share core configuration,
  storage, encryption, and message attachment helpers. Keep their existing
  upload-session and durable cleanup protocols together. Extract a resource
  owner when work on this domain can test upload expiry, partial storage writes,
  attachment commits, and cleanup recovery as one boundary.
- **Calls:** Membership loss commits call leave/end facts in the same room
  batch. `CallModel` owns key cleanup, participant eviction, and reconciliation.
  Keep this collaboration so a process failure does not lose the durable effect.

These are current limits, not claims that all core helpers have been extracted.
The [runtime inventory](../architecture/runtime-components.md) records the
concrete owners. Future extraction must improve ownership and carry its own
concurrency and recovery tests.

The decision does not change public APIs, persisted data formats, or existing
extension contracts. It requires no client or data migration.
