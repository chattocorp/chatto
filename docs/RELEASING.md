# Releases

All releasable packages use Sweet Release. Markdown notes select the next
version and supply its changelog. Commits do not select releases. See
[ADR-122](adr/ADR-122-changeset-driven-releases.md).

## Release units

| Package path          | Tag prefix            | Version targets                          |
| --------------------- | --------------------- | ---------------------------------------- |
| `.`                   | `v`                   | Chatto server, client, frontend, desktop |
| `authling`            | `authling/v`          | `version.go`                             |
| `packages/runling`    | `runling/v`           | `package.json`, `version.txt`            |
| `packages/translatto` | `translatto/v`        | `package.json`, `version.txt`            |
| `pkg/events`          | `pkg/events/v`        | Package manifest                         |
| `pkg/appconfig`       | `pkg/appconfig/v`     | Package manifest                         |
| `pkg/datacrypto`      | `pkg/datacrypto/v`    | Package manifest                         |
| `pkg/jetstreamutil`   | `pkg/jetstreamutil/v` | Package manifest                         |
| `pkg/jobs`            | `pkg/jobs/v`          | Package manifest                         |
| `pkg/natsruntime`     | `pkg/natsruntime/v`   | Package manifest                         |

Each unit owns `.sweet.yml`, `.sweet/` notes, and `CHANGELOG.md`. The client,
frontend, and desktop belong to the root release unit; they do not have separate
Sweet manifests. Dependency updates and consumer release notes remain explicit.

## Prepare and tag releases

Add notes with `mise x -- sweet change <package-path>`. `fix` selects a patch,
`feature` and `breaking` select a minor while versions are below 1.0. Review
this policy before any package reaches 1.0. `chore` alone does not release.

