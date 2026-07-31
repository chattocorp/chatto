# Licensing

Chatto uses per-file SPDX license metadata following the
[REUSE](https://reuse.software/) specification. The canonical machine-readable
license boundary is in [REUSE.toml](REUSE.toml).

The default repository license is the GNU Affero General Public License version
3 or any later version (`AGPL-3.0-or-later`). This covers the Chatto server,
CLI, and bundled server release artifacts unless a more specific license is
declared.

Apache-2.0 exceptions are used where permissive reuse is intentional. These
include the independently versioned `pkg/events`, `pkg/natsruntime`,
`pkg/datacrypto`, and `pkg/appconfig` shared framework modules, the standalone
frontend source and image, public protocol/API definitions, generated
TypeScript API client/types, documentation, and deployment examples. The
shared modules remain explicitly pre-1.0; their permissive license does not
imply API stability.

Full license texts are available in [LICENSE](LICENSE) and [LICENSES/](LICENSES/).
