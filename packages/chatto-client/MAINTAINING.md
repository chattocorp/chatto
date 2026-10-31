# Maintain the Chatto client

Workspace consumers import TypeScript source. npm consumers import compiled
ES modules and declarations. The release build includes `@chatto/api-types`
inside the package, because that workspace package is private. Both the
client and its generated protocol code use Apache-2.0.

Run these commands from the repository root:

```sh
mise test-chatto-client
mise x -- pnpm --filter @chatto/client test:package
mise x -- pnpm --filter @chatto/client check:release
```

The package test installs a tarball outside the workspace. It checks Node
imports without Svelte, all client modules with Svelte, and TypeScript
declarations. Set `CHATTO_CLIENT_RELEASE_DIR` to save the verified tarball.
`check:release` rejects versions already in npm history. Set `RELEASE_TAG`
to also check that the tag matches the package version.

Release-please gives the client, Chatto server, frontend, and desktop the same
version and release PR. Client changes contribute to the root changelog.
Tags use `v<VERSION>`. After the `chatto-release` approval job succeeds,
`release.yml` calls the reusable `publish-chatto-client.yml` workflow. It
checks and saves a tarball, then publishes that exact artifact. Manual workflow
runs verify and save an artifact without publishing it. Prereleases use the npm
`next` tag; stable releases use `latest`. npm publication runs independently of
server and desktop publication, so completion times can differ.

The former `0.1.x` versions, `chatto-client/v*` tags, and package changelog remain
historical records. The next client publication uses the next shared Chatto
version. Matching client and server versions are not required to connect;
the existing compatibility rules still apply.

Before automated publication, configure an npm trusted publisher for
`@chatto/client`: owner `chattocorp`, repository `chatto`, workflow
`release.yml`, environment `npm`. Allow `npm publish`. Replace the previous
`publish-chatto-client.yml` trusted publisher before the next release: npm
checks the calling workflow identity for reusable workflows.
See [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/).
Configure it when the release is ready, because a new configuration must
complete its first successful publication within two days.

If the package must be created before its trusted publisher can be configured,
use an npm account with write access to the `@chatto` scope to publish the
verified first-release tarball:

```sh
npm publish <verified-tarball.tgz> --access public --ignore-scripts --registry=https://registry.npmjs.org
```

Do not publish the same version again. If the first version is published
manually, align the release-please manifest and release tag before the next
release. Use pnpm to build and pack from source; `npm publish` from the source
directory does not apply pnpm's publication-directory settings.
