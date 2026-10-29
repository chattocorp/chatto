/** Bundled as CommonJS at package time; Electron stays an external dependency. */
import { MacUpdater, NsisUpdater, AppImageUpdater } from 'electron-updater';
import { ElectronAppAdapter } from 'electron-updater/out/ElectronAppAdapter.js';
import { ElectronHttpExecutor } from 'electron-updater/out/electronHttpExecutor.js';

/** Use the full release SemVer, not macOS's numeric CFBundleShortVersionString. */
export function createUpdater(version) {
  class ReleaseAppAdapter extends ElectronAppAdapter {
    get version() {
      return version;
    }
  }
  const Updater =
    process.platform === 'darwin'
      ? MacUpdater
      : process.platform === 'win32'
        ? NsisUpdater
        : AppImageUpdater;
  const updater = new Updater(undefined, new ReleaseAppAdapter());
  // Passing an adapter disables the default HTTP executor in electron-updater.
  updater.httpExecutor = new ElectronHttpExecutor();
  return updater;
}
