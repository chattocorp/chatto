# Releasing Chatto

Chatto uses release-please to prepare beta releases from `main`. Stable releases
and maintenance patches come from `release-x.y` branches. Each branch uses the
same `.release-please-config.json` and `.release-please-manifest.json` paths; the
configuration committed to that branch determines whether it produces
prereleases or stable releases.

## Prereleases from main

The release-please configuration on `main` uses prerelease versioning. Feature
work merges into `main`, and release-please prepares versions such as
`0.5.0-beta.1`, `0.5.0-beta.2`, and so on. Prereleases publish the `next`
container tags.

When development moves to a new version series, force its first version with a
`Release-As` footer. For example:

```sh
git switch -c begin-0.6 origin/main
git commit --allow-empty \
  -m "chore(release): begin 0.6 prereleases" \
  -m "Release-As: 0.6.0-beta.1"
git push -u origin begin-0.6
```

Merge this branch into `main`, preserving the `Release-As` footer in the squash
commit or pull request body.

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
