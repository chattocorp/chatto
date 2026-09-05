# ADR-097: Deliver Outbound Bot Webhooks from EVT

**Date:** 2026-09-05

## Context

Bots need HTTP integrations with independent retries. Notification preferences
must not control bot activation. Successful delivery does not need permanent
event history, and JetStream already tracks pending work.

## Decision

Consume direct mentions and DM messages from EVT. Publish one protobuf job per
bot endpoint to the shared `JOBS` work queue from ADR-098. Confirm every publish
before acknowledging the source message. Jobs contain message references,
endpoint generation, attempt limit, retry delay, and source-time expiry.
They contain no plaintext message body or destination credentials.

Scope the stable delivery ID by job type for publish deduplication. JetStream
deduplicates publications for two minutes, or the queue retention age if
shorter. Source retries outside that window can produce duplicates. Receivers must tolerate repeated delivery IDs.

Let the durable queue consumer own pending jobs and delivery counts. Calculate
exponential backoff from its delivery count and use delayed negative
acknowledgement. Double-acknowledge success and intentional skips. Do not use
KV attempt reservations, success records, or application-owned pending state.

Append only terminal failures to EVT. Aggregate OCC permits one failure fact
per delivery ID. Acknowledge the job after that failure commits. Workers record
job-expiry failures while jobs remain in the queue. The shared queue discards
all outstanding jobs after seven days by default, including jobs with no
recorded failure. No EVT request, success, or skip facts are written.

Keep one encrypted endpoint configuration per bot. Use the bot's PII key for
its URL, optional Authorization value, and signing secret. Replacement creates
a new generation and stops older pending deliveries. New configurations do not
receive messages that precede their EVT position.

Use current authorization and message content before sending. Retraction,
deletion, and access loss stop delivery. Notification state has no effect.

## Consequences

A failed destination retries independently of successful destinations. The
receiver can still see duplicates, for example after an HTTP response is lost.
Double acknowledgement confirms queue progress; it does not make the HTTP
operation atomic. No ordering guarantee applies across deliveries.

Delivery counts include work that stops before HTTP starts. Restart preserves
consumer counts and pending jobs. Backup includes the queue and its consumers,
after the EVT snapshot. Restoring a backup can repeat previously completed work.
Existing job deadlines and policy do not change. Operators must preserve the
named consumers during normal operation; recreating them can replay work.

Operators set retry and expiry policy in TOML or ENV. Private-network access
and HTTP require an explicit option. Redirects are never followed. Request
timeouts and worker concurrency bound active HTTP work.

The bot page shows the latest recorded failure for the current configuration.
Later success does not clear that failure. The projection retains encrypted
settings and one failure per bot. Detailed failures remain in the owner event
log. Payload text remains the currently readable message text on each attempt.
