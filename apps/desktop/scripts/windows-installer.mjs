/** Build the Windows installer after packaging (and, for releases, signing) the app. */
import { execFileSync } from 'node:child_process';
import { access, rm } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build, Platform, Arch } from 'electron-builder';
import packageJson from '../package.json' with { type: 'json' };
import { windowsInstallerConfig } from './windows-installer-config.mjs';

if (process.platform !== 'win32' || process.arch !== 'x64') {
  throw new Error('Build the Windows x64 installer on Windows x64.');
}
const desktopRoot = fileURLToPath(new URL('../', import.meta.url));
const bundle = path.join(desktopRoot, 'dist/windows');
await access(path.join(bundle, 'chatto-desktop.exe'));
await rm(path.join(desktopRoot, 'dist/installer'), { recursive: true, force: true });

// This hook signs both the generated uninstaller before embedding and the final
// installer. The preceding Azure action installs the ArtifactSigning module.
const sign =
  process.env.CHATTO_WINDOWS_SIGN_INSTALLER === 'true'
    ? async ({ path: file }) => {
        execFileSync(
          'pwsh',
          [
            '-NoProfile',
            '-NonInteractive',
            '-File',
            path.join(desktopRoot, 'scripts/sign-windows-installer.ps1'),
            '-File',
            file
          ],
          { stdio: 'inherit' }
        );
      }
    : undefined;

const config = windowsInstallerConfig(packageJson.version, sign);
config.electronVersion = packageJson.devDependencies.electron;
await build({
  projectDir: desktopRoot,
  prepackaged: bundle,
  targets: Platform.WINDOWS.createTarget('nsis', Arch.x64),
  publish: 'never',
  config
});
