# Authling

Authling is a standalone, self-hostable OpenID Connect identity provider. It is
at the initial scaffolding stage and does not implement an identity-provider
flow yet.

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
Reusable NATS and event-sourcing mechanics live in the unstable shared
[`hmans.de/chatto/pkg/events`](../pkg/events/README.md) module. Authling will
consume that module only when a concrete event-sourced use case requires it.

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
