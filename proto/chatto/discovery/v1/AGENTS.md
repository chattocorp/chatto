# Instructions for Agents Working in `proto/chatto/discovery/v1/`

Follow [the shared protocol rules](../../../AGENTS.md) for API shape, comments,
compatibility, and code generation.

This directory contains the public unauthenticated `chatto.discovery.v1`
ConnectRPC API.

## API Surface

- Keep this package small. Use it for pre-authentication bootstrap and
  discovery metadata.
- Keep public auth/capability-token flows in `chatto.auth.v1`; do not put them
  here just because they intentionally do not require a normal user session.
- Do not move ordinary authenticated integration APIs here. Those belong in
  `chatto.api.v1` unless they are visibly administrative, in which case they
  belong in `chatto.admin.v1`.
- Public-but-authenticated read APIs are not discovery APIs.
- Reflection remains public Connect infrastructure, not a discovery service.

## Comments And Authorization

- Each service and RPC comment must say if the method is public or needs a
  capability token.

## Reused Shapes

- Reuse canonical messages from `chatto.api.v1` when the returned resource or
  shared shape already has the right public visibility, for example public
  server profile/login metadata and linked external identity metadata.
- Own messages in `chatto.discovery.v1` when their natural lifecycle is a
  discovery/capability-token request or response.

## Client Compatibility

- Do not expose implementation-level feature or method support as public
  discovery capabilities. The bundled client keeps explicit minimum server
  versions for features it gates.
- Do not make the server declare minimum client versions. Clients own their
  minimum supported server release and compare it with the discovered server
  software version. Third-party clients should pin and test supported releases.
