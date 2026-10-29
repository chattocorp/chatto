/** Own update checks and installation. Only an explicit user action restarts Chatto. */
export function createDesktopUpdates({ updater, preferences, save, notify, now = Date.now }) {
  let busy = false;
  let manual = false;
  let downloaded;
  let pending;
  let downloadFailed = false;
  let installing = false;
  updater.logger = null; // Upstream error messages can contain full request URLs.
  updater.autoDownload = false;
  updater.autoInstallOnAppQuit = false;
  updater.disableWebInstaller = true;
  // Our existing signed ZIP/NSIS pipeline publishes full archives, without blockmaps.
  updater.disableDifferentialDownload = true;
  updater.isUserWithinRollout = async (info) => {
    if (manual || preferences.channel === 'beta') return true;
    const hours = info.rolloutHours ?? 36;
    const released = Date.parse(info.releaseDate);
    if (!Number.isFinite(released) || !Number.isFinite(hours) || hours < 0) return false;
    return hours === 0 || now() - released >= preferences.bucket * hours * 3_600_000;
  };
  updater.on('error', () => {
    downloadFailed = true;
    // Squirrel.Mac verifies and stages the signed app after quitAndInstall
    // returns. Keep controls locked until quit or report that later failure.
    if (installing) {
      installing = false;
      busy = false;
      downloaded = undefined;
      void notify('error');
    }
  });
  updater.on('update-downloaded', (info) => {
    if (pending === info.version) downloaded = info.version;
  });

  function configure() {
    updater.allowPrerelease = preferences.channel === 'beta';
    updater.channel = preferences.channel === 'beta' ? 'beta' : 'latest';
    // The channel setter enables downgrades. Always override it afterwards.
    updater.allowDowngrade = false;
  }

  /** Serialize checks and downloads so channel changes cannot install a stale result. */
  async function check(isManual = false) {
    if (busy || (!isManual && !preferences.automatic)) return;
    busy = true;
    manual = isManual;
    downloadFailed = false;
    try {
      configure();
      const result = await updater.checkForUpdates();
      if (!result?.isUpdateAvailable) {
        downloaded = undefined;
        pending = undefined;
        await notify(isManual ? 'current' : 'idle');
        return;
      }
      pending = result.updateInfo.version;
      const alreadyDownloaded = downloaded === pending;
      if (downloaded !== pending) {
        downloaded = undefined;
        await updater.downloadUpdate();
      }
      if (downloadFailed || downloaded !== pending) throw new Error('Download did not complete');
      if (!alreadyDownloaded || isManual) await notify('ready', pending, isManual);
    } catch {
      downloaded = undefined;
      pending = undefined;
      await notify(isManual ? 'error' : 'idle');
    } finally {
      manual = false;
      busy = false;
    }
  }

  return {
    preferences,
    check,
    /** Persist preferences before applying them; keep a download on its original channel. */
    async setPreference(key, value) {
      if (busy) return false;
      if (key === 'channel' && !['stable', 'beta'].includes(value)) return false;
      if (key === 'automatic' && typeof value !== 'boolean') return false;
      if (!['channel', 'automatic'].includes(key)) return false;
      busy = true;
      try {
        await save({ ...preferences, [key]: value });
        preferences[key] = value;
        downloaded = undefined;
        pending = undefined;
        return true;
      } finally {
        busy = false;
      }
    },
    /** Revalidate the feed before restarting, in case the release was withdrawn or replaced. */
    async install() {
      if (busy || !downloaded) return;
      busy = true;
      manual = true;
      try {
        configure();
        const result = await updater.checkForUpdates();
        if (!result?.isUpdateAvailable || result.updateInfo.version !== downloaded) {
          downloaded = undefined;
          await notify('changed');
          return;
        }
        installing = true;
        updater.quitAndInstall(false, true);
      } catch {
        installing = false;
        await notify('error');
      } finally {
        manual = false;
        busy = installing;
      }
    }
  };
}

/** Default to the build's channel, preserve valid saved choices, and reject custom feed URLs. */
export function updatePreferences(value, bucket, version) {
  // Build metadata can contain hyphens without making a version a prerelease.
  const defaultChannel = version.split('+', 1)[0].includes('-') ? 'beta' : 'stable';
  return {
    channel: ['stable', 'beta'].includes(value?.channel) ? value.channel : defaultChannel,
    automatic: value?.automatic !== false,
    bucket:
      Number.isFinite(value?.bucket) && value.bucket >= 0 && value.bucket < 1
        ? value.bucket
        : bucket
  };
}
