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
set -euo pipefail

: "${VERSION:?}" "${PRERELEASE:?}" "${HIGHEST:?}"

for repository in chatto-client chatto; do
  image="ghcr.io/chattocorp/${repository}"
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
