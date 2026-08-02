# Authling Runtime Architecture Inventory

This directory records Authling's current runtime components and operational
contracts. Keep planned architecture in ADRs until it is implemented.

## Process

The [`authling` command](../../cmd/authling/main.go) exposes `help`, `version`,
and `run`. `run` loads the standalone configuration, opens Authling's NATS
storage, starts every required projection, waits for startup replay, starts the
HTTP listener, and then runs until its process context is cancelled.

The HTTP surface contains server-rendered signup, login, consent, account, and
logout pages plus embedded browser assets. It also exposes OpenID Connect
discovery, authorization, token, UserInfo, and JWKS endpoints. Authling still
exposes no public account-management API.

## Configuration

The runtime reads `authling.toml` by default. `AUTHLING_*` environment variables
override TOML values. Unknown TOML fields fail decoding.

`http.bind_address` selects the public HTTP listener and defaults to
`127.0.0.1:8080`. `AUTHLING_HTTP_BIND_ADDRESS` overrides it.

`http.public_url` declares Authling's externally visible origin and controls
browser cookie transport policy. An `http://` origin is valid only when both
the origin and listener are loopback; every other deployment must configure an
`https://` origin. `AUTHLING_HTTP_PUBLIC_URL` provides the equivalent override.
Requests with another `Host` are rejected, and unsafe browser requests must
carry a matching `Origin`; Fetch Metadata is an additional cross-site signal.
The listener itself is plain HTTP, so production deployments terminate HTTPS
at a reverse proxy. HTTPS deployments use a host-bound `__Host-` session cookie;
the unprefixed cookie name exists only for loopback development.

`authentication.password_minimum_length` sets the local signup password
minimum and defaults to ten Unicode characters. Values from eight through 128
are accepted. `AUTHLING_AUTHENTICATION_PASSWORD_MINIMUM_LENGTH` provides the
equivalent environment override; the 1,024-byte maximum remains fixed.

The `smtp` section configures transactional email. When enabled, `host`,
`port`, and `from` are required. TLS defaults to mandatory STARTTLS (or
implicit TLS on port 465); `opportunistic` is an explicit local-development
fallback. Fields have corresponding `AUTHLING_SMTP_*` environment overrides.

Each `[[oidc.clients]]` table declares a conventional OIDC client with `id`,
`name`, and one or more exact `redirect_uris`. An omitted `secret` creates a
public client; a secret of at least 32 characters enables
`client_secret_basic`. URL client IDs are reserved for CIMD and need no local
configuration. HTTPS redirects are mandatory outside loopback development.
`oidc.cimd_trusted_private_hosts` is an explicit development-network exception
that permits named CIMD hosts to resolve to private, but no other special-use,
addresses.

Operators must select exactly one NATS mode:

- `nats.embedded.enabled = true` starts a private in-process NATS server with
  JetStream and no TCP listener. Its file-backed state lives in
  `nats.embedded.data_dir`, which defaults to `.authling/nats`.
- `nats.client` connects to an external URL using a NATS credentials file.
  Credentials are mandatory so Authling uses its own NATS account.

JetStream resources use one replica by default. Explicit replica counts may be
one, three, or five.

The application-neutral embedded server lifecycle comes from
`hmans.de/chatto/pkg/natsruntime`; Authling retains its private-listener,
storage-path, logging, and deployment policy.

## NATS and JetStream

| Resource | Kind | Storage | Subjects | Purpose |
|----------|------|---------|----------|---------|
| `AUTHLING_EVT` | Stream | File, S2-compressed | `authling.evt.>` | Authoritative Authling event history |
| `AUTHLING_RUNTIME_STATE` | KV bucket | File, history 1 | Opaque HMAC-derived keys | Encrypted signup, session, OIDC request, code, and access-token state, plus bounded delivery and login-attempt counters |
| `AUTHLING_KEYS` | KV bucket | File, history 1 | Opaque key references | Workflow, OIDC signing, user, and wrapped credential data keys |

`AUTHLING_EVT` enables JetStream atomic publication for future multi-event
commands. The key bucket is a separate, exceptionally sensitive backup and
restore boundary.

Credential provisioning writes an opaque operation record before creating its
user and data keys, then removes the marker after the referencing event
commits. Normal command failures compensate immediately. Crash orphans remain
discoverable by their durable marker; Authling does not use time alone as
authority to delete keys that an in-flight replica could still reference.

## Persisted events and subjects

Persisted records use the `authling.core.v1.Event` protobuf envelope:

