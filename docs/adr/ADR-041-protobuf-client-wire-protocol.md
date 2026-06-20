# ADR-041: Protobuf Client Wire Protocol for the First-Party App

**Date:** 2026-06-20

## Context

Chatto's bundled web client needs to work against multiple registered servers from one app bundle. That makes GraphQL schema evolution unusually painful: when the client requests a field or union member that an older server does not know, GraphQL rejects the whole operation during validation. This is correct GraphQL behavior, but it is a poor fit for a multi-server chat client where one newly deployed server should not make every older registered server unusable.

This became visible around room events. The first-party client wants rich, varied event shapes: message bodies, attachments, video processing state, link previews, reactions, thread metadata, actor views, channel echo links, and future event-specific UI state. Adding a new GraphQL event type or a new field on an existing event can break the entire query against older servers. Splitting every optional feature into follow-up GraphQL queries reduces blast radius, but creates latency, state coordination, and an ever-growing compatibility matrix.

At the same time, Chatto already stores durable domain facts as protobuf messages and already has a server-side live delivery boundary (`StreamMyEvents`) that authorizes, filters, and waits for projections before events leave the process. The durable EVT facts are intentionally not the shape the UI wants to render. Public message facts are bodyless, folded facts such as reactions and attachment processing are better represented as current snapshots, and some client fields are derived from projections rather than stored on the event itself.

We considered:

- **Keep GraphQL as the only first-party client API.** This keeps strong tooling and one schema, but leaves the multi-version validation failure mode in the most important client paths.
- **Use many GraphQL compatibility fragments or precompiled query variants.** This can work for a small number of capabilities, but does not scale to a rich event timeline with many optional shapes and mixed server versions.
- **Move the first-party client to REST.** REST avoids GraphQL validation failures, but would require a separate custom live protocol anyway and would give up much of the typed contract we already have.
- **Expose NATS directly to browsers.** This would reuse existing broker concepts, but exposes too much transport and authorization detail and makes the client depend on broker semantics rather than a Chatto-owned protocol.
- **Introduce a Chatto-owned protobuf wire protocol.** This lets the server send client-shaped view messages over an authenticated websocket, and lets request/response calls share the same connection as live delivery.

## Decision

Introduce a Chatto-owned protobuf wire protocol as the preferred API direction for the first-party bundled web client.

The protocol is browser-facing, authenticated, and server-owned:

- `/api/server` advertises optional live protocol discovery metadata.
- `POST /api/live-token` mints a short-lived ticket. Cross-origin clients authenticate this token request with their bearer token; same-origin clients may use their existing cookie session.
- `GET /api/live?ticket=...` upgrades to a binary protobuf websocket.
- The server starts live delivery immediately after the websocket is authenticated. The client does not send a nested subscription request.
- The same websocket carries live push frames and typed request/response frames keyed by client request IDs.

The protocol exposes **client-facing view messages**, not the durable event log:

- Durable `corev1.Event` remains the persisted EVT fact shape and an internal delivery input.
- The websocket edge hydrates authorized EVT facts into `corev1.LiveEvent` / `LiveRoomEvent` payloads before sending them to the client.
- Historical message reads return hydrated `LiveRoomEvent` items plus stream-sequence cursors.
- Folded facts such as reactions and attachment/video-processing state are sent as current client views, not as raw durable fact rows the client must join itself.

GraphQL remains in the product, but its role changes:

- Existing GraphQL queries, mutations, and subscriptions remain available while this migration is experimental.
- GraphQL can remain an integrations/admin/ad-hoc query API if that continues to be useful.
- New first-party client surfaces should prefer the protobuf client wire protocol when mixed-version compatibility, live state, rich event rendering, or latency-sensitive reads matter.
- We will not expose `corev1.Event` as the public client API unless we make a separate explicit decision to publish durable domain facts for integrations.

The first production candidate scope is deliberately narrow:

- live server event delivery for the bundled client,
- message timeline initial load, older/newer pagination, around-message loading, single-event preview loading, and thread history loading,
- explicit capability advertisement,
- bounded per-connection request handling,
- connect/disconnect, request count/outcome/duration, and protocol error metrics.

## Consequences

### Positive

- **Mixed-version tolerance improves.** Protobuf clients can ignore unknown fields, and the server can omit fields or event variants a client does not understand without making the whole operation invalid.
- **The first-party client gets renderable data in one step.** Message posts, edits, reactions, attachment processing updates, and historical page rows can arrive as hydrated view messages without follow-up GraphQL queries.
- **Live and request/response share one authenticated transport.** The client can receive push events and issue history reads over the same websocket.
- **The wire protocol matches Chatto's architecture.** It sits after authorization and projection readiness, close to `StreamMyEvents`, while still hiding raw broker subjects and durable EVT internals from browsers.
- **We retain optional GraphQL for places where it is strong.** Integrations, admin tools, and exploratory querying may still benefit from GraphQL's introspection and ad-hoc selection model.

### Negative

- **We are creating a second API surface.** Until enough first-party paths migrate, the client has to straddle GraphQL and the protobuf wire protocol.
- **Tooling is less discoverable than GraphQL.** Protobuf gives strong generated types, but not GraphQL-style introspection, GraphiQL, or ad-hoc query composition. If this becomes an integrations API, we need SDKs, examples, and documentation.
- **The protocol needs product-level lifecycle discipline.** Message names, request types, capability flags, error codes, timeout behavior, and compatibility rules become public contracts once deployed broadly.
- **The websocket still needs rollout discipline.** The initial production candidate has request concurrency limits plus request/connection/error metrics, but we still need dashboards/alerts and real traffic feedback before expanding the protocol surface.
- **GraphQL compatibility does not disappear immediately.** Existing remote servers, existing clients, and non-migrated surfaces still depend on GraphQL behavior.

### Compatibility Rules

- Persisted protobuf messages in EVT remain more stable than transient client wire messages.
- Client wire protobufs should still evolve additively wherever practical.
- New client wire request types must have stable names and explicit error behavior.
- Server hello/capability advertisement must be used before the bundled client depends on optional wire features.
- Unknown live event variants must be safe for clients to ignore.
- The raw durable `corev1.Event` server-frame path is deprecated and must not be used for browser-facing delivery.

### Open Follow-Up Decisions

- Whether to publish the client wire protos as an external integrations API.
- Whether integrations should use the same websocket request/response transport, an HTTP protobuf RPC transport, generated SDKs, or a retained GraphQL API.
- How to version capabilities once multiple deployed client/server versions need simultaneous support.
- Which GraphQL surfaces remain long-term and which become compatibility-only.
