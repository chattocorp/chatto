/** Wrap the existing Linux bundle in an update-capable AppImage. */
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build, Platform, Arch } from 'electron-builder';
import packageJson from '../package.json' with { type: 'json' };

if (process.platform !== 'linux' || process.arch !== 'x64') {
  throw new Error('Build the Linux AppImage on Linux x64.');
}
const root = fileURLToPath(new URL('../', import.meta.url));
await build({
  projectDir: root,
  prepackaged: path.join(root, 'dist/chatto-desktop'),
  targets: Platform.LINUX.createTarget('AppImage', Arch.x64),
  publish: 'never',
  config: {
    appId: 'run.chatto.desktop',
    productName: 'Chatto',
    executableName: 'chatto-desktop',
    electronVersion: packageJson.devDependencies.electron,
    directories: { output: 'dist/appimage' },
    publish: null,
    linux: { category: 'Network', icon: path.join(root, '../frontend/static/icons/icon-512.png') },
    appImage: {
      artifactName: `chatto-desktop_${packageJson.version}_Linux_X64.AppImage`,
      // Replace builder's default --no-sandbox argument; keep Chromium sandboxed.
      executableArgs: ['--class=chatto-desktop']
    }
  }
});
