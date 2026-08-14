# Events

`hmans.de/chatto/pkg/events` supplies the core reusable mechanics for the
repository-wide [Loom Architecture](../../docs/adr/ADR-073-define-the-loom-architecture.md).
It is an envelope-neutral event-sourcing framework for NATS JetStream,
providing optimistic-concurrency-controlled publication, ordered projection
replay, startup and read-your-writes barriers, optional snapshot or checkpoint
restore, bounded subject reads, and bounded durable pull-worker execution.

The intended reader is an application integrator. This module owns ordering,
OCC, replay, and delivery mechanics; the application owns event codecs,
subjects, authorization, stream identity, consumer configuration, and domain
completion rules. The package must not import an application's envelope or
domain types.

## Publish opaque events

Mutation callbacks choose their consistency boundary explicitly:

- `AtSubject(filter)` reruns a decision when that subject or aggregate filter
  changes; and
- `AtStreamTail()` reruns a decision after any intervening stream event, for
  invariants that span the complete stream.

`ExecuteMutation` invokes the application decision, atomically commits opaque
records with OCC, and reruns the complete decision after conflicts. Logical
event IDs must remain stable across attempts. Multi-record decisions require
the bound stream to enable JetStream `AllowAtomicPublish`.

```go
result, err := log.ExecuteMutation(ctx, events.AtSubject(aggregateFilter),
	func(ctx context.Context, attempt events.MutationAttempt) ([]events.EncodedMutationEntry, error) {
		// Wait for application projections, authorize, and derive the event here.
		return []events.EncodedMutationEntry{{Subject: subject, Record: record}}, nil
	})
```

## Build a projection

Projection implementations must be non-nil pointers. The pointer is an
ownership invariant: the projector and the application's read side must share
one mutable state object.

```go
projection := &MyProjection{}
projector := events.NewDecodedProjector(js, stream, projection, decodeEvent, logger)

go projector.Run(ctx) // one Run call per Projector instance
if err := projector.WaitForStartup(ctx); err != nil {
	// Do not serve reads after a failed startup replay.
}
```

`Run` is single-use. After cancellation or failure, construct a new projector.
Use `WaitFor`, `WaitForCurrent`, or a subject-aware `StreamPosition` when a
caller needs read-your-writes visibility.

## Snapshots and checkpoints

Snapshots are disposable replay accelerators, not authoritative event data.
Configure either `ConfigureSnapshots` or `ConfigureCheckpoint` before `Run`,
never both. Snapshot sources must return the requested contract ID, stream
name, stream incarnation, and a cutoff no newer than the startup target. The
projector validates those bindings again and cold-replays retained events when
a snapshot is unavailable or invalid. Stream identity changes during restore
fail the run rather than applying state across stream incarnations.

`CaptureSnapshot` takes an apply barrier and includes the applied cutoff and
stream identity. Publication remains application-owned and must use its own
cross-replica OCC. Checkpoint implementations must atomically commit their
derived state and cutoff, and must implement reset/rebuild behavior for an
invalid checkpoint.

## Read in bounded pages

Use `SubjectRecordsAfterPage` for potentially long subject histories. Pass the
returned `LastSequence` as the next cursor; it is an opaque stream position and
does not expose a JetStream consumer name or storage coordinate.

```go
for {
	page, err := log.SubjectRecordsAfterPage(ctx, subject, afterSeq, 500, 16<<20)
	if err != nil {
		return err
	}
	for _, record := range page.Records {
		// Decode and process the opaque bytes.
	}
	if !page.More {
		break
	}
	afterSeq = page.LastSequence
}
```

`SubjectRecordsAfter` remains as a compatibility convenience for callers that
explicitly need one materialized result. New code should use pages or process
each page before requesting the next one.

## Run durable work

`DurableWorker` runs an already configured JetStream pull consumer with bounded
concurrency, progress heartbeats, confirmed acknowledgements, delayed retry,
poison-delivery termination, and reconnect-safe fetch retries. It never
creates, deletes, or retires the consumer; those are application-owned
resource migrations. A worker is single-use, and handlers must honor context
cancellation and be safe under at-least-once delivery.

```go
worker, err := events.NewDurableWorker(consumer, handle, events.DurableWorkerOptions{
	MaxConcurrent: 4,
})
if err != nil {
	return err
}
return worker.Run(ctx)
```

## Logging and data safety

The framework accepts a nil `Logger` and replaces it with a no-op logger. When
a logger is supplied, the framework may emit caller-provided subjects,
projection keys, event IDs, stream identities, and handler errors as
diagnostic fields. Callers must ensure those values are opaque operational
identifiers and contain no personal data, credentials, tokens, raw request
values, or secrets. The framework does not redact caller-provided metadata.

## Status

This module is an incubation surface. Its API is not yet covered by a stability
promise, and releases remain pre-1.0 while concrete applications establish the
smallest useful public contract.

## Development

Run the module tests independently:

```sh
mise test-events
```

From this directory, the equivalent standalone check is:

```sh
GOWORK=off go test ./...
```

The package-level API documentation is available through `go doc
hmans.de/chatto/pkg/events`.

## License

The module is licensed under
[`Apache-2.0`](LICENSE). Its permissive license does not imply API stability;
the module remains pre-1.0 while its public contract matures.
