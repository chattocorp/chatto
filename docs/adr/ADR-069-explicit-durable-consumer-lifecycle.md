# ADR-069: Manage Durable Consumer Lifecycles Explicitly

**Date:** 2026-08-11

**Status:** Accepted

**Updated:** 2026-08-19

## Context

Chatto uses named JetStream durable pull consumers to recover asynchronous
outcomes from application-owned event logs. Most consume `EVT`, including the
`chatto-notification-materializer-v1` handoff into the bounded notification
log. `chatto-notification-alert-delivery-v1` consumes
`notifications.signalled` from `NOTIFICATIONS`. Every replica of one worker
role shares the same consumer, so its delivery and acknowledgement position
belongs to the server deployment rather than to one process. The consumer
continues to exist while workers are offline
and is included when Chatto snapshots its owning stream for backup. Notification
backups capture `EVT`, then `RUNTIME_STATE`, then `NOTIFICATIONS` so accepted
source work remains either materialized or replayable from the restored
consumer position; see ADR-076.

That persistence is part of the recovery guarantee. Deleting a consumer loses
its acknowledgement and pending-delivery state. Recreating one of Chatto's
current `DeliverAll` consumers then replays all retained matching facts. The
handlers are deliberately idempotent or terminal-state guarded, so replay is
safe, but it can repeat expensive work and must not happen accidentally.

JetStream can delete a durable consumer after an inactivity threshold, but a
pull consumer is inactive when the server receives no pull requests. That does
not mean its backlog is complete or that its effect is no longer required. A
worker may intentionally be scaled to zero, unavailable during maintenance, or
stopped while another replica starts.

Consumer names, filters, delivery policy, acknowledgement policy, and domain
completion semantics therefore form a persisted operational contract. Removing
a worker, changing that contract, and cleaning up the old consumer are a data
resource migration rather than ordinary process shutdown.

## Decision

Applications own durable consumer creation, configuration, versioned names,
rollout, and retirement. `pkg/events.DurableWorker` owns bounded execution of an
already configured consumer and must not create, delete, or garbage-collect
that consumer.

Durable effect consumers required for recovery have no automatic inactivity
cleanup. A worker must not delete its consumer when it stops, and one replica
must never delete a consumer shared with other replicas. Chatto must not delete
unknown `chatto-*` consumers merely because the current binary does not declare
them; they may belong to an older binary during a rolling deployment or to a
newer binary during rollback.

Keep the same consumer name only when the updated configuration and work
interpretation are compatible for every binary that may use it. A materially
different filter, delivery starting point, acknowledgement contract, or domain
completion rule receives a new versioned name such as `-v2` and an explicit
migration plan.

Retire a named consumer through a staged, application-owned migration:

1. Identify every producer of matching work and decide whether existing work
   must drain, move to a replacement, or be deliberately abandoned.
2. Stop producing the retired work while its worker remains available to
   drain, or deploy the replacement alongside it with idempotent or
   OCC-protected overlap.
3. Exclude old binaries that could recreate the retired consumer or produce
   work understood only by it, then close the supported rollback window.
4. Establish a stable completion boundary. If matching production has stopped,
   perform a final check that both pending and acknowledgement-pending counts
   are zero immediately before deletion. If matching facts continue for a
   replacement, prove that the replacement covers retained work through an
   explicit durable cutoff before treating any old-consumer remainder as
   redundant. Any other skipped work is deliberate abandonment and must be
   documented as such.
5. Delete the consumer. Concurrent deletion attempts treat an already absent
   consumer as success.
6. Remove the consumer from the NATS resource inventory and update rollout,
   backup/restore, diagnostics, and focused migration tests as applicable.

Deleting one of these consumers removes its consumer state without deleting
records from the owning stream. Reintroducing a consumer later must
nevertheless define its delivery starting point and replay behavior explicitly. On bounded
logs, retained source facts may expire independently according to that log's
application and broker retention policy.

Mechanical retirement helpers may be shared only when concrete applications
demonstrate the same safe preconditions. A helper may inspect and delete a
named consumer, but it cannot decide whether unfinished domain effects may be
discarded or whether a mixed-version deployment is safe.

## Consequences

Worker shutdown, replica handover, maintenance, and scale-to-zero preserve
pending effects. No individual replica can accidentally erase cluster-wide
delivery progress.

Obsolete consumers may remain visible in diagnostics and backups during a
deliberate compatibility window. Removing them costs a staged migration rather
than one cleanup call, but makes the point at which retry state is discarded
reviewable and testable.

Versioned names make incompatible contracts and rollback boundaries explicit.
Running old and new consumers together can duplicate delivery, so domain
handlers and migration plans must preserve at-least-once safety.

The shared framework stays application-neutral and avoids implying that
consumer existence or retirement policy can be inferred from worker process
lifecycle.

## Related

- [ADR-001](ADR-001-nats-jetstream-as-primary-data-store.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-041](ADR-041-runtime-units.md)
- [ADR-056](ADR-056-extractable-nats-event-sourcing-framework.md)
- [ADR-066](ADR-066-durable-asset-processing-runtime-unit.md)
- [ADR-076](ADR-076-deterministic-notification-occurrences.md)
- [NATS JetStream consumer configuration](https://docs.nats.io/nats-concepts/jetstream/consumers)
