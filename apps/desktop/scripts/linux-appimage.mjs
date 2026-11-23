/** Wrap the existing Linux bundle in an update-capable AppImage. */
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build, Platform, Arch } from 'electron-builder';
import packageJson from '../package.json' with { type: 'json' };

if (process.platform !== 'linux' || !['x64', 'arm64'].includes(process.arch)) {
  throw new Error('Build the Linux AppImage on Linux x64 or ARM64.');
}
const root = fileURLToPath(new URL('../', import.meta.url));
await build({
  projectDir: root,
  prepackaged: path.join(root, 'dist/chatto-desktop'),
  targets: Platform.LINUX.createTarget('AppImage', Arch[process.arch]),
  publish: 'never',
  config: {
    appId: 'run.chatto.desktop',
    productName: 'Chatto',
    executableName: 'chatto-desktop',
    electronVersion: packageJson.devDependencies.electron,
    directories: { output: 'dist/appimage' },
    publish: null,
    toolsets: { appimage: '1.0.3' },
    linux: {
      category: 'Network',
      icon: path.join(root, '../frontend/static/icons/icon-512.png'),
      // Keep the launcher filename, StartupWMClass, and Electron's app_id equal.
      syncDesktopName: true
    },
    appImage: {
      artifactName: `chatto-desktop_${packageJson.version}_Linux_${process.arch.toUpperCase()}.AppImage`,
      // Set the window class and keep the Chromium sandbox enabled.
      executableArgs: ['--class=chatto-desktop']
    }
  }
});
