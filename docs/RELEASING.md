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
A release PR that changes only these release files runs no product jobs in CI.
The full checks run on `main` after the merge, before CI makes the tag. See
[CI Job Selection](../CONTRIBUTING.md#ci-job-selection).

After all selected CI jobs succeed, the `tag` job in CI calls Sweet's `tag.yml`
for the push's complete before/after range. Tags point to the commits that
introduced each version. Tags are never moved. The job pushes tags with
`GITHUB_TOKEN`, so a tag push starts no workflow. The same CI run publishes the
tagged versions: the `publish-releases` job calls `publish-releases.yml`, which
starts one publication job for each release unit from Sweet's tag outputs.

| Release unit          | Publication                                                 |
| --------------------- | ----------------------------------------------------------- |
| `.` (Chatto)          | `release.yml`, and the TestFlight upload                    |
| `authling`            | `build-authling-image.yml`, and a GitHub release with notes |
| `packages/runling`    | `npm.yml`, and a GitHub release with notes                  |
| `packages/translatto` | `npm.yml`, and a GitHub release with notes                  |
| `pkg/*`               | A GitHub release with notes                                 |

To retry a failed tag push or publication, rerun the failed jobs of the original
CI run. Existing matching tags are kept, so the tag job pushes only the missing
tags and returns the same outputs. A later push does not recover an earlier
failed range. CI never cancels or replaces a push run, because each run owns
its commit range. A manual tag push starts no build. If a newer version of the
same package was released after the failure, rerun all jobs instead: the tag
job then computes `highest` again, so the retry does not move `latest` back.

Each publication job uses the newest version of its package in the push. If one
push has two release commits of the same package, CI publishes only the newer
version and shows a warning for the older tag.

Each Sweet tag entry has `prerelease` and `highest` fields. `prerelease` is true
for a version with a prerelease suffix. `highest` is true when the version is
the highest stable version of its package. The publication jobs use these
fields for release channels. They do not compare tags and version files again:
Sweet made the tag from the committed version files of a checked commit.

The migration commit is a baseline for all nine manifests. It must not tag
existing versions again. Packages at `0.0.0` remain unreleased until a bump.
The Prepare releases workflow uses the `RELEASE_PLEASE_PAT` secret to create
release PRs, so the PRs start CI. Tagging and publication do not use it.
No release-please action or configuration remains.

All Sweet references in the workflows use the same Sweet Release commit. This
commit is not a Sweet release: it adds the tag outputs and
`sweet github release`. Steps that use `sweet github release` build the Sweet
CLI from that commit, because the `mise.toml` version does not have this
command. Move all references to the next Sweet release tag when it is available.

## Prereleases

To release prereleases of a package, set `prerelease: <label>` in its
`.sweet.yml`, for example `prerelease: beta`. Remove the label to release the
final version. Sweet finalizes a prerelease manifest version that has no label
on its next bump. The publication workflows accept prerelease versions.

## GitHub Latest release

Only the highest stable Chatto version claims **Latest**. `sweet github release`
creates each GitHub release as a draft, with the title `<name> <version>` and the
matching Sweet changelog entry. It publishes the draft, marks prerelease versions
as prereleases, and makes only the highest stable version of the root package
Latest. Component releases never claim Latest.

The Chatto job publishes its release after all artifacts are complete. The
`components` job publishes the notes of all other tags in the push. Authling and
npm artifacts have separate jobs; a component GitHub release does not prove that
its artifact publication succeeded. Check both jobs.

A rerun reuses a draft and replaces its assets. A published release stays
unchanged. Do not change published tag targets or rebuild a version with new
content.

## iOS TestFlight

For each Chatto `v*` tag, CI also starts the independent `Publish iOS to
TestFlight` workflow as a manual run on the tag. That workflow keeps its own run
number for the iOS build number. It rebuilds the bundled frontend and uploads a
signed device build. Configure
its `mobile-testflight` environment and Apple credentials as described in the
[mobile release guide](../apps/mobile/README.md#automated-testflight-updates).
A successful server or desktop release does not confirm a TestFlight upload.
The workflow can also run manually from `main`.

## Maintenance branches

Release preparation runs on `main` only. CI also tags and publishes pushes to
`release-*` branches, with the same checks. To release a maintenance version,
make the release commit on the branch with `mise x -- sweet bump <package-path>`
and push it. Create the branch with a separate push first: CI does not tag the
push that creates a branch. `highest` keeps a maintenance version from moving
Latest, `latest` image tags, the npm `latest` tag, Homebrew, and the stable
documentation. The environments `chatto-release`, `desktop-signing`, and
`desktop-windows-signing` must allow the `release-*` branches. Do not use old
commit footers to request releases. A branch that does not contain this release
workflow does not publish its tags.

## Publication approval

The Chatto release requests all environment approvals at the start. In
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
assets to the draft release, pushes the saved container images, and publishes
the GitHub Release. Every release gets its version image tags. A prerelease also
gets `next`. A stable release gets the server's `<major>.<minor>` tag. The
highest stable version also gets `latest`, then updates Homebrew and starts the
stable documentation build. There is no second
approval to make the GitHub Release public. Publication credentials are
repository secrets; do not move them into `chatto-release`, because the
publication job does not enter that environment. Signing secrets and Windows
OIDC configuration remain in their existing environments. Keep the required
reviewers on all three environments. The release jobs run in the CI run of a
branch push, so the environments must allow `main` and the `release-*`
branches. Manual desktop verification builds also use `main`.

The same `approve-release` job also gates the client npm publication. Client
publication waits for the server build and desktop assembly to succeed.

All npm packages publish through `npm.yml` in the `npm` environment. Prereleases
use the npm `next` tag. The highest stable version uses `latest`, and an older
stable version uses `maintenance`. npm trusted publishing checks the workflow
that started the run, not the reusable workflow. Thus the trusted publisher of
`@chatto/client`, `runling`, and `@chatto/translatto` is owner `chattocorp`,
repository `chatto`, workflow `ci.yml`, and environment `npm`. Run `npm.yml`
manually to check and save a package tarball without publishing it.

Publication across GitHub and GHCR is not atomic. If publication fails, some
images or assets can already be available. Rerun the failed jobs on the same
CI run to reuse the saved artifacts; they are retained for seven days. Uploads
replace the assets of a draft, and image pushes can be repeated. A published
GitHub release stays unchanged. A retry of
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

After the highest stable `vX.Y.Z` release completes successfully, the release
workflow builds the docs from that exact tag and publishes
`ghcr.io/chattocorp/chatto-docs:vX.Y.Z`. Flux selects the highest stable SemVer
tag for `docs.chatto.run`. Prerelease tags never update the stable docs site.

Stable docs images are immutable release snapshots. Corrections to the stable
documentation ship with the next Chatto patch release rather than replacing an
existing `vX.Y.Z` image.

## Runling releases

Runling is versioned independently under `packages/runling/`. Its releases
use `runling/vX.Y.Z` tags. The `npm.yml` workflow tests the npm tarball before
publishing it. For the one-time npm trusted
publisher setup and local verification commands, see
[Runling development and releases](../packages/runling/README.md#monorepo-development-and-releases).

## Chatto Desktop releases

Chatto Desktop shares the server and frontend version, release commit, and
`vX.Y.Z` tag. The root Sweet manifest updates both package versions
and `cli/version.go`. Desktop-only changes contribute to the root changelog
and the next Chatto release. The old desktop changelog and
`chatto-desktop/v*` tags are historical records, not a separate release stream.

Merging a Chatto release PR starts CI, which creates the shared tag after
checks pass. The same CI run starts the release workflow. It checks and builds
macOS, Windows, and Linux bundles from the tagged commit, signs and notarises the macOS bundle, signs and verifies
every Windows executable, requires the expected ChattoCorp publisher on the
main application, and uploads archives and SHA-256 checksums to that release.
Windows releases also include a per-user `*_Windows_X64_Setup.exe` installer.
Its installer and embedded uninstaller are signed and verified before upload.
The ZIP remains available. See the desktop README for installation and the
interactive Windows verification checklist; PR builds are unsigned.
Publication waits for server and desktop builds, including signing, to succeed.
If signing fails, retry the failed jobs on the same CI run. A signing
outage blocks publication. Linux and ordinary CI artifacts remain unsigned
experimental builds.
Windows and macOS use separate protected signing environments. Signing-service
provisioning, protected-environment settings, renewal, and emergency revocation
are documented in [`apps/desktop/README.md`](../apps/desktop/README.md).

Release desktop builds use the exact shared version for the shell and frontend.
Manual verification builds retain the checked-in package version for native
packaging, but identify the frontend as `{development-version}+{commit-sha}`.
Matching release versions do not require matching client and server versions;
the existing client-server compatibility rules still apply. See
[ADR-118](adr/ADR-118-synchronized-chatto-desktop-releases.md).

Before a release, the release workflow can verify the complete desktop
packaging path without creating a release or building a Chatto server image.
Run the `release` workflow manually, select the `desktop` target, and optionally
provide a branch, tag, or commit reachable from `origin/main` or an
`origin/release-*` branch in the `ref`
input. Signed desktop builds reject other commits before running repository
code or requesting signing credentials. The workflow builds and packages all
three platforms, generates the same checksum file used by a release, and
uploads the assembled files as a seven-day verification artifact.

Release commits are on `main` or a `release-*` branch, so the same check
passes for them. Do not rerun old tags to add synchronized desktop assets.
