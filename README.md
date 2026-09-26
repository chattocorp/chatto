[![CI](https://github.com/chattocorp/chatto/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/chattocorp/chatto/actions/workflows/ci.yml?query=branch%3Amain)
[![Release](https://github.com/chattocorp/chatto/actions/workflows/release.yml/badge.svg)](https://github.com/chattocorp/chatto/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/chattocorp/chatto?include_prereleases&sort=semver)](https://github.com/chattocorp/chatto/releases)
[![License: AGPL-3.0-or-later with Apache-2.0 exceptions](https://img.shields.io/badge/license-AGPL--3.0--or--later%20with%20Apache--2.0%20exceptions-blue.svg)](LICENSE)

# Chatto

<p><img width="1920" height="1196" alt="It's Chatto!" src="https://github.com/user-attachments/assets/a6a8ef8c-9f56-48ed-8740-53115273c22e" /></p>

A really good chat application for teams and communities, free and easy to self-host, with [cloud hosting available soon](https://chatto.run/cloud).

- [Website](https://chatto.run)
- [Documentation](https://docs.chatto.run)
- [Official Chatto Community](https://chat.chatto.run/)
- [Releases](https://github.com/chattocorp/chatto/releases)
- [Security Policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

This repository temporarily incubates the early
[Authling](authling/README.md) identity-provider module. Authling is developed
and released independently from Chatto and is intended to move to its own
repository once it no longer needs frequent atomic changes with the shared
[event-sourcing framework](pkg/events/README.md),
[embedded NATS runtime](pkg/natsruntime/README.md),
[data-cryptography primitives](pkg/datacrypto/README.md), and
[application-configuration loader](pkg/appconfig/README.md).

## Local Development Stack

The root pnpm workspace contains the JavaScript apps, examples, and libraries.
[`@chatto/client`](packages/chatto-client/README.md) provides shared request,
message, thread, reaction, and typing helpers for bot integrations.
The independent [Runling](packages/runling/README.md) workflow and agent
orchestrator lives in `packages/runling/` and is published to npm as `runling`.
It keeps its own version and MIT license. The Chatto bot uses this local package.
Use `mise check-runling`, `mise test-runling`, and `mise test-runling-package`
to verify it without running the complete Chatto test suite.

[ChattoBot](packages/chattobot/README.md) is a separate private workspace
package. Run `mise dev-chattobot` to start its realtime bot and Runling console.
Use `mise check-chattobot` and `mise test-chattobot` to verify it.

Root pnpm scripts use Turborepo to build workspace dependencies before their
consumers. Prefer `mise` tasks or root scripts such as `mise x -- pnpm run
check:frontend`; a command inside a package only runs that package's script.
Library builds use a local `.turbo/cache`; app builds and verification tasks
run without Turbo caching. Remote caching and telemetry are disabled by the
repository configuration and scripts. See [ADR-102](docs/adr/ADR-102-turborepo-workspace-tasks.md)
for the task and cache boundaries.

Run Chatto, Authling, Mailpit, LiveKit, and the Runling bot:

```sh
mise trust
mise install
mise setup
(cd authling && mise trust && mise install && mise deps)
mise dev
```

`mise dev` builds the frontend and a Chatto binary that includes it. Then it
runs the services in one supervised process group. The builds run only when
their sources change. Restart `mise dev` to see a change in Chatto, its
frontend, or Authling. `mise setup` builds the shared API types, Lingua, and
Runling packages.

All services use plain HTTP. In Conductor, `<workspace>` is the workspace name
and the base port is `$CONDUCTOR_PORT`. Outside Conductor, `<workspace>` is
`local` and the base port is `4000`:

| Service  | URL                                                |
| -------- | -------------------------------------------------- |
| Chatto   | `http://chatto.<workspace>.localhost:<base>`       |
| Authling | `http://authling.<workspace>.localhost:<base + 2>` |
| Runling  | `http://localhost:<base + 3>`                      |
| Mailpit  | `http://localhost:<base + 9>`                      |

Browsers resolve names beneath `.localhost` to your computer. You do not need
to change DNS or `/etc/hosts`. Each workspace has its own hostnames, so the
workspaces do not share browser cookies. The comment above the `dev` task in
`mise.toml` lists all ports.

For hot module replacement during frontend work, run this command in another
terminal:

```sh
mise dev-frontend
```

Vite then serves the frontend at `http://chatto.<workspace>.localhost:<base + 1>`
and sends API requests to the Chatto server of `mise dev`. To use a different
Chatto server, set `CHATTO_BACKEND_URL`, for example
`CHATTO_BACKEND_URL=https://dev.chatto.run mise dev-frontend`.

Create an Authling account, read its verification code in Mailpit, then choose
**Authling** on the Chatto login screen. Chatto asks for a username at first
login. The stack also creates Chatto owner `alice` and member `bob`; both use
the development-only password `foobar123`.

The stack starts the [Runling bot example](examples/runling-bot/README.md)
on loopback at the base port plus three (`http://localhost:4003` outside
Conductor). It uses the bootstrap TestBot account and receives the backend URL
and API key path automatically. On an empty server, bootstrap also creates
TestBot’s outbound webhook. Existing servers keep their saved configuration.

Chatto uses Authling as its development OIDC provider. Chatto stores embedded
NATS data in `cli/data/nats/` and search data in `cli/data/search/`. Authling
identity data is in `.context/dev/<workspace>/authling/`.

These credentials and accounts are for local development only. Stop `mise dev`
to stop the services. With the stack stopped, remove `cli/data/` to reset
Chatto, or remove the Authling identity directory to reset Authling. A new
Conductor workspace name also creates a new Authling issuer and state
directory.

If a worktree has NATS data in the former `cli/data/jetstream/` location, use
the migration steps in [CONTRIBUTING.md](CONTRIBUTING.md#local-chatto-data).

### Generate Test Data

With `mise dev` running, run this command in another terminal:

```sh
mise seed -- --seed 42 --users 20 --rooms 5 --messages 200 --thread-replies 40
```

This adds 20 users with generated names and 200 messages across five rooms.
The message total includes 40 thread replies. Users join different rooms, with
joins and some leaves interleaved with messages. Sign in as `alice` to browse
them; generated users have no password.

The same seed and counts reproduce content with the same generator version;
IDs and timestamps change. Each run adds data. A failed run can leave partial
data. Add `--json` to get the generated IDs and text, or `--help` for options.
Seeding is available only in development and test builds.

See [Synthetic Test Data](CONTRIBUTING.md#synthetic-test-data) for e2e use.

## License

Chatto is licensed under `AGPL-3.0-or-later` by default. The independently
versioned shared framework modules, standalone frontend, integration surfaces,
documentation, and examples use Apache-2.0. Runling uses MIT. See
[LICENSING.md](LICENSING.md) and [REUSE.toml](REUSE.toml) for the exact
boundary.

The project licenses do not grant permission to use Chatto names or logos as
official branding for a fork or modified version; see [NOTICE](NOTICE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local development notes. This project is **not accepting outside contributions** at this time.
