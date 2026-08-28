<!--
SPDX-FileCopyrightText: 2024-present Chatto contributors
SPDX-License-Identifier: AGPL-3.0-or-later
-->

# Core storage compatibility fixtures

These Buf descriptor images freeze the protobuf schema before the core package
split. The compatibility test uses them to verify that stored records can move
to lifecycle-specific packages without a data migration.

- `v0.4.20.binpb` contains the core schema from the latest Chatto 0.4 release.
- `pre-refactor-80a112609.binpb` contains the schema from `origin/main` before
  the package split.

To replace a fixture, check out the applicable revision in a temporary worktree.
Run this command from its `proto` directory:

```sh
mise x -- buf build --exclude-source-info --path chatto/core/v1 \
  --output /path/to/the/fixture.binpb
```

Do not replace an old fixture only because the current schema changes. A fixture
is an immutable record of a released or persisted schema.