| Event | Subject | Aggregate | Contents |
|-------|---------|-----------|----------|
| `AccountCreatedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account ID and envelope creation time |
| `EmailClaimedEvent` | `authling.evt.account-registry` | Account registry | Opaque account ID only |
| `IssuerEstablishedEvent` | `authling.evt.issuer` | Issuer singleton | Immutable issuer URL and opaque signing-key reference and ID |

The account ID is restricted to one NATS-safe token. Structural account
creation uses per-account OCC. Verified local account creation atomically
publishes `AccountCreatedEvent` to the per-account subject and
`EmailClaimedEvent` to the PII-free registry subject. OCC guards both the new
account aggregate and current registry tail, serializing email claims across
replicas without a durable email-derived index.

## Models

The account model consumes `authling.evt.account.*` and
`authling.evt.account-registry`. It maps opaque account IDs to creation times.
During replay it resolves and decrypts local credentials and rebuilds a keyed
digest index of normalized emails. It retains encrypted verifier fields and
opaque key references, but neither plaintext email nor plaintext password
verifiers. Local authentication resolves and decrypts a verifier only for one
bounded Argon2id comparison; absent accounts resolve a persistent synthetic
key hierarchy and encrypted dummy verifier through the same storage path.

The runtime does not become ready until the projection has replayed its captured
startup history. A decode or apply failure fails the projection and runtime.
After account creation commits, the account service waits for the committed
stream position before returning the projected account.

The account projection is currently cold-replay-only. It has no snapshot or
local-checkpoint persistence.

The issuer projection consumes the singleton `authling.evt.issuer` subject.
On first initialization, its service creates or resolves the RS256 signing key
and establishes the issuer with subject-level OCC. Every later startup requires
the configured public URL and stored signing-key identity to match that event.
Issuer or key drift prevents readiness.

## HTTP interface

The HTTP handler renders HTML with templ. Vite compiles Tailwind CSS, IBM Plex
Sans, and Iconify glyphs during the build; the resulting assets are embedded
in the Go executable and served below `/assets/`. The runtime has no Node.js or
third-party asset-host dependency.

The initial Content Security Policy prohibits scripts and third-party content.
All essential future authentication interactions must continue to work through
ordinary server-rendered links and forms.

`GET /signup` renders the email form. Three POST endpoints start a flow, verify
its code, and complete account creation with a password. Unsafe requests reject
cross-origin browser submissions. The browser carries a random opaque flow
token in hidden fields; raw email addresses, OTPs, and passwords never enter
URLs.

`GET /login` renders local credential login. `POST /login` applies a shared,
keyed attempt limit before checking the encrypted credential and creates a
fresh browser session on success. `GET /account` requires that session, and
same-origin `POST /logout` revokes it. Successful signup also starts a session.
The host-only browser cookie carries only a random opaque bearer and is
`HttpOnly`, `SameSite=Lax`, scoped to `/`, non-persistent, and secure outside
the explicit loopback development mode.

Session records are authenticated-encrypted in runtime state beneath
HMAC-derived keys. They have a 24-hour absolute lifetime and a one-hour
inactivity limit. Activity updates use OCC and never extend the absolute
deadline. Logout deletes the server record before clearing the cookie.

OpenID Connect mounts discovery at `/.well-known/openid-configuration` and its
protocol endpoints below `/oauth/`. Authorization accepts only code flow,
exactly the `openid` scope, and S256 PKCE. Signed-out requests resume through an
opaque server-side request ID after login; `GET` and same-origin `POST`
`/oidc/consent` display and record per-request consent.

Conventional clients resolve from configuration. Unconfigured HTTPS URL client
IDs resolve through the bounded CIMD fetcher, which disables redirects and
proxies, validates DNS destinations before fetch and dial, and caps fetch time,
body size, concurrency, and cache lifetime. Pending requests, code mappings,
and opaque access-token records are encrypted and expire in runtime state.
Authorization-code claim uses KV OCC so concurrent exchange has at most one
winner. ID tokens use the persistent RS256 key; JWKS publishes only its public
part. The initial UserInfo response contains only the account ID as `sub`.

The HTTP server bounds header, body-read, response-write, and idle time. Signup
also caps request bodies, globally limits OTP delivery, and bounds concurrent
SMTP calls per process.

## Deliberately absent

The runtime does not yet contain recovery, account erasure, session lists or
account-wide session revocation, OIDC refresh tokens or key rotation,
app-scoped documents, diagnostic endpoints, or backup tooling.
