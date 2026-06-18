# Docker Assets

This directory contains Docker build assets used by development, CI, and release
automation.

## Files

- `Dockerfile.goreleaser` builds the backend release image that GoReleaser
  publishes as `ghcr.io/chattocorp/chatto`. The release image uses
  `/config/chatto.toml` as its default config path and `/data` as the embedded
  NATS data directory. It defaults the runtime user to `1000:1000` and supports
  `PUID`/`PGID` environment variables for matching host volume ownership.
- `docker-entrypoint.sh` is copied into the backend release image. It writes a
  NATS CLI context from Chatto's runtime NATS environment, applies the runtime
  user/group, and drops privileges before starting the `chatto` binary. It does
  not recursively change ownership of mounted operator directories.
- `Dockerfile.frontend.prebuilt` packages the already-built frontend static
  files into the release-only `ghcr.io/chattocorp/chatto-client` image.
- `Dockerfile.dev` is the backend development image used by Tilt-oriented local
  or cluster development.
- `Dockerfile.frontend.dev` is the frontend development image used by
  Tilt-oriented local or cluster development.
- `*.dockerignore` files are scoped to individual root-context Dockerfiles.
  Keep them next to the Dockerfile they apply to instead of recreating a broad
  root `.dockerignore`.

Copyable deployment examples still live under `examples/`, for example
`examples/dockercompose/`.
