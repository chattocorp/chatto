# ADR-042: Protobuf-First Public API with ConnectRPC and Realtime WebSocket

**Date:** 2026-06-22

**Supersedes:** [ADR-003](ADR-003-graphql-as-primary-api.md) for public API direction and implementation. Supersedes [ADR-004](ADR-004-authorization-at-api-boundary.md) for public API authorization placement.

## Context

GraphQL gave Chatto a fast early path for typed queries, mutations, and subscriptions from the embedded web client. It is now a poor fit for the direction of the product:

- Chatto needs a stable public integration surface for bots, SDKs, tooling, and alternate clients.
- Mixed-version clients are becoming normal because users can self-host and separately hosted clients may connect to older servers.
- GraphQL query validation makes additive schema changes riskier for multi-server clients than they should be.
- GraphQL subscriptions are not an ideal long-running realtime transport for Chatto's single app-session live-event stream.
- Chatto's backend is already event-sourced and protobuf-based internally, but GraphQL forces an additional schema and generated model layer at the public boundary.
- JSON field names, nullability, enum strings, and error shapes become a long-lived public contract as soon as they are exposed.

Chatto needs a public API contract that is typed, efficient, SDK-friendly, and compatible with its command/projection/live-event architecture. The contract should not expose storage internals such as raw EVT messages, NATS subjects, or JetStream sequence numbers.

## Decision

Chatto will move toward a protobuf-first public API.

Public API `.proto` files will be the authoritative contract for the next API generation. They should live separately from persisted event protos, for example under `proto/chatto/api/v1/`. Public API messages may reuse concepts from core models, but they are compatibility contracts for callers and must not simply expose durable EVT payloads.

The API has two primary transports:

1. **ConnectRPC over HTTP** for request/response service calls.
2. **A Chatto-defined protobuf WebSocket protocol** for long-lived realtime delivery.

ConnectRPC over HTTP is the canonical transport for requestful interactions:

- projection reads,
- commands and mutations,
- admin and tooling operations,
- bounded streaming operations such as imports, exports, uploads, downloads, or progress reporting.

Chatto will not use ConnectRPC streaming as the primary app-session realtime subscription mechanism. Connect streaming remains available for bounded flows, but long-lived live-event delivery needs app-level control over resume, heartbeat, cancellation, reconnect, backpressure, and multiplexing semantics.

The realtime WebSocket protocol will use binary protobuf frames. Its initial required scope is:

- connection hello and server capability/version announcement,
- authentication using the same effective identity model as the HTTP API,
- subscription to the caller's authorized live event stream,
- opaque resume cursors,
- heartbeat/ping behavior,
- protocol errors,
- close semantics and reconnect guidance.

WebSocket live events are public API events, not raw EVT facts. They may be derived from durable EVT events and transient live sync signals, but they must preserve authorization checks, projection readiness, and replay compatibility at the API boundary.

The WebSocket protocol should reserve frame shapes for future multiplexed RPC-over-WebSocket. That fast path is a future optimization, not part of the initial required implementation. If implemented, it must:

- use the same protobuf service contracts as ConnectRPC,
- call the same backend handlers or shared service boundary,
- preserve the same authorization, validation, OCC, read-your-writes, idempotency, and error semantics,
- avoid WebSocket-only product APIs except connection-control operations.

JSON is deferred. Chatto may later expose JSON encodings or generated JSON/HTTP convenience endpoints for selected public API protos, but JSON is not the source of truth for the new API contract.

Existing non-RPC HTTP endpoints are reviewed separately from the ConnectRPC API surface. In particular, `/api/server` remains a high-stability discovery surface, and auth, OAuth, uploads, asset delivery, webhooks, and health/metrics endpoints may remain explicit HTTP APIs where that shape is still appropriate.

New protobuf API methods must not duplicate operation-specific authorization in each transport. HTTP ConnectRPC and future RPC-over-WebSocket should call the same internal operation model for the use case. Transports authenticate the caller, decode/encode protocol messages, and map transport-specific errors. Internal models own authorization, validation, domain invariants, OCC/write orchestration, read-your-writes waits, and response shaping for the operation.

## Consequences

The public API contract moves from GraphQL schema files to protobuf service and message definitions. Field numbers, service names, method names, enum values, oneof shapes, streaming semantics, and error details become long-lived compatibility commitments.

Generated clients become a first-class part of the API strategy. Bots and integrations should normally use generated SDKs rather than hand-written HTTP calls. This makes strongly typed Go, TypeScript, Python, Rust, mobile, and other clients more realistic.

The first-party web client gets a cleaner split between request/response operations and app-session realtime delivery. The live connection can be designed around Chatto's actual needs instead of fitting them through GraphQL subscriptions or generic HTTP streaming.

Deferring JSON keeps the initial compatibility surface smaller. The cost is that casual `curl`-style integrations are less ergonomic until JSON support, SDKs, CLI tooling, or debug helpers exist.

Separating public API protos from persisted EVT protos prevents storage compatibility requirements from leaking into caller-facing contracts. It also means some mapping code is unavoidable.

ConnectRPC over HTTP remains the baseline for debuggability, infrastructure compatibility, and non-browser clients. RPC-over-WebSocket is only a latency and connection-count optimization for already-connected clients, so the project can defer its complexity until there is a concrete need.

The migration plan has completed: the bundled frontend uses ConnectRPC plus the realtime websocket, and the gqlgen GraphQL API is no longer mounted. Historical GraphQL compatibility is not retained in the current runtime.

Moving authorization from GraphQL resolvers into operation models reverses part of ADR-004. This is intentional: multiple public transports make resolver-local authorization a drift risk. Lower-level core helpers may still assume trusted callers, but public use cases should get model methods whose names and signatures encode the authorized operation.
