# Events

`hmans.de/chatto/pkg/events` is an envelope-neutral event-sourcing framework
for NATS JetStream. It provides optimistic-concurrency-controlled publication,
ordered projection replay, startup-readiness and read-your-writes barriers, and
optional snapshot or checkpoint lifecycles.

Mutation callbacks can choose their consistency boundary explicitly:

- `AtSubject(filter)` reruns a decision only when that subject or aggregate
  filter changes; and
- `AtStreamTail()` reruns a decision after any intervening stream event, which
  is useful for invariants that genuinely span the complete stream.

`EncodedEventLog.ExecuteMutation` captures the chosen boundary, invokes the
application decision, atomically commits its opaque records with OCC, and
reruns the complete decision after conflicts. Applications still own
projection catch-up and authorization, and logical event IDs must remain
stable across attempts.

Single-record decisions use an ordinary OCC publish. Multi-record decisions
use JetStream atomic batch publication and therefore require the bound stream
to enable `AllowAtomicPublish`.

```go
result, err := log.ExecuteMutation(ctx, events.AtSubject(aggregateFilter),
	func(ctx context.Context, attempt events.MutationAttempt) ([]events.EncodedMutationEntry, error) {
		// Wait for application projections, authorize, and derive the event here.
		return []events.EncodedMutationEntry{{Subject: subject, Record: record}}, nil
	})
```

Applications retain ownership of their event codecs, subject policy, stream
identity, storage coordinates, and runtime composition. The module does not
import Chatto or Authling domain packages.

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
