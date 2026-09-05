# Instructions for Agents Working in `proto/chatto/realtime/v1/`

This directory contains the public `chatto.realtime.v1` protobuf WebSocket
protocol at `/api/realtime`.

## API Surface

- Keep realtime WebSocket frames and protocol-control messages in
  `package chatto.realtime.v1`.
- Do not add unary ConnectRPC services here.
- Prefer importing stable public enums/messages from `chatto.api.v1` over
  duplicating shared client-visible semantics.
- Keep client-visible event payloads in `events.proto`. These messages are
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
  Negotiate new required client behavior with discovery metadata or a new
  protocol version.
- `chatto.realtime.v1` is the protobuf namespace; protocol version 4 is the
  only accepted handshake. Do not reintroduce older compatibility paths.
- Protect resume cursors with authenticated encryption.
  Use `cli/internal/publiccursor` with the realtime purpose and the viewer scope.
  Keep the EVT sequence, stream incarnation, version, and lifetime in the
  encrypted payload. Do not use JWTs. Read the position from the encrypted
  payload. Do not search candidate positions with HMAC comparisons.
- Resume cursors expire after 15 minutes. Use a cursor only after all
  validation checks pass. If replay is not possible, use the snapshot or
  live-only fallback specified in the subscription.
- Keep cursor validation separate from replay work limits. A cursor can be
  valid for a minimum-cursor RPC even when the replay work exceeds these limits.
- For a minimum-cursor RPC, wait until the local Server Content View includes
  the requested position. Limit the wait to 10 seconds. If the caller specifies
  a shorter deadline, use that deadline. Do not return historical state.
  Do not wait for other projections, effects, or events after the requested
  position.
- Use `cursor` in events, heartbeats, and `caught_up` frames.
  Use `resume_cursor` in the subscription. Set `caught_up.recovery` to the
  result: `RESUMED`, `SNAPSHOT`, or `LIVE_ONLY`. Use `RESUMED` when replay
  succeeds, including when there are no events to send.
- A client must never advance its resume cursor across an undecodable frame or
  unknown top-level frame. Additive semantic event variants are skippable
  because common event metadata and the cursor remain outside the event
  `oneof`. Use a new behavioral protocol version when a new variant needs
  required client behavior.
- `RealtimeEvent.event` is the public event catalogue. Its names and field
  numbers do not identify the internal source. Public payload field numbers
  are independent of EVT field numbers. Keep reserved tags and names.
  Do not change field numbers to fill gaps after you remove variants.
- When a new `Event` or `PubSubEvent` variant must reach clients, update
  `events.proto`, the `RealtimeEvent.event` union, the explicit mapper and
  exhaustive tests, the frontend event
  reducer or reconciliation path, generated clients, architecture and public
  documentation, and compatibility notes in the same change.
- A client-facing `PubSubEvent` variant must reference its public payload from
  `events.proto`. Keep the private pubsub union as the allow-list for cursorless
  delivery. Deep-copy the public event before caller-specific filtering.
- Live delivery and replay must use the same internal-to-public mapping path.
  A public union member without a valid mapping must fail tests and fail closed
  at runtime. A `PubSubEvent` is public only when the catalogue contains an
  explicit semantic mapping. Internal control events can map to protocol
  frames instead. `session_terminated` maps to `close`.
