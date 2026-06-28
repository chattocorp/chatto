# Instructions for Agents Working in `proto/`

Protobuf definitions feed persisted state, generated Go/TypeScript bindings,
ConnectRPC services, and the public API reference.

## Public API Protos

For `chatto/api/v1`:

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
- Do not change a field type at an existing tag. Add a new tag instead.
- Removing a persisted field requires both `reserved <tag>` and
  `reserved "<name>"`.
- Renames are wire-safe but code-breaking; update generated consumers in the
  same change.
- Persisted protobufs in `EVT`, `RUNTIME_STATE`, `ENCRYPTION_KEYS`, and object
  metadata need additive evolution plus repair/migration code when existing data
  changes shape.
- Transient live-event protos are less stable, but still consider public API
  behavior and mixed-version clients.

## Presence And API Shape

- Public ConnectRPC API protos should default to `package chatto.api.v1`. Do not
  introduce an app-only namespace for ordinary frontend-used features unless the
  shape is clearly unsuitable for external integrations.
- Reuse shared messages when the semantics are shared. Offset-based list RPCs
  should take `PageRequest page` and return `PageInfo page`; do not add
  service-local `limit`, `offset`, `total_count`, or `has_more` fields for new
  list APIs.
- Reuse canonical user shapes when they fit: `UserSummary` for lightweight
  render/cache references, `UserPresenceSummary` when presence/custom status is
  part of the result, and `DirectoryMember` only for directory/member rows with
  roles and membership-oriented metadata.
- For singular lookups, return `NOT_FOUND` when absence means the requested
  resource does not exist. Use optional response fields only when absence is a
  successful, meaningful state. Batch/list APIs may omit missing resources or
  return empty result lists.
- For public API messages under `chatto/api/v1`, use proto3 `optional` scalar
  fields when clients must distinguish absent/unhydrated/unknown from a scalar
  default.
- Avoid parallel `*_present` booleans for simple scalar presence.
- Use enums or oneofs only when modeling multiple meaningful availability states.

## Code Generation

- Public `.proto` or ConnectRPC service changes require `mise codegen-proto`.
- Commit all generated Go/TypeScript bindings and docs-website ConnectRPC
  reference outputs.
- New public services also need entries in `proto/buf.gen.yaml` and the docs
  sidebar in `apps/docs-website/astro.config.mjs`.
