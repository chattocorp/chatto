# Releasing Chatto

Chatto uses release-please to prepare alpha releases from `main`. Stable releases
and maintenance patches come from `release-x.y` branches. Each branch uses the
same `.release-please-config.json` and `.release-please-manifest.json` paths; the
configuration committed to that branch determines whether it produces
prereleases or stable releases.

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

## Chatto Desktop releases

Chatto Desktop is an independently versioned component under `apps/desktop`.
Its release-please package owns `apps/desktop/CHANGELOG.md` and
`apps/desktop/package.json`, and its tags use
`chatto-desktop/vX.Y.Z`. Desktop-only changes are excluded from Chatto's root
server release component.

Merging a Chatto Desktop release PR creates a draft GitHub release and the
component tag. The release workflow checks and builds macOS, Windows, and Linux
bundles from that tag, uploads archives and SHA-256 checksums, then publishes
the release. These artifacts are currently unsigned experimental builds.
Platform signing and notarisation must be added before presenting them as
trusted general-user downloads.

The desktop shell version and the bundled Chatto frontend version answer
different questions. The desktop version identifies packaging and runtime
changes; client-server compatibility continues to use the official frontend
version embedded by the tagged commit.

Before publishing a tag, the release workflow can verify the complete desktop
packaging path without creating a release or building a Chatto server image.
Run the `release` workflow manually, select the `desktop` target, and optionally
provide a branch, tag, or commit in the `ref` input. The workflow builds and
packages all three platforms, generates the same checksum file used by a tagged
release, and uploads the assembled files as a one-day verification artifact.

## Create a stable release branch

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
