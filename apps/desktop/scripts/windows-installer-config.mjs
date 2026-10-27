import { releaseBuildVersion } from './version.mjs';

/** Keep this identity stable: NSIS derives its upgrade and uninstall identity from it. */
export const windowsAppId = 'run.chatto.desktop';

/** Wrap the existing Packager bundle without repacking it or changing its profile. */
export function windowsInstallerConfig(version, sign) {
  return {
    appId: windowsAppId,
    productName: 'Chatto Desktop',
    executableName: 'chatto-desktop',
    buildVersion: releaseBuildVersion(version),
    directories: { output: 'dist/installer', buildResources: 'icons' },
    // Publication belongs to the release workflow. There is no updater yet.
    publish: null,
    forceCodeSigning: Boolean(sign),
    win: {
      target: [{ target: 'nsis', arch: ['x64'] }],
      icon: 'icons/icon.ico',
      signExecutable: Boolean(sign),
      signtoolOptions: sign ? { sign, signingHashAlgorithms: ['sha256'] } : undefined
    },
    nsis: {
      artifactName: `chatto-desktop_${version}_Windows_X64_Setup.exe`,
      oneClick: true,
      perMachine: false,
      packElevateHelper: false,
      createDesktopShortcut: false,
      createStartMenuShortcut: true,
      shortcutName: 'Chatto Desktop',
      deleteAppDataOnUninstall: false,
      differentialPackage: false,
      runAfterFinish: true
    }
  };
}
