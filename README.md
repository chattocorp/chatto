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

## Complete Local Stack

The root [`pitchfork.toml`](pitchfork.toml) runs Chatto, Authling, Mailpit,
LiveKit, Storybook, and the Chatto docs website as native processes managed by
[Pitchfork](https://pitchfork.jdx.dev/). `mise` installs their pinned tools and
dependencies. Pitchfork supervises the processes, starts dependencies in
order, restarts Go services after source changes, and gives each checkout
workspace-specific HTTPS origins. Small Caddy adapters give multi-port and
child-process-based services an unambiguous target for Pitchfork's proxy.

```sh
mise trust
mise install
mise setup
(cd authling && mise trust && mise install && mise deps)
mise dev
```

`mise dev` registers the proxy routes and starts a Pitchfork project session.
Vite, Astro, and Storybook reload their own source changes; Pitchfork rebuilds
and restarts the Go services when their sources change and keeps the shared API
types and Lingua packages compiled in watch mode. Chatto and Authling keep
separate embedded-NATS state beneath `.context/dev/`.

For public workspace slug `<workspace>` (the Conductor workspace name, or the
checkout directory name outside Conductor), open these Pitchfork-managed HTTPS
origins:

- Chatto: `https://chatto-<workspace>.localhost:42443`
- Authling: `https://authling-<workspace>.localhost:42443`
- Mailpit: `https://mailpit-<workspace>.localhost:42443`
- LiveKit signaling: `https://livekit-<workspace>.localhost:42443`
- Storybook: `https://storybook-<workspace>.localhost:42443`
- Docs website: `https://docs-<workspace>.localhost:42443`

Pitchfork derives each daemon namespace from the checkout directory name.
`mise dev` uses Conductor's current workspace name for proxy routes and public
service URLs, falling back to the directory name outside Conductor. Proxy
routes share the global slug registry of the user's Pitchfork supervisor.
Pitchfork does not auto-start stopped daemons from proxy requests, and
`mise dev-archive` removes the current checkout's routes.

Create an Authling account, read its verification code in Mailpit, then choose
**Authling** on Chatto's login screen. Chatto asks for a username on the first
login because Authling's initial OIDC profile intentionally shares only its
stable account ID. The stack also bootstraps Chatto owner `alice` and member
`bob`; both use the development-only password `foobar123`.

Authling is configured as an ordinary OIDC provider for the development Chatto
server. Chatto's server catalogue, login tokens, and cached user details stay
on the current device; Authling stores identity-provider state only.

The checked-in credentials and bootstrap accounts are for local development
only. Stop the attached run command to leave its Pitchfork project session and
stop the workspace's processes. Remove `.context/dev/` while the stack is
stopped to delete every local workspace identity and both products' data, then
establish a fresh Authling issuer on the next start. Pitchfork installs its
local certificate authority in the macOS login keychain so browsers and the Go
OIDC clients trust its HTTPS origins. This is not a production deployment
example.

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
