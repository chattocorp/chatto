# Live Events

For the canonical "adding a new live event type" checklist, see
`.claude/rules/backend.md` (the **Event Patterns** and **Live Event
Authorization** sections). The notes below capture pitfalls that didn't
fit there.

## Common Pitfall: Race Conditions with KV State

If your event relies on KV state that's set alongside the event (e.g.,
setting a processing status in RUNTIME KV and then publishing an event),
make sure the KV write happens **before** the action that triggers the
subscription event. The subscription delivers events immediately, and
field resolvers that read KV will see stale/missing data if the write
hasn't happened yet.

Example: Video processing sets PENDING state in KV *before*
`PostMessage` publishes to JetStream, so that when the subscription
resolves `Attachment.videoProcessing`, the KV entry already exists.

## Common Pitfall: Double-Publishing

The `SERVER_EVENTS` stream's RePublish config wires every accepted
message from `server.>` to `live.server.>` automatically. So if you
publish a durable event via `publishServerEvent(...)` AND also publish
the same conceptual event via `publishLiveEvent(...)` / a
`publishLive*Event` helper, subscribers will receive it twice. Pick one
path per event type.
