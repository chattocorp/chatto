# Authling

Authling is a standalone, self-hostable OpenID Connect identity provider. Its
experimental runtime currently provides verified-email signup, encrypted local
credentials, password login and reset, verified email change, signed-in
password change, revocable browser sessions, durable OIDC authorization grants,
and a small Authorization Code OpenID Provider for conventional and CIMD
clients. It also stores only
identity-provider state; application data and synchronization are outside its
scope.

Contributors must read [`AGENTS.md`](AGENTS.md) before making Authling changes.
Authling's ADRs, FDRs, architecture inventory, and glossary live under
[`docs/`](docs/README.md).

## Container image

Build from the containing repository root:

```sh
docker build -f authling/Dockerfile -t authling:local .
docker run --rm --network none --read-only authling:local version
```

The image includes the web assets and runs as UID 1000. Its default command is
`run`; mount a configuration file and pass `run --config /config/authling.toml`,
or configure it through environment variables. The development configuration
is not loaded from the image's `/data` working directory.

The `build Authling image` workflow builds and checks Linux amd64 images for
pull requests. Relevant changes on `main` and manual workflow runs also publish
`ghcr.io/chattocorp/authling:<full-commit-sha>` when run from a branch.

Merging Authling's Release Please PR creates an independent `authling/v<version>`
tag. A tag push builds and publishes `ghcr.io/chattocorp/authling:<version>`;
for example, `authling/v0.1.0-alpha.1` publishes `:0.1.0-alpha.1`. The workflow
rejects a release tag that differs from `version.go` or the Release Please
manifest, and checks the executable's version before publishing. Manual runs
on an Authling release tag use the same checks and versioned image name.
Release versions can include a prerelease suffix, but not `+` build metadata.
The workflow does not publish floating `latest` or `stable` tags.

The workflow summary reports the published image digest. Deploy that digest to
staging and verify the release image, then promote the same digest to production
through a separate deployment PR. Flux applies the digest recorded in Git;
publishing a release does not select or deploy a production version. A workflow
rerun can rebuild a tag, so use the verified digest as the deployment identity.
Never rebuild an image as part of production promotion.

For an external NATS server with a private CA, mount its CA in a dedicated
directory and add that directory to `SSL_CERT_DIR` alongside `/etc/ssl/certs`.
This extends Go's process-wide trust store, including outbound HTTPS. Do not
disable TLS verification. Authling starts its HTTP listener only after startup
replay and issuer initialization finish. A TCP startup/readiness probe can
check that milestone, but does not prove continuing NATS or JetStream health.

When the external account uses tiered JetStream quotas, provide an R1 tier for
Authling's temporary ordered consumers and browser-session KV watcher, even
when `AUTHLING_NATS_REPLICAS=3` puts all three data streams in R3. An R3-only
account can reject those consumers with NATS error 10120 (no applicable tier).
Keep the R3 tier and its storage allowance when adding R1. Accounts with an
applicable default tier do not need separate R1/R3 tiers.

If a required background task fails during startup, Authling exits and reports
the underlying error. For error 10120, the message also identifies the account
tiers to check. Consumer-limit errors require sufficient consumer capacity;
restarting alone does not correct account quotas.

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

### Official OIDC conformance tests

On macOS or Linux with Docker Compose running, use:

```sh
mise test-conformance
```

The task starts the pinned OpenID Foundation suite, an isolated Authling
instance, and Mailpit. It runs the discovery test, then prints a link to a
running S256 PKCE test. Open that link in Chrome and follow the test's browser
link. Create a synthetic account if needed, get its verification code from
Mailpit, and complete consent. The suite then checks the token exchange.
Accept the self-signed certificate only for this local test site. No system
certificate settings change.

Use the suite at `https://conformance.localhost:8443/` to inspect results and
run more tests. Run one test at a time because they share a callback alias.
The two confidential suite clients explicitly set `require_pkce = false`.
Public and CIMD clients retain mandatory PKCE. This task does not run the
complete Basic OP plan or prove certification. See the
[local conformance baseline](tools/conformance/README.md) for the full manual
run and its remaining warnings and review results.

For automated provider regression checks, run `mise test-conformance-ci`.
This task creates a synthetic account, drives login and consent, and checks
discovery, supported scopes, POST client authentication, and S256 PKCE. It
uses fresh state and removes the processes, containers, and database volume
when it finishes. `conformance-results.json` contains only module outcomes
and condition names. Unexpected warnings, failures, skips, review results,
and timeouts fail the task. See the [baseline](tools/conformance/README.md)
for the explicit warning policy and coverage limits.

The repository integration task `mise test-oidc-conformance`, run from the
repository root, also checks Chatto as a relying party. Authling's task does
not require Chatto. CI runs the combined task and uploads only its summary.

For the interactive task, press Ctrl-C to stop the processes and containers. Test accounts, suite results,
and generated client configuration remain in `.authling/conformance/`. This
directory is separate from normal development data. Do not publish it: the
suite stores test tokens and secrets. Only use synthetic accounts.

The task uses ports 8443, 9443, 19400, 19408, and 19409. It needs `curl`
and Docker Compose, OpenSSL, and Playwright Chromium. On Linux the isolated
Authling listener binds all host interfaces so the Docker proxy can reach it;
use a trusted development host or an isolated CI runner. Image downloads contact GitLab and Docker registries;
test traffic stays local. The suite runs in development mode without a hosted
login. The suite and TLS proxy bind only to loopback.

