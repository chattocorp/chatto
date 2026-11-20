# ADR-123: Durable Background Jobs

**Date:** 2026-11-14

## Context

Media processing, outbound webhooks, and the existing durable effect workers
need common execution and recovery controls. Webhook handoff previously acknowledged source events before HTTP
completion and retained pending deliveries only in memory. Media processing, asset cleanup, call-key cleanup, push-subscription cleanup,
user-key shredding, and notification alerts each had their own worker pool.
Media and the cleanup and key-deletion workers retried without a limit.
Operators could not inspect or retry individual work items.

## Decision

Use one durable job queue in each application. The application-neutral
mechanics live in the shared, Apache-2.0 `pkg/jobs` module: job facts, the
worker, backoff, manual retry, the job view, retention, and the effect adapter.
Chatto and Authling use it; Authling's account erasure is the second consumer
that validates the shared boundary. Each application owns its resource names,
consumers, job types, policies, handlers, and user interfaces. The module owns
the format of its facts, which is JSON. This decision applies to the repository. For Chatto,
it replaces the execution and queue choices in ADR-066 and ADR-097. Their media
lifecycle, endpoint security, and source selection rules still apply.

### The job stream is an event log

The queue follows the Loom architecture on a small scale. One job stream holds
the facts of each job on `{prefix}.{type}.{id}.{fact}`. The state of a job is
the fold of its facts in stream order. The facts are:

| Fact             | Written by | Meaning                                                  |
| ---------------- | ---------- | -------------------------------------------------------- |
| `queued`         | Enqueue    | The job was accepted. It carries the payload and policy. |
| `started`        | Worker     | An attempt started.                                      |
| `attempt_failed` | Worker     | An attempt failed; the job waits for its next attempt.   |
| `succeeded`      | Worker     | The job is complete.                                     |
| `failed`         | Worker     | The job failed permanently, used its budget, or expired. |
| `retried`        | Retry      | An operator started a new attempt budget.                |
| `deleted`        | Delete     | An operator deleted a failed job.                        |

The job stream is a second log beside EVT, not a part of it. Job facts are
operational facts, not domain facts. EVT keeps the full history; the job stream
forgets facts by TTL and by purge. There is no job record in a key-value
bucket.

Chatto uses the file-backed `JOBS` stream on `jobs.>` with per-message TTLs and
no maximum age. Include the stream and its consumer positions
in backups.

### Enqueue and retry use OCC

Enqueue appends `queued` only when the job has no facts. A redelivered source
message then changes nothing. Source consumers acknowledge their message only
after enqueue returns.

A manual retry appends `retried` only when the `failed` fact that the operator
viewed is still the latest fact of the job. Concurrent or stale actions start
one new budget. `retried` carries a copy of the payload and policy. It never
extends an original deadline.

A delete appends `deleted` under the same condition, so a retry and a delete of
the same failure cannot both take effect. The public API exposes a job version
that is a digest of the job ID and the sequence of the latest fact, never the
broker position.

### A durable consumer runs the jobs

Each job type has one durable consumer, filtered to the `queued` and `retried`
facts of that type, with its own worker pool. A slow or blocked type cannot
delay another type. The consumer gives the queue its execution rules:

- The acknowledgement wait is the claim. The worker renews it with a progress
  heartbeat every fifteen seconds while the handler runs.
- The delivery count is the attempt number.
- A failed attempt returns the delivery with a delay. The delay doubles from
  the initial delay of the type, up to thirty minutes.
- After a crash, the acknowledgement wait ends and JetStream delivers the fact
  to another replica. On shutdown, the worker returns the delivery immediately.

A job that waits for a retry holds an unacknowledged delivery. The consumers
therefore do not limit pending acknowledgements; the worker pool bounds
running jobs.

The worker writes `succeeded` or `failed` before it acknowledges. `started` and
`attempt_failed` only feed the view; a failed write of these facts does not
stop or repeat the attempt. External effects remain at least once: a lost HTTP
response or a failed outcome write can cause a duplicate.

An attempt that a shutdown or a crash interrupted counts against the attempt
budget, because it was a delivery. A delivery that returns before the handler
runs, because the worker cannot read the job stream, also counts. A handler that finished before shutdown
keeps its success. A failure during shutdown is not recorded, because the
cancellation can be its cause; the next delivery runs the job again.

A delivery can repeat. Before an attempt, the worker reads the latest fact of
the job. When the job already has an outcome, or an operator retried it after
the delivered fact, the worker only finishes the purge and acknowledges. When
earlier deliveries ended without a stored outcome, their result is unknown:
the job runs again, also beyond the attempt budget. When the purge after an
outcome fails, the delivery returns, and the next delivery finishes the purge.

### Retention

The stream has no maximum age. Each job state has its own rule:

- **Unfinished:** The facts stay until the job finishes. Work that must
  finish, such as key deletion, never leaves the stream by age.
- **Succeeded:** The worker writes `succeeded` with a per-message TTL of three
  days and then purges the earlier facts of that job. A succeeded job leaves
  one small fact for three days.
- **Failed:** The worker writes `failed` with a copy of the payload and policy
  and then purges the earlier facts of that job. A failed job is one fact. By
  default it has no TTL: it stays until an operator retries or deletes it.
  `core.job_failed_retention` sets a TTL for operators who prefer automatic
  removal.
- **Deleted:** Delete writes `deleted` with a TTL of one hour and purges the
  earlier facts. The fact tells the view of each replica to drop the job.

