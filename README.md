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

The root [`compose.yml`](compose.yml) runs Chatto, Authling, Mailpit, LiveKit,
Storybook, and the Chatto docs website together on
[OrbStack](https://docs.orbstack.dev/docker/domains). It builds every
repository-owned service from the current checkout, gives Chatto and Authling
separate persistent embedded-NATS storage, and configures Chatto to use
Authling as an OpenID Connect provider through Chatto's public Client ID
Metadata Document, without preregistering Chatto in Authling. Compose derives
the project name from the checkout directory, keeping containers and OrbStack
domains isolated between worktrees.

```sh
docker compose up --build
```

For a checkout in a directory named `<project>`, open these OrbStack-managed
HTTPS origins:

- Chatto: `https://chatto.<project>.orb.local`
- Authling: `https://authling.<project>.orb.local`
- Mailpit: `https://mailpit.<project>.orb.local`
- LiveKit signaling: `https://livekit.<project>.orb.local`
- Storybook: `https://storybook.<project>.orb.local`
- Docs website: `https://docs-website.<project>.orb.local`

Create an Authling account, read its verification code in Mailpit, then choose
**Authling** on Chatto's login screen. Chatto asks for a username on the first
login because Authling's initial OIDC profile intentionally shares only its
stable account ID. The stack also bootstraps a Chatto owner named
`compose-admin` with the development-only password `compose-admin`.

After login, select the cloud button below the server list. Authling asks for
separate permission to read and write account data. The cloud turns green when
the TinyBase connection is active. Chatto then stores the public server list in
Authling. Server URLs, names, icons, and registration times synchronize; Chatto
login tokens and user details stay only in that browser. The frontend shows a
trusted Authling sign-in action and can use the resulting browser session to
start login on Chatto servers that advertise the same issuer. The frontend and
each Chatto server still receive separate tokens and scopes.

The frontend reads its Authling issuer from `/client-config.json`. The Compose
stack sets `CHATTO_FRONTEND_AUTHLING_ISSUER`, so the bundled Chatto server
publishes that document and a separate frontend CIMD identity automatically.
A standalone web, desktop, or mobile client can publish or inject the same
versioned JSON contract from its own trusted application origin.

The checked-in credentials and bootstrap account are for local development
only. Stop the stack with `docker compose down`; add `--volumes` to delete both
products' data and establish a fresh Authling issuer on the next start. The
stack relies on OrbStack's default Compose domains, automatic HTTPS proxy, and
container CA installation; it is not a production deployment example.

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
