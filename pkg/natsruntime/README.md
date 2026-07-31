# NATS Runtime

`hmans.de/chatto/pkg/natsruntime` provides application-neutral lifecycle
mechanics for an embedded NATS server: creation, startup readiness, failure
cleanup, in-process client connections, and orderly shutdown.

Applications pass native `nats-server` options and retain ownership of
configuration, listeners, authentication, monitoring, logging, storage paths,
and deployment policy. The runtime always disables NATS signal handling so the
embedding application remains responsible for its process lifecycle.

## Status

This module is an incubation surface shared by Chatto and Authling. Its API is
not yet covered by a stability promise, and releases remain pre-1.0 while both
applications establish the smallest useful contract.

## Development

Run the module tests independently:

```sh
mise test-natsruntime
```

From this directory, the equivalent standalone check is:

```sh
GOWORK=off go test ./...
```

## License

The module is licensed under
[`Apache-2.0`](LICENSE). Its permissive license does not imply API stability;
the module remains pre-1.0 while its public contract matures.
