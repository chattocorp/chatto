# @chatto/api-client

`@chatto/api-client` contains the tiny shared ConnectRPC client plumbing used by
the bundled Chatto frontend.

Use the generated `@chatto/api-types` ConnectRPC clients when reviewing or
building against the public API shape.

This package must stay a transport/auth utility layer:

- Generated clients from `@chatto/api-types` are the source of truth.
- Shared transport, auth headers, and auth-expiry hooks live in `src/connect.ts`.
- App-local compatibility DTOs and view-model mappers live in
  `apps/frontend/src/lib/api-client`, not in this package.
- New RPC coverage should be consumed through generated clients first.
