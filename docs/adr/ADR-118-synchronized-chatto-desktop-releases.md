# ADR-118: Synchronize Chatto and Desktop Releases

**Date:** 2026-10-25

**Status:** Accepted

## Context

ADR-067 gave Chatto Desktop an independent release stream. Desktop embeds the
same frontend as Chatto, but its release workflow replaced the frontend version
with the desktop version. Users could not compare the displayed version with
the corresponding web frontend release.

The products share a repository and frontend source. Independent desktop
versions add release work without a current need for a separate release cadence.

## Decision

Chatto Desktop, the server, and the frontend share one version, release commit,
release PR, and `vX.Y.Z` tag. Every new Chatto release builds desktop artifacts.
Desktop-only changes contribute to the shared changelog and release version.
This supersedes the independent desktop release decision in ADR-067.

Tagged desktop builds display the shared release version. Manual verification
builds identify the frontend with the development version and commit SHA; their
native package metadata retains the checked-in version.

Desktop archives and checksums are attached to the same GitHub release. The
server job alone controls publication. It does not depend on desktop build or
signing jobs. Desktop uploads can finish after publication, and failed desktop
jobs can be retried against the same tag.

Signing remains subject to protected-environment approval and trusted source
checks. Source commits must be reachable from `main` or a maintenance release
branch. Old desktop alpha tags and changelogs remain historical records.

## Consequences

Users can identify a Chatto release consistently across desktop and web clients.
Client-server compatibility rules remain unchanged; matching versions are not
required to connect.

Each release requires desktop builds and signing approvals. Desktop-only fixes
also advance the Chatto version. A signing outage can delay desktop downloads,
but cannot block publication of a server security fix. Independent desktop
releases can be reconsidered if release costs justify the additional versioning.
