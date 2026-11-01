/** Keep release tags and desktop channel names independent during discovery. */
import { CancellationToken, parseXml } from 'builder-util-runtime';
import {
  GitHubProvider,
  computeReleaseNotes
} from 'electron-updater/out/providers/GitHubProvider.js';
import { parseUpdateInfo } from 'electron-updater/out/providers/Provider.js';
import { desktopUpdateChannel } from './update_channel.mjs';

/** Use GitHub's stable selection, and include beta and RC releases in Beta. */
export class DesktopGitHubProvider extends GitHubProvider {
  async getLatestVersion() {
    if (!this.updater.allowPrerelease) return super.getLatestVersion();

    const cancellationToken = new CancellationToken();
    const feed = parseXml(
      await this.httpRequest(
        new URL(`${this.basePath}.atom`, this.baseUrl),
        { accept: 'application/xml, application/atom+xml, text/xml, */*' },
        cancellationToken
      )
    );
    // GitHub orders the feed by publication time. The real updater still checks
    // SemVer and prevents downgrades after this provider selects a release.
    for (const release of feed.getElements('entry')) {
      const tag = release
        .element('link')
        .attribute('href')
        .match(/\/tag\/([^/]+)$/)?.[1];
      if (!tag) continue;
      const channel = desktopUpdateChannel(tag.replace(/^v/, ''));
      if (!channel) continue;
      const filename = `${this.getCustomChannelName(channel)}.yml`;
      const url = new URL(this.getBaseDownloadPath(tag, filename), this.baseUrl);
      const info = parseUpdateInfo(
        await this.httpRequest(url, undefined, cancellationToken),
        filename,
        url
      );
      return {
        ...info,
        tag,
        releaseName: info.releaseName ?? release.elementValueOrEmpty('title'),
        releaseNotes:
          info.releaseNotes ??
          computeReleaseNotes(
            this.updater.currentVersion,
            this.updater.fullChangelog,
            feed,
            release
          )
      };
    }
    throw new Error('No published desktop release is available.');
  }
}
