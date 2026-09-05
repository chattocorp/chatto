# ADR-097: Deliver Outbound Bot Webhooks from EVT

**Date:** 2026-09-05

## Context

Bots can read semantic realtime events. Realtime recovery has a time limit.
An HTTP integration needs independent retries and a durable delivery result.
Notification preferences must not control bot activation.

## Decision

Use two named durable consumers on EVT. The source consumer selects direct
mentions and DM messages. It commits one delivery request per bot endpoint.
The request records the source reference, endpoint generation, attempt limit,
retry delay, and expiry. The delivery consumer sends the HTTP request.
No new stream is required.

Use a deterministic delivery ID and a separate aggregate per delivery. OCC
prevents duplicate requests and permits one terminal outcome. Store success,
terminal failure, and intentional skip outcomes in EVT. Store attempt
reservations in RUNTIME_STATE with revision-based updates. Delete this runtime
state after the terminal outcome commits. Do not store individual HTTP errors
or response bodies in EVT.

Keep one encrypted endpoint configuration per bot. Use the bot's PII key for
its URL, optional Authorization value, and signing secret. Configuration
replacement creates a new generation and cancels older pending deliveries.
New configurations do not receive messages that precede their EVT position.

Use current authorization and current message content before each send. A
message edit can change the body on a retry. Retraction, deletion, and access
loss stop delivery. Notification state has no effect on these checks.

## Consequences

Partial source fan-out is recoverable. Successful destinations do not repeat
because another destination fails. The endpoint must deduplicate the stable
delivery ID: an HTTP success followed by a process failure can cause a repeat.
A reserved attempt counts toward the limit after a crash because Chatto cannot
know whether the endpoint accepted it.

Delivery expiry uses the original message time. A restore cannot extend an
existing request's deadline or attempt limit. Runtime attempts and consumer
state use the existing backup surfaces. Terminal outcomes remain in EVT.
There is no ordering guarantee across deliveries.

An endpoint that is slow can occupy worker capacity. Concurrency, request
timeouts, exponential retry delay, and expiry bound this work. Operators set
retry and expiry policy in TOML or ENV. Private-network access and HTTP require
an explicit operator option. Redirects are never followed.

The bot management UI reads the latest outcome for the current configuration.
The projection retains encrypted settings and this bounded result only.
Detailed history remains available through the existing owner event log.
