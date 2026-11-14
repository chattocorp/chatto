#!/usr/bin/env bash
# Check every native binary, including the capture helper's bundled frameworks.
set -euo pipefail

bundle="${1:-apps/desktop/dist/Chatto.app}"
target="${CHATTO_DESKTOP_ARCH:-$(uname -m)}"
case "$target" in
  universal) architectures=(x86_64 arm64) ;;
  x64|x86_64) architectures=(x86_64) ;;
  arm64) architectures=(arm64) ;;
  *) echo "Unsupported macOS architecture: $target" >&2; exit 1 ;;
esac

test -x "$bundle/Contents/MacOS/Chatto"
test -x "$bundle/Contents/Helpers/Chatto Capture Helper.app/Contents/MacOS/chatto-macos-capture-probe"
while IFS= read -r -d '' binary; do
  if file -b "$binary" | grep -q 'Mach-O'; then
    for architecture in "${architectures[@]}"; do
      lipo "$binary" -verify_arch "$architecture"
    done
  fi
done < <(find "$bundle" -type f -print0)
