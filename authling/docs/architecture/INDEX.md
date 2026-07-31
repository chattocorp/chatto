# Authling Runtime Architecture Inventory

This directory records Authling's current runtime components and operational
contracts. Keep planned architecture in ADRs until it is implemented.

## Process

The [`authling` command](../../cmd/authling/main.go) exposes `help`, `version`,
and `run`. `run` loads the standalone configuration, opens Authling's NATS
storage, starts every required projection, waits for startup replay, starts the
HTTP listener, and then runs until its process context is cancelled.

The HTTP surface contains a server-rendered status page, a verified-email
signup flow, and embedded browser assets. Authling still exposes no login,
session, account-management, or OpenID Connect interface.

## Configuration

The runtime reads `authling.toml` by default. `AUTHLING_*` environment variables
override TOML values. Unknown TOML fields fail decoding.

`http.bind_address` selects the public HTTP listener and defaults to
`127.0.0.1:8080`. `AUTHLING_HTTP_BIND_ADDRESS` overrides it.

The `smtp` section configures transactional email. When enabled, `host`,
`port`, and `from` are required. TLS defaults to mandatory STARTTLS (or
implicit TLS on port 465); `opportunistic` is an explicit local-development
fallback. Fields have corresponding `AUTHLING_SMTP_*` environment overrides.

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
| `AUTHLING_RUNTIME_STATE` | KV bucket | File, history 1 | Opaque HMAC-derived keys | Encrypted signup flows and bounded delivery counters |
| `AUTHLING_KEYS` | KV bucket | File, history 1 | Opaque key references | Workflow, user, and wrapped credential data keys |

`AUTHLING_EVT` enables JetStream atomic publication for future multi-event
commands. The key bucket is a separate, exceptionally sensitive backup and
restore boundary.

Credential provisioning writes an opaque operation record before creating its
user and data keys, then removes the marker after the referencing event
commits. Normal command failures compensate immediately. Crash orphans remain
discoverable by their durable marker; Authling does not use time alone as
authority to delete keys that an in-flight replica could still reference.

## Persisted events and subjects

Persisted records use the `authling.core.v1.Event` protobuf envelope. The
envelope currently has one payload with two compatible forms:

| Event | Subject | Aggregate | Contents |
|-------|---------|-----------|----------|
| `AccountCreatedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account ID and envelope creation time |
| `EmailClaimedEvent` | `authling.evt.account-registry` | Account registry | Opaque account ID only |

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
digest index of normalized emails. It retains neither plaintext email nor
password verifiers.

The runtime does not become ready until the projection has replayed its captured
startup history. A decode or apply failure fails the projection and runtime.
After account creation commits, the account service waits for the committed
stream position before returning the projected account.

The account projection is currently cold-replay-only. It has no snapshot or
local-checkpoint persistence.

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

The HTTP server bounds header, body-read, response-write, and idle time. Signup
also caps request bodies, globally limits OTP delivery, and bounds concurrent
SMTP calls per process.

## Deliberately absent

The runtime does not yet contain login, sessions, recovery, account erasure,
OIDC state, app-scoped documents, diagnostic endpoints, or backup tooling.
