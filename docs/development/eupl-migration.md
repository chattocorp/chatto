# EUPL migration

Updated on 2026-11-09. The maintainer authorized implementation after merging
`origin/main`. [ADR-122](../adr/ADR-122-product-license-boundaries.md) records
the accepted repository-wide decision.

## Implemented boundary

| Area | License |
| --- | --- |
| Chatto server, CLI, and previously AGPL-covered repository files | EUPL-1.2 with its standard later-version provision |
| Runling, ChattoBot, and Authling | Apache-2.0 |
| Existing Apache-2.0 clients, modules, protocols, documentation, and examples | Apache-2.0 |
| Third-party code and assets | Their existing licenses |

[REUSE.toml](../../REUSE.toml) and explicit file metadata define the exact
boundary. [LICENSING.md](../../LICENSING.md) describes source access and
earlier grants. The switch applies from the commit introducing ADR-122 and
to releases that contain it. Previous releases retain their grants.

## HEIC result

The merged main branch uses `github.com/gen2brain/h265 v0.2.4`, under MIT.
The former `gen2brain/heic` wrapper and embedded `imazen/heic` decoder are
no longer server dependencies. The release configuration no longer selects
the old WASM decoder. This migration makes no decoder changes.

## Distribution checks

- Run `mise license-check` to check file metadata.
- Run `mise sync-cli-legal` before building Chatto. Confirm `chatto license`
  prints EUPL and the source repository notice.
- Check archive license files, Docker build contexts, OCI labels, and the
  generated Homebrew formula against the EUPL declaration.
- Check that Authling images copy its local Apache-2.0 license and notice.
- Check that Runling packages include its Apache-2.0 license and the retained
  MIT notice. ChattoBot also retains its earlier MIT notice.
- Preserve third-party notices and old release tags. Remaining AGPL mentions
  in historical records and EUPL's compatibility appendix are intentional.

REUSE and packaging checks do not prove copyright ownership or complete
dependency compliance. No release is published by this migration.
