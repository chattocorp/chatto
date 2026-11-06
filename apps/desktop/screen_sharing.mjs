import { hasAppOrigin } from './security.mjs';

/**
 * Install display capture for the app origin. PipeWire owns source selection;
 * other capturers require a choice in the host dialog. No system audio is granted.
 */
export function configureScreenSharing(
  appSession,
  { desktopCapturer, dialog, getMainWindow, platform = process.platform }
) {
  appSession.setDisplayMediaRequestHandler(async (request, callback) => {
    if (!hasAppOrigin(request.securityOrigin) || !request.userGesture) {
      callback(null);
      return;
    }

    // Electron requires null for denial; an empty object throws when video
    // was requested and leaves the async handler with an unhandled rejection.
    let streams = null;
    try {
      const sources = await desktopCapturer.getSources({ types: ['screen', 'window'] });
      if (sources.length === 1 && platform === 'linux' && sources[0].name === '') {
        // PipeWire exposes one unnamed source for both screens and windows.
        // The portal chooses the real source and enforces consent. Do not show
        // this opaque source as an empty button or require a second choice.
        streams = { video: sources[0] };
      } else if (sources.length > 0) {
        const choice = await dialog.showMessageBox(getMainWindow(), {
          type: 'question',
          title: 'Share your screen',
          message: 'Choose what Chatto may share',
          buttons: [
            ...sources.map(
              (source, index) =>
                source.name.trim() ||
                `${source.id.startsWith('screen:') ? 'Screen' : 'Window'} ${index + 1}`
            ),
            'Cancel'
          ],
          cancelId: sources.length,
          defaultId: sources.length,
          noLink: true
        });
        const source = sources[choice.response];
        if (source) streams = { video: source };
      }
    } catch {
      // Portal cancellation and capture errors must reject getDisplayMedia.
    }
    callback(streams);
  });
}
