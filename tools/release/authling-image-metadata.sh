#!/usr/bin/env bash
# Resolve the Authling image metadata of the checked-out source and write it
# to GITHUB_OUTPUT. Run it in the root of the source checkout.
#
# Environment (all values are untrusted; this script validates them):
# - RELEASE_TAG: an Authling release tag, such as authling/v0.3.0, or empty.
# - HIGHEST: `true` when the release is the highest stable Authling version.
# - GITHUB_EVENT_NAME and GITHUB_REF: the event and ref of the calling run.
#
# Outputs:
# - sha and version: the commit and the version in authling/version.go.
# - tag: the image tag. A release uses its version, other builds the commit.
# - stream_tag: `latest` for the highest stable release, `dev` for a push to
#   main, otherwise empty.
# - publish: `true` for a release and for a push to main.
# - published: `true` when the release image is in the registry already.
set -euo pipefail

: "${GITHUB_OUTPUT:?}"
release_tag="${RELEASE_TAG:-}"
image="ghcr.io/chattocorp/authling"

sha="$(git rev-parse HEAD)"
version="$(sed -n 's/^const Version = "\([^"]*\)".*/\1/p' authling/version.go)"
if [[ -z "$version" ]]; then
  echo "::error::Could not read the Authling version from authling/version.go."
  exit 1
fi

tag="$sha"
stream_tag=""
publish=false
published=false
if [[ -n "$release_tag" ]]; then
  prefix="authling/v"
  if [[ "$release_tag" != "$prefix"* ]]; then
    echo "::error::Only authling/v release tags are supported."
    exit 1
  fi
  tag="${release_tag#"$prefix"}"
  # SemVer without build metadata: Docker tags cannot contain `+`.
  number='(0|[1-9][0-9]*)'
  identifier='(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
  semver="^${number}\.${number}\.${number}(-${identifier}(\.${identifier})*)?$"
  if [[ ! "$tag" =~ $semver ]] || (( ${#tag} > 128 )); then
    echo "::error::The release version is not a supported SemVer image tag."
    exit 1
  fi
  manifest="$(sed -n 's/^version: \([^[:space:]]*\)[[:space:]]*$/\1/p' authling/.sweet.yml)"
  if ! [[ "$tag" == "$version" && "$tag" == "$manifest" ]]; then
    echo "::error::The release tag, authling/version.go, and authling/.sweet.yml must agree."
    exit 1
  fi
  if [[ "${HIGHEST:-}" == true ]]; then
    stream_tag=latest
  fi
  publish=true
  if docker buildx imagetools inspect "${image}:${tag}" > /dev/null 2>&1; then
    published=true
  fi
elif [[ "${GITHUB_EVENT_NAME:-}" == push && "${GITHUB_REF:-}" == refs/heads/main ]]; then
  stream_tag=dev
  publish=true
fi

{
  echo "sha=${sha}"
  echo "version=${version}"
  echo "tag=${tag}"
  echo "stream_tag=${stream_tag}"
  echo "publish=${publish}"
  echo "published=${published}"
} >> "$GITHUB_OUTPUT"
