# @chatto/api-client

`@chatto/api-client` is a private convenience package for the bundled Chatto
frontend. Its modules are grouped around frontend state and UI workflows, so they
are not the canonical public SDK surface.

Use the generated `@chatto/api-types` ConnectRPC clients when reviewing or
building against the public API shape.

This package must stay a thin compatibility/adaptation layer:

- Generated clients from `@chatto/api-types` are the source of truth.
- Shared transport, auth headers, and auth-expiry hooks live in `src/connect.ts`.
- Service modules may adapt protobuf messages into app-local render/state models,
  but should not introduce new public API concepts or duplicate transport setup.
- New RPC coverage should be available through generated clients first; add
  wrapper methods only when the bundled frontend needs a stable app adapter.
