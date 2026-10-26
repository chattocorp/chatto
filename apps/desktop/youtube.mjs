import { hasAppOrigin } from './security.mjs';

// YouTube requires an HTTPS app identity because Chromium omits Referer for
// our custom scheme. Match the installed app ID, not a user's server or room.
const desktopReferrer = 'https://run.chatto.desktop/';

/**
 * Install the session's header hook before opening desktop windows. Only direct
 * YouTube player frames in the local frontend receive the fixed app identity.
 * Electron supports one onBeforeSendHeaders listener per session.
 */
export function configureYouTubeEmbeds(appSession) {
  appSession.webRequest.onBeforeSendHeaders(
    { urls: ['https://www.youtube-nocookie.com/embed/*'] },
    (details, callback) => {
      if (details.resourceType !== 'subFrame' || !hasAppOrigin(details.frame?.parent?.url)) {
        callback({});
        return;
      }

      const requestHeaders = { ...details.requestHeaders };
      // Header names are case-insensitive. Do not retain a second Referer.
      for (const name of Object.keys(requestHeaders)) {
        if (name.toLowerCase() === 'referer') delete requestHeaders[name];
      }
      requestHeaders.Referer = desktopReferrer;
      callback({ requestHeaders });
    }
  );
}
