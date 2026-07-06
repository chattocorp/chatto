# Instructions for Agents Working in `proto/chatto/discovery/v1/`

This directory defines the public unauthenticated `chatto.discovery.v1`
ConnectRPC surface.

## API Surface

- Keep this package narrow: it is for pre-authentication bootstrap and
  discovery metadata.
- Keep public auth/capability-token flows in `chatto.auth.v1`; do not put them
  here just because they intentionally do not require a normal user session.
- Do not move ordinary authenticated integration APIs here. Those belong in
  `chatto.api.v1` unless they are visibly administrative, in which case they
  belong in `chatto.admin.v1`.
- Public-but-authenticated read APIs are not discovery APIs.
- Reflection remains public Connect infrastructure, not a discovery service.

## Comments And Authorization

- Every service and RPC comment must state whether the method is fully public or
  requires a capability token.
- Comments should describe caller-visible behavior and notable absence/error
  semantics.
- Avoid implementation workflow text in comments that render into public docs.

## Reused Shapes

- Reuse canonical messages from `chatto.api.v1` when the returned resource or
  shared shape already has the right public visibility, for example public
  server profile/login metadata and linked external identity metadata.
- Own messages in `chatto.discovery.v1` when their natural lifecycle is a
  discovery/capability-token request or response.

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
