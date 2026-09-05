# ADR-098: Use One Shared Background Job Queue

**Date:** 2026-09-05

## Context

Reliable background work needs pending state and retry delivery. Creating a
stream for every feature repeats storage policy and adds runtime resources.
JetStream already owns pending messages, acknowledgements, and delivery counts.

## Decision

Use one file-backed `JOBS` stream with WorkQueue retention and `jobs.>` subjects.
The application-owned `internal/jobqueue` package creates the stream and
publishes opaque job bytes. It adds no generic envelope, scheduler, registry,
KV records, or status database. Features own their job schemas and handlers.

Use one durable consumer per job type with a non-overlapping subject filter.
Replicas share that consumer. The existing durable worker supplies delayed NAK,
progress heartbeats, and double acknowledgement. Features own retry limits,
backoff, authorization checks, and failure facts.

Scope publish deduplication IDs by job type. Confirm publication before the
source consumer acknowledges its event. The duplicate window is two minutes,
or the queue retention age if shorter. This is bounded deduplication, not an
exactly-once HTTP guarantee.

Set `MaxAge` to seven days by default through `jobs.max_age` or
`CHATTO_JOBS_MAX_AGE`. Do not set byte or message-count limits. Acknowledged jobs
are removed immediately. Every outstanding job expires at the hard age limit,
including jobs that no worker picked up. This deletion needs no failure event.

Core storage initializes the queue with the configured replica count. Backup
includes the stream and its consumers after EVT. Outbound bot webhooks are the
first user on `jobs.bot_webhook.deliver`. Existing systems are not migrated as
part of this decision.

## Consequences

New job types add subjects and consumers rather than streams. All job types
share storage, stream availability, and hard retention policy. Their consumers
and concurrency remain separate. A job type that requires incompatible
retention or storage isolation needs a separate decision.

Seven days bounds the age of retained work, not its size. A burst of jobs can
still consume substantial storage. Outstanding work older than the configured
age is deliberately abandoned. Feature-level expiry can stop retries earlier
and record failure while a job remains available.

Normal restarts preserve named consumers. Consumer recreation or backup restore
can repeat work. Payloads and downstream effects must tolerate duplicates.
