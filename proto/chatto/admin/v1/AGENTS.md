# Instructions for Agents Working in `proto/chatto/admin/v1/`

This directory defines the public `chatto.admin.v1` ConnectRPC surface for
administrative operations. These APIs are public and generated for clients, but
their names and docs should make the administrative scope obvious.

## API Surface

- Keep administrative ConnectRPC services in `package chatto.admin.v1`.
- Do not move ordinary client/integration behavior here just because it is
  currently used by the bundled frontend.
- Reuse shared shapes from `chatto.api.v1` when the semantics are the same.
- Keep authorization expectations explicit in service and RPC comments.

## Compatibility

- Follow the public API compatibility rules in `proto/AGENTS.md`.
- The package split is about API clarity and generated-client scope, not about
  making admin routes private or unstable.
- Breaking changes still need an explicit compatibility note and generated
  client/docs updates.
