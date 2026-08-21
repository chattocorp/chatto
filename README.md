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

The root `mise dev` task runs the services needed for regular development as
native processes: the Chatto backend and Vite frontend, Authling, Mailpit, and
LiveKit. [Portless](https://portless.sh/) gives each browser-facing process a
stable, worktree-aware HTTPS URL. The Portless-backed mise tasks pin it as an
isolated npm tool and run it with Node.js 24 without changing the Node.js 22
version used by the rest of the repository.

```sh
mise trust
mise install
mise setup
(cd authling && mise trust && mise install && mise deps)
mise dev
```

`mise dev` starts all five processes in one supervised process group and
prefixes their combined output. Vite reloads frontend source changes itself;
restart `mise dev` after changing Chatto or Authling Go code. The shared API
types and Lingua packages are built once by `mise setup` instead of running
permanent package watchers. Chatto and Authling keep separate embedded-NATS
state: Chatto uses the worktree-local `cli/data/`, while Authling
uses `.context/dev-portless/<workspace>/nested/authling/` because its issuer is
bound to the workspace hostname.

The useful browser endpoints use the Conductor workspace name on the shared
Portless development proxy port `42444`:

- Chatto: `https://chatto.<workspace>.localhost:42444`
- Authling: `https://authling.<workspace>.localhost:42444`
- Mailpit: `https://mailpit.<workspace>.localhost:42444`
- LiveKit: `https://livekit.<workspace>.localhost:42444`

Conductor still allocates ten ports to every local workspace. With base port
`$CONDUCTOR_PORT`, the processes bind only to their allocated loopback ports:
Chatto uses
the base port for Vite, `+1` for its backend, and `+4` for embedded NATS;
Authling uses `+2`; LiveKit uses `+5` through `+7`; and Mailpit uses `+8` and
`+9`. Portless terminates trusted development HTTPS and proxies each worktree
hostname to the corresponding listener. Outside Conductor, the listener layout
falls back to base port `4000`.

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
only. Stop the attached run command to stop the workspace's processes and
unregister its Portless routes. Remove `cli/data/` while the stack is stopped to
delete Chatto's local data. Remove
`.context/dev-portless/<workspace>/nested/authling/` to delete that workspace's
Authling identity and establish a fresh issuer on the next start. Changing the
Conductor workspace name changes the HTTPS issuer and selects a fresh Authling
state namespace. Portless generates and trusts a development CA on its first run. If
the non-interactive run cannot request macOS authorization, run
`mise x node@24 npm:portless@0.15.5 -- portless trust` once in an
interactive terminal. The services' internal listener and webhook
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
