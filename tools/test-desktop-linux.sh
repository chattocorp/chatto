#!/usr/bin/env bash
# Check the icon and window identity in the Linux bundle and AppImage.
set -euo pipefail

repository_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repository_root"
bundle="$repository_root/apps/desktop/dist/chatto-desktop"
icon="$repository_root/apps/frontend/static/icons/icon-512.png"
appimages=("$repository_root"/apps/desktop/dist/appimage/*.AppImage)
[[ ${#appimages[@]} -eq 1 && -f ${appimages[0]} ]]

mkdir -p .context
scratch="$(mktemp -d "$repository_root/.context/linux-desktop.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT

# --appimage-extract does not need FUSE or a graphical session.
cd "$scratch"
"${appimages[0]}" --appimage-extract > /dev/null
entry="$scratch/squashfs-root/chatto-desktop.desktop"
test -f "$entry"
grep -Fx 'Name=Chatto' "$entry"
grep -Fx 'StartupWMClass=chatto-desktop' "$entry"
grep -Fx 'Icon=chatto-desktop' "$entry"
grep -Fx 'Exec=AppRun --class=chatto-desktop %U' "$entry"
cmp "$icon" "$scratch/squashfs-root/chatto-desktop.png"
cmp "$icon" "$scratch/squashfs-root/.DirIcon"
cmp "$icon" "$bundle/resources/build/icons/icon-512.png"

# Read the shipped metadata, rather than only checking the source package.json.
mise x -- pnpm --dir "$repository_root/apps/desktop" exec node --input-type=module \
  - "$bundle" "$scratch/squashfs-root" <<'JS'
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import path from 'node:path';

const require = createRequire(path.join(process.cwd(), 'package.json'));
const packagerRequire = createRequire(require.resolve('@electron/packager'));
const { extractFile } = packagerRequire('@electron/asar');
for (const directory of process.argv.slice(2)) {
  const metadata = JSON.parse(extractFile(path.join(directory, 'resources/app.asar'), 'package.json'));
  assert.equal(metadata.desktopName, 'chatto-desktop.desktop');
}
JS
echo 'Linux launcher identity and Chatto icons verified.'
