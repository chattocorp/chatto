import { releaseBuildVersion } from './version.mjs';

/** Keep this identity stable: NSIS derives its upgrade and uninstall identity from it. */
export const windowsAppId = 'run.chatto.desktop';

/** Wrap the existing Packager bundle without repacking it or changing its profile. */
export function windowsInstallerConfig(version, sign) {
  return {
    appId: windowsAppId,
    productName: 'Chatto',
    executableName: 'chatto-desktop',
    buildVersion: releaseBuildVersion(version),
    directories: { output: 'dist/installer', buildResources: 'icons' },
    // The release assembler writes manifests for the signed artifacts. Packager
    // embeds the updater configuration before signing; do not replace it here.
    publish: null,
    forceCodeSigning: Boolean(sign),
    win: {
      target: [
        { target: 'nsis', arch: ['x64'] },
        { target: 'msi', arch: ['x64'] }
      ],
      icon: 'icons/icon.ico',
      signExecutable: Boolean(sign),
      signtoolOptions: sign ? { sign, signingHashAlgorithms: ['sha256'] } : undefined
    },
    msi: {
      artifactName: `chatto-desktop_${version}_Windows_X64_Setup.msi`,
      // Keep this identity stable across releases; MSI owns managed upgrades.
      upgradeCode: 'D3DBF560-E6EB-4BD1-ABFE-D3C01F63BB54',
      perMachine: true,
      oneClick: false,
      runAfterFinish: false,
      createDesktopShortcut: false,
      createStartMenuShortcut: true,
      shortcutName: 'Chatto'
    },
    nsis: {
      artifactName: `chatto-desktop_${version}_Windows_X64_Setup.exe`,
      oneClick: true,
      perMachine: false,
      packElevateHelper: false,
      createDesktopShortcut: false,
      createStartMenuShortcut: true,
      shortcutName: 'Chatto',
      deleteAppDataOnUninstall: false,
      differentialPackage: false,
      runAfterFinish: true
    }
  };
}
