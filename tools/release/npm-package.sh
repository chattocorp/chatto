#!/usr/bin/env bash
# Check, test, and pack one npm release unit, and save the tested tarball in
# NPM_RELEASE_DIR. The publish step (npm-publish.sh) handles a version that is
# already on npm, so a release can run again.
#
# Usage: npm-package.sh <package-path>
set -euo pipefail

package_path="${1:?Usage: npm-package.sh <package-path>}"
release_dir="${NPM_RELEASE_DIR:?}"

case "$package_path" in
  packages/runling)
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
    mise x -- pnpm --filter "$name" check:release --verify-only
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
