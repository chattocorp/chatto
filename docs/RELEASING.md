# Releasing Chatto

Chatto uses release-please to prepare stable releases from `main` and beta
prereleases from `next`. Both branches use `.release-please-config.json` and
`.release-please-manifest.json`; the configuration on each branch determines
which kind of release it produces. Stable releases publish the `latest`
container tags, while prereleases publish the `next` container tags.

## Start a prerelease cycle

Reset `next` to the latest stable `main`, then add these properties to
`.release-please-config.json`:

```json
"versioning": "prerelease",
"prerelease": true,
"prerelease-type": "beta.1"
```

Commit that configuration change with an explicit `Release-As` footer:

```sh
git fetch origin
git switch next
git reset --hard origin/main
git add .release-please-config.json
git commit \
  -m "chore(release): begin 0.6 prereleases" \
  -m "Release-As: 0.6.0-beta.1"
git push --force-with-lease origin next
```

Replace `0.6` with the version being developed. Release-please will prepare the
first prerelease as `0.6.0-beta.1`; later prerelease PRs increment it to
`beta.2`, `beta.3`, and so on.

## Promote a prerelease to stable

Create a promotion branch from `next`. Remove `versioning`, `prerelease`, and
`prerelease-type` from `.release-please-config.json`, then commit that change
with the intended stable version:

```sh
git switch -c promote-0.6 origin/next
git add .release-please-config.json
git commit \
  -m "chore(release): promote 0.6 to stable" \
  -m "Release-As: 0.6.0"
git push -u origin promote-0.6
```

Open this branch as a pull request into `main`, and include `Release-As: 0.6.0`
in the pull request body so a squash merge preserves the footer. Keeping the
stable configuration change on the promotion branch prevents `next` from
switching out of prerelease mode before the promotion is merged. After the
promotion reaches `main`, release-please prepares the stable release PR for
`0.6.0`.
