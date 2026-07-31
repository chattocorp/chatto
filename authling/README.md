# Authling

Authling is a standalone, self-hostable OpenID Connect identity provider. Its
experimental runtime currently provides verified-email signup and persists
encrypted local credentials, but it does not implement login, sessions, OIDC,
or a public API yet.

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

Run Authling's tasks from the Authling directory:

```sh
cd authling
mise setup
mise test
```

`mise setup` installs the Go and web dependencies, including Playwright's
Chromium build. You can then run the browser end-to-end suite with:

```sh
mise test-e2e
```

Each end-to-end test starts dedicated Authling and Mailpit processes with an
isolated temporary embedded-NATS directory and Mailpit database. The harness
removes that state after the test. Set `AUTHLING_E2E_KEEP_STATE=1` to preserve
it while diagnosing a failure.

Build and inspect the executable:

```sh
mise build
./bin/authling version
```

Start Mailpit in one terminal, then run Authling with the checked-in development
configuration in another:

```sh
mise mailpit
```

```sh
mise authling run
```

Or run both development processes together:

```sh
mise dev
```

The development configuration serves Authling at <http://127.0.0.1:8080>, with
signup at <http://127.0.0.1:8080/signup>. Mailpit receives SMTP on port 1025 and
shows captured messages at <http://127.0.0.1:8025>. Set
`AUTHLING_HTTP_BIND_ADDRESS` to override the Authling listener.

Authling renders its user interface with templ. Vite compiles Tailwind CSS and
locally packaged fonts and icons into assets that are embedded in the Go
binary; Node.js is not needed to run the resulting executable.

Embedded NATS is opt-in and has no TCP listener. For an external NATS
deployment, configure `nats.client.url` and `nats.client.credentials_file`
instead. Equivalent `AUTHLING_NATS_*` environment variables override TOML.

The runtime currently has no login, session, public account-management, or
identity protocol.
