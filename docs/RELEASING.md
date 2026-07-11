# Releasing Chatto

Chatto uses release-please to prepare stable releases from `main` and beta
prereleases from `next`. Stable releases publish the `latest` container tags;
prereleases publish the `next` container tags.

## Start a prerelease cycle

Release-please remembers the version from the last release PR merged into a
target branch. Resetting `next` to `main` therefore does not, by itself, start a
new prerelease version series. Start each cycle with an explicit `Release-As`
commit:

```sh
git fetch origin
git switch next
git reset --hard origin/main
git commit --allow-empty \
  -m "chore(release): begin 0.6 prereleases" \
  -m "Release-As: 0.6.0-beta.1"
git push --force-with-lease origin next
```

Replace `0.6` with the version being developed. The first release PR will use
the requested `beta.1` version; later prerelease PRs increment it to `beta.2`,
`beta.3`, and so on.

Do not omit the `Release-As` commit after resetting `next`. Otherwise,
release-please can recover the previous prerelease manifest and continue the old
version series.
