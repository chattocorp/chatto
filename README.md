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

The root [`pitchfork.toml`](pitchfork.toml) runs the services needed for regular
development as native processes managed by
[Pitchfork](https://pitchfork.jdx.dev/): the Chatto backend and Vite frontend,
Authling, Mailpit, and LiveKit. `mise` installs their pinned tools and
dependencies. Pitchfork supervises the processes, starts dependencies in
order, and restarts the Go services after source changes.

```sh
mise trust
mise install
mise setup
(cd authling && mise trust && mise install && mise deps)
mise dev
```

`mise dev` records the workspace's Conductor port base, starts a Pitchfork
project session, and attaches to the five daemon logs. Vite reloads frontend
source changes itself; Pitchfork rebuilds and restarts the Go services when
their sources change. The shared API types and Lingua packages are built once
by `mise setup` instead of running permanent package watchers. Chatto and
Authling keep separate embedded-NATS state beneath
`.context/dev/<workspace>/`.

Conductor allocates ten ports to every local workspace. With base port
`$CONDUCTOR_PORT`, Pitchfork exposes the useful browser endpoints as:

- Chatto: `https://chatto-<workspace>.localhost:42443`
- Authling: `https://authling-<workspace>.localhost:42443`
- Mailpit: `https://mailpit-<workspace>.localhost:42443`
- LiveKit: `https://livekit-<workspace>.localhost:42443`

The daemons still bind only to their workspace's allocated ports: Chatto uses
the base port for Vite, `+1` for its backend, and `+4` for embedded NATS;
Authling uses `+2`; LiveKit uses `+5` through `+7`; and Mailpit uses `+8` and
`+9`. Pitchfork terminates trusted development HTTPS and proxies each hostname
directly to the corresponding daemon's declared browser listener; no Caddy
process or other adapter is involved. `mise dev` prints the browser URLs,
starts the dependency group, then declares the HTTP listener when restarting
the two multi-port daemons. Outside Conductor, the port layout falls back to
base port `4000`.

Pitchfork derives each daemon namespace from the checkout directory name, so
concurrent worktrees remain isolated. `mise dev` registers one workspace-named
proxy route for each browser-facing service, and `mise dev-archive` removes
them after stopping the current checkout's daemons.

Create an Authling account, read its verification code in Mailpit, then choose
**Authling** on Chatto's login screen. Chatto asks for a username on the first
login because Authling's initial OIDC profile intentionally shares only its
stable account ID. The development issuer uses CIMD and has no pre-registered
conventional OIDC client. The stack also bootstraps Chatto owner `alice` and
member `bob`; both use the development-only password `foobar123`.

Authling is configured as an ordinary OIDC provider for the development Chatto
server. Chatto's server catalogue, login tokens, and cached user details stay
on the current device; Authling stores identity-provider state only.

The checked-in credentials and bootstrap accounts are for local development
only. Stop the attached run command to leave its Pitchfork project session and
stop the workspace's processes. Remove `.context/dev/<workspace>/` while the
stack is stopped to delete that workspace identity and both products' data,
then establish a fresh Authling issuer on the next start. Changing the
Conductor workspace name changes the HTTPS issuer and therefore selects a new
state directory; the old directory remains available until it is removed.
Pitchfork generates a development CA, trusts it once, and issues certificates
for the workspace service hostnames. Their internal listener and webhook
connections remain on plain-HTTP loopback. This is not a production deployment
example.

State from the former plain-HTTP, unnamespaced development setup is not reused
by this HTTPS setup and may be removed from `.context/dev/authling/` and
`.context/dev/chatto/` when it is no longer needed.

## License

Chatto is licensed under `AGPL-3.0-or-later` by default. The independently
versioned shared framework modules, standalone frontend, integration surfaces,
documentation, and examples use Apache-2.0. See
[LICENSING.md](LICENSING.md) and [REUSE.toml](REUSE.toml) for the exact
boundary.

The project licenses do not grant permission to use Chatto names or logos as
official branding for a fork or modified version; see [NOTICE](NOTICE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local development notes. This project is **not accepting outside contributions** at this time.
