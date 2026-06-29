# Instructions for Agents Working in `proto/chatto/realtime/v1/`

This directory defines the public `chatto.realtime.v1` protobuf WebSocket
protocol used at `/api/realtime`.

## API Surface

- Keep realtime WebSocket frames and protocol-control messages in
  `package chatto.realtime.v1`.
- Do not add unary ConnectRPC services here.
- Prefer importing stable public enums/messages from `chatto.api.v1` over
  duplicating shared client-visible semantics.
- Keep comments focused on wire behavior, connection lifecycle, authentication,
  and reconnect/catch-up expectations.

## Compatibility

- Follow the public API compatibility rules in `proto/AGENTS.md`.
- Realtime compatibility includes protocol behavior, not just protobuf field
  tags. New required client behavior must be negotiated through hello/capability
  fields or a new protocol version.
- Version 1 is live-only and has no acknowledgement or replay cursor contract.