### Build and run

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
<http://localhost:8080/password-reset>. Signed-in accounts can change their
verified email address or password, review or revoke other browser sessions,
and manage authorized OIDC apps from <http://localhost:8080/account>.
Authling also redirects `/.well-known/change-password` to the signed-in
password-change page for compatible password managers.
Mailpit receives SMTP on port 1025 and shows captured messages at
<http://127.0.0.1:8025>. Set
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
Requests through another hostname, port, or scheme redirect to `http.public_url`
with HTTP 307 before Authling processes them. The redirect preserves the path,
query, and request method. DNS, certificates, and proxy routes must still allow
the alias to reach Authling. OIDC clients must use the canonical issuer URL.

Configure the public site name separately from the Authling software name:

```toml
[site]
name = "chatto.id"
description = "Your account for Chatto."
```

`AUTHLING_SITE_NAME` and `AUTHLING_SITE_DESCRIPTION` override these TOML values.
The name appears in page headers, page titles, account messages, and transactional
email subjects and bodies. The optional description appears on the home page and
in page metadata. Values are plain text; HTML is escaped. Names allow up to 120
Unicode characters and descriptions up to 500, with no control characters.
Leading and trailing spaces are removed.

When the name is empty, the site uses the hostname from `http.public_url` (or the
local listener default). An empty description is omitted. Request host headers
do not set the display name. Restart Authling after changing these settings.
Display settings do not change the issuer URL, account IDs, client names, SMTP
sender address, or authentication policy. Set `smtp.from` separately to use your
site's sender name and address.

Authling renders its user interface with templ. Vite compiles Tailwind CSS and
locally packaged fonts and icons into assets that are embedded in the Go
binary; Node.js is not needed to run the resulting executable.

## OpenID Connect

Authling publishes discovery at `/.well-known/openid-configuration`. The
provider supports Authorization Code, requires `openid` and S256 PKCE
by default, and signs ID tokens with RS256. Request scopes for the information
your client needs:

| Scope | Claims in ID tokens and UserInfo |
| --- | --- |
| `openid` | Stable account ID as `sub`, plus authentication claims in ID tokens |
| `profile` | Non-empty `preferred_username` and optional `name` |
| `email` | Current verified `email` and `email_verified` |

For example, send `scope=openid profile email` to request all supported account
information. Unknown and duplicate scopes are rejected. Authling exposes no
application-data scopes.

Clients that previously requested only `openid` must add `profile` to keep
receiving names. Add `email` only when needed. Existing signed ID tokens remain
valid until expiry; UserInfo follows the token's stored scopes and the current
claim-release rules. Consent now uses disclosure version 2. Version-1 grants
remain readable but require fresh consent before reuse. Upgrade all Authling
replicas together: older binaries cannot replay version-2 grants, so rollback
to those binaries is not supported after the first new grant is recorded.

Authling rotates its RS256 signing key automatically every 90 days. A new
public key is published before use, and the preceding public key remains in
JWKS until its ID tokens have expired. Configure a whole-day interval from one
through 3,650 days with:

```toml
[oidc]
signing_key_rotation_interval_days = 90
```

The equivalent environment variable is
`AUTHLING_OIDC_SIGNING_KEY_ROTATION_INTERVAL_DAYS`. Rotation runs inside the
Authling process and therefore works with private embedded NATS. Authling does
not yet expose a manual emergency-rotation command.

Explicit consent creates a durable authorization grant for the exact client
ID and exactly the approved scopes. Later covered requests skip repeated consent unless the
client sends `prompt=consent`. The account page lists and revokes these grants.
`prompt=none` checks the current session and grant without showing login or
consent. It returns a code when both permit access, or `login_required` or
`consent_required` when interaction is needed. New users can create an account
from the OIDC login page and then continue to consent.

New authorization requests have a shared limit of 1,000 admissions with a
ten-minute quiet window. Exhaustion returns HTTP 429; existing consent and
token exchanges can still finish. Password-reset requests have separate
non-refundable limits of ten per address and 1,000 globally with a 15-minute
quiet window. Each successful admission restarts its counter's window;
failed work does not refund it. Update all replicas to enforce these limits.

Revocation makes future requests ask again; it does not end already issued
five-minute tokens or sessions held by the relying party.

Admission of unregistered clients is disabled by default. To allow it, set
`AUTHLING_OIDC_ALLOW_UNREGISTERED_CLIENTS=true` or add this to `authling.toml`:

```toml
[oidc]
allow_unregistered_clients = true
```

This setting controls client admission, not the metadata format or client
authentication method. Currently, unregistered clients are discovered through
CIMD. Explicitly registered CIMD URL clients are not yet supported.

Restart Authling after changing the setting. Existing CIMD deployments must
explicitly enable it when upgrading. All replicas must use the same setting.
When disabled, discovery advertises `client_id_metadata_document_supported`
as false and unregistered URL clients are rejected without DNS or metadata
requests. Trusted-host exceptions do not allow unregistered clients on their own.

When enabled, CIMD public clients use their HTTPS metadata-document URL as
`client_id`; they need no per-client Authling registration. Authling contacts
the document host, which can observe the issuer server's outbound IP address
and requested document path. The user's browser does not fetch the document.
PKCE remains mandatory for these public clients.

Conventional consumers can be declared regardless of the CIMD setting:

```toml
[[oidc.clients]]
id = 'example-app'
name = 'Example App'
redirect_uris = ['https://app.example.com/oidc/callback']
# Omit secret for a public client, or configure at least 32 characters for
# client_secret_basic or client_secret_post authentication.
secret = 'replace-with-a-secret-from-your-secret-store'
# Optional compatibility exception for confidential clients only.
# Prefer true; clients that opt out need other code-injection defenses.
require_pkce = true
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
