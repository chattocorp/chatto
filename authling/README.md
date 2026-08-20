# Authling

Authling is a standalone, self-hostable OpenID Connect identity provider. Its
experimental runtime currently provides verified-email signup, encrypted local
credentials, password login and reset, revocable browser sessions, and a small
Authorization Code OpenID Provider for conventional and CIMD clients. It also
stores only identity-provider state; application data and synchronization are
outside its scope.

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

The development configuration serves Authling at <http://localhost:8080>, with
signup at <http://localhost:8080/signup>, login at
<http://localhost:8080/login>, and password reset at
<http://localhost:8080/password-reset>. Mailpit receives SMTP on port 1025 and shows
captured messages at <http://127.0.0.1:8025>. Set
`AUTHLING_HTTP_BIND_ADDRESS` to override the Authling listener and
`AUTHLING_HTTP_PUBLIC_URL` to its externally visible origin. The checked-in
configuration declares a loopback HTTP origin for local development.

Local passwords require ten Unicode characters by default. Configure
`authentication.password_minimum_length` (or
`AUTHLING_AUTHENTICATION_PASSWORD_MINIMUM_LENGTH`) to choose a minimum from
eight through 128; passwords remain limited to 1,024 UTF-8 bytes. Authling also
rejects exact, case-insensitive matches from its small built-in list of common
passwords. This baseline list is not yet a comprehensive compromised-password
corpus.

Authling's HTTP listener does not terminate TLS. Production deployments must
place it behind an HTTPS reverse proxy and configure an `https://` public URL.
Plain HTTP is supported only when both the public URL and listener are loopback.
When the proxy overwrites `X-Forwarded-Host` and `X-Forwarded-Proto`, set
`http.trust_proxy_headers = true` (or `AUTHLING_HTTP_TRUST_PROXY_HEADERS=true`)
so canonical-host and same-origin checks use that browser-facing origin. Never
enable this for a listener directly reachable by untrusted clients.
Authling renders its user interface with templ. Vite compiles Tailwind CSS and
locally packaged fonts and icons into assets that are embedded in the Go
binary; Node.js is not needed to run the resulting executable.

## OpenID Connect

Authling publishes discovery at `/.well-known/openid-configuration`. The
initial profile supports Authorization Code, requires `openid` and S256 PKCE
for every client, signs ID tokens with RS256, and exposes a minimal UserInfo
response containing only the account ID as `sub`. It exposes no
application-data scopes.

CIMD public clients use their HTTPS metadata-document URL directly as
`client_id`; they need no Authling-side registration. Conventional consumers
can instead be declared in `authling.toml`:

```toml
[[oidc.clients]]
id = 'example-app'
name = 'Example App'
redirect_uris = ['https://app.example.com/oidc/callback']
# Omit secret for a public client, or configure at least 32 characters for
# client_secret_basic authentication.
secret = 'replace-with-a-secret-from-your-secret-store'
```

CIMD fetches reject private and other special-use destinations by default.
Controlled development environments may explicitly list exact hostnames with
`oidc.cimd_trusted_private_hosts` or `oidc.cimd_trusted_loopback_hosts`; the
equivalent environment variables are
`AUTHLING_OIDC_CIMD_TRUSTED_PRIVATE_HOSTS` and
`AUTHLING_OIDC_CIMD_TRUSTED_LOOPBACK_HOSTS`. Each exception permits only its
named address class. Link-local, multicast, and all other special-use
destinations remain blocked.

Redirect matching is exact. Production redirects require HTTPS; loopback HTTP
is accepted only when Authling itself is in loopback development mode. The
configured `http.public_url` becomes the deployment's immutable issuer after
first startup. Reusing its data directory with another public URL fails
readiness deliberately.

Embedded NATS is opt-in and has no TCP listener. For an external NATS
deployment, configure `nats.client.url` and `nats.client.credentials_file`
instead. Equivalent `AUTHLING_NATS_*` environment variables override TOML.

The runtime currently has no public account-management, application-data,
document, or synchronization API.
