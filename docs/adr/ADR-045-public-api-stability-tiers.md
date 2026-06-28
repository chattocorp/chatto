# ADR-045: Public API Stability Tiers

**Date:** 2026-06-28

## Context

Chatto now uses protobuf definitions as the source of truth for its public
request/response API and realtime WebSocket frames. The bundled frontend is the
first and most complete consumer, but the API is also meant for integrations,
bots, tooling, SDKs, and alternate clients.

The project is still pre-1.0, so it can make intentional breaking changes.
However, Chatto is public, self-hosted, and some deployments track moving
images. Mixed client/server versions can exist even before 1.0, and API
compatibility needs to be explicit enough that contributors know when they are
changing an integration contract.

Previous discussion considered splitting the surface by default into
integration and app packages. The API cleanup work instead kept a broad,
coherent `chatto.api.v1` base surface and moved reusable user, pagination, and
response semantics into shared shapes. That leaves one remaining question:
which parts of the protobuf API carry which stability promise?

## Decision

Chatto defines three public API stability tiers:

1. **Integration API**: `chatto.api.v1` ConnectRPC services and shared messages
   that represent coherent external-client behavior. This is the default home
   for public and frontend-used APIs.
2. **Bundled app API**: exceptional protobuf APIs that are inherently tied to
   one bundled web-client workflow and do not make sense as an external
   integration contract. These may use a separate app-specific namespace later,
   but only with an explicit reason.
3. **Realtime protocol**: `chatto.api.v1.Realtime*` protobuf frames exchanged at
   `/api/realtime`. These are documented separately from ConnectRPC because
   their compatibility model includes long-lived connection control, heartbeat,
   and reconnect behavior.

`chatto.api.v1` remains integration-first. Frontend-used APIs should stay in
that package when they can be explained as normal external-client operations.
The project should not move rough edges into an app package merely to avoid
cleaning the integration surface.

The integration API follows these rules:

- Field numbers and field types are stable once consumed by generated clients.
- Additive fields, enum values, methods, and services are preferred.
- Removing a field requires reserving both its tag and name.
- Renames are wire-safe but code-breaking and need an explicit PR note.
- Singular reads return `NOT_FOUND` for absent resources unless absence is a
  successful, documented nullable result.
- Offset-list APIs use `PageRequest` and `PageInfo`; cursor/window APIs use
  opaque cursor fields with documented direction semantics.
- Connect status codes are part of caller-visible behavior for common outcomes:
  unauthenticated, permission denied, invalid argument, not found, conflict, and
  failed precondition.
- Public comments in `.proto` files are consumer documentation.

The bundled app API tier, if introduced, is less strict about long-term SDK
ergonomics but must still tolerate reasonable bundled client/server skew. It
uses additive protobuf evolution where feasible, preserves auth and error
semantics for deployed clients, and documents any intentional skew boundary.

The realtime protocol is versioned by protocol behavior, not by ConnectRPC
method shape. Version 1 is live-only and does not provide replay cursors or
acknowledgements. Future frame additions must be additive where possible, and
new required client behavior must be negotiated through hello/capability fields
or a new protocol version.

Generated API reference documentation is split by domain instead of presented
as one large mixed page. ConnectRPC service pages, shared ConnectRPC types, and
realtime frames are distinct references, while `proto/chatto/api/v1/*.proto`
remains the source of truth.

CI runs Buf breaking-change checks against `origin/main` and codegen drift
checks. These checks are guardrails, not a replacement for compatibility review:
pre-1.0 breaking changes can still be accepted when the PR states the
compatibility plan.

## Consequences

Contributors have a default answer for new RPCs: put them in `chatto.api.v1`
and make the semantics clean enough for integrations. A separate app namespace
is available only for clearly app-specific behavior.

Generated docs become easier to navigate because ConnectRPC services are
grouped by domain and realtime frames are not mixed into the service reference.

The compatibility bar is higher than "pre-1.0 can break anything." Breaking API
changes are still possible, but they need to be intentional and visible in
review.

Buf breaking checks will catch many tag, type, enum, service, and method
compatibility mistakes. They will not catch every semantic break, such as a
changed error code or pagination interpretation, so reviewers still need to
apply the API conventions in `proto/chatto/api/v1/AGENTS.md`.
