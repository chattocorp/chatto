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

EVT already republishes committed facts once onto `live.evt.>`. If that
fact is deliverable to `myEvents`, do not also publish a `LiveEvent` for
the same conceptual UI update. Pick one path per event type: durable facts
go through EVT, transient sync goes through `live.sync.>`.

`SERVER_EVENTS` no longer has a live-delivery role. Do not add new
`live.server.>` subjects or direct Event-envelope mirrors.
