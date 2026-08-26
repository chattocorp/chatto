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

Run the local Chatto backend and Vite frontend, Authling, Mailpit, and LiveKit:

```sh
mise trust
mise install
mise setup
(cd authling && mise trust && mise install && mise deps)
mise dev
```

`mise dev` runs the services in one supervised process group. Vite reloads
frontend changes. Restart it after you change Chatto or Authling Go code.
`mise setup` builds the shared API types and Lingua packages.

[Portless](https://portless.sh/) provides HTTPS routes for browser-facing
services. In Conductor, replace `<workspace>` with the workspace name:

- Chatto: `https://chatto.<workspace>.localhost:42444`
- Authling: `https://authling.<workspace>.localhost:42444`
- Mailpit: `https://mailpit.<workspace>.localhost:42444`
- LiveKit: `https://livekit.<workspace>.localhost:42444`

Outside Conductor, Portless uses the `local` route suffix. Services listen on
loopback ports from base port `4000` (or `$CONDUCTOR_PORT` in Conductor).

Create an Authling account, read its verification code in Mailpit, then choose
**Authling** on the Chatto login screen. Chatto asks for a username at first
login. The stack also creates Chatto owner `alice` and member `bob`; both use
the development-only password `foobar123`.

Chatto uses Authling as its development OIDC provider. Chatto data is in
`cli/data/`; Authling identity data is in
`.context/dev-portless/<workspace>/nested/authling/`.

These credentials and accounts are for local development only. Stop `mise dev`
to stop the services and unregister the routes. With the stack stopped, remove
either data directory to reset that service. A new Conductor workspace name
also creates a new Authling issuer and state directory.

Portless creates and trusts a development CA on its first run. If macOS cannot
show its authorization prompt, run this command once in an interactive terminal:

```sh
mise x node@24 npm:portless@0.15.5 -- portless trust
```

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
