# Releasing Chatto

Chatto uses release-please to prepare alpha releases from `main`. Stable releases
and maintenance patches come from `release-x.y` branches. Each branch uses the
same `.release-please-config.json` and `.release-please-manifest.json` paths; the
configuration committed to that branch determines whether it produces
prereleases or stable releases.

## GitHub Latest release

The repository-wide **Latest** designation belongs only to the newest stable
Chatto version. Prereleases and independently versioned components such as
Translatto, Runling, and `@chatto/client` must not claim it.

Release-please creates component releases as drafts and creates their tags.
The next workflow step publishes those component releases with `--latest=false`.
Their tag-triggered package workflows continue to run as before. The root Chatto
release stays a draft until its build workflow publishes it, explicitly setting
Latest only when it is the highest stable Chatto tag.

If component publication fails after release-please creates its draft, publish
that existing release with `gh release edit <component-tag> --draft=false
--latest=false`. Do not create another release or tag. Release-please may not
emit an already-created release in its outputs on a rerun.

Maintenance branches carry their own workflow and release configuration.
Backport this policy before publishing from those branches.

## Documentation channels

The public documentation has two independently deployed channels:

- `https://docs.chatto.run` serves the highest stable Chatto release.
- `https://dev-docs.chatto.run` serves the newest documentation build from
  `main`.

Relevant pushes to `main` publish an immutable docs image tagged as
`main-<UTC timestamp>-<short SHA>`. Flux deploys the newest sortable tag to the
development docs site. The site identifies itself as unreleased, displays the
source revision, and opts out of search indexing.

After a stable `vX.Y.Z` release completes successfully, the release workflow
builds the docs from that exact tag and publishes
`ghcr.io/chattocorp/chatto-docs:vX.Y.Z`. Flux selects the highest stable SemVer
tag for `docs.chatto.run`. Prerelease tags never update the stable docs site.

Stable docs images are immutable release snapshots. Corrections to the stable
documentation ship with the next Chatto patch release rather than replacing an
existing `vX.Y.Z` image.

## Prereleases from main

The release-please configuration on `main` uses prerelease versioning. Feature
work merges into `main`, and release-please prepares versions such as
`0.5.0-alpha.1`, `0.5.0-alpha.2`, and so on. Prereleases publish the `next`
container tags.

When development moves to a new version series, force its first version with a
`Release-As` footer. For example:

```sh
git switch -c begin-0.6 origin/main
git commit --allow-empty \
  -m "chore(release): begin 0.6 prereleases" \
  -m "Release-As: 0.6.0-alpha.1"
git push -u origin begin-0.6
```

Merge this branch into `main`, preserving the `Release-As` footer in the squash
commit or pull request body.

## Runling releases

Runling is versioned independently under `packages/runling/`. Its releases
use stable versions and `runling/vX.Y.Z` tags. The `publish-runling` workflow
tests the npm tarball before publishing it. For the one-time npm trusted
publisher setup and local verification commands, see
[Runling development and releases](../packages/runling/README.md#monorepo-development-and-releases).

## Chatto Desktop releases

Chatto Desktop shares the server and frontend version, release commit, and
`vX.Y.Z` tag. The root release-please component updates both package versions
and `cli/version.go`. Desktop-only changes contribute to the root changelog
and the next Chatto release. The old desktop changelog and
`chatto-desktop/v*` tags are historical records, not a separate release stream.

Merging a Chatto release PR creates a draft GitHub release and the shared
tag. The release workflow checks and builds macOS, Windows, and Linux
bundles from that tag, signs and notarises the macOS bundle, signs and verifies
every Windows executable, requires the expected ChattoCorp publisher on the
main application, and uploads archives and SHA-256 checksums to that release.
Windows releases also include a per-user `*_Windows_X64_Setup.exe` installer.
Its installer and embedded uninstaller are signed and verified before upload.
The ZIP remains available. See the desktop README for installation and the
interactive Windows verification checklist; PR builds are unsigned.
The server job alone publishes the release and does not wait for Desktop.
Desktop assets can arrive before or after publication. If signing fails, retry
the failed jobs on the same tagged run; a server security release must not wait
for a signing service. Linux and ordinary CI artifacts remain unsigned experimental builds.
Windows and macOS use separate protected signing environments. Signing-service
provisioning, protected-environment settings, renewal, and emergency revocation
are documented in [`apps/desktop/README.md`](../apps/desktop/README.md).

Tagged desktop builds use the exact shared version for the shell and frontend.
Manual verification builds retain the checked-in package version for native
packaging, but identify the frontend as `{development-version}+{commit-sha}`.
Matching release versions do not require matching client and server versions;
the existing client-server compatibility rules still apply. See
[ADR-118](adr/ADR-118-synchronized-chatto-desktop-releases.md).

Before publishing a tag, the release workflow can verify the complete desktop
packaging path without creating a release or building a Chatto server image.
Run the `release` workflow manually, select the `desktop` target, and optionally
provide a branch, tag, or commit reachable from `origin/main` or an
`origin/release-*` branch in the `ref`
input. Signed desktop builds reject other commits before running repository
code or requesting signing credentials. The workflow builds and packages all
three platforms, generates the same checksum file used by a tagged release, and
uploads the assembled files as a one-day verification artifact.

Desktop release tags must also point to commits reachable from `origin/main`
or an `origin/release-*` branch.
The signing jobs enforce this before running repository code.

Before the first synchronized release, add the `v*` tag policy to both protected
signing environments. After this change lands, close the pending independent
Desktop release PR and remove the old `chatto-desktop/v*` environment policies.
Keep required reviewers and the `main` policy for manual verification. Existing
maintenance branches keep their old workflow until this release change is
backported; do not rerun old tags to add synchronized desktop assets.

## Create a stable release branch

Before a release with MCP, use the
[MCP interoperability checklist](MCP-INTEROPERABILITY.md). Keep MCP marked as
experimental. Record the tested protocol, host versions, conformance results,
and unresolved blockers. A successful Chatto unit test run does not prove
conformance or agent-host compatibility.

Create `release-x.y` from the commit intended for the stable release. On that
branch, remove `versioning`, `prerelease`, and `prerelease-type` from
`.release-please-config.json`. Commit the stable configuration with an explicit
`Release-As` footer:

```sh
git switch -c release-0.5 <stable-candidate>
git add .release-please-config.json
git commit \
  -m "chore(release): prepare 0.5 stable releases" \
  -m "Release-As: 0.5.0"
git push -u origin release-0.5
```

Release-please then prepares the stable `0.5.0` release PR on `release-0.5`.
Stable releases publish `latest` only when they are the highest stable version.

## Maintain a stable release

When a fix applies to both current development and a stable series, land it on
`main` first and backport that commit through a pull request targeting
`release-x.y`. Use conventional `fix:` commits so release-please prepares the
next patch release, such as `0.5.1`.

If a bug exists only in the stable series, fix it directly on `release-x.y`.
Forward-port a release-first fix through a separate `main` pull request only
when current development also needs it.

Never merge a `release-x.y` branch wholesale into `main`. Stable branches carry
their own release-please configuration, manifests, changelog commits, and
embedded stable versions. Backport or forward-port the applicable product and
automation commits instead.
