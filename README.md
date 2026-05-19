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

[Conductor](https://conductor.build) workspaces launch a fully containerized dev stack via Docker Compose. The `run` script in `conductor.json` invokes:

```sh
docker compose -p instance-name up
```

The stack is then reachable via OrbStack at:

```
https://instance-name.orb.local/
```

Each instance is bootstrapped with the same dev credentials (configured in `compose.yml`):

- **Login:** `admin`
- **Email:** `admin@dev.com`
- **Password:** `foobar123`

## Instructions for Coding Agents

- Please consult the additional rules in `.claude/rules/**` for more specific guidelines on coding style, architecture, planning, and testing.
- Please use this project's GitHub Issues for planning work.

## License

Chatto is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE). This means:

- You are free to use, modify, and distribute Chatto.
- If you run a modified version as a network service, you must make the source code of your modifications available to its users.
- Any derivative work must also be licensed under the AGPL-3.0.

For full details, see the [LICENSE](LICENSE) file or run `chatto license`.

## Contributing

This project is **not accepting outside contributions** at this time. If you have feedback, bug reports, or ideas, please [get in touch](mailto:hendrik@mans.de) — we'd love to hear from you.
