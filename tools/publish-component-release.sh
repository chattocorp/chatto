#!/usr/bin/env bash
# Publish component notes independently of package/image artifact workflows.
set -euo pipefail

mise x -- sweet status --json > "$RUNNER_TEMP/packages.json"
package="$(jq -er --arg tag "$GITHUB_REF_NAME" '
  [.[] | select(.path != "." and (.tagPrefix + .version) == $tag)] |
  if length == 1 then .[0] else error("Tag must match exactly one component") end
' "$RUNNER_TEMP/packages.json")"
path="$(jq -r '.path' <<< "$package")"
version="$(jq -r '.version' <<< "$package")"
name="$(jq -r '.name // .path' <<< "$package")"
mise x -- sweet changelog --version "$version" -- "$path" > "$RUNNER_TEMP/component-notes.md"
flags=()
if [[ "$version" == *-* ]]; then flags+=(--prerelease); fi
if ! gh release view "$GITHUB_REF_NAME" >/dev/null 2>&1; then
  gh release create "$GITHUB_REF_NAME" --verify-tag --draft --latest=false \
    --title "$name $version" --notes-file "$RUNNER_TEMP/component-notes.md" "${flags[@]}"
fi
gh release edit "$GITHUB_REF_NAME" --draft=false --latest=false
