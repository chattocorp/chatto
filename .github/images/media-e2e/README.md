# Media E2E image

This image supplies Chromium, its system libraries, and FFmpeg to the media
E2E job. The job checks out and builds the application source as before.
The image uses the official Playwright Ubuntu Noble image. Its version comes
from the installed frontend dependency, which the pnpm lockfile selects.

The `build media E2E image` workflow builds and checks the image on pull
requests. On main, or on a same-repository pull request with the
`ci:publish-media-image` label, it publishes the checked image to
`ghcr.io/chattocorp/media-e2e`. A weekly build refreshes the base image and
OS packages. Manual builds also refresh these packages. Each publication
has a unique tag. Builds do not change the image used by CI automatically.

## Publication and updates

1. Add `ci:publish-media-image` to a same-repository pull request to publish
   and verify an image before merge, or use the main-branch build. Fork pull
   requests cannot publish images. Remove the label after publication to
   stop subsequent commits from publishing more images.
2. Make the GHCR package public so fork pull requests can pull it without
   credentials. The image contains public tools, not application source.
3. Review the build and its codec and Chromium checks. Copy the full
   `ghcr.io/chattocorp/media-e2e@sha256:...` reference from the job summary.
4. To verify before merge, update the default digest in the pull request's
   media job. This does not change other branches. The repository Actions
   variable `MEDIA_E2E_IMAGE`, when set, overrides this default; clear it
   before verifying a new default, or set it to the same digest.
5. Run CI and check the media job. Restore the previous digest to roll back.

The job uses a published custom image pinned by digest in `ci.yml`. It does
not install FFmpeg or download browsers. It checks that the image contains
FFmpeg, FFprobe, and the same Playwright version as the frontend dependency.
The package must be public so all jobs, including fork pull requests, can
pull it without credentials.

When the locked Playwright version changes, build and select a matching image.
The media job rejects a custom image with a different Playwright version;
it does not download replacement browsers. Update the default digest with
the dependency so the change can pass CI before merge.

The image build checks H.264 encoding, FFprobe, and a headless Chromium
launch. The normal media job remains responsible for full application E2E
coverage. Image publication alone does not establish that this coverage passes.
