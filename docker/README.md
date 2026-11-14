# Docker Assets

This directory contains Docker build assets used by CI and release automation.

## Files

- `Dockerfile.goreleaser` builds the backend release image that GoReleaser
  publishes as `ghcr.io/chattocorp/chatto`. The release image uses
  `/config/chatto.toml` as its default config path and `/data` as the embedded
  NATS data directory. It defaults the runtime user to `1000:1000` and supports
  `PUID`/`PGID` environment variables for matching host volume ownership.
- `docker-entrypoint.sh` is copied into the backend release image. It applies
  the runtime user/group and drops privileges before starting the `chatto`
  binary. It does not recursively change ownership of mounted operator
  directories.
- `nats-wrapper.sh` makes the bundled NATS CLI use Chatto's runtime NATS
  environment without writing a CLI context. Explicit `NATS_URL` or
  `NATS_CONTEXT` settings leave connection configuration under operator control.
- `nats-wrapper_test.sh` verifies the wrapper's derived connection settings and
  explicit operator overrides.
- `Dockerfile.frontend.prebuilt` packages the already-built frontend static
  files into the release-only `ghcr.io/chattocorp/chatto-client` image.
- `*.dockerignore` files are scoped to individual root-context Dockerfiles.
  Keep them next to the Dockerfile they apply to instead of recreating a broad
  root `.dockerignore`.

Copyable deployment examples still live under `examples/`, for example
`examples/dockercompose/`.

## Standalone frontend OAuth identity

Set `CHATTO_FRONTEND_ORIGIN=https://app.chatto.run` on the `chatto-client`
container to enable remote Chatto sign-in. Use the public HTTPS origin, with
an optional port. DNS hostnames are supported; IPv6 literals are not.
Do not include credentials, a path, a query, or a fragment. A trailing slash
is accepted. Startup converts the hostname to lowercase and removes port 443.

The entrypoint writes `/usr/share/nginx/html/oauth/frontend-client-metadata.json`
before it starts Nginx. The file identifies the frontend and registers its
`/servers/callback?mode=popup` callback. It contains no secrets. Remote Chatto
servers must be able to fetch this public HTTPS URL. The container needs write
access to the `oauth` directory at startup.

An invalid value stops startup. An unset or empty value removes the generated
metadata file and leaves static serving enabled; the metadata URL returns 404.
Use a different origin for each deployment. No image rebuild is needed.

Run `sh docker/frontend-entrypoint_test.sh` to check metadata generation,
input validation, restart cleanup, and command forwarding.
