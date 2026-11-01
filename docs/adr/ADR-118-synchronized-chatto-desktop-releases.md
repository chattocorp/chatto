# ADR-118: Synchronize Chatto and Desktop Releases

**Date:** 2026-10-25

**Status:** Partially superseded

## Context

ADR-067 gave Chatto Desktop an independent release stream. Desktop embeds the
same frontend as Chatto, but its release workflow replaced the frontend version
with the desktop version. Users could not compare the displayed version with
the corresponding web frontend release.

The products share a repository and frontend source. Independent desktop
versions add release work without a current need for a separate release cadence.

## Decision

Chatto Desktop, the server, the frontend, and `@chatto/client` share one version, release commit,
release PR, and `vX.Y.Z` tag. Every new Chatto release builds desktop artifacts.
Desktop-only changes contribute to the shared changelog and release version.
This supersedes the independent desktop release decision in ADR-067.

The client package joins this release group on 2026-10-26. Client-only changes
also contribute to the shared changelog and version. Release-please updates its
package version through the root component. The former independent client
tags and changelog remain historical records.

Each shared tag starts the client npm publication workflow. Prereleases use
the npm `next` tag; stable releases use `latest`. The workflow verifies the
package version against the shared tag and publishes the tested tarball.
[ADR-121](ADR-121-github-desktop-updates.md) requires the server build and desktop
assembly to succeed before npm publication. GitHub release publication also
requires npm publication to succeed. A failed npm job can be retried against
the same tag if that package version is still unpublished.

Tagged desktop builds display the shared release version. Manual verification
builds identify the frontend with the development version and commit SHA; their
native package metadata retains the checked-in version.

Desktop archives and checksums are attached to the same GitHub release. The
original decision let the server job publish before desktop builds completed.
[ADR-121](ADR-121-github-desktop-updates.md) supersedes this part: the release
stays in draft until both server and desktop jobs succeed. The shared version
and release identity remain unchanged.

Signing remains subject to protected-environment approval and trusted source
checks. Source commits must be reachable from `main` or a maintenance release
branch. Old desktop alpha tags and changelogs remain historical records.

## Consequences

Users can identify a Chatto release consistently across desktop and web clients.
Package consumers can also identify the corresponding `@chatto/client` version.
Client-server compatibility rules remain unchanged; matching versions are not
required to connect.

Each release requires desktop builds and signing approvals. Desktop-only fixes
also advance the Chatto version. A signing outage can delay desktop downloads,
and now blocks GitHub publication under ADR-121. Independent desktop
releases can be reconsidered if release costs justify the additional versioning.
