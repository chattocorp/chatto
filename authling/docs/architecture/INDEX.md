# Authling Runtime Architecture Inventory

This directory records Authling's current runtime components and operational
contracts. Keep planned architecture in ADRs until it is implemented.

## Process

The [`authling` command](../../cmd/authling/main.go) exposes `help`, `version`,
and `run`. `run` loads the standalone configuration, opens Authling's NATS
storage, starts every required projection, waits for startup replay, and then
runs until its process context is cancelled.

Authling still exposes no HTTP, authentication, account-management, or OpenID
Connect interface.

## Configuration

The runtime reads `authling.toml` by default. `AUTHLING_*` environment variables
override TOML values. Unknown TOML fields fail decoding.

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

`AUTHLING_EVT` enables JetStream atomic publication for future multi-event
commands. Authling currently publishes one event per command.

## Persisted events and subjects

Persisted records use the `authling.core.v1.Event` protobuf envelope. The
envelope currently has one payload:

| Event | Subject | Aggregate | Contents |
|-------|---------|-----------|----------|
| `AccountCreatedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account ID and envelope creation time |

The account ID is restricted to one NATS-safe token. Account creation publishes
with an expected aggregate sequence of zero, so an existing account history
causes an OCC conflict.

## Models

The account model is an in-memory projection consuming
`authling.evt.account.*`. It maps opaque account IDs to creation times.

The runtime does not become ready until the projection has replayed its captured
startup history. A decode or apply failure fails the projection and runtime.
After account creation commits, the account service waits for the committed
stream position before returning the projected account.

The account projection is currently cold-replay-only. It has no snapshot or
local-checkpoint persistence.

## Deliberately absent

The runtime does not yet contain protected personal data, user or data keys,
credentials, sessions, HTTP endpoints, OIDC state, app-scoped documents,
diagnostic endpoints, or backup tooling.