If the purge after an outcome fails, the delivery returns and the next delivery
finishes the purge.

Failed jobs are the only state that can accumulate. Operators delete failed
jobs that can never succeed, such as deliveries to an endpoint that no longer
exists.

### The job view is a projection

Each server replica runs an in-memory projection of the job stream: one
summary per job, without the payload. Listing and detail requests read the
view, never storage. After a retry, the queue waits until the view has applied
the new fact. The view drops a job when the stream no longer retains it. It is
derived state and is built again from the retained facts on each start.

Processes that only run jobs, such as a standalone media process, do not run
the view.

### Effects

Existing durable EVT and NOTIFICATIONS consumers keep their persisted names and
filters. They enqueue one job per relevant source message before they
acknowledge it. The job stores only the source subject, sequence, and message
digest. Each attempt reads the source message again and runs the unchanged
effect handler. A handler termination fails the job permanently. A missing
source fails the job, except for notification alerts, where expiry
reconciliation owns the outcome.

### Chatto policies

| Job type                                                                                                                                           | Attempts                    | Initial delay | Pool size per replica                  |
| -------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------- | ------------- | -------------------------------------- |
| Webhook                                                                                                                                            | Configured (`bot_webhooks`) | Configured    | `core.job_concurrency` (default 8)     |
| Notification alert                                                                                                                                 | 5                           | 15 seconds    | 16                                     |
| Email                                                                                                                                              | Unlimited until expiry      | 5 seconds     | 8 (HTTP server replicas)               |
| Media                                                                                                                                              | Unlimited                   | 30 seconds    | `asset_processing.max_concurrent_jobs` |
| Neighborhood refresh                                                                                                                               | Unlimited                   | 1 minute      | 1 per replica                          |
| Asset cleanup, branding cleanup, message-body erasure, call-key cleanup, push-subscription cleanup, runtime credential cleanup, user-key shredding | Unlimited                   | 30 seconds    | `core.job_concurrency` for each type   |

Media, cleanup, and key deletion keep the unlimited retries of the workers that
they replace. A long tool, KMS, or object-store outage must not leave
attachments unprocessed or keys undeleted. Their timing changes: the workers
that they replace retried every thirty seconds, and each cleanup and
key-deletion worker ran sixteen attempts at a time. The jobs now retry with
delays that double from thirty seconds to thirty minutes, and each type runs
`core.job_concurrency` attempts at a time. After a long outage ends, a waiting
job can therefore start up to thirty minutes later. An operator cannot start a
waiting job early.

A notification alert expires two minutes after its source. Its attempts start
0, 15, 45, and 105 seconds after the first one, so four attempts fit into that
time; the alert worker before ran four attempts at a fixed thirty seconds. The
alert consumer has a one-minute acknowledgement wait, so an alert that a
crashed replica held can still be delivered. The other job consumers wait two
minutes. Unprocessable media records a
terminal asset failure instead. The admin API omits the maximum for an
unlimited budget. Server replicas with media processing and standalone media
processes share the media consumer.

Keep domain completion rules in handlers. Media handlers confirm a terminal
asset outcome, including a failed conversion or deletion. Webhook handlers check
current endpoint configuration, access, message state, and expiry before HTTP.
Jobs store opaque references and policy, never content or endpoint credentials.
The public API exposes an explicit safe summary and action availability.

## Consequences

Operators can inspect and retry failed work through one settings page. Webhook
work survives restart. Media can run inside server replicas or in standalone
processes without changing its domain facts. The queue has no claims, leases,
or repair pass of its own; JetStream consumer semantics do this work.

V1 has no cancellation of unfinished jobs, priorities, scheduling, bulk
actions, or separate dead-letter stream. Job state has no realtime event; the Jobs page polls.

- Failed jobs accumulate until an operator retries or deletes them, or until
  `core.job_failed_retention` when it is set. There is no bulk delete.
- A deleted job is gone. Its work is abandoned. One hour later, when the
  `deleted` fact has expired, a handoff of the same source can accept it again
  as a new job.
- A replica whose view misses the `deleted` fact for more than one hour shows
  the deleted job until it restarts.
- A deploy or a crash during an attempt uses one attempt of a limited budget.
- Each process that runs the view holds a summary of every retained job in
  memory, and replays the retained facts on start.
- A restore that rewinds a source consumer past an expired succeeded fact can
  repeat an idempotent effect.
- Effect source consumer diagnostics show handoff progress, not job
  completion. Each job type consumer appears in worker diagnostics.
- Rolling back to a server version without the queue leaves accepted,
  unfinished jobs in the stream; their sources are already acknowledged. Roll
  forward again to run them.
- Notification occurrence projection, link-preview work, and periodic
  reconciliation passes remain outside the queue. Transactional email uses a
  separate job consumer and encrypted, expiring runtime content. Its job holds
  only an opaque content ID and the original delivery deadline.

Authling keeps job facts in a dedicated `AUTHLING_JOBS` stream with the same
retention rules for erasure. Email uses its own consumer and pool; failed mail
facts expire after three days. Code jobs expire with their workflow; old-address
security notices expire after 24 hours. SMTP failures retry within that deadline.
Its erasure source acknowledges unrelated account events without a job through
the effect `Accept` filter. Erasure jobs use the unlimited attempt budget,
because Authling has no manual retry. Authling shows no jobs and does not run
the job view.
