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
[event-sourcing framework](pkg/events/README.md) and
[embedded NATS runtime](pkg/natsruntime/README.md).

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
