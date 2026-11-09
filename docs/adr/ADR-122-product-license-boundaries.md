# ADR-122: EUPL for Chatto and Apache-2.0 for Independent Products

**Date:** 2026-11-09

## Context

This is a repository-wide licensing decision. Chatto needs a reciprocal
license for its server and CLI. The maintainer selected EUPL 1.2 for this
purpose and Apache-2.0 for the independent Runling, ChattoBot, and Authling
products. Existing permissive boundaries for clients, shared modules, public
protocols, documentation, and examples must remain intact.

The former HEIC wrapper embedded an AGPL decoder. Before this change, main
replaced it with the MIT-licensed `gen2brain/h265`; `cli/go.mod` now pins
v0.2.4. The license switch does not change decoding behavior.

## Decision

Use `EUPL-1.2` as the repository default, including Chatto's server, CLI,
tooling, and experiments that previously used the default AGPL license.
Retain EUPL's standard later-version provision; do not add a version-only
restriction. Use the registered SPDX identifier `EUPL-1.2` in metadata.

Use Apache-2.0 for `authling/`, `packages/runling/`, and
`packages/chattobot/`. Preserve their copyright attribution and earlier
license notices. Give these products local license files so their
distribution does not depend on Chatto's root license.

Keep all existing Apache-2.0 boundaries and third-party license exceptions.
REUSE.toml and explicit file metadata define the exact boundary.
This decision replaces ADR-059's statement that the repository default
remains AGPL; its shared-module Apache-2.0 decision still applies.

Apply the new grants from the commit introducing this record and to releases
that include it. Do not rewrite previous tags or withdraw earlier AGPL or
MIT grants. Do not infer new ownership or permissions for third-party code.

## Consequences

Synchronize package declarations, embedded legal files, image labels,
archive contents, public license guidance, and agent instructions. Authling
images ship its own Apache-2.0 license. Runling's npm package includes its
NOTICE with the earlier MIT attribution and permission text.

Chatto distributions include a source repository notice. Released source is
identified by the matching tag; development source is identified by its
commit. Distributors of modified builds must provide the corresponding
source under the applicable license terms.

REUSE verification checks file metadata. It does not establish copyright
ownership or prove complete dependency compliance.
