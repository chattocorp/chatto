#!/usr/bin/env bash
# Publish the multi-platform Authling image from the checked per-platform
# images, and set its stream tag.
#
# Environment:
# - TAG and STREAM_TAG: the outputs of authling-image-metadata.sh.
# - RELEASE: `true` for a release image.
# - DIGEST_AMD64 and DIGEST_ARM64: the digests of the checked linux/amd64 and
#   linux/arm64 images that the build jobs pushed without a tag.
#
# A release image that is in the registry already stays unchanged: a run that
# publishes a tag again does not replace it with a rebuild. Its stream tag is
# set again from it, because an earlier run can have failed between the two
# tags. The per-platform digests are then not necessary.
set -euo pipefail

: "${TAG:?}" "${RELEASE:?}" "${GITHUB_STEP_SUMMARY:?}"
stream_tag="${STREAM_TAG:-}"
image="ghcr.io/chattocorp/authling"

if [[ "$RELEASE" == true ]] && docker buildx imagetools inspect "${image}:${TAG}" > /dev/null 2>&1; then
  echo "::notice::${image}:${TAG} is already published; it stays unchanged."
  if [[ -n "$stream_tag" ]]; then
    docker buildx imagetools create --tag "${image}:${stream_tag}" "${image}:${TAG}"
  fi
else
  sources=()
  for digest in "${DIGEST_AMD64:-}" "${DIGEST_ARM64:-}"; do
    if [[ ! "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
      echo "::error::A platform image digest is missing or not valid."
      exit 1
    fi
    sources+=("${image}@${digest}")
  done
  tags=(--tag "${image}:${TAG}")
  if [[ -n "$stream_tag" ]]; then
    tags+=(--tag "${image}:${stream_tag}")
  fi
  docker buildx imagetools create "${tags[@]}" "${sources[@]}"
fi

digest="$(docker buildx imagetools inspect "${image}:${TAG}" --format '{{.Manifest.Digest}}')"
{
  echo "### Authling image"
  echo
  echo "- Image: \`${image}:${TAG}\`"
  echo "- Digest: \`${image}@${digest}\`"
  if [[ -n "$stream_tag" ]]; then
    echo "- Stream tag: \`${image}:${stream_tag}\`; Flux tracks its digest."
  fi
} >> "$GITHUB_STEP_SUMMARY"
