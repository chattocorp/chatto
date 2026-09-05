# Instructions for Agents Working in `proto/chatto/api/v1/`

Follow [the shared protocol rules](../../../AGENTS.md) for API shape, comments,
compatibility, and code generation. This package is a public integration API.

## Ownership

- Keep ordinary authenticated APIs in `chatto.api.v1`, bootstrap discovery in
  `chatto.discovery.v1`, and public auth flows in `chatto.auth.v1`.
- Add a bundled-client-only namespace only when external integrations cannot
  use the behavior. Document that reason and the required version support.
- Name services for their resource and scope. Do not combine unrelated
  resources only because they share a scope.
- `RoomService` owns room lifecycle, timeline, read state, attachments,
  moderation, membership, and typing. Split a resource out only when it needs
  independent identity, authorization, or resource operations.
- `UserService` owns the server-wide user directory. Room membership reads and
  writes belong in `RoomService`.
- Do not repeat service scope in method names. For example, use `ListMembers`,
  `GetMember`, and `BatchGetMembers` on a room-scoped service.

## Shared Messages

- Offset lists use `PageRequest page` and `PageInfo page`. Do not add local
  `limit`, `offset`, `total_count`, or `has_more` fields. Keep cursor and window
  APIs separate when they do not use offset pagination.
- Use `User` for public identity, avatar, presence, and custom status.
  Use `DirectoryMember` for directory rows with roles and membership metadata.
- Add a different user message only when visibility or lifecycle requires it.
  Explain the reason in its comment. Prefer a canonical message with extra
  fields to copied identity fields.
- For extensible keys such as permissions, use keyed rows such as
  `{ permission, granted }`. Follow the shared rules for finite key sets.
- Use optional response fields when absence is a successful result.
  Avatar URLs must be optional when they can be unavailable.
