# ADR-044: ConnectRPC Service Conventions

**Date:** 2026-06-25

## Context

ADR-042 moves Chatto toward protobuf-first public APIs served over ConnectRPC for request/response operations. The first migrated services established useful patterns, but the project now needs those patterns written down before the migration grows.

Without a shared convention, new ConnectRPC methods can drift in several risky ways:

- public/private exposure can be hidden in HTTP wiring instead of being visible in one registry;
- authentication can run after request decoding or validation, exposing inconsistent behavior across services;
- operation-specific authorization can be copied into every transport;
- request-size limits, protobuf validation, and error mapping can diverge service by service;
- generated public API docs and client bindings can fall out of sync with service implementation.

ConnectRPC should remain a transport boundary. Chatto's domain behavior still belongs in core models and projections, especially for event-sourced writes where authorization, OCC, projection readiness, and replay compatibility must stay consistent.

## Decision

All public ConnectRPC services live under `proto/chatto/api/v1` and are implemented through generated Connect handlers. Public API protobuf comments are part of the API documentation and should describe caller-visible behavior, not implementation workflow.

`chatto.api.v1` is the broad base API surface for both integrations and the bundled web client. Frontend-used features should stay in this base API when their semantics can be made coherent for external clients. A separate app-specific API namespace is acceptable only for behavior that is inherently tied to one bundled client implementation; those APIs still need enough stability for mixed bundled client/server versions. ADR-045 defines the integration API, bundled app API, and realtime protocol stability tiers.

Repeated public semantics should use shared protobuf shapes instead of service-local copies. Offset-based list RPCs use `PageRequest page` and return `PageInfo page` unless they need a cursor/window model such as room timeline reads. Singular lookup RPCs return `NOT_FOUND` when the requested resource is absent. Batch/list RPCs may omit missing resources or return empty lists. Optional response fields should represent a successful nullable result, not a hidden not-found status.

Public user-shaped payloads should reuse the narrowest canonical shape that represents their semantics. `User` is the lightweight render/cache identity shape. `UserProfile` adds live presence and custom status for notification and call surfaces. `DirectoryMember`, `ViewerUser`, and `AdminMember` remain separate because they carry directory, self-service, and admin-only fields with different visibility rules.

`connectapi.API.Handlers()` is the authoritative registry for mounted ConnectRPC services. Each registered handler includes:

- the generated service path;
- the generated HTTP handler;
- an explicit authentication policy.

The HTTP server owns route mounting and authentication middleware. Public services are mounted without caller injection. Authenticated services are wrapped with Connect-compatible authentication middleware before request decoding and protovalidate validation. Middleware resolves the effective user through the same bearer-token/cookie model used by the rest of the app and stores a `connectapi.Caller` in the Connect auth context.

Connect service methods use `requireCaller` for authenticated methods. They do not read transport-specific legacy auth context or duplicate HTTP session logic.

Every public Connect handler uses the shared `connectapi.HandlerOptions()` set. That set includes the public request-size limit and the protovalidate interceptor. Authenticated services should authenticate first, then decode and validate requests.

Protobuf validation handles stable wire-shape constraints such as required IDs, simple length bounds, enum domains, and pagination limits. Semantic validation remains in core operation models when it depends on permissions, room kind, projections, persisted state, or domain-specific invariants.

Public operation behavior should be centralized in focused core models. ConnectRPC handlers and future protobuf WebSocket RPC handlers should call the same operation model for the same user-facing action. Transports are responsible for:

- authenticating the caller;
- translating protocol messages into model inputs;
- translating model outputs into protocol responses;
- mapping model/domain errors to transport errors.

Core operation models are responsible for:

- operation-specific authorization;
- room kind and membership resolution;
- domain validation and invariants;
- event-sourced write orchestration and OCC;
- read-your-writes waits;
- response semantics shared across transports.

ConnectRPC errors are mapped through the shared `connectError` helper so core authentication, authorization, validation, not-found, conflict, and room-state errors produce consistent Connect status codes. Handlers should return `connect.NewResponse` for success and avoid service-local status-code mapping unless the public method has a deliberate protocol-specific error.

Adding a new public ConnectRPC service requires the same change set:

- public `.proto` service/messages with API-consumer comments;
- generated Go, TypeScript, and ConnectRPC API reference output from `mise codegen-proto`;
- `connectapi.API.Handlers()` registration with an explicit auth policy;
- tests that lock service path and auth policy;
- transport tests for auth-before-validation and validation behavior where applicable;
- operation tests for authorization and response semantics at the shared core model boundary;
- updates to `docs/ARCHITECTURE.md` and relevant FDRs;
- generated docs grouping in `tools/split-connectrpc-docs.mjs`;
- docs website sidebar/reference wiring when a new generated reference page is introduced.

## Consequences

ConnectRPC services become predictable to review: public surface, auth policy, validation, and handler options are visible in one place.

Some small mapping code remains in each handler. That is intentional. The handler layer should be thin and explicit rather than hiding service behavior behind broad reflection or generic transport abstractions.

Operation-specific authorization lives in shared core operation models for public API actions. This reduces transport drift, but it means older trusted core helpers can coexist with newer operation models. Trusted/internal callers may still use lower-level helpers; public transports should use operation models.

Authenticated malformed requests return unauthenticated before validation when no caller is present. Authenticated malformed requests then return validation errors. Tests should preserve that ordering because clients and security reviews will depend on it.

New service work carries a documentation and generation burden. That cost is acceptable because protobuf service definitions are the public API contract and generated clients/docs are part of that contract.
