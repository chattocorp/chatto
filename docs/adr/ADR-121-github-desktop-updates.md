# ADR-121: Publish Complete GitHub Releases for Desktop Updates

**Date:** 2026-10-29

**Status:** Accepted

## Context

ADR-118 allowed desktop uploads to finish after server publication. A desktop
updater that discovers releases through GitHub can then select a release whose
packages or manifests are missing. Desktop and server already share a version
and a draft release.

## Decision

Use `electron-updater` with the GitHub provider for whole-application updates.
Keep the frontend inside the application. Support signed macOS apps, signed
Windows NSIS installations, and Linux AppImages. Keep the existing archive
downloads for manual installation.

Stable and Beta are user-selectable channels. Prerelease builds default to
Beta; stable builds default to Stable. Preserve saved channel choices across
upgrades. Disable downgrades. Stable
automatic checks use a 36-hour rollout; manual checks and Beta bypass it.
Download in the background and require an explicit restart to install. Recheck
the current release before installation to reject a superseded download.

Server and desktop jobs build concurrently. Desktop assembly requires all
update packages and generates manifests from their exact bytes. A final job
publishes the draft only after server and desktop jobs succeed. This supersedes
the independent-publication part of ADR-118. Component library releases remain
independent.

Only tagged release builds enable the updater. The Windows feed configuration
pins the expected publisher subject. Linux updates use HTTPS and manifest
checksums; they do not have a separate publisher signature.

## Consequences

No update service or separate storage account is required. Installed apps
depend on the GitHub repository identity. Changing hosts requires an updater
transition release while the existing repository remains available.

Desktop signing or build failures now delay GitHub publication, including
server releases. Container and npm publication are not an atomic transaction
with the GitHub release and can finish earlier. Published update artifacts are
immutable in practice: fixes require a new version.

GitHub receives update requests and users' IP addresses. The updater sends no
Chatto account or connected-server information. Users can disable automatic
checks. Startup always serves the bundled local frontend.
