# Instructions for Agents Working in `proto/chatto/realtime/v1/`

This directory contains the public `chatto.realtime.v1` protobuf WebSocket
protocol at `/api/realtime`.

## API Surface

- Keep realtime WebSocket frames and protocol-control messages in
  `package chatto.realtime.v1`.
- Do not add unary ConnectRPC services here.
- Prefer importing stable public enums/messages from `chatto.api.v1` over
  duplicating shared client-visible semantics.
- In comments, describe wire behavior, connection lifecycle, authentication,
  and reconnect or catch-up behavior.

## Compatibility

- Follow the public API compatibility rules in `proto/AGENTS.md`.
- Realtime compatibility includes protocol behavior and protobuf field tags.
  Negotiate new required client behavior with hello/capability fields or a new
  protocol version.
- `chatto.realtime.v1` is the protobuf namespace; protocol version 2 is the
  only accepted handshake. Do not reintroduce protocol-v1 compatibility paths.
- Resume cursors are encrypted, authenticated, and viewer-bound. They may use
  EVT coordinates internally but must never disclose NATS/JetStream identities,
  sequences, subjects, or other persistence details on the wire.
- Resume cursors have a limited public lifetime (currently 24 hours). An
  expired cursor must use compacted current state. Do not use a partly trusted
  replay position.
- A client must never advance its resume cursor across an undecodable frame,
  unknown top-level frame, or unknown projection operation. Protocol evolution must preserve that
  fail-closed invariant and provide an explicit negotiation/migration path for
  newly required operations.
