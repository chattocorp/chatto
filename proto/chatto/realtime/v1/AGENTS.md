# Instructions for Agents Working in `proto/chatto/realtime/v1/`

This directory contains the public `chatto.realtime.v1` protobuf WebSocket
protocol at `/api/realtime`.

## API Surface

- Keep realtime WebSocket frames and protocol-control messages in
  `package chatto.realtime.v1`.
- Do not add unary ConnectRPC services here.
- Prefer importing stable public enums/messages from `chatto.api.v1` over
  duplicating shared client-visible semantics.
- Keep client-visible event payloads in the small domain event files. These messages are
  the public event catalogue and must not import `chatto.core` payload types.
- Public payloads contain only fields that a client can receive. Do not add
  storage placeholders, encrypted fields, key references, private moderation
  data, or runtime field-redaction annotations.
- Put decrypted delivery values in public `*_plaintext` fields. Do not add
  those fields to EVT messages.
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
- `RealtimeEvent.event` is the public variant catalogue. Keep each member's
  name and field number aligned with its matching canonical Event member.
  Public payload field numbers are independent.
- When a new canonical event must reach clients, update the applicable event file, the
  `RealtimeEvent.event` union, the explicit mapper and exhaustive tests, the frontend event
  reducer or reconciliation path, generated clients, architecture and public
  documentation, and compatibility notes in the same change.
- Live delivery and replay must use the same canonical-to-public mapper. A
  public union member without a valid mapping must fail tests and fail closed
  at runtime.
