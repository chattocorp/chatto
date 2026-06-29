# Instructions for Agents Working in `proto/chatto/api/v1/`

This directory defines the public `chatto.api.v1` ConnectRPC surface. Treat
these protos as the integration API first, even when the bundled frontend is the
first consumer.

## API Surface

- Keep ordinary public/frontend-used API in `package chatto.api.v1`.
- Do not introduce an app-only namespace unless the behavior is inherently tied
  to one bundled client implementation and is unsuitable for external
  integrations.
- If app-only API is added later, document why it is app-only and still keep it
  stable enough for mixed bundled client/server versions.
- Prefer a broad, coherent base API over moving rough edges into a separate
  namespace.

## Reused Shapes

- Reuse shared messages when semantics are shared.
- Offset-based list RPCs should take `PageRequest page` and return
  `PageInfo page`.
- Do not add service-local `limit`, `offset`, `total_count`, or `has_more`
  fields for new offset-list APIs.
- Keep cursor/window APIs separate when the model is not offset pagination, for
  example timeline cursors or event-log sequence scans.
- Reuse canonical user shapes when they fit:
  - `User` for public identity fields.
  - `UserProfile` when presence/custom status is part of the result.
  - `DirectoryMember` for directory/member rows with roles and
    membership-oriented metadata.
- Add a new user-shaped message only when shared shapes cannot represent the
  visibility or lifecycle semantics. Prefer embedding or returning a canonical
  shape plus extra fields over copying identity fields into another local type.
- When a separate user-shaped message is still needed, explain the visibility
  reason in the message comment.

## Absence Semantics

- Singular lookup/read RPCs should return `NOT_FOUND` when absence means the
  requested resource does not exist.
- Use optional response fields only when absence is a successful, meaningful
  result state.
- Batch and list APIs may omit missing resources or return empty result lists.
- Prefer explicit comments on RPCs when missing-resource behavior is important
  to clients.

## Field Presence

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
