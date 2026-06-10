# chatto

A really good chat application for teams and communities, free and easy to self-host, with [cloud hosting available](https://chatto.run/cloud).

- [Website](https://www.chatto.run)
- [Documentation](https://docs.chatto.run)

## Warning: Alpha Software 🚧

While Chatto is moving forward at a rapid pace, we can't yet give any guarantees about stability, security, or performance; we also at this point can't support data migrations.

We are providing the source code here for transparency and to allow early adopters to experiment and provide feedback. If you choose to actually run it, **be prepared to lose some or all of your data at any time**.

A lot of projects say this and people often ignore it, so let me spell things out a bit more:

- You **will** lose runtime and permission configuration and will be required to manually fix things.
- You **will** lose data for experimental features that we decide to remove or significantly change.
- You **will** experience breaking changes in the GraphQL API.
- You **will** lose user and message data to bugs, or if we need to make breaking changes to the data model.

It should be no surprise that we are working hard to move towards a release that can give better guarantees, but we're not there yet.

## Development with Conductor

[Conductor](https://conductor.build) workspaces run the dev stack natively via `tilt up` — no Docker or Kubernetes required. The `run` script in `.conductor/settings.toml` wires Conductor's assigned `$CONDUCTOR_PORT` (and `+1` / `+2`) into the env vars the `Tiltfile` reads:

| Port              | Process                              |
| ----------------- | ------------------------------------ |
| `$CONDUCTOR_PORT` | Vite dev server (user-facing URL)    |
| `+1`              | Go backend (`CHATTO_WEBSERVER_PORT`) |
| `+2`              | Embedded NATS                        |
| `+9`              | Tilt web UI                          |

Outside Conductor, `mise x -- tilt up --stream` uses the defaults from the `Tiltfile` (Vite 5173, backend 4000, NATS 4555).

The repository-level Conductor settings are shared in `.conductor/settings.toml`. The run command starts Tilt in streaming mode and lets Tilt supervise the backend, frontend, and GraphQL codegen processes. Put machine-specific overrides in `.conductor/settings.local.toml`; that file is gitignored and wins over shared settings on your machine. Conductor also reads `.worktreeinclude` to copy gitignored local environment files, such as `.env` and `.env.*`, into new workspaces.

Each instance is bootstrapped with the same dev credentials (configured in `cli/chatto.toml` under `[[bootstrap.users]]`):

- **Login:** `alice`
- **Email:** `alice@example.com`
- **Password:** `foobar123`

## License

Chatto is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE). This means:

- You are free to use, modify, and distribute Chatto.
- If you run a modified version as a network service, you must make the source code of your modifications available to its users.
- Any derivative work must also be licensed under the AGPL-3.0.

For full details, see the [LICENSE](LICENSE) file or run `chatto license`.

## Contributing

This project is **not accepting outside contributions** at this time. If you have feedback, bug reports, or ideas, please [get in touch](mailto:hendrik@mans.de) — we'd love to hear from you.
