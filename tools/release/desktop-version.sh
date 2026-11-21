#!/usr/bin/env bash
# Write the desktop and bundled frontend versions to GITHUB_OUTPUT.
#
# A release (RELEASE_TAG set) bundles the frontend with the package version.
# A verification build identifies its source with the development version and
# the commit, so it never looks like the last release.
set -euo pipefail

version="$(jq -r '.version' apps/desktop/package.json)"
frontend_package_version="$(jq -r '.version' apps/frontend/package.json)"
sha="$(git rev-parse HEAD)"

if [[ -z "$version" || "$version" == "null" ]]; then
  echo "::error::Could not determine the Chatto Desktop version."
  exit 1
fi
if [[ -z "$sha" ]]; then
  echo "::error::Could not determine the checked-out commit."
  exit 1
fi
if [[ "$version" != "$frontend_package_version" ]]; then
  echo "::error::Desktop and frontend package versions must match."
  exit 1
fi

if [[ -n "${RELEASE_TAG:-}" ]]; then
  frontend_version="$version"
else
  development_version="$(mise env --json | jq -r '.CHATTO_DEVELOPMENT_VERSION // empty')"
  if [[ -z "$development_version" ]]; then
    echo "::error::CHATTO_DEVELOPMENT_VERSION is required for verification builds."
    exit 1
  fi
  frontend_version="${development_version}+${sha:0:12}"
fi

{
  echo "version=${version}"
  echo "sha=${sha}"
  echo "frontend_version=${frontend_version}"
} >> "$GITHUB_OUTPUT"

{
  echo "### Chatto Desktop"
  echo
  echo "- Commit: \`${sha}\`"
  echo "- Desktop version: \`${version}\`"
  echo "- Bundled frontend version: \`${frontend_version}\`"
} >> "$GITHUB_STEP_SUMMARY"
