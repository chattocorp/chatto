# ADR-115: Message Post Idempotency in Runtime State

**Date:** 2026-10-08

**Status:** Accepted

## Context

A message post can commit before its caller receives a response. A retry must
work across replicas and restarts. ConnectRPC, MCP, and the shared client need
the same operation. Caller-controlled keys must not add data or subjects to
EVT. See [ADR-036](ADR-036-runtime-state-kv-boundary.md),
[ADR-087](ADR-087-request-time-authorization-with-aggregate-occ.md), and
[ADR-114](ADR-114-jetstream-storage-conventions.md).

## Decision

The canonical message operation accepts an optional UUID. Normalize it to
lowercase hyphenated form. Scope its KV key to the operation and authenticated
account: `message_post.{actorId}.{uuid}`. The room and all effective caller
arguments are bound by a deterministic request fingerprint. Use the server
secret to calculate an HMAC, so the stored fingerprint does not expose message
text through guesses.

Create an immutable `runtime_state.v1.MessagePostClaim` in `RUNTIME_STATE`.
It contains only a server-generated message event ID and the fingerprint.
Use a per-key TTL of 30 minutes. Retries do not refresh this window. Keep the
existing bucket expiry markers. A removed or expired key can be claimed again.

KV `Create` reserves the identity. It does not prove that a message was posted.
After a collision, read through the stream leader again; an in-flight claim
can still fail. Reject a different fingerprint. Exact retries use the reserved
message event ID.

Before deciding that the message is absent, catch the local room projection up
to a captured room tail. Repeat the identity check before write authorization
inside every room OCC attempt. If EVT contains that ID, return it under current
message-read authorization. If it is absent, append through the existing atomic
message command and room OCC guard. Bound active attempts by the claim expiry.
An edit or retraction does not remove the original post identity.

No caller key, key hash, fingerprint, receipt event, or new field enters EVT.
EVT remains the sole evidence that the message committed. There is no lease,
completion update, or new resource. No-key posts use the existing behavior.

The shared client retains one key and prepared request per logical send,
including uploaded asset IDs. It does not persist these objects on the device
([ADR-107](ADR-107-keep-chat-data-out-of-device-storage.md)). A retry reaches the
server again, so current authorization and message state govern its response.

## Consequences

Concurrent replicas converge on one message without a transport-specific
domain path. A crash after reservation can resume the same identity. A lost
publish acknowledgement or response can be resolved from EVT.

The collision window is bounded. Callers must stop retries before 30 minutes
and check the original result before starting a new send. After expiry, the
same key can create another message. A failed post can hold its claim until
expiry; changed arguments need a new logical send.

The public API change is additive and needs no EVT migration. Retry safety
requires every serving replica to implement this operation. Older servers
can ignore the field and cannot supply this guarantee.
