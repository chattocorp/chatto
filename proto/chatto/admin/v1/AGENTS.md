# Instructions for Agents Working in `proto/chatto/admin/v1/`

This directory defines the public `chatto.admin.v1` ConnectRPC surface for
administrative operations. These APIs are public and generated for clients, but
their names and docs should make the administrative scope obvious.

## API Surface

- Keep administrative ConnectRPC services in `package chatto.admin.v1`.
- Do not move ordinary client/integration behavior here just because it is
  currently used by the bundled frontend.
- Reuse shared shapes from `chatto.api.v1` when the semantics are the same.
- Do not reuse response-rich messages as request inputs when some fields are
  ignored. Add a request-only input message instead.
- Keep authorization expectations explicit in service and RPC comments.
- Prefer service names that name the administrative resource directly. Do not
  keep a narrower name when the service owns broader admin behavior; for
  example, admin user/member management should be named around users when it
  includes identity, password, cooldown, role, and member-detail operations.
- Avoid creating extra admin services only to separate self-service and admin
  mutations. The public `MyAccountService` owns current-caller self-service;
  admin user management belongs in an explicitly named admin service with
  permission-gated RPC comments.
- Administrative services should be exhaustive for their resource and scope,
  not limited to the current admin UI. If a normal admin client would expect
  list, get, batch get, create, update, or delete behavior for the resource,
  either provide it or document why that operation is intentionally absent.
- Follow the CRUD-like naming pattern for ordinary admin resource APIs:
  `List<ResourcePlural>`, `Get<Resource>`, `BatchGet<ResourcePlural>`,
  `Create<Resource>`, `Update<Resource>`, and `Delete<Resource>`. Use domain
  verbs only when a CRUD name would obscure lifecycle, authorization, audit, or
  product semantics.
- Add batch hydration when admin lists, audit/detail surfaces, or realtime
  events expose resource IDs that clients are expected to hydrate. Avoid API
  shapes that force N+1 reads.
- Prefer rich protobuf response messages over scalars when returning the
  mutated/read resource is cheap and does not change authorization. Scalar
  booleans are acceptable for simple acknowledgements where returning a rich
  resource would require extra work or misleading visibility.
- Keep canonical resource shapes shared with `chatto.api.v1` where visibility
  allows. Add admin-specific resource shapes only for admin-only fields or
  different authorization/visibility semantics, and document that reason.
- Keep list/get/batch authorization boundaries coherent. Do not let a list
  disclose state that the corresponding get/batch APIs cannot hydrate unless
  the response explicitly models a redacted indicator.
- `Update*` request messages should use patch semantics by default. Use
  proto3 `optional` scalar fields or a field mask so clients can distinguish
  "leave unchanged" from "set to default/empty". If an operation is a full
  replacement, name it `Replace*` or document the compatibility rationale.
- When one operation targets the same resource by multiple equivalent
  identifiers, model the target as a request `oneof`. Do not use parallel
  optional/string identifier fields. Split into separate RPCs only when the
  identifiers have different authorization, visibility, absence semantics,
  response shape, or performance behavior.

## Compatibility

- Follow the public API compatibility rules in `proto/AGENTS.md`.
- The package split is about API clarity and generated-client scope, not about
  making admin routes private or unstable.
- Breaking changes still need an explicit compatibility note and generated
  client/docs updates.
