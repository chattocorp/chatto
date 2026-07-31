# Authling

Authling is a standalone, self-hostable OpenID Connect identity provider. Its
experimental runtime currently persists and replays opaque accounts, but it
does not implement an identity-provider flow or expose a public API yet.

Contributors must read [`AGENTS.md`](AGENTS.md) before making Authling changes.
Authling's ADRs, FDRs, architecture inventory, and glossary live under
[`docs/`](docs/README.md).

Authling is a separate product from Chatto:

- it is built from its own Go module;
- it runs as its own process with its own configuration and lifecycle;
- it connects through credentials for its own NATS account; and
- it has an independent version, changelog, and `authling/v*` release tags.

The repository-level `go.work` file supports local development across Authling
and Chatto. Authling must not import Chatto domain or `internal` packages.
Reusable event-sourcing mechanics live in the unstable shared
[`hmans.de/chatto/pkg/events`](../pkg/events/README.md) module, while embedded
NATS lifecycle mechanics live in
[`hmans.de/chatto/pkg/natsruntime`](../pkg/natsruntime/README.md). Authling
consumes shared modules only for concrete runtime needs.

Authling is incubated in this repository temporarily. Once the shared framework
can be consumed through a stable, versioned boundary, Authling is intended to
move to its own repository.

An embedding adapter may be added later, but the standalone runtime remains
the primary deployment model and an embedded Authling instance must still use
its own NATS account.

## Development

Run the Authling tests from the repository root:

```sh
mise test-authling
```

Build and inspect the executable:

```sh
mise build-authling
./authling/bin/authling version
```

Run the experimental standalone account runtime using the checked-in
development configuration in `authling.toml`:

```sh
mise authling run
```

Embedded NATS is opt-in and has no TCP listener. For an external NATS
deployment, configure `nats.client.url` and `nats.client.credentials_file`
instead. Equivalent `AUTHLING_NATS_*` environment variables override TOML.

The runtime currently has no HTTP endpoints or account-management command.
Account creation is exercised through the internal domain boundary while the
public identity protocol is designed.
