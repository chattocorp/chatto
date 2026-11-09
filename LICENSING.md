# Licensing

Chatto uses per-file SPDX license metadata following the
[REUSE](https://reuse.software/) specification. The canonical machine-readable
license boundary is in [REUSE.toml](REUSE.toml).

The default repository license is the European Union Public Licence version
1.2 (`EUPL-1.2`), with its standard later-version provision. This covers the
Chatto server, CLI, and bundled server release artifacts unless a more
specific license is declared.

Apache-2.0 exceptions are used where permissive reuse is intentional. These
include the independently versioned `pkg/events`, `pkg/jetstreamutil`,
`pkg/natsruntime`, `pkg/datacrypto`, and `pkg/appconfig` shared framework
modules, the
framework-neutral `packages/translatto` internationalisation runtime, the
standalone frontend source and image, public protocol/API definitions,
generated TypeScript API client/types, documentation, and deployment examples.
The shared modules remain explicitly pre-1.0; their permissive license does not
imply API stability.

Full license texts are available in [LICENSE](LICENSE) and [LICENSES/](LICENSES/).

The independent Runling package under `packages/runling/` uses Apache-2.0.
See [packages/runling/LICENSE](packages/runling/LICENSE). Its original MIT
notice is retained in [packages/runling/NOTICE](packages/runling/NOTICE).

The `@chatto/client` package under `packages/chatto-client/` uses Apache-2.0.
See [packages/chatto-client/LICENSE](packages/chatto-client/LICENSE).

The private ChattoBot package under `packages/chattobot/` uses Apache-2.0.
See [packages/chattobot/LICENSE](packages/chattobot/LICENSE).

The independent Authling product under `authling/` uses Apache-2.0.
See [authling/LICENSE](authling/LICENSE).

## Source access and earlier versions

The license switch applies from the commit that introduces
[ADR-123](docs/adr/ADR-123-product-license-boundaries.md) and to subsequent
releases that contain it. Earlier releases retain their existing license
grants. The switch does not withdraw earlier AGPL or MIT permissions.
Third-party code and assets retain their own licenses and notices.

When you distribute or communicate EUPL-covered software, provide its source
code or a repository where the source is freely available, as required by
the license. This includes providing access to the software's essential
functions over a network. Preserve notices and provide the license text.
The compatibility provisions in EUPL Article 5 also apply.

Chatto source is available at <https://github.com/chattocorp/chatto>.
For a release, use its matching tag; for a development build, use its commit.
If you distribute a modified build, make the source for that build available.
