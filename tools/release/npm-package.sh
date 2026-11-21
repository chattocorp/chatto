#!/usr/bin/env bash
# Check, test, and pack one npm release unit, and save the tested tarball in
# NPM_RELEASE_DIR. With NPM_PUBLISH=true, also reject a version that npm has
# already published, including a version that was later removed.
#
# Usage: npm-package.sh <package-path>
set -euo pipefail

package_path="${1:?Usage: npm-package.sh <package-path>}"
release_dir="${NPM_RELEASE_DIR:?}"
publish="${NPM_PUBLISH:-false}"

case "$package_path" in
  packages/runling)
    if [[ "$publish" == true ]]; then
      mise x -- node packages/runling/scripts/check-release.mjs
    fi
    mise check-runling
    mise test-runling
    RUNLING_RELEASE_DIR="$release_dir" mise test-runling-package
    ;;
  packages/translatto | packages/chatto-client)
    if [[ "$package_path" == packages/translatto ]]; then
      name=@chatto/translatto
      export TRANSLATTO_RELEASE_DIR="$release_dir"
    else
      name=@chatto/client
      export CHATTO_CLIENT_RELEASE_DIR="$release_dir"
    fi
    if [[ "$publish" == true ]]; then
      mise x -- pnpm --filter "$name" check:release
    else
      mise x -- pnpm --filter "$name" check:release --verify-only
    fi
    mise x -- pnpm --filter "$name" check
    mise x -- pnpm --filter "$name" test
    mise x -- pnpm --filter "$name" test:package
    ;;
  *)
    echo "::error::${package_path} is not an npm release unit."
    exit 1
    ;;
esac

shopt -s nullglob
packages=("$release_dir"/*.tgz)
if [[ "${#packages[@]}" -ne 1 ]]; then
  echo "::error::Expected one tested tarball in ${release_dir}."
  exit 1
fi
