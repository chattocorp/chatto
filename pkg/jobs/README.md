# Jobs

`github.com/chattocorp/chatto/pkg/jobs` runs durable background jobs on NATS
JetStream. Chatto and Authling use it.

The module is licensed under Apache-2.0. It is an unstable pre-1.0 incubation
surface with no API stability promise.

## Model

- **Records.** A key-value bucket holds the current record of each job under an
  application-chosen key prefix. Records are protobufs that this module owns.
  The bucket must allow per-message TTLs. Bind it with
  `jetstreamutil.NewKeyValue`, so reads go through the stream leader.
- **Dispatches.** A stream holds one reference per job on
  `{SubjectPrefix}.{type}.{id}`. Keep one message per subject and set a maximum
  age; a repair pass publishes again for unfinished jobs.
- **Execution.** `Queue.Run` takes an application-owned consumer and a handler
  per job type. A worker claims a job with a revision-checked write and renews
  a one-minute lease every fifteen seconds. Several processes can share one
  consumer, or overlap through different consumers.
- **Retries.** A failed attempt waits with exponential backoff, capped at thirty
  minutes, without holding a worker. A `*jobs.Failure` sets a safe category and
  can stop retries. Exhausted and permanently failed jobs stay for inspection.
  `Queue.Retry` starts one new attempt budget; `RetryVersion` fences stale
  operator actions.
- **Retention.** Succeeded records expire after `SucceededRetention` (default
  seven days). Failed and unfinished records do not expire.

## Effects

`NewEffect` moves an existing durable consumer onto the queue. The consumer
keeps its persisted name and filter. For each accepted message, the effect
stores a job with the source subject, sequence, and a SHA-256 digest of the
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
execution: a lost acknowledgement or an expired lease can run an attempt again.
