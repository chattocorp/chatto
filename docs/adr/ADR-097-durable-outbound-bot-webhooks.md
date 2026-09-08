# ADR-097: Deliver Best-Effort Outbound Bot Webhooks from EVT

**Date:** 2026-09-06

## Context

Bots need HTTP integrations with retries. Notification preferences must not
control bot activation. A generic durable job queue adds a subsystem for one
current use case. This version accepts loss of pending delivery on restart.

## Decision

Consume direct mentions and DM messages through one shared durable EVT
consumer. Put one delivery per selected endpoint into a process-local channel
with 64 slots. Acknowledge the source after all destinations enter the channel.
Eight workers per process send HTTP requests and wait between retries. A full
channel blocks source handoff. No separate stream, persisted job protobuf, or
KV state is used.

Capture the EVT tail before the endpoint projection check. Share that check
across source messages through the captured tail to reduce JetStream queries
during bursts and replay. Messages after that tail require a new check.
HTTP attempts still check current endpoint state.

Each delivery holds message references, endpoint ID and activation sequence, attempt limit,
retry delay, and source-time expiry. It holds no plaintext body or credentials.
Workers count attempts and use cancellable timers for exponential backoff,
with a 30-minute delay cap. Operators set retry and expiry policy in TOML or
ENV. Shutdown cancels requests and timers and discards accepted work.

Terminal failure storage is defined in [ADR-098](ADR-098-retained-operational-log.md).
New terminal failures enter retained LOG history. Success and intentional skips
produce no records. Delivery IDs remain stable across source handoffs.

Keep up to 20 independent encrypted endpoints per bot, including paused ones.
Use the bot's PII key for each name, URL, optional Authorization value, and
signing secret. Names and signing secrets are fixed after creation. Managers
can change the URL and replace or remove the Authorization header. Edits record
a new encrypted configuration with the same endpoint ID and preserve the original
creation time. Omitted fields keep their current values on each OCC retry.
Edits cancel queued work for the previous configuration. Pause and
resume record state changes without encrypting new credentials. Each enabled
period has an EVT sequence cutoff. Old work stays cancelled after resume.
Revocation permanently removes one endpoint. User-aggregate OCC enforces the
collection limit and lifecycle across replicas. Each endpoint has one stable ID
from creation until revocation.

Use current authorization and message content before sending. Retraction,
deletion, and access loss stop delivery. Notification state has no effect.
Require public HTTPS destinations. Permit HTTP and HTTPS for `localhost` and
`*.localhost` only when all resolved addresses are loopback. Validate addresses
at connection time and dial them without another lookup. IP literals and
other hosts have no private-address exception. Redirects are never followed. Each request has a ten-second timeout, bounded by expiry.

## Consequences

Delivery is best effort. A restart can lose work after source acknowledgement.
A lost response, partial source handoff, or lost source acknowledgement can
repeat a request. No ordering guarantee applies. The durable source consumer
prevents routine replay of accepted work but does not make HTTP delivery
reliable across restart. Restoring or recreating the source consumer can
repeat previously accepted work.

Concurrency and buffered work are bounded per process. Retries occupy worker
slots while they wait, so failed endpoints can delay other deliveries. A
future durable implementation can keep the public webhook contract.

The bot page shows the latest retained failure and a paginated history for each
endpoint. Expiry removes these diagnostics. The projection retains only endpoint
configuration and activation state. Payload text is the currently readable
message text on each attempt.
