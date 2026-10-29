import { mkdir, readdir, rename, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { packager } from '@electron/packager';
import { bundleUpdater } from './bundle-updater.mjs';
import packageJson from '../package.json' with { type: 'json' };
import { pruneElectronLocales } from './locales.mjs';
import { embedMacOSCaptureHelper } from './macos-capture-helper.mjs';
import { macOSDistributionOptions } from './macos-signing.mjs';
import { macOSVersions, releaseBuildVersion } from './version.mjs';
import { desktopBuildProfile } from '../build_profile.mjs';

const desktopRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = path.resolve(desktopRoot, '../..');
const distRoot = path.join(desktopRoot, 'dist');
const packagerOut = path.join(distRoot, '.packager');
const platform = process.platform;
const profile = desktopBuildProfile(process.env.CHATTO_DESKTOP_BUILD);
const autoUpdate =
  process.env.CHATTO_DESKTOP_AUTO_UPDATE === 'true' && profile.variant === 'release';
if (autoUpdate && platform === 'win32' && !process.env.CHATTO_WINDOWS_EXPECTED_PUBLISHER) {
  throw new Error('Update-capable Windows builds require CHATTO_WINDOWS_EXPECTED_PUBLISHER.');
}
const electronChecksum = process.env.CHATTO_ELECTRON_CHECKSUM;
const electronArchiveName = `electron-v${packageJson.devDependencies.electron}-${platform}-${process.arch}.zip`;
const embedCaptureHelper = platform === 'darwin';
const macVersions = platform === 'darwin' ? macOSVersions(packageJson.version) : undefined;
const macOSDistribution = macOSDistributionOptions(platform, process.env);
let signingStartedAt;
const supportedLocales = (
  await readdir(path.resolve(desktopRoot, '../frontend/messages'), {
    withFileTypes: true
  })
)
  .filter((entry) => entry.isDirectory())
  .map((entry) => entry.name);

await rm(distRoot, { recursive: true, force: true });
await mkdir(packagerOut, { recursive: true });

const [bundleRoot] = await packager({
  dir: desktopRoot,
  out: packagerOut,
  overwrite: true,
  asar: true,
  name: profile.productName,
  executableName: platform === 'darwin' ? undefined : 'chatto-desktop',
  appVersion: macVersions?.shortVersion ?? packageJson.version,
  buildVersion: macVersions?.bundleVersion ?? releaseBuildVersion(packageJson.version),
  appBundleId: profile.bundleIdentifier,
  afterCopy: [
    async ({ buildPath }) => {
      // Electron reads this metadata before main starts. Separate names also
      // separate its default session and single-instance lock.
      await writeFile(
        path.join(buildPath, 'package.json'),
        JSON.stringify(
          {
            ...packageJson,
            name: profile.packageName,
            productName: profile.productName,
            chattoBuildVariant: profile.variant,
            chattoAutoUpdate: autoUpdate
          },
          null,
          2
        )
      );
      // Keep pnpm's symlink layout out of the app. Bundle the updater and its
      // dependencies into one CommonJS file, with only Electron external.
      await bundleUpdater(buildPath);
    }
  ],
  icon:
    platform === 'win32'
      ? path.join(desktopRoot, 'icons/icon.ico')
      : platform === 'darwin'
        ? path.join(desktopRoot, 'icons/icon.icns')
        : undefined,
  extraResource: [
    path.resolve(desktopRoot, '../frontend/build'),
    path.join(desktopRoot, 'node_modules/electron/dist/LICENSE'),
    path.join(desktopRoot, 'node_modules/electron/dist/LICENSES.chromium.html'),
    path.join(repositoryRoot, 'NOTICE'),
    path.join(repositoryRoot, 'LICENSES')
  ],
  usageDescription: {
    AudioCapture: 'Chatto captures game audio only while you choose to stream it.',
    Camera: 'Chatto uses the camera when you choose to share video in a call.',
    Microphone: 'Chatto uses the microphone when you join a voice or video call.'
  },
  download: electronChecksum
    ? { checksums: { [electronArchiveName]: electronChecksum } }
    : undefined,
  afterPrune: embedCaptureHelper
    ? [
        async ({ buildPath }) => {
          const appBundle = path.resolve(buildPath, '../../..');
          const prunedLocales = await pruneElectronLocales(appBundle, platform, supportedLocales);
          logPrunedLocales(prunedLocales);
          await embedMacOSCaptureHelper(appBundle, {
            ...macVersions,
            bundleIdentifier: `${profile.bundleIdentifier}.capture-helper`
          });
        }
      ]
    : undefined,
  osxSign: macOSDistribution.osxSign,
  osxNotarize: macOSDistribution.osxNotarize,
  afterCopyExtraResources: [
    async ({ buildPath }) => {
      if (autoUpdate) {
        // This must be inside Resources before macOS signing. JSON is valid YAML.
        const resources =
          platform === 'darwin'
            ? path.join(buildPath, `${profile.productName}.app/Contents/Resources`)
            : path.join(buildPath, 'resources');
        await writeFile(
          path.join(resources, 'app-update.yml'),
          JSON.stringify({
            provider: 'github',
            owner: 'chattocorp',
            repo: 'chatto',
            updaterCacheDirName: 'chatto-desktop-updater',
            ...(platform === 'win32'
              ? { publisherName: process.env.CHATTO_WINDOWS_EXPECTED_PUBLISHER }
              : {})
          })
        );
      }
      if (!macOSDistribution.osxSign) return;
      signingStartedAt = performance.now();
      console.log(
        macOSDistribution.osxNotarize
          ? 'Signing and notarising macOS app and nested code...'
          : 'Signing macOS app and nested code...'
      );
    }
  ],
  ignore: [
    /^\/dist(?:\/|$)/,
    /^\/native(?:\/|$)/,
    /^\/node_modules(?:\/|$)/,
    /^\/scripts(?:\/|$)/,
    /\.test\.mjs$/
  ]
});

if (signingStartedAt !== undefined) {
  console.log(
    `macOS signing and packaging finished in ${((performance.now() - signingStartedAt) / 1000).toFixed(1)}s`
  );
}

const appBundle =
  platform === 'darwin' ? path.join(bundleRoot, `${profile.productName}.app`) : bundleRoot;
if (platform !== 'darwin') {
  logPrunedLocales(await pruneElectronLocales(appBundle, platform, supportedLocales));
}

if (platform === 'darwin') {
  await rename(appBundle, path.join(distRoot, `${profile.productName}.app`));
} else if (platform === 'win32') {
  await rename(bundleRoot, path.join(distRoot, 'windows'));
} else {
  await rename(bundleRoot, path.join(distRoot, 'chatto-desktop'));
}

await rm(packagerOut, { recursive: true, force: true });

console.log(`Desktop build complete: ${distRoot}`);
if (platform === 'darwin' && profile.variant === 'development') {
  console.log(
    'To open it from the repository root: open "apps/desktop/dist/Chatto Desktop Dev.app"'
  );
  console.log('To build and open it next time: mise desktop-run-dev');
}

function logPrunedLocales(prunedLocales) {
  console.log(
    `Removed ${prunedLocales.removedLocales} unused Electron locale resources (${(prunedLocales.removedBytes / 1024 / 1024).toFixed(1)} MiB)`
  );
}
