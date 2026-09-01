---
name: "chatto-api-rules"
description: "Design rules concerning Chatto's ConnectRPC API, both resources and realtime"
---

### ConnectRPC Resource API

- Define one canonical public protobuf for each resource. Reuse that protobuf
  across list, get, batch, mutation-response, and snapshot surfaces when the
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
- Use `chatto.core.evt.v1.Event` as the one semantic event vocabulary for
  durable facts, transient signals, replay, and public delivery. Do not create
  parallel public payload messages for the same event.
- Keep transport concerns in `chatto.realtime.v1` wrappers. Handshakes,
  subscriptions, resource snapshots, cursors, heartbeats, errors, and close
  guidance are not domain events.
- Create a fresh authorized Event for each public delivery. Omit internal
  variants and storage-only fields. Never send stored bytes or mutate a stored
  event during redaction.
- Use canonical public resource protobufs in snapshot chunks. Do not attach
  resource sidecars to normal event frames. Use ConnectRPC reads when a client
  needs complete or paginated resource state after an event.
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
- Keep protobuf comments, public API overviews, tutorials, compatibility
  guidance, and release notes current.
