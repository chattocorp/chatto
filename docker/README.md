# Docker Assets

This directory contains Docker build assets used by development, CI, and release
automation.

## Files

- `Dockerfile.goreleaser` builds the backend release image that GoReleaser
  publishes as `ghcr.io/chattocorp/chatto`.
- `docker-entrypoint.sh` is copied into the backend release image. It writes a
  NATS CLI context from Chatto's runtime NATS environment before starting the
  `chatto` binary.
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
