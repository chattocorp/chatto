# Instructions for Agents Working in `proto/chatto/api/v1/`

This directory defines the public `chatto.api.v1` ConnectRPC surface. Treat
these protos as the integration API first, even when the bundled frontend is the
first consumer.

## API Surface

- Keep ordinary public/frontend-used API in `package chatto.api.v1`.
- Keep unauthenticated bootstrap discovery in `package chatto.discovery.v1`.
- Keep public auth/capability-token flows in `package chatto.auth.v1`.
- Do not put ordinary authenticated integration APIs in either namespace just
  because they are public.
- Do not introduce an app-only namespace unless the behavior is inherently tied
  to one bundled client implementation and is unsuitable for external
  integrations.
- If app-only API is added later, document why it is app-only and still keep it
  stable enough for mixed bundled client/server versions.
- Prefer a broad, coherent base API over moving rough edges into a separate
  namespace.
- Prefer service names that make the resource and scope obvious to generated
  API consumers, even if that creates more services. Do not collapse unrelated
  resources into broad catch-all services just because they share a scope.
- A scoped service may own operations whose authorization and state naturally
  belong to that scope. `RoomService` is intentionally the home for room-scoped
  lifecycle, timeline, read-state, attachment-list, moderation, membership, and
  typing operations; split a resource out only when it needs an independent
  resource identity, authorization model, or CRUD/batch surface.
- A scoped lifecycle service may own the small resource reads that make that
  scope complete. Server-wide user directory reads belong on `UserService`.
  Room membership reads and commands belong on `RoomService` because their
  authorization and state are room-scoped alongside room lifecycle, timeline,
  moderation, attachments, read-state, and typing operations.
- Once a service name carries the scope, use concise resource method names such
  as `ListMembers`, `GetMember`, and `BatchGetMembers` rather than repeating
  the scope in every RPC name.
- Services should be exhaustive for their resource and scope, not limited to
  the current frontend workflow. If a normal client would expect list, get,
  batch get, create, update, or delete behavior for the resource, either provide
  it or document why that operation is intentionally absent.
- Follow the CRUD-like naming pattern for ordinary resource APIs:
  `List<ResourcePlural>`, `Get<Resource>`, `BatchGet<ResourcePlural>`,
  `Create<Resource>`, `Update<Resource>`, and `Delete<Resource>`. Use domain
  verbs only when a CRUD name would obscure lifecycle, authorization, audit, or
  product semantics.
- Add batch hydration when list/realtime surfaces expose resource IDs that
  clients are expected to hydrate. Avoid API shapes that force N+1 reads.
- Do not add `includes`-style response properties by default. Prefer direct
  resource fields plus `BatchGet*` follow-up hydration. `includes` maps are an
  exception for proven hot paths where one paginated response carries many rows
  that repeatedly reference the same related render data and the extra batch
  call would be materially harmful.

## Reused Shapes

- Reuse shared messages when semantics are shared.
- Do not reuse response-rich messages as request inputs when some fields are
  ignored. Add a request-only input message instead.
- Offset-based list RPCs should take `PageRequest page` and return
  `PageInfo page`.
- Do not add service-local `limit`, `offset`, `total_count`, or `has_more`
  fields for new offset-list APIs.
- Keep cursor/window APIs separate when the model is not offset pagination, for
  example timeline cursors or event-log sequence scans.
- Reuse canonical user shapes when they fit:
  - `User` for public identity, avatar, presence, and custom-status fields.
  - `DirectoryMember` for directory/member rows with roles and
    membership-oriented metadata.
- Add a new user-shaped message only when shared shapes cannot represent the
  visibility or lifecycle semantics. Prefer embedding or returning a canonical
  shape plus extra fields over copying identity fields into another local type.
- When a separate user-shaped message is still needed, explain the visibility
  reason in the message comment.
- Treat canonical shape reuse as a resource-wide rule, not only a user-shape
  rule. Avoid adding frontend-specific flavors of the same resource unless
  distinct authorization, visibility, lifecycle, or performance semantics
  require it and are documented.
- Prefer rich protobuf response messages over scalars when returning the
  mutated/read resource is cheap and does not change authorization. Scalar
  booleans are acceptable for simple acknowledgements where returning a rich
  resource would require extra work or misleading visibility.
- For extensible key spaces such as permissions, avoid one protobuf field per
  key. Use repeated keyed rows, for example `{ permission, granted }`, so
  built-in and integration-defined keys share one forward-compatible shape.

## Absence Semantics

- Singular lookup/read RPCs should return `NOT_FOUND` when absence means the
  requested resource does not exist.
- Use optional response fields only when absence is a successful, meaningful
  result state.
- Batch and list APIs may omit missing resources or return empty result lists.
- Prefer explicit comments on RPCs when missing-resource behavior is important
  to clients.
- Keep list/get/batch authorization boundaries coherent. Do not let a list
  disclose state that the corresponding get/batch APIs cannot hydrate unless
  the response explicitly models a redacted indicator.

## Field Presence

- `Update*` request messages should use patch semantics by default. Use
  proto3 `optional` scalar fields or a field mask so clients can distinguish
  "leave unchanged" from "set to default/empty". If an operation is a full
  replacement, name it `Replace*` or document the compatibility rationale.
- When one operation targets the same resource by multiple equivalent
  identifiers, model the target as a request `oneof`. Do not use parallel
  optional/string identifier fields. Split into separate RPCs only when the
  identifiers have different authorization, visibility, absence semantics,
  response shape, or performance behavior.
- Use proto3 `optional` scalar fields when clients must distinguish
  absent/unhydrated/unknown from a scalar default.
- Avoid parallel `*_present` booleans for simple scalar presence.
- Use enums or oneofs only when modeling multiple meaningful availability
  states.
- Avatar URL fields should be optional when the URL may be unavailable.

## Comments And Documentation

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

- Do not renumber fields that may be persisted or consumed by clients.
- Do not change a field type at an existing tag without an explicit compatibility
  decision. Prefer adding a new tag.
- Removing a field requires both `reserved <tag>` and `reserved "<name>"`.
- Renames are wire-safe but code-breaking; update generated consumers in the
  same change.
- The project is pre-1.0, but public API breakage still needs an explicit plan
  and PR compatibility note.
- Follow [ADR-045](../../../../docs/adr/ADR-045-public-api-stability-tiers.md)
  for integration API, bundled app API, and realtime protocol stability tiers.

## Code Generation

- Public `.proto` or ConnectRPC service changes require `mise codegen-proto`.
- Commit all generated Go/TypeScript bindings and docs-website ConnectRPC
  reference outputs.
- New public services also need generated docs grouping in
  `tools/split-connectrpc-docs.mjs`; the splitter fails codegen when a service
  is not assigned to a reference page.
- New generated reference pages need docs sidebar entries in
  `apps/docs-website/astro.config.mjs`.
