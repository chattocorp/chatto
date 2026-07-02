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

Public ConnectRPC services live under purpose-specific public proto packages and
are implemented through generated Connect handlers. Public API protobuf comments
are part of the API documentation and should describe caller-visible behavior,
not implementation workflow.

`chatto.discovery.v1` is the narrow unauthenticated bootstrap and
capability-token surface. It is for calls a client can make before it has a
normal Chatto session, such as server metadata/login discovery and pending
external-identity confirmation. It is not a home for ordinary authenticated
read-only APIs.

`chatto.api.v1` is the broad base API surface for both integrations and the bundled web client. Frontend-used features should stay in this base API when their semantics can be made coherent for external clients. A separate app-specific API namespace is acceptable only for behavior that is inherently tied to one bundled client implementation; those APIs still need enough stability for mixed bundled client/server versions. ADR-045 defines the discovery, integration, bundled app, and realtime protocol stability tiers.

Public API packages should be resource-complete within their domain. The
`chatto.discovery.v1`, `chatto.api.v1`, `chatto.admin.v1`, and
`chatto.realtime.v1` surfaces are not shaped only around the bundled frontend's
current screens. When Chatto exposes a resource publicly, the service should
provide the natural discovery, public, administrative, or realtime operations
for that resource unless an operation is intentionally unsupported and
documented. The absence of a current bundled frontend caller is not enough
reason to omit a coherent public operation.

Resource-oriented ConnectRPC services should use consistent operation
vocabulary. The default lifecycle verbs are:

- `List<ResourcePlural>` for paginated or filtered collection reads;
- `Get<Resource>` for singular lookup by ID or other stable key;
- `BatchGet<ResourcePlural>` for keyed multi-read when clients commonly need to
  hydrate references or avoid N+1 request patterns;
- `Create<Resource>` for creating a resource;
- `Update<Resource>` for changing mutable resource fields;
- `Delete<Resource>` for removing, revoking, or permanently deleting a resource
  when that is the resource's actual lifecycle operation.

Domain verbs remain appropriate when the operation is not a normal CRUD action
or when the verb captures important authorization, audit, lifecycle, or product
semantics more clearly than a generic update. Examples include archive/restore,
join/leave, mark read, ban/unban, reorder, rotate, import/export, and call
control operations. Even then, method names should establish a repeatable
pattern for future services instead of mirroring one frontend control.

Service boundaries should optimize for API comprehension. Prefer explicit
resource-and-scope service names over broad catch-all services when the scoped
resources have distinct authorization, visibility, or absence semantics. The
scope belongs in the service name when it makes the resource easier to reason
about; once the service carries that scope, RPC names can stay concise. For
example, server membership rows and room membership rows are easier to discover
as `ServerMemberService.ListMembers` / `GetMember` / `BatchGetMembers` and
`RoomMemberService.ListMembers` / `GetMember` / `BatchGetMembers` than as one
generic member directory service with every method carrying the scope. This
explicitness is preferred over minimizing the number of generated services.

Public resource messages should be canonical per resource. Add narrower,
expanded, or package-specific messages only when visibility, security, lifecycle,
or transport semantics are genuinely different. Prefer returning or embedding
the canonical resource plus explicit related fields or include maps over
creating multiple frontend-shaped flavors of the same resource.

Request messages should make client intent explicit. `Update*` operations use
patch semantics by default, with proto3 `optional` scalar fields or a field mask
to distinguish "leave unchanged" from "set to default/empty". Full resource
replacement should be named `Replace<Resource>` or have an explicit compatibility
rationale. When one operation targets the same resource by multiple equivalent
identifiers, the request should use a `oneof` target instead of parallel
optional/string fields; separate RPCs are reserved for identifiers with
different authorization, visibility, absence, response-shape, or performance
semantics. Request inputs should not reuse response-rich messages when some
fields are ignored; define request-only input messages for those cases.

Response contracts should lean toward returning resource-shaped protobuf
messages instead of scalar acknowledgements when the server can do so without
changing authorization or forcing expensive extra reads. This keeps list/get/
batch families aligned, gives clients useful state after mutations, and leaves
room for additive response fields. Scalar responses remain appropriate for
simple predicates, counts, generated secrets/tokens, and commands whose updated
resource is unavailable or not meaningful.

Performance is part of the public API shape. Resources that are commonly
rendered together, referenced from other resources, emitted in realtime events,
or hydrated by ID should provide either batch lookup methods, documented
include-map responses, or another bounded fanout pattern so well-behaved clients
do not need N+1 RPC calls.

Repeated public semantics should use shared protobuf shapes instead of service-local copies. Offset-based list RPCs use `PageRequest page` and return `PageInfo page` unless they need a cursor/window model such as room timeline reads. Singular lookup RPCs return `NOT_FOUND` when the requested resource is absent. Batch/list RPCs may omit missing resources or return empty lists. Optional response fields should represent a successful nullable result, not a hidden not-found status.

Public user-shaped payloads should reuse the narrowest canonical shape that represents their semantics. `User` is the lightweight identity shape for embeds and include maps. `UserProfile` adds live presence and custom status for user-directory, notification, and call surfaces. Member rows, `ViewerUser`, and `AdminMember` remain separate because they carry membership, self-service, and admin-only fields with different visibility rules. Membership row services should be named by their scope, for example server members versus room members, rather than by the implementation concept of a directory.

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

Read responses that hydrate projected data from multiple sources should keep that fanout out of service handlers. Handlers stay responsible for caller/auth, request translation, pagination parameters, and response wrapping. Per-response loading, batching, bounded concurrency, and include-map construction should live in small response assemblers or focused helper functions near the service that owns the response shape.

These assemblers should stay concrete until repetition proves otherwise. Do not introduce a generic ConnectRPC loader package just because several endpoints hydrate data: most hydration code also owns response-shape details such as protobuf messages, include maps, viewer visibility, nullable fields, and endpoint-specific absence behavior. Shared helpers are appropriate for truly generic mechanics such as bounded parallel mapping, or after multiple assemblers share the same non-trivial loading behavior with the same semantics.

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

Some small request and response wrapping code remains in each handler. That is intentional. The handler layer should be thin and explicit rather than hiding service behavior behind broad reflection or generic transport abstractions. Heavier read-response assembly belongs in named assemblers/helpers so batching and concurrency are reusable without spreading loader control flow through handlers. A separate loader layer should be extracted only when the shared semantics are already visible in concrete assemblers.

Operation-specific authorization lives in shared core operation models for public API actions. This reduces transport drift, but it means older trusted core helpers can coexist with newer operation models. Trusted/internal callers may still use lower-level helpers; public transports should use operation models.

Authenticated malformed requests return unauthenticated before validation when no caller is present. Authenticated malformed requests then return validation errors. Tests should preserve that ordering because clients and security reviews will depend on it.

New service work carries a documentation and generation burden. That cost is acceptable because protobuf service definitions are the public API contract and generated clients/docs are part of that contract.
