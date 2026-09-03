---
name: "chatto-api-rules"
description: "Design rules concerning Chatto's ConnectRPC API, both resources and realtime"
---

### ConnectRPC Resource API

- Define one canonical public protobuf for each resource. Reuse that protobuf
  across list, get, batch, and mutation-response surfaces when the
  authorization and lifecycle semantics are the same.
- Keep services complete for their resource and scope. Add bounded batch reads
  when events or related resources commonly expose IDs that clients must
  hydrate.
- Keep commands, explicit reads, pagination, history, and read-your-writes
  responses in ConnectRPC. Realtime delivery does not replace these APIs.

### Realtime API

- Cover the complete useful product event surface for integrations, bots, and
  clients. Do not limit the public event catalogue to events used by the
  bundled frontend.
- Use `chatto.core.evt.v1.Event` for durable EVT facts and
  `chatto.core.live.v1.LiveEvent` for transient NATS Core signals. Use the dedicated
  `chatto.realtime.v1.RealtimeEvent` union and domain payload catalog
  for public delivery. Keep a semantic one-to-one relationship for selected
  public events without importing core payload types into the public schema.
- Keep transport concerns in `chatto.realtime.v1` wrappers. Handshakes,
  subscriptions, catch-up, cursors, heartbeats, errors, and close
  guidance are not domain events.
- Create a fresh authorized `RealtimeEvent` for each public delivery. The
  public payload schema must omit internal variants and storage-only fields.
  Never send stored bytes or mutate a stored event during mapping.
- Keep current resources in ConnectRPC. Let a resource client bind reads to an
  opaque realtime start cursor, then close the interval with event catch-up.
  Do not attach resource sidecars to normal event frames.
- Keep public cursors opaque, confidential, integrity-protected, and bound to
  their viewer and scope. Do not expose NATS or JetStream coordinates.
- Keep wire volume bounded. Resume must have sequence, event-count, time, and
  concurrency limits. Large and lazy collections stay in paginated ConnectRPC
  APIs.
- Use consistent product terminology and names. Public comments must not
  depend on backend terms such as projections, NATS, or JetStream.
- Use a new behavioral protocol version when a change requires all clients to
  change their behavior. Do not use a capability matrix to restate required
  frame semantics.
- When an `Event` or `LiveEvent` variant must reach clients, update the dedicated payload,
  public union member, authorization and mapper coverage, consuming reducers,
  generated clients, architecture inventory, public documentation, and
  compatibility notes in the same change. Keep durable public members aligned
  with their EVT name and union number. Map live members explicitly in the
  public transient number range. Use independent public payload field numbers
  and explicit typed mapping.
- Keep protobuf comments, public API overviews, tutorials, compatibility
  guidance, and release notes current.
