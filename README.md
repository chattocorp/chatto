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

## License

Chatto is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE). This means:

- You are free to use, modify, and distribute Chatto.
- If you run a modified version as a network service, you must make the source code of your modifications available to its users.
- Any derivative work must also be licensed under the AGPL-3.0.

For full details, see the [LICENSE](LICENSE) file or run `chatto license`.

The AGPL-3.0 license does not grant permission to use Chatto names or logos as
official branding for a fork or modified version; see [NOTICE](NOTICE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local development notes. This project is **not accepting outside contributions** at this time, but feedback, bug reports, and ideas are welcome by [email](mailto:hendrik@mans.de).
