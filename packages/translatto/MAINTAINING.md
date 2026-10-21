# Maintain Translatto

Workspace imports use TypeScript source. Run these package scripts from the
repository root after `mise setup`:

```sh
mise x -- pnpm --filter @chatto/translatto check
mise x -- pnpm --filter @chatto/translatto test
mise x -- pnpm --filter @chatto/translatto test:package
```

`check` checks types. `test` runs unit tests. `test:package` builds and packs
the package, installs it outside the workspace, and checks its JavaScript
exports and type declarations. `prepack` runs `build` automatically.

Run `mise x -- pnpm --filter @chatto/translatto check:release` to check the
package identity and npm version history. Set `RELEASE_TAG` to also check
that a release tag matches the package version.

To build and publish Translatto, run these commands from the repository root
with pnpm available through mise:

```sh
cd packages/translatto
pnpm publish
```

`pnpm publish` runs `prepack`, which runs `build`. It then packs the compiled
JavaScript and type declarations and publishes the package to npm with public
access and the `latest` tag. pnpm applies `publishConfig` to replace the
workspace source exports with compiled exports. An npm login with write
access to the `@chatto` scope is required.

Use `pnpm publish --dry-run` to check this process without uploading a
package. pnpm checks that the Git working tree is clean and the branch is
current with its remote. It permits publication from `main` or `master` by
default. For a release from another branch, use
`pnpm publish --publish-branch <branch>`. For a dry run of uncommitted changes,
use `pnpm publish --dry-run --no-git-checks`.

Use pnpm for publication from the package directory. `npm publish` does not
apply pnpm's export overrides. CI packs with pnpm, tests the tarball, and
publishes that exact artifact with npm.

Translatto has independent release-please releases. A `translatto/v<VERSION>`
tag starts `publish-translatto.yml`. The workflow checks the version, tests
the tarball, and publishes that artifact. A manual workflow run only tests
and saves the artifact. Version `0.1.0` was published manually. Use a new
version for each later release. The current manual workflow also checks npm
version history, so it rejects a version that is already published.

Before the next automated release, configure the npm trusted publisher for repository
`chattocorp/chatto`, workflow `publish-translatto.yml`, and GitHub environment
`npm`. Enable publishing. Configure this when the next release is ready:
a new trust configuration must complete a successful publish within two days.
See [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/).
Do not run the publish job for `0.1.0` again.
Later release tags publish through OIDC without an npm token in GitHub.

After each release, check the npm version and `latest` tag, and install the
published version in a clean project. The registry check rejects a version
that npm has already published, including a version later removed.
