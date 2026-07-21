# ADR-045: Public API Stability Tiers

**Date:** 2026-06-28

## Context

Chatto now uses protobuf definitions as the source of truth for its public
request/response API and realtime WebSocket frames. The bundled frontend is the
first and most complete consumer, but the API is also meant for integrations,
bots, tooling, SDKs, and alternate clients.

The project is still pre-1.0 and its public API is changing in response to
implementation experience and early integrator feedback. Chatto therefore
cannot yet promise that every released public API shape remains compatible.
However, Chatto is public, self-hosted, and some deployments track moving
images. Mixed client/server versions can exist even before 1.0, and API
compatibility needs to be explicit enough that contributors know when they are
changing an integration contract.

Previous discussion considered splitting the surface by default into
integration and app packages. The API cleanup work kept a broad, coherent
`chatto.api.v1` base surface, moved reusable user, pagination, and response
semantics into shared shapes, split public auth/capability-token flows into
`chatto.auth.v1`, split unauthenticated discovery/bootstrap into
`chatto.discovery.v1`, and then split clearly administrative and realtime
protocol concerns into separate packages. That leaves one remaining question:
which parts of the protobuf API carry which stability promise?

## Decision

Chatto defines six public API stability tiers:

1. **Auth API**: `chatto.auth.v1` ConnectRPC services for public
   authentication flows with a distinct capability-token security model, such
   as pending external identity confirmation.
2. **Discovery API**: `chatto.discovery.v1` ConnectRPC services for
   unauthenticated bootstrap, such as server/login discovery.
3. **Integration API**: `chatto.api.v1` ConnectRPC services and shared messages
   that represent coherent external-client behavior. This is the default home
   for authenticated public and frontend-used APIs.
4. **Administrative API**: `chatto.admin.v1` ConnectRPC services for public but
   visibly administrative operations such as server settings, roles,
   permissions, member administration, diagnostics, and audit-log reads.
5. **Bundled app API**: exceptional protobuf APIs that are inherently tied to
   one bundled web-client workflow and do not make sense as an external
   integration contract. These may use a separate app-specific namespace later,
   but only with an explicit reason.
6. **Realtime protocol**: `chatto.realtime.v1.Realtime*` protobuf frames exchanged at
   `/api/realtime`. These are documented separately from ConnectRPC because
   their compatibility model includes long-lived connection control, heartbeat,
   and reconnect behavior.

`chatto.auth.v1` is intentionally narrow. It contains public auth-flow RPCs
whose security boundary is a short-lived capability token carried in the
request. `chatto.discovery.v1` is also intentionally narrow: it contains
bootstrap metadata RPCs that must work before the caller has a normal session.
Public-but-authenticated read APIs remain integration APIs, not discovery APIs.

`chatto.api.v1` remains integration-first. Frontend-used APIs should stay in
that package when they can be explained as normal external-client operations.
Administrative services should live in `chatto.admin.v1` even when the bundled
frontend is their first consumer. The project should not move rough edges into
an app package merely to avoid cleaning the integration surface.

The public API is experimental while Chatto is pre-1.0. Compatibility is
preferred, not guaranteed. Package names ending in `v1` identify the current
wire namespace; until Chatto explicitly graduates the API, they are not a
long-term stability promise. Intentional breaking changes are allowed when
they materially improve the API, but must carry an explicit compatibility
plan, generated-client updates, public documentation updates, and release-note
guidance. Persisted `chatto.core.v1` messages remain subject to the stronger
non-breaking storage contract regardless of this public API posture.

Clients discover protocol support through `ServerDiscoveryService.GetServer`.
Protocol capability keys are distinct from server feature configuration and
authenticated viewer permissions. The bundled web client evaluates advertised
capabilities first and uses the server software version only as a fallback for
servers that predate compatibility metadata. A missing optional capability
should disable or degrade the affected feature, not make the whole server
unusable. New required client behaviour must be negotiated or accompanied by
an explicit minimum bundled-client version.

