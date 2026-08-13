# Docker Assets

This directory contains Docker build assets used by development, CI, and release
automation.

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
- `Dockerfile.dev` is the Go, Node.js, and file-watching toolchain image used by
  the root Compose development stack. Compose bind-mounts the checkout instead
  of baking project source into this image.
- `compose-dev-entrypoint.sh` installs container-native workspace dependencies
  and starts Chatto, Authling, or Storybook with polling live reloads.
- `Dockerfile.frontend.dev` is the frontend development image used by
  containerized local or cluster development.
- `*.dockerignore` files are scoped to individual root-context Dockerfiles.
  Keep them next to the Dockerfile they apply to instead of recreating a broad
  root `.dockerignore`.

Copyable deployment examples still live under `examples/`, for example
`examples/dockercompose/`.
