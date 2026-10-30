#!/usr/bin/env bash
# Mount the disk image and verify a copied app without installing or launching it.
set -euo pipefail

image="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
mkdir -p .context
scratch="$(mktemp -d "$PWD/.context/macos-dmg-test.XXXXXX")"
mountpoint="$scratch/mounted"
cleanup() {
  if mountpoint_output="$(hdiutil detach "$mountpoint" 2>&1)"; then
    rm -rf "$scratch"
  else
    echo "$mountpoint_output" >&2
    echo "Could not detach $mountpoint; retained $scratch" >&2
    return 1
  fi
}
hdiutil verify "$image"
hdiutil attach "$image" -readonly -nobrowse -mountpoint "$mountpoint"
trap cleanup EXIT
test "$(readlink "$mountpoint/Applications")" = /Applications
ditto "$mountpoint/Chatto.app" "$scratch/Chatto.app"
codesign --verify --deep --strict --verbose=2 "$scratch/Chatto.app"
test "$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$scratch/Chatto.app/Contents/Info.plist")" = run.chatto.desktop
test -x "$scratch/Chatto.app/Contents/Helpers/Chatto Capture Helper.app/Contents/MacOS/chatto-macos-capture-probe"
