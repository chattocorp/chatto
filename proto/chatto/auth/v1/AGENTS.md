# Instructions for Agents Working in `proto/chatto/auth/v1/`

Follow [the shared protocol rules](../../../AGENTS.md) for API shape, comments,
compatibility, and code generation.

This directory contains public ConnectRPC authentication and authorization
flows. These flows are not ordinary authenticated account API calls.

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

- Each service and RPC comment must say if the method is public, needs a
  capability token, or needs an authenticated user.

## Reused Shapes

- Reuse canonical messages from `chatto.api.v1` when the returned resource or
  shared shape already has the right public visibility.
- Own messages in `chatto.auth.v1` when their natural lifecycle is an auth flow
  request or response.
