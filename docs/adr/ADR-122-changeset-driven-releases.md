# ADR-122: Changeset-Driven Releases

**Date:** 2026-11-09

**Status:** Accepted

## Context

Commit filtering makes Chatto release notes difficult to control. The repository
also contains independently released products and shared modules. Release intent
must be explicit for each unit, without coupling their version schedules.

## Decision

This repository-wide decision replaces release-please with Sweet Release for
all nine release units listed in [the release guide](../RELEASING.md). Each unit
owns a manifest and Markdown change notes. Chatto retains its synchronized
server, client, frontend, and desktop release group from ADR-118.

Preserve existing versions, changelogs, and tag prefixes. Treat the migration
commit as a baseline rather than a new release. Prepare separate PRs per unit
from `main`. After selected CI checks succeed, tag the full push range. Keep
artifact publication in the existing tag workflows. Component GitHub release
notes never claim Latest; only the highest stable Chatto release can claim it.

## Consequences

Authors must write notes for each affected release unit. Commit titles no longer
select versions or changelog content. Dependency updates are explicit; Sweet
does not propagate releases to consumers. Maintainers review generated PRs.

Sweet 0.3.0 bumps prerelease baselines to ordinary stable versions. Authling's
next release may be stable. Prerelease channels and maintenance-branch automation
remain future work. Component release notes and artifact publication are separate
jobs and must both be checked.
