[![CI](https://github.com/chattocorp/chatto/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/chattocorp/chatto/actions/workflows/ci.yml?query=branch%3Amain)
[![Latest release](https://img.shields.io/github/v/release/chattocorp/chatto?include_prereleases&sort=semver)](https://github.com/chattocorp/chatto/releases)
[![License: EUPL-1.2 with Apache-2.0 exceptions](https://img.shields.io/badge/license-EUPL--1.2%20with%20Apache--2.0%20exceptions-blue.svg)](LICENSE)

# Chatto

<p><img width="1920" height="1196" alt="It's Chatto!" src="https://github.com/user-attachments/assets/a6a8ef8c-9f56-48ed-8740-53115273c22e" /></p>

A fully-featured chat application for teams and communities, running off a single executable (two if you want video and voice calls.) Free and easy to self-host, with [cloud hosting available soon](https://chatto.run/cloud).

- [Website](https://chatto.run)
- [Documentation](https://docs.chatto.run) ([Prereleases](https://dev-docs.chatto.run))
- [Official Chatto Community](https://chat.chatto.run/)
- [Releases](https://github.com/chattocorp/chatto/releases)
- [Security Policy](SECURITY.md)

Chatto is built, designed and maintained by humans with the help of coding agents. Read [Chatto is Robots](https://www.hmans.dev/blog/chatto-is-robots) to learn more.

## What Is in This Repository

- **Chatto**: the [chat server and CLI](cli/) and [bundled web frontend](apps/frontend/).
- **[@chatto/client](packages/chatto-client/README.md)**: the TypeScript client for Chatto frontends, bots, and integrations. It manages connections, sessions, and reactive server data without a UI framework dependency.
- **[ChattoBot](packages/chattobot/README.md)**: the Chatto project's assistant for documentation questions and maintainer tasks.
- **[Translatto](packages/translatto/README.md)**: a JavaScript and TypeScript internationalisation library, published as `@chatto/translatto`.
- **[Runling](packages/runling/README.md)**: an independent workflow and agent orchestrator, published to npm as `runling`.
- **[Authling](authling/README.md)**: an independent identity provider. It is here temporarily and will move to its own repository.
- **Public API definitions**: [protobuf sources](proto/) and [generated TypeScript API types](packages/api-types/).
- **Shared framework modules**: [events](pkg/events/README.md), [jetstreamutil](pkg/jetstreamutil/README.md), [natsruntime](pkg/natsruntime/README.md), [datacrypto](pkg/datacrypto/README.md), and [appconfig](pkg/appconfig/README.md).

### Examples

- **[Botto](examples/botto/README.md)**: a TypeScript bot built with Runling and `@chatto/client`.
- **[Runling bot](examples/runling-bot/README.md)**: a bot that receives Chatto webhooks and posts agent replies through the API.
- **[Docker Compose](examples/dockercompose/README.md)**: a Chatto deployment with NATS, LiveKit, and Caddy.
- **Kubernetes**: a [Chatto server deployment](examples/k8s/README.md) and [standalone frontend manifests](examples/k8s-client/).

## Development

```sh
mise dev
```

This installs the tools and dependencies of Chatto, then starts it. `mise dev-full` also starts Authling, Runling, Mailpit, and LiveKit. Open `http://chatto.local.localhost:4000` and sign in as `alice` with the password `foobar123`. See [CONTRIBUTING.md](CONTRIBUTING.md) for the complete development guide.

## License

Chatto uses `EUPL-1.2` by default. The shared framework modules, standalone frontend, integration surfaces, documentation, and examples use Apache-2.0. Runling, ChattoBot, and Authling also use Apache-2.0. See [LICENSING.md](LICENSING.md) and [REUSE.toml](REUSE.toml) for the exact boundary.

The licenses do not give permission to use Chatto names or logos as official branding for a fork or modified version. See [NOTICE](NOTICE).

## Contributing

This project does **not accept outside contributions** at this time. See [CONTRIBUTING.md](CONTRIBUTING.md) for bug reports and development notes.
