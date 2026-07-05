# Licensing

Chatto uses per-file SPDX license metadata following the
[REUSE](https://reuse.software/) specification. The canonical license texts are
stored in [LICENSES/](LICENSES/).

Unless a file declares another license through an SPDX header, adjacent
`.license` file, or [REUSE.toml](REUSE.toml) annotation, files in this
repository are licensed under the Apache License 2.0 (`Apache-2.0`). The full
Apache-2.0 text is available in [LICENSE-APACHE](LICENSE-APACHE) and
[LICENSES/Apache-2.0.txt](LICENSES/Apache-2.0.txt).

The `cli/` directory is licensed under the GNU Affero General Public License
version 3 or any later version (`AGPL-3.0-or-later`). This covers the Chatto
server and CLI implementation, including the executable artifact that embeds
the frontend assets. The full AGPL-3.0-or-later text is available in
[LICENSE-AGPL](LICENSE-AGPL) and
[LICENSES/AGPL-3.0-or-later.txt](LICENSES/AGPL-3.0-or-later.txt).

The top-level [LICENSE](LICENSE) file is a mixed-license notice rather than a
single-license grant. It exists to avoid presenting the repository as
Apache-2.0-only while still giving readers an obvious place to start. The
top-level `LICENSE-APACHE` and `LICENSE-AGPL` files are exact license texts so
GitHub and other repository scanners can detect both licenses.

Generated code inherits the license of the generated file unless the generator
emits a more specific SPDX license identifier.

## Checking License Metadata

Run the REUSE linter before changing license metadata or adding many files:

```sh
mise license-check
```

The task runs `reuse lint` through `uvx` so contributors do not need a global
`reuse` installation.
