# Events

`hmans.de/chatto/pkg/events` is an envelope-neutral event-sourcing framework
for NATS JetStream. It provides optimistic-concurrency-controlled publication,
ordered projection replay, read-your-writes barriers, and optional snapshot or
checkpoint lifecycles.

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
[`AGPL-3.0-or-later`](LICENSE).
