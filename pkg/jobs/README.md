# Jobs

`github.com/chattocorp/chatto/pkg/jobs` runs durable background jobs on NATS
JetStream. Chatto and Authling use it.

The module is licensed under Apache-2.0. It is an unstable pre-1.0 incubation
surface with no API stability promise.

## Model

The queue follows the Loom architecture on a small scale: commands append
facts, a durable worker produces the outcomes, and a projection gives the
view.

- **Facts.** One job stream holds the facts of each job on
  `{SubjectPrefix}.{type}.{id}.{fact}`: `queued`, `started`, `attempt_failed`,
  `succeeded`, `failed`, `retried`, and `deleted`. Facts are protobufs that this module
  owns. The state of a job is the fold of its facts. The stream must allow
  per-message TTLs and should not set a maximum age.
- **Enqueue.** `Queue.Enqueue` appends `queued` only when the job has no
  facts, so a repeated handoff accepts the job once.
- **Execution.** `Queue.Work` takes an application-owned durable consumer and
  one handler. The consumer filters to `Queue.FilterSubjects` of its job
  types, which are their `queued` and `retried` facts. The acknowledgement
  wait is the claim, renewed by a progress heartbeat. The delivery count is
  the attempt number. Several processes can share one consumer. Give each job
  type its own consumer, so one type cannot delay another.
- **Retries.** A failed attempt returns its delivery with exponential backoff,
  capped at thirty minutes. The waiting job holds an unacknowledged delivery,
  so the consumer should not limit pending acknowledgements. A `*jobs.Failure`
  sets a safe category and can stop retries. `Queue.Retry` starts one new
  attempt budget for a failed job; `Version` fences stale operator actions.
- **Shutdown and crashes.** On shutdown, the worker returns the delivery, and
  another process takes the job over. After a crash, the acknowledgement wait
  ends first. The interrupted delivery counts as an attempt.
- **View.** `Queue.Run` keeps an in-memory projection of the stream: one
  summary per job, without the payload. `Queue.Get`, `Queue.List`,
  `Queue.Retry`, and `Queue.Delete` need it. `Queue.List` returns jobs newest update first, with an
  optional status filter and an opaque cursor. It reads no storage. Processes
  that only enqueue or work do not need `Queue.Run`.
- **Retention.** Unfinished jobs stay until they finish. When a job succeeds,
  the worker writes `succeeded` with a TTL of `SucceededRetention` (default
  three days) and purges the earlier facts of the job. When a job fails, the
  worker writes `failed` with a copy of the specification and purges the
  earlier facts; the fact has a TTL only when `FailedRetention` is set.
- **Delete.** `Queue.Delete` removes a failed job, for example one that can
  never succeed. `Version` fences it like a retry, so a retry and a delete of
  the same failure cannot both take effect.

## Effects

`NewEffect` moves an existing durable consumer onto the queue. The consumer
keeps its persisted name and filter. For each accepted message, the effect
enqueues a job with the source subject, sequence, and a SHA-256 digest of the
message, then acknowledges the message. Each attempt reads the message again and
runs the unchanged `events.DurableDeliveryHandler`. A handler error from
`events.TerminateDelivery` fails the job permanently. `Accept` skips messages
that need no job. `SourceMayExpire` lets a job succeed when stream limits have
removed its source and another process owns that outcome.

## Application responsibilities

Applications own stream, bucket, and consumer names and configurations, job
types, attempt budgets, handlers, payload contents, and every user interface or
API over the queue. Payloads must not contain content, credentials, or other
data that a job listing must not reveal. Handlers must tolerate repeated
execution: a lost acknowledgement, an expired acknowledgement wait, or a failed
outcome write can run an attempt again. Job IDs must be unique across job
types.
