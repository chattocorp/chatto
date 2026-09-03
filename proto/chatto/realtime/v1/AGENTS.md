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
- `chatto.realtime.v1` is the protobuf namespace; protocol version 4 is the
  only accepted handshake. Do not reintroduce older compatibility paths.
- Resume cursors are signed JWTs that are viewer-bound. Their `p` claim is an
  HMAC of the stream incarnation, viewer, subscription scope, and EVT
  sequence. Never put NATS or JetStream identities, sequences, subjects, or
  other persistence details in JWT claims.
- Resume cursors have a 15-minute lifetime. An invalid, expired, foreign, or
  out-of-window cursor must use the requested snapshot or live-only fallback.
  Do not use a partly trusted replay position.
- A client must never advance its resume cursor across an undecodable frame or
  unknown top-level frame. Additive semantic event variants are skippable
  because common event metadata and the cursor remain outside the event
  `oneof`. Use a new behavioral protocol version when a new variant needs
  required client behavior.
- `RealtimeEvent.event` is the public variant catalogue. Each member must use
  the same name, field number, and payload message as its matching canonical
  Event member. Do not add a second public payload message for the same
  semantics.
