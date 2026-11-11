# ADR-123: Durable Background Jobs

**Date:** 2026-11-10

## Context

Media processing, outbound webhooks, and the existing durable effect workers
need common execution and recovery controls. Webhook handoff previously acknowledged source events before HTTP
completion and retained pending deliveries only in memory. Media processing, asset cleanup, call-key cleanup, push-subscription cleanup,
user-key shredding, and notification alerts each had their own worker pool.
Operators could not inspect or retry individual work items.

## Decision

Use one Chatto-owned job queue. This decision replaces the execution and queue
choices in ADR-066 and ADR-097. Their media lifecycle, endpoint security, and
source selection rules still apply.

Keep current job records as protobufs under `job.{id}` in `RUNTIME_STATE`.
Keep dispatch references in a file-backed `JOBS` stream on
`jobs.{type}.{id}`. Retain the latest dispatch per subject. Include both
resources and consumer positions in backups. Capture EVT first, runtime state
next, and dispatches last. Recovery must tolerate snapshots taken at different
times.

Existing durable EVT consumers persist deterministic jobs before acknowledging
their source. Effect consumers on EVT and NOTIFICATIONS keep their persisted
names and filters. Their jobs store only the source subject and sequence. Each
attempt reads the source message again and runs the unchanged effect handler.
A handler termination fails the job permanently. A missing source fails the
job, except for notification alerts, where expiry reconciliation owns the
outcome. A filtered runtime-state watcher repairs missing dispatches at
startup and during operation. It retains only unfinished jobs in its index.
Record creation uses create-if-absent; source redelivery cannot reset a job.

Each server replica runs one bounded pull-worker pool. A standalone media
process can consume media jobs through a separate filtered consumer. Job record
revision claims prevent normal concurrent execution across these consumers.
Claims have a one-minute lease, renewed every fifteen seconds. A failed renewal
cancels the attempt. Graceful shutdown releases the claim; crash recovery waits
for lease expiry. External effects remain at least once: a lost HTTP response or
a failed completion write can cause a duplicate.

Automatic retries use capped exponential backoff, up to thirty minutes, and do
not retain a worker slot while waiting. Each type supplies an attempt budget
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
dead-letter stream. Jobs share capacity and can delay each other. Completed and
failed records and their dispatches have no automatic expiry. This retains
permanent source deduplication but causes storage to grow with accepted work.
Listing sorts job keys before hydrating a bounded page; large deployments may
need a dedicated index and a retention contract later. Privacy cleanup shares
capacity with webhooks and media; an exhausted cleanup job remains failed
until an operator retries it. Effect source consumer diagnostics show handoff
progress, not job completion. Notification occurrence projection, link-preview
work, and periodic reconciliation passes remain outside the queue. The current
server has no separate email worker: account emails send on the request path.
