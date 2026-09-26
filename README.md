[![CI](https://github.com/chattocorp/chatto/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/chattocorp/chatto/actions/workflows/ci.yml?query=branch%3Amain)
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

Chatto is built with the help of coding agents. Read [Chatto is Robots](https://www.hmans.dev/blog/chatto-is-robots) to learn more.

## What Is in This Repository

- **Chatto**: the chat server, CLI, and bundled web frontend.
- **[Runling](packages/runling/README.md)**: an independent workflow and agent orchestrator, published to npm as `runling`.
- **[Authling](authling/README.md)**: an independent identity provider. It is here temporarily and will move to its own repository.
- **Shared framework modules**: [events](pkg/events/README.md), [natsruntime](pkg/natsruntime/README.md), [datacrypto](pkg/datacrypto/README.md), and [appconfig](pkg/appconfig/README.md).

## Development

```sh
mise trust
mise install
mise setup
mise dev
```

Then open `http://chatto.local.localhost:4000` and sign in as `alice` with the password `foobar123`. See [CONTRIBUTING.md](CONTRIBUTING.md) for the complete development guide.

## License

Chatto uses `AGPL-3.0-or-later` by default. The shared framework modules, standalone frontend, integration surfaces, documentation, and examples use Apache-2.0. Runling uses MIT. See [LICENSING.md](LICENSING.md) and [REUSE.toml](REUSE.toml) for the exact boundary.

The licenses do not give permission to use Chatto names or logos as official branding for a fork or modified version. See [NOTICE](NOTICE).

## Contributing

This project does **not accept outside contributions** at this time. See [CONTRIBUTING.md](CONTRIBUTING.md) for bug reports and development notes.
