# Instructions for Agents Working in `proto/chatto/auth/v1/`

This directory defines public authentication and authorization-related
ConnectRPC flows that are not ordinary authenticated account API calls.

## API Surface

- Keep this package focused on public auth flows with a distinct security model,
  such as capability-token external identity confirmation.
- Do not put server metadata/bootstrap discovery here; that belongs in
  `chatto.discovery.v1`.
- Do not put ordinary authenticated account management here; self-service user
  account behavior belongs in `chatto.api.v1.MyAccountService`.
- Capability-token RPCs must validate the token inside the service/core model
  before exposing resource state or performing changes.

## Comments And Authorization

- Every service and RPC comment must state whether the method is fully public,
  requires a capability token, or requires an authenticated user.
- Comments should describe caller-visible behavior and notable absence/error
  semantics.
- Avoid implementation workflow text in comments that render into public docs.

## Reused Shapes

- Reuse canonical messages from `chatto.api.v1` when the returned resource or
  shared shape already has the right public visibility.
- Own messages in `chatto.auth.v1` when their natural lifecycle is an auth flow
  request or response.

## Compatibility

- Follow the public API compatibility rules in `proto/AGENTS.md`.
- The project is pre-1.0, but package, service, and method path changes still
  need an explicit plan and PR compatibility note.

## Code Generation

- Public `.proto` or ConnectRPC service changes require `mise codegen-proto`.
- Commit generated Go/TypeScript bindings and generated docs output.
- New services need generated docs grouping in
  `tools/split-connectrpc-docs.mjs` and sidebar entries in
  `apps/docs-website/astro.config.mjs`.
