#!/usr/bin/env bash
# Push the loaded release images and their multi-architecture tags.
#
# Environment: VERSION, and PRERELEASE and HIGHEST from Sweet's tag entry.
# - Every release gets its version tag.
# - A prerelease also gets `next`.
# - A stable release gets the server's `<major>.<minor>` tag, and `latest`
#   only when it is the highest stable version. A maintenance release of an
#   older line thus does not move `latest` back.
# The frontend image goes first, so the server stream never advances before
# its frontend is available.
#
# A repository that already has the version tag is left unchanged: it was
# published by an earlier run, with its other tags in the same step. So a run
# that publishes a tag again does not replace published images with a rebuild.
set -euo pipefail

: "${VERSION:?}" "${PRERELEASE:?}" "${HIGHEST:?}"

for repository in chatto-client chatto; do
  image="ghcr.io/chattocorp/${repository}"
  if docker buildx imagetools inspect "${image}:${VERSION}" > /dev/null 2>&1; then
    echo "::notice::${image}:${VERSION} is already published; it stays unchanged."
    continue
  fi
  docker push "${image}:${VERSION}-amd64"
  docker push "${image}:${VERSION}-arm64"
  tags=(--tag "${image}:${VERSION}")
  if [[ "$PRERELEASE" == true ]]; then
    tags+=(--tag "${image}:next")
  else
    if [[ "$repository" == chatto ]]; then
      tags+=(--tag "${image}:${VERSION%.*}")
    fi
    if [[ "$HIGHEST" == true ]]; then
      tags+=(--tag "${image}:latest")
    fi
  fi
  docker buildx imagetools create "${tags[@]}" \
    "${image}:${VERSION}-amd64" "${image}:${VERSION}-arm64"
done
