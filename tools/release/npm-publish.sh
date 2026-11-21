#!/usr/bin/env bash
# Publish one tested npm tarball with trusted publishing. Safe to run again:
# - The version is not on npm: publish it.
# - The same tarball is already on npm (equal integrity): do nothing.
# - Other content is already on npm with this version: fail. npm versions
#   never change; release a new version instead.
#
# Usage: npm-publish.sh <tarball> <dist-tag>
set -euo pipefail

tarball="${1:?Usage: npm-publish.sh <tarball> <dist-tag>}"
dist_tag="${2:?Usage: npm-publish.sh <tarball> <dist-tag>}"
registry=https://registry.npmjs.org

manifest="$(tar -xOzf "$tarball" package/package.json)"
name="$(jq -er .name <<< "$manifest")"
version="$(jq -er .version <<< "$manifest")"
integrity="sha512-$(openssl dgst -sha512 -binary "$tarball" | openssl base64 -A)"

# `npm view` prints nothing for a missing version and fails for a missing package.
published="$(npm view "${name}@${version}" dist.integrity --registry="$registry" 2> /dev/null || true)"
if [[ -n "$published" ]]; then
  if [[ "$published" == "$integrity" ]]; then
    echo "::notice::${name}@${version} is already on npm with the same content; nothing to publish."
    exit 0
  fi
  echo "::error::${name}@${version} is already on npm with other content. npm versions never change; release a new version."
  exit 1
fi

npm publish "$tarball" --tag "$dist_tag" --access public --provenance --ignore-scripts --registry="$registry"
