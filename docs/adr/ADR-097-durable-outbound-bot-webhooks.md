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

Each delivery holds message references, endpoint generation, attempt limit,
retry delay, and source-time expiry. It holds no plaintext body or credentials.
Workers count attempts and use cancellable timers for exponential backoff,
with a 30-minute delay cap. Operators set retry and expiry policy in TOML or
ENV. Shutdown cancels requests and timers and discards accepted work.

Append only terminal failures to EVT. Aggregate OCC permits one failure fact
per delivery ID. If failure recording fails, log a safe category and stop.
Shutdown losses have no failure fact. Success and intentional skips have no
facts. Delivery IDs remain stable across repeated source handoffs so receivers
can detect duplicates.

Keep one encrypted endpoint configuration per bot. Use the bot's PII key for
its URL, optional Authorization value, and signing secret. Replacement creates
a new generation and stops older pending deliveries. New configurations do not
receive messages that precede their EVT position.

Use current authorization and message content before sending. Retraction,
deletion, and access loss stop delivery. Notification state has no effect.
Private-network access and HTTP require an explicit option. Redirects are
never followed. Each request has a ten-second timeout, bounded by expiry.

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

The bot page shows the latest recorded failure for the current configuration.
Later success does not clear that failure. The projection retains encrypted
settings and one failure per bot. Detailed failures remain in EVT. Payload text
is the currently readable message text on each attempt.
