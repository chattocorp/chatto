# Instructions for Agents Working in `proto/`

Protobuf definitions feed persisted state, generated Go/TypeScript bindings,
ConnectRPC services, and the public API reference.

## Public API Protos

For public API packages:

- Follow [chatto/api/v1/AGENTS.md](chatto/api/v1/AGENTS.md) for public
  ConnectRPC API consistency rules.
- Follow [chatto/admin/v1/AGENTS.md](chatto/admin/v1/AGENTS.md) for
  administrative ConnectRPC API consistency rules.
- Follow [chatto/auth/v1/AGENTS.md](chatto/auth/v1/AGENTS.md) for public auth
  and capability-token ConnectRPC API consistency rules.
- Follow [chatto/discovery/v1/AGENTS.md](chatto/discovery/v1/AGENTS.md) for
  unauthenticated discovery/bootstrap ConnectRPC API consistency rules.
- Follow [chatto/realtime/v1/AGENTS.md](chatto/realtime/v1/AGENTS.md) for the
  realtime WebSocket protobuf protocol.
- Write comments for API consumers, not Chatto maintainers.
- Every public service, RPC, message, enum, enum value, and important field
  should have useful comments.
- Explain what the call reads or changes, required IDs, pagination/cursor
  semantics, login availability, and notable response behavior.
- Keep field comments short enough for generated tables; put longer behavior
  notes on messages or RPCs.
- Do not include maintainer workflow text such as "run codegen" in comments that
  render into public docs.

## Compatibility

- The public auth, discovery, integration, admin, and realtime `v1` packages
  are experimental while Chatto is pre-1.0. Compatibility is preferred, not
  guaranteed. Breaking changes require an explicit design benefit,
  compatibility plan, generated-client/docs updates, release-note guidance,
  and the `api-breaking-change` PR label.
- Do not renumber fields that may be persisted or consumed by clients.
- Do not change a field type at an existing tag. Add a new tag instead.
- Removing a persisted field requires both `reserved <tag>` and
  `reserved "<name>"`.
- Renames are wire-safe but code-breaking; update generated consumers in the
  same change.
- Persisted protobufs in `EVT`, `RUNTIME_STATE`, `ENCRYPTION_KEYS`, and object
  metadata need additive evolution plus repair/migration code when existing data
  changes shape.
- Transient live-event protos are less stable, but `chatto/realtime/v1` is still
  a public wire protocol and must consider mixed-version clients.
- For new client behaviour, prefer stable discovery or realtime protocol
  capability keys over software-version checks. Keep protocol support distinct
  from server feature configuration and viewer authorization.
- Public cursor fields are confidential, integrity-protected tokens. Never
  serialize raw or reversibly encoded NATS/JetStream stream identities,
  subjects, sequences, revisions, or consumer positions into them. Explicit,
  owner-only broker diagnostics and event-log inspection messages may expose
  clearly named operational details; do not reuse those shapes as client or
  integration contracts.
- The `chatto.realtime.v1` package suffix is a protobuf namespace. It currently
  implements only behavioural protocol version 2; older handshakes are rejected.

## Presence And API Shape

- Public ConnectRPC and realtime surfaces are product APIs, not adapters for the
  current frontend. Define the natural public, administrative, auth, discovery,
  or realtime operation even when the bundled frontend does not call every RPC
  yet.
- Prefer resource-oriented services with obvious ownership and scope. New
  resource services should establish a repeatable CRUD-like pattern for future
  features:
  - `List<ResourcePlural>` for visible collections.
  - `Get<Resource>` for one resource by ID or equivalent lookup target.
  - `BatchGet<ResourcePlural>` when clients commonly hydrate many resources or
    realtime/list results carry only IDs.
  - `Create<Resource>`, `Update<Resource>`, and `Delete<Resource>` for normal
    writes.
  - Domain verbs only when CRUD names would hide important lifecycle,
    authorization, audit, or product semantics.
- Keep services exhaustive for their resource and scope. Do not omit sensible
  list/get/batch/update/delete operations only because the current frontend does
  not need them yet.
- Batch hydration is part of the API design, not an optimization afterthought.
  If a resource can appear by ID in lists, notifications, realtime events, or
  related resources, provide `BatchGet*` or another documented bounded-fanout
  pattern so clients do not need N+1 reads.
- Actively discourage `includes`-style response properties as a default API
  shape. Prefer singular resources plus explicit `BatchGet*` hydration.
  `includes` maps are reserved for proven hot paths where one paginated response
  contains many rows that repeatedly reference the same related render data and
  avoiding a follow-up batch call is materially important.
- Public resource messages should be canonical per resource. Avoid multiple
  frontend-shaped variants of the same thing. Add narrower messages only when
  authorization, visibility, lifecycle, or performance semantics differ, and
  document that reason in the message comment.
- Prefer returning rich protobuf messages over scalar acknowledgements when the
  server can do so without changing authorization or forcing expensive extra
  reads. This keeps create/update/delete responses forward-compatible and
  aligned with list/get/batch shapes.
- For public API messages under `chatto/api/v1`, `chatto/admin/v1`,
  `chatto/auth/v1`, `chatto/discovery/v1`, and `chatto/realtime/v1`, use proto3
  `optional` scalar fields when clients must distinguish
  absent/unhydrated/unknown from a scalar default.
- Avoid parallel `*_present` booleans for simple scalar presence.
- Use enums or oneofs only when modeling multiple meaningful availability states
  or mutually exclusive request targets.
- When one operation targets the same resource by multiple equivalent
  identifiers, use a request `oneof`; do not use parallel optional/string
  identifier fields. Split into separate RPCs only when authorization,
  visibility, absence semantics, response shape, or performance behavior differ.
- `Update*` request messages should model patch semantics with optional scalar
  fields or a field mask. Full replacement operations should be named
  `Replace*` or have an explicit compatibility rationale.
- Avoid using response-rich messages as request inputs when some fields are
  ignored. Prefer request-only input messages that contain exactly the accepted
  fields.
- Keep authorization, visibility, and absence boundaries consistent across
  related APIs. A `List*` result should not reveal resource state that the
  matching `Get*` or `BatchGet*` contract then refuses to hydrate, unless that
  redacted/indicator state is explicitly modeled.
- Singular `Get*` methods return `NOT_FOUND` when absence is the error result.
  `BatchGet*` and list methods may omit missing or inaccessible resources, but
  document that behavior on the RPC.
- Generated public docs and TypeScript bindings are part of the API surface.
  When adding public RPCs, regenerate `@chatto/api-types` and docs in the same
  change. Do not recreate a handwritten API-client package; bundled frontend
  adapters belong under `apps/frontend/src/lib/api-client`.

## Code Generation

- Public `.proto` or ConnectRPC service changes require `mise codegen-proto`.
- Commit all generated Go/TypeScript bindings and docs-website ConnectRPC
  reference outputs.
- New public services also need generated docs grouping in
  `tools/split-connectrpc-docs.mjs`; the splitter fails codegen when a service
  is not assigned to a reference page.
- New generated reference pages need docs sidebar entries in
  `apps/docs-website/astro.config.mjs`.