If a change amends an existing unreleased note in the same release unit, you
can update that note or combine the notes and remove duplicates. Preserve the
final behavior, required upgrade actions, and release type needed for the
combined change. Do not rewrite published release notes. See
[Writing change notes](../CONTRIBUTING.md#writing-change-notes).

The Prepare releases workflow runs on `main` when notes, manifests, Sweet
configuration, or the workflow change. It uses Sweet's pinned reusable workflow
to create one PR per package. Run it manually to refresh a PR or retry a failure.
A release PR consumes notes, updates version targets, and adds a changelog entry.

After selected CI jobs succeed, tagging uses the push's complete before/after
range. Tags point to the commits that introduced each version. Rerunning the
original CI run retries missing remote pushes; a later push does not recover
an earlier failed range. Tags are never moved. Pushes send tags individually
so GitHub starts each artifact workflow.

The migration commit is a baseline for all nine manifests. It must not tag
existing versions again. Packages at `0.0.0` remain unreleased until a bump.
The existing `RELEASE_PLEASE_PAT` secret is retained only as a credential name
for PR creation and tag pushes; no release-please action or configuration remains.
Use a token that can write contents and PRs and start downstream workflows.

## GitHub Latest release

Only the highest stable Chatto version claims **Latest**. The Chatto publication
job creates its draft from the matching Sweet changelog entry and publishes it
with the completed artifacts. Component tags start a separate notes workflow,
which creates drafts and publishes them with `--latest=false`. Authling and npm
artifact workflows run separately; a component GitHub release does not prove
that its artifact publication succeeded. Check both runs.

Rerun failed component notes jobs against the same tag. Existing releases are
reused. Do not change published tag targets or rebuild a version with new content.

## iOS TestFlight

Chatto `v*` tags also start the independent `Publish iOS to TestFlight` workflow.
It rebuilds the bundled frontend and uploads a signed device build. Configure
its `mobile-testflight` environment and Apple credentials as described in the
[mobile release guide](../apps/mobile/README.md#automated-testflight-updates).
A successful server or desktop release does not confirm a TestFlight upload.
The workflow can also run manually from `main`.

## Current limits

Preparation and tagging run on `main`. Maintenance-branch automation is not
configured. Do not use old commit footers to request releases. Backport the
release workflow explicitly before enabling releases on another branch.

Sweet 0.3.0 does not preserve prerelease channels during a bump. Authling keeps
its imported `0.1.0-alpha.3` baseline, but its next ordinary bump can be stable
(for example, a fix selects `0.1.1`). Automated alpha/beta sequences need future
Sweet support. Existing artifact workflows still accept prerelease tags.

## Publication approval

The tagged workflow requests all environment approvals at the start. In
**Review deployments**, select `chatto-release`, `desktop-signing`, and
`desktop-windows-signing`, then approve them together. The `approve-release`
job and the desktop signing jobs have no build dependencies, so all three
environments are available in the same review stage.

This review authorizes both signing and publication. The workflow then builds
server archives, server and frontend container images, and signed desktop
packages. The build jobs save these files as workflow artifacts. They do not
upload release assets or push release images to GHCR.

After the approval job, all builds, and desktop assembly succeed, the `release`
job continues automatically. It checks the archive checksums, uploads the
assets, pushes the saved container images, and publishes the GitHub Release.
Stable releases then
update Homebrew and start the stable documentation build. There is no second
approval to make the GitHub Release public. Publication credentials are
repository secrets; do not move them into `chatto-release`, because the
publication job does not enter that environment. Signing secrets and Windows
OIDC configuration remain in their existing environments. Keep the required
reviewers and branch/tag policies on all three environments.

The same `approve-release` job also gates the client npm workflow. Client
publication waits for the server build and desktop assembly to succeed.
The npm job keeps its `npm` environment. Configure its trusted publisher with
the calling workflow `release.yml`; see
[client maintenance](../packages/chatto-client/MAINTAINING.md).

Publication across GitHub and GHCR is not atomic. If publication fails, some
images or assets can already be available. Rerun the failed jobs on the same
workflow run to reuse the saved artifacts; they are retained for seven days.
Uploads replace matching assets, and image pushes can be repeated. A retry of
an environment-protected job can require approval again. Do not rebuild a
published version to change its contents.

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

## Runling releases

Runling is versioned independently under `packages/runling/`. Its releases
use stable versions and `runling/vX.Y.Z` tags. The `publish-runling` workflow
tests the npm tarball before publishing it. For the one-time npm trusted
publisher setup and local verification commands, see
[Runling development and releases](../packages/runling/README.md#monorepo-development-and-releases).

## Chatto Desktop releases

Chatto Desktop shares the server and frontend version, release commit, and
`vX.Y.Z` tag. The root Sweet manifest updates both package versions
and `cli/version.go`. Desktop-only changes contribute to the root changelog
and the next Chatto release. The old desktop changelog and
`chatto-desktop/v*` tags are historical records, not a separate release stream.

Merging a Chatto release PR starts CI, which creates the shared tag after
checks pass. The publication job creates the GitHub draft from the changelog. The release workflow checks and builds macOS, Windows, and Linux
bundles from that tag, signs and notarises the macOS bundle, signs and verifies
every Windows executable, requires the expected ChattoCorp publisher on the
main application, and uploads archives and SHA-256 checksums to that release.
Windows releases also include a per-user `*_Windows_X64_Setup.exe` installer.
Its installer and embedded uninstaller are signed and verified before upload.
The ZIP remains available. See the desktop README for installation and the
interactive Windows verification checklist; PR builds are unsigned.
Publication waits for server and desktop builds, including signing, to succeed.
If signing fails, retry the failed jobs on the same tagged run. A signing
outage blocks publication. Linux and ordinary CI artifacts remain unsigned
experimental builds.
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
uploads the assembled files as a seven-day verification artifact.

Desktop release tags must also point to commits reachable from `origin/main`
or an `origin/release-*` branch.
The signing jobs enforce this before running repository code.

Before the first synchronized release, add the `v*` tag policy to both protected
signing environments. After this change lands, close the pending independent
Desktop release PR and remove the old `chatto-desktop/v*` environment policies.
Keep required reviewers and the `main` policy for manual verification. Existing
maintenance branches keep their old workflow until this release change is
backported; do not rerun old tags to add synchronized desktop assets.
