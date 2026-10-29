/** Wrap the existing app without changing its signature, ticket, or profile. */
import { access, rm } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build, Platform, Arch } from 'electron-builder';
import packageJson from '../package.json' with { type: 'json' };
import { signMacOSDmg } from './macos-dmg-signing.mjs';

if (process.platform !== 'darwin') {
  throw new Error('Build the macOS DMG on macOS.');
}
const desktopRoot = fileURLToPath(new URL('../', import.meta.url));
const bundle = path.join(desktopRoot, 'dist/Chatto Desktop.app');
await access(path.join(bundle, 'Contents/MacOS/Chatto Desktop'));
await rm(path.join(desktopRoot, 'dist/dmg'), { recursive: true, force: true });
const artifactName = `chatto-desktop_${packageJson.version}_macOS_${process.arch.toUpperCase()}.dmg`;
await build({
  projectDir: desktopRoot,
  prepackaged: bundle,
  targets: Platform.MAC.createTarget('dmg', Arch[process.arch]),
  publish: 'never',
  config: {
    appId: 'run.chatto.desktop',
    productName: 'Chatto Desktop',
    electronVersion: packageJson.devDependencies.electron,
    directories: { output: 'dist/dmg', buildResources: 'icons' },
    publish: null,
    mac: { identity: null, notarize: false },
    dmg: {
      artifactName,
      // Release signing is explicit below and must fail if credentials are absent.
      sign: false,
      writeUpdateInfo: false,
      title: 'Chatto Desktop',
      background: path.join(desktopRoot, 'icons/background.png'),
      window: { width: 540, height: 380 },
      contents: [
        { x: 150, y: 115, type: 'file', path: bundle },
        { x: 390, y: 115, type: 'link', path: '/Applications' }
      ]
    }
  }
});
if (process.argv.includes('--release')) {
  signMacOSDmg(path.join(desktopRoot, 'dist/dmg', artifactName), process.env);
}