Within each tier, public API design follows the resource-completeness and
operation-vocabulary rules in ADR-044. The auth, discovery, integration,
administrative, and realtime surfaces should be coherent enough for external
clients and future contributors to infer where new features belong. A current
frontend screen is a useful consumer, but it is not the boundary of the API
contract.

The integration API follows these compatibility-by-default rules:

- Preserve field numbers and field types unless an intentional pre-1.0
  compatibility decision says otherwise.
- Additive fields, enum values, methods, and services are preferred.
- Removing a field requires reserving both its tag and name.
- Avoid renames; they are wire-safe but code-breaking and need an explicit PR
  compatibility note when they materially improve the experimental API.
- Singular reads return `NOT_FOUND` for absent resources unless absence is a
  successful, documented nullable result.
- Offset-list APIs use `PageRequest` and `PageInfo`; cursor/window APIs use
  opaque cursor fields with documented direction semantics.
- Connect status codes are part of caller-visible behavior for common outcomes:
  unauthenticated, permission denied, invalid argument, not found, conflict, and
  failed precondition.
- Public comments in `.proto` files are consumer documentation.

The administrative API follows the same protobuf evolution rules as the
integration API. The package split is about naming, generated-client scope, and
documentation grouping, not about making admin routes private or unstable.

The bundled app API tier, if introduced, is less strict about long-term SDK
ergonomics but must still tolerate reasonable bundled client/server skew. It
uses additive protobuf evolution where feasible, preserves auth and error
semantics for deployed clients, and documents any intentional skew boundary.

The realtime protocol is versioned by protocol behavior, not by its protobuf
package suffix. The `chatto.realtime.v1` namespace currently accepts only
protocol version 2 and provides compacted projection bootstrap plus bounded
resume. Public cursors are encrypted, authenticated capabilities and never
expose NATS/JetStream coordinates. Future frame additions must be additive where
possible, and new required client behavior must be negotiated through
hello/capability fields or a new protocol version.

Generated API reference documentation is split by domain instead of presented
as one large mixed page. ConnectRPC service pages, shared ConnectRPC types, and
realtime frames are distinct references, while `proto/chatto/api/v1/*.proto`,
`proto/chatto/admin/v1/*.proto`, `proto/chatto/discovery/v1/*.proto`, and
`proto/chatto/realtime/v1/*.proto` remain the source of truth.

CI runs Buf breaking-change checks against `origin/main` and codegen drift
checks. These checks are guardrails, not a replacement for compatibility review:
pre-1.0 public API breaking changes can still be accepted when the PR carries
the `api-breaking-change` label and states the compatibility plan. That label
only suppresses public API breaking checks for `chatto/auth/v1`,
`chatto/api/v1`, `chatto/admin/v1`, `chatto/discovery/v1`, and
`chatto/realtime/v1`; storage and internal protobuf checks, including
`chatto/core/v1`, still run. The local root-equivalent `chatto.operator.v1`
surface is reviewed separately and is not part of the public network API
posture.

Chatto can graduate these experimental packages to a compatibility guarantee
before or at 1.0 when the main service boundaries and semantic conventions have
settled, external integrations have exercised the API, supported schema or SDK
artifacts are published, and incompatible evolution can reasonably use a new
API version instead of reshaping `v1`.

## Consequences

Contributors have a default answer for new RPCs: put unauthenticated bootstrap
or capability-token behavior in `chatto.discovery.v1`, put normal
client/integration behavior in `chatto.api.v1`, put administrative behavior in
`chatto.admin.v1`, and make the semantics clean enough for their intended
public consumers. A separate app namespace is available only for clearly
app-specific behavior.

Generated docs become easier to navigate because ConnectRPC services are
grouped by domain, administrative services are visibly separate, and realtime
frames are not mixed into the service reference.

The compatibility bar is higher than "pre-1.0 can break anything." Breaking API
changes are still possible, but they need to earn their migration cost and be
intentional and visible in review and release notes.

Buf breaking checks will catch many tag, type, enum, service, and method
compatibility mistakes. They will not catch every semantic break, such as a
changed error code or pagination interpretation, so reviewers still need to
apply the API conventions in `proto/AGENTS.md` and the package-local
instructions.
