#!/usr/bin/env bash
# Write the commit and version of a manual image verification build to GITHUB_OUTPUT.
set -euo pipefail

sha="$(git rev-parse HEAD)"
development_version="$(mise env --json | jq -r '.CHATTO_DEVELOPMENT_VERSION // empty')"
if [[ -n "$development_version" ]]; then
  version="${development_version}+${sha:0:12}"
else
  # Historical refs predate CHATTO_DEVELOPMENT_VERSION. Preserve the
  # arbitrary-ref build contract by using their checked-in release
  # version instead.
  release_version="$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' cli/version.go)"
  if [[ -z "$release_version" ]]; then
    echo "::error::Could not determine the Chatto version."
    exit 1
  fi
  version="${release_version}+${sha:0:12}"
fi

echo "sha=${sha}" >> "$GITHUB_OUTPUT"
echo "version=${version}" >> "$GITHUB_OUTPUT"

{
  echo "### Image"
  echo
  echo "- Commit: \`${sha}\`"
  echo "- Version: \`${version}\`"
  echo "- Image: \`ghcr.io/chattocorp/chatto:${sha}\`"
  echo "- Platform: \`linux/amd64\`"
} >> "$GITHUB_STEP_SUMMARY"
