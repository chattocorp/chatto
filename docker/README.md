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

The entrypoint writes `/var/run/chatto-frontend/oauth/frontend-client-metadata.json`
before it starts Nginx, and Nginx serves it at
`/oauth/frontend-client-metadata.json`. The file identifies the frontend and
registers its `/servers/callback?mode=popup` callback. It contains no secrets.
Remote Chatto servers must be able to fetch this public HTTPS URL.

An invalid value stops startup. An unset or empty value removes the generated
metadata file and leaves static serving enabled; the metadata URL returns 404.
Use a different origin for each deployment. No image rebuild is needed.

## Standalone frontend link previews

The entrypoint also writes the page title, description, OpenGraph tags, and
Twitter card tags into `200.html` before it starts Nginx. Link previews then
show the frontend in chat apps and social networks. The defaults describe
Chatto. Set these variables to change them:

| Variable                      | Default                     | Use                                                      |
| ----------------------------- | --------------------------- | -------------------------------------------------------- |
| `CHATTO_FRONTEND_TITLE`       | `Chatto`                    | Page title and `og:title`                                |
| `CHATTO_FRONTEND_DESCRIPTION` | A short Chatto description  | Page description and `og:description`                    |
| `CHATTO_FRONTEND_IMAGE`       | `/icons/social-preview.png` | `og:image`: an HTTPS URL, or a path that starts with `/` |

Crawlers require an absolute image URL. A path is resolved against
`CHATTO_FRONTEND_ORIGIN`. Without an origin, the page has no `og:url` and only
an HTTPS URL gives an image. Startup HTML-escapes all values. An invalid image
value stops startup.

The image keeps the unmodified document in `/usr/share/nginx/200.html.template`.
Each start renders `/var/run/chatto-frontend/200.html` from it again, so a
restart with different values replaces the earlier tags.

## Standalone frontend on a read-only root file system

The entrypoint writes only to `/var/run/chatto-frontend`. The web root stays
unchanged. To run the container with a read-only root file system, mount
writable temporary file systems at the paths that Nginx itself needs:

```sh
docker run --read-only --tmpfs /var/cache/nginx --tmpfs /var/run ghcr.io/chattocorp/chatto-client
```

Run `sh docker/frontend-entrypoint_test.sh` to check metadata generation,
link-preview rendering, input validation, restart cleanup, and command
forwarding. Run `sh docker/frontend-image_test.sh <image>` to check a built
image on a read-only root file system. CI runs it on the amd64 image before
it publishes a main-branch or release image.
