# Media E2E image

This image supplies Chromium, its system libraries, and FFmpeg to the media
E2E job. The job checks out and builds the application source as before.
The image uses the official Playwright Ubuntu Noble image. Its version comes
from the installed frontend dependency, which the pnpm lockfile selects.

The `build media E2E image` workflow builds and checks the image on pull
requests. On main, it publishes the checked image to
`ghcr.io/chattocorp/media-e2e`. A weekly build refreshes the base image and
OS packages. Manual builds also refresh these packages. Each publication
has a unique tag. Builds do not change the image used by CI automatically.

## First publication and updates

1. Merge the image workflow and let its main-branch build finish.
2. Make the GHCR package public so fork pull requests can pull it without
   credentials. The image contains public tools, not application source.
3. Review the build and its codec and Chromium checks. Copy the full
   `ghcr.io/chattocorp/media-e2e@sha256:...` reference from the job summary.
4. Set the repository Actions variable `MEDIA_E2E_IMAGE` to that reference.
5. Run CI and check the media job. Restore the previous digest to roll back.

Until the variable is set, the job uses Playwright `1.63.0` from the official
image and installs FFmpeg if it is absent. This permits the first merge
before the custom image exists. Clear the variable only to return to this
bootstrap configuration.

When the locked Playwright version changes, build and select a matching image.
The media job rejects a custom image with a different Playwright version;
it does not download replacement browsers. When using the bootstrap image,
update its version in `ci.yml` with the dependency.

The image build checks H.264 encoding, FFprobe, and a headless Chromium
launch. The normal media job remains responsible for full application E2E
coverage. Image publication alone does not establish that this coverage passes.
