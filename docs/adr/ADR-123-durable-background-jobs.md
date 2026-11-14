# ADR-123: Durable Background Jobs

**Date:** 2026-11-10

## Context

Media processing, outbound webhooks, and the existing durable effect workers
need common execution and recovery controls. Webhook handoff previously acknowledged source events before HTTP
completion and retained pending deliveries only in memory. Media processing, asset cleanup, call-key cleanup, push-subscription cleanup,
user-key shredding, and notification alerts each had their own worker pool.
Operators could not inspect or retry individual work items.

## Decision

Use one durable job queue in each application. The application-neutral
mechanics live in the shared, Apache-2.0 `pkg/jobs` module: records,
dispatches, claims, backoff, manual retry, repair, retention, and the effect
adapter. Chatto and Authling use it; Authling's account erasure is the second
consumer that validates the shared boundary. Each application owns its
resource names, consumers, job types, policies, handlers, and user interfaces.
The module owns its protobuf record format. This decision applies to the
repository. For Chatto, it replaces the execution and queue choices in ADR-066
and ADR-097. Their media lifecycle, endpoint security, and source selection
rules still apply.

Chatto keeps current job records under `job.{id}` in `RUNTIME_STATE`.
Keep dispatch references in a file-backed `JOBS` stream on
`jobs.{type}.{id}`. Retain the latest dispatch per subject for seven days;
the repair pass publishes again for unfinished jobs. A succeeded record
expires seven days after completion through a per-key TTL. Failed records stay
until an operator retries them. Include both
resources and consumer positions in backups. Capture EVT first, runtime state
next, and dispatches last. Recovery must tolerate snapshots taken at different
times.

Existing durable EVT consumers persist deterministic jobs before acknowledging
their source. Effect consumers on EVT and NOTIFICATIONS keep their persisted
names and filters. Their jobs store only the source subject, sequence, and message digest. Each
attempt reads the source message again and runs the unchanged effect handler.
A handler termination fails the job permanently. A missing source fails the
job, except for notification alerts, where expiry reconciliation owns the
outcome. Record creation uses create-if-absent; source redelivery cannot reset
a job.

Each process runs one filtered record watcher. It keeps a payload-free summary
of every current record for listing, and it publishes a dispatch whenever an
unfinished job is due: after a lost publish, at its retry time, at the end of
an abandoned lease, and after a restore. Every replica does this; dispatch
message IDs include the attempt count, so concurrent publications collapse.

A worker acknowledges a dispatch when an attempt ends, also after a failed
attempt, a claim held by another worker, or a type without a local handler.
Jobs that wait for a retry therefore hold no consumer capacity; the watcher
publishes them again when they are due. Each server replica runs one bounded
worker pool on the shared consumer. Media has its own consumer, which server
replicas with media processing and standalone media processes share. A media
backlog, or a replica without media processing, then cannot delay other jobs. Job record
revision claims prevent normal concurrent execution across these consumers.
Claims have a one-minute lease, renewed every fifteen seconds. A failed renewal
cancels the attempt. Graceful shutdown releases the claim, and the interrupted attempt
does not count against the budget; crash recovery waits for lease expiry. External effects remain at least once: a lost HTTP response or
a failed completion write can cause a duplicate.

Automatic retries use capped exponential backoff, up to thirty minutes, and
hold neither a worker nor an unacknowledged dispatch while waiting. Each type supplies an attempt budget
and initial delay. Permanent failures and exhausted budgets remain failed.
Manual retry checks the viewed attempt-budget version and uses revision OCC to
start one new generation with the same job ID
and payload. It never extends an original deadline. Old dispatch generations
cannot execute a new generation. A repair pass recovers a retry accepted before
dispatch publication.

Keep domain completion rules in handlers. Media handlers confirm a terminal
asset outcome, including a failed conversion or deletion. Webhook handlers check
current endpoint configuration, access, message state, and expiry before HTTP.
Jobs store opaque references and policy, never content or endpoint credentials.
The public API exposes an explicit safe summary and action availability.

## Consequences

Operators can inspect and retry failed work through one settings page. Webhook
work survives restart. Media can run inside the shared pool or in standalone
processes without changing its domain facts.

V1 has no cancellation, priorities, scheduling, bulk actions, or separate
dead-letter stream. Non-media jobs share one worker pool and can delay each
other while they run. Succeeded
records expire after seven days, so storage follows recent work. Source
consumers acknowledge before expiry, so expiry does not re-enqueue work. A
restore that rewinds a source consumer past an expired record can repeat an
idempotent effect. Failed records have no automatic expiry. Listing reads the
in-memory summary index, newest update first, with an optional status filter.
Each process holds a summary of every retained record in memory. Privacy cleanup shares
capacity with webhooks and media; an exhausted cleanup job remains failed
until an operator retries it. Effect source consumer diagnostics show handoff
progress, not job completion. Notification occurrence projection, link-preview
work, and periodic reconciliation passes remain outside the queue. The current
server has no separate email worker: account emails send on the request path.

Authling keeps job records in a dedicated `AUTHLING_JOB_STATE` bucket, because
its runtime-state bucket holds only expiring state, and dispatches in
`AUTHLING_JOBS`. Its erasure source acknowledges unrelated account events
without a job through the effect `Accept` filter. Erasure jobs have an
effectively unlimited attempt budget, because Authling has no manual retry.
